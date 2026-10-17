#!/usr/bin/env bash
# Test driver for opt/scripts/system/install_htop.sh and opt/etc/htop/htop-fit.sh
#
# What must hold:
#   * the fitted htoprc leaves exactly HTOP_PROC_ROWS rows for processes: the
#     tallest meter column fills the terminal minus htop's own chrome;
#   * graphs are added before Blank padding, GPU meter + GPU% column only when
#     the GPU is readable (HTOP_FIT_GPU forces either way);
#   * install seeds htoprc (backing up an unmanaged one), updates a pristine
#     managed one, never clobbers one changed since (unless forced);
#   * remove (flag off) deletes the wrapper and moves the managed htoprc aside
#     so htop returns to its defaults, leaving an unmanaged htoprc alone;
#   * install.sh gates it on install.tools.htop-fit as a config-phase key and
#     runs remove when the flag is off; features.yaml defaults it on.
#
# Run: bash opt/scripts/system/install_htop_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/install_htop.sh"
FIT="${REPO_ROOT}/opt/etc/htop/htop-fit.sh"
SEED="${REPO_ROOT}/opt/etc/htop/htoprc"
INSTALL_SH="${REPO_ROOT}/install.sh"
FEATURES="${REPO_ROOT}/.github/gff/features.yaml"

assert_file_exists "${SCRIPT}" "install_htop.sh exists"
assert_file_exists "${FIT}" "htop-fit.sh exists"
assert_file_exists "${SEED}" "seed htoprc exists"
assert_exit_code 0 "install_htop.sh parses with bash -n" bash -n "${SCRIPT}"
assert_exit_code 0 "htop-fit.sh parses with bash -n" bash -n "${FIT}"
if command -v zsh >/dev/null 2>&1; then
  assert_exit_code 0 "htop-fit.sh parses with zsh -n" zsh -n "${FIT}"
fi
set +e

# --- wiring ------------------------------------------------------------------
assert_grep "features.yaml declares install.tools.htop-fit" 'path: install\.tools\.htop-fit$' "${FEATURES}"
assert_eq "$(grep -A2 'path: install\.tools\.htop-fit$' "${FEATURES}" | grep -c 'boolDefault: true')" "1" \
  "install.tools.htop-fit defaults on"
assert_grep "install.sh gates on install.tools.htop-fit" 'gff_on install\.tools\.htop-fit;' "${INSTALL_SH}"
assert_grep "install.sh removes when the flag is off" 'install_htop\.sh" remove' "${INSTALL_SH}"
assert_eq "$(grep -E '^_IP_CONFIG_FLAGS=' "${INSTALL_SH}" | grep -c -w 'INSTALL_TOOLS_HTOP_FIT')" "1" \
  "INSTALL_TOOLS_HTOP_FIT is a config-phase key"
assert_eq "$(grep -E '^_IP_DEPS_FLAGS=' "${INSTALL_SH}" | grep -c -w 'INSTALL_TOOLS_HTOP_FIT')" "0" \
  "INSTALL_TOOLS_HTOP_FIT is NOT a deps-phase key"
for rc in .zshrc .bashrc; do
  assert_grep "${rc} sources the deployed wrapper" '\.config/htop/htop-fit\.sh" \]' "${REPO_ROOT}/opt/profiles/${rc}"
done
assert_grep "seed sorts by CPU%" '^\.sort_key=PERCENT_CPU$' "${SEED}"
assert_grep "seed has tree view off (it would ignore the sort)" '^tree_view=0$' "${SEED}"
assert_grep "seed uses the default two-column layout" '^header_layout=two_50_50$' "${SEED}"

# --- render ------------------------------------------------------------------
# shellcheck source=../../etc/htop/htop-fit.sh
. "${FIT}"

# Header rows of one column, recomputed independently of the awk.
col_rows() { # file col ncpu
  awk -v col="$2" -v ncpu="$3" '
    function ceil(x) { return (x == int(x)) ? x : int(x) + 1 }
    $0 ~ "^column_meters_" col "=" { sub(/^[^=]*=/, ""); n = split($0, m, " ") }
    $0 ~ "^column_meter_modes_" col "=" { sub(/^[^=]*=/, ""); split($0, d, " ") }
    END {
      for (i = 1; i <= n; i++) {
        per = (d[i] == 3) ? 4 : (d[i] == 4) ? 3 : 1; name = m[i]
        if (name ~ /CPUs[0-9]*$/) {
          cpus = (name ~ /^Left/) ? ceil(ncpu / 2) : (name ~ /^Right/) ? int(ncpu / 2) : ncpu
          c = name; sub(/.*CPUs/, "", c); if (c == "") c = 1; per *= ceil(cpus / c)
        }
        h += per
      }
      print h + 0
    }' "$1"
}

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

# Seed chrome: column titles + tab bar + function bar + 2 margin rows = 5.
for case in "40 22 4" "24 22 4" "60 4 4" "50 8 10"; do
  # shellcheck disable=SC2086 # split "lines ncpu procs" into $1 $2 $3
  set -- ${case}
  _htop_fit_render "${SEED}" "$1" "$3" "$2" 0 >"${TMP}/out"
  h0=$(col_rows "${TMP}/out" 0 "$2"); h1=$(col_rows "${TMP}/out" 1 "$2")
  hmax=$((h0 > h1 ? h0 : h1))
  assert_eq "${hmax}" "$(($1 - 5 - $3))" "lines=$1 ncpu=$2: header leaves exactly $3 process rows"
done

_htop_fit_render "${SEED}" 40 4 22 0 >"${TMP}/out"
assert_grep "graphs come before padding" '^column_meters_0=LeftCPUs4 Memory Swap CPU Memory Swap' "${TMP}/out"
assert_grep "per-core CPU graph fills the remainder" '^column_meters_0=.* LeftCPUs[0-9]*( Blank)*$' "${TMP}/out"
assert_grep_negative "no GPU meter without a readable GPU" 'GPU' "${TMP}/out"
assert_eq "$(grep -c '' "${TMP}/out")" "$(grep -c '' "${SEED}")" "render keeps every other setting line"

_htop_fit_render "${SEED}" 40 4 22 1 >"${TMP}/out"
assert_grep "GPU graph leads the right column when readable" '^column_meters_1=RightCPUs4 Tasks LoadAverage Uptime GPU ' "${TMP}/out"
assert_grep "GPU% column added before Command" '^screen:Main=.* GPU_PERCENT Command$' "${TMP}/out"

_htop_fit_render "${SEED}" 10 4 22 0 >"${TMP}/out"
assert_eq "$(grep '^column_meters_0=' "${TMP}/out")" "column_meters_0=LeftCPUs4 Memory Swap" \
  "a short terminal adds nothing"

printf 'column_meters_0=CPU\ncolumn_meter_modes_0=1\ncolumn_meters_1=!\ncolumn_meter_modes_1=!\n' >"${TMP}/empty.rc"
_htop_fit_render "${TMP}/empty.rc" 30 4 4 0 >"${TMP}/out"
assert_grep "an empty column (!) is filled without a stray !" '^column_meters_1=NetworkIO ' "${TMP}/out"

assert_eq "$(HTOP_FIT_GPU=1 _htop_fit_gpu && echo y)" "y" "HTOP_FIT_GPU=1 forces the GPU meter on"
assert_eq "$(HTOP_FIT_GPU=0 _htop_fit_gpu || echo n)" "n" "HTOP_FIT_GPU=0 forces it off"

# --- install / remove --------------------------------------------------------
D="${TMP}/cfg"
run() { HTOP_CONFIG_DIR="${D}" bash "${SCRIPT}" "$@" >/dev/null; }

run install
assert_file_exists "${D}/htop-fit.sh" "install copies the wrapper"
assert_exit_code 0 "fresh install seeds htoprc" cmp -s "${SEED}" "${D}/htoprc"
assert_file_exists "${D}/.dotfiles-managed" "install writes the managed marker"

run install
assert_exit_code 0 "re-install is idempotent" cmp -s "${SEED}" "${D}/htoprc"

echo "delay=99" >>"${D}/htoprc"
run install
assert_grep "a changed managed htoprc is kept" '^delay=99$' "${D}/htoprc"
HTOP_CONFIG_FORCE=1 run install
assert_exit_code 0 "HTOP_CONFIG_FORCE=1 replaces it" cmp -s "${SEED}" "${D}/htoprc"
assert_grep "forced replace keeps a .bak" '^delay=99$' "${D}/htoprc.bak"

# Simulate an older seed: pristine managed file differing from the repo copy.
grep -v '^delay=' "${SEED}" >"${D}/htoprc"
cksum <"${D}/htoprc" | awk '{print $1 "-" $2}' >"${D}/.dotfiles-managed"
run install
assert_exit_code 0 "a pristine managed htoprc is updated" cmp -s "${SEED}" "${D}/htoprc"

run remove
assert_exit_code 1 "remove deletes the wrapper" test -e "${D}/htop-fit.sh"
assert_exit_code 1 "remove takes htoprc away so htop uses its defaults" test -e "${D}/htoprc"
assert_file_exists "${D}/htoprc.dotfiles.bak" "remove keeps the managed htoprc as .dotfiles.bak"
assert_exit_code 1 "remove clears the marker" test -e "${D}/.dotfiles-managed"

U="${TMP}/user"
mkdir -p "${U}"
echo "user=1" >"${U}/htoprc"
HTOP_CONFIG_DIR="${U}" bash "${SCRIPT}" remove >/dev/null
assert_grep "remove leaves an htoprc dotfiles never managed" '^user=1$' "${U}/htoprc"
HTOP_CONFIG_DIR="${U}" bash "${SCRIPT}" install >/dev/null
assert_grep "install backs up an unmanaged htoprc" '^user=1$' "${U}/htoprc.pre-dotfiles.bak"
assert_exit_code 0 "then installs the seed" cmp -s "${SEED}" "${U}/htoprc"

_test_report
