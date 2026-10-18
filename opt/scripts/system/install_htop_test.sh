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
  # shellcheck disable=SC2016 # literal ${XDG_CONFIG_HOME...} is what we grep for
  assert_grep "${rc} sources the wrapper from the XDG path install uses" '^\[ -f "\$\{XDG_CONFIG_HOME:-\$HOME/\.config\}/htop/htop-fit\.sh" \]' "${REPO_ROOT}/opt/profiles/${rc}"
done
assert_grep "flag off never undoes under --phase deps" 'INSTALL_PHASE" != "deps"' "${INSTALL_SH}"
assert_grep "CPU count ignores OMP_NUM_THREADS/affinity (getconf, not bare nproc)" 'ncpu=\$\(getconf _NPROCESSORS_ONLN' "${FIT}"
assert_grep "temp htoprc is removed even on a signal" "trap 'rm -f \"\\\$tmp\"' EXIT HUP INT TERM" "${FIT}"
assert_grep "seed sorts by CPU%" '^\.sort_key=PERCENT_CPU$' "${SEED}"
assert_grep "seed has tree view off (it would ignore the sort)" '^tree_view=0$' "${SEED}"
assert_grep "seed uses the default two-column layout" '^header_layout=two_50_50$' "${SEED}"

# --- render ------------------------------------------------------------------
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

# htop-fit.sh is a no-op without htop on PATH (CI runners may lack it): stub it.
mkdir -p "${TMP}/bin"
# shellcheck disable=SC2016 # expanded by the stub at run time, not here
printf '#!/bin/sh\necho "htop ${STUB_HTOP_VERSION:-3.4.1}"\n' >"${TMP}/bin/htop"
chmod +x "${TMP}/bin/htop"
PATH="${TMP}/bin:${PATH}"

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

sed 's/^hide_function_bar=.*/hide_function_bar=1/' "${SEED}" >"${TMP}/esc.rc"
_htop_fit_render "${TMP}/esc.rc" 40 4 22 0 >"${TMP}/out"
assert_eq "$(col_rows "${TMP}/out" 0 22)" "31" "hide_function_bar=1 (hidden only after ESC) still counts the bar"
sed 's/^hide_function_bar=.*/hide_function_bar=2/' "${SEED}" >"${TMP}/hid.rc"
_htop_fit_render "${TMP}/hid.rc" 40 4 22 0 >"${TMP}/out"
assert_eq "$(col_rows "${TMP}/out" 0 22)" "32" "hide_function_bar=2 frees the bar row"

assert_eq "$(STUB_HTOP_VERSION=3.0.5 _htop_fit_gpu || echo n)" "n" "auto GPU is off on htop older than 3.4"
assert_eq "$(HTOP_FIT_GPU=1 _htop_fit_gpu && echo y)" "y" "HTOP_FIT_GPU=1 forces the GPU meter on"
assert_eq "$(HTOP_FIT_GPU=0 _htop_fit_gpu || echo n)" "n" "HTOP_FIT_GPU=0 forces it off"

# --- install / remove --------------------------------------------------------
D="${TMP}/cfg"
B="${TMP}/optbin"
run() { HTOP_CONFIG_DIR="${D}" HTOP_BIN_DIR="${B}" bash "${SCRIPT}" "$@" >/dev/null; }

run install
assert_file_exists "${D}/htop-fit.sh" "install copies the wrapper"
assert_exit_code 0 "install puts an htop-fit command on PATH (~/opt/bin)" test -x "${B}/htop-fit"
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
assert_exit_code 1 "remove deletes the htop-fit command" test -e "${B}/htop-fit"
assert_exit_code 1 "remove takes htoprc away so htop uses its defaults" test -e "${D}/htoprc"
assert_file_exists "${D}/htoprc.dotfiles.bak" "remove keeps the managed htoprc as .dotfiles.bak"
assert_exit_code 1 "remove clears the marker" test -e "${D}/.dotfiles-managed"

# Flag cycling never overwrites an earlier backup.
run install
echo "delay=42" >>"${D}/htoprc"
run remove
assert_eq "$(find "${D}" -name 'htoprc.dotfiles.bak*' | wc -l | tr -d ' ')" "2" "a second remove keeps the first .dotfiles.bak"
assert_grep "remove still moves a changed managed htoprc aside" '^delay=42$' "$(find "${D}" -name 'htoprc.dotfiles.bak.*')"

U="${TMP}/user"
mkdir -p "${U}"
echo "user=1" >"${U}/htoprc"
HTOP_CONFIG_DIR="${U}" bash "${SCRIPT}" remove >/dev/null
assert_grep "remove leaves an htoprc dotfiles never managed" '^user=1$' "${U}/htoprc"
HTOP_CONFIG_DIR="${U}" bash "${SCRIPT}" install >/dev/null
assert_grep "install backs up an unmanaged htoprc" '^user=1$' "${U}/htoprc.pre-dotfiles.bak"
assert_exit_code 0 "then installs the seed" cmp -s "${SEED}" "${U}/htoprc"
HTOP_CONFIG_DIR="${U}" bash "${SCRIPT}" remove >/dev/null
echo "user=2" >"${U}/htoprc"
HTOP_CONFIG_DIR="${U}" bash "${SCRIPT}" install >/dev/null
assert_grep "re-enabling never overwrites the original pre-dotfiles backup" '^user=1$' "${U}/htoprc.pre-dotfiles.bak"

# --- htop-fit command ----------------------------------------------------------
# The wrapper is a shell function, so programs that start htop themselves
# (mono runs panes with /bin/sh -c) got the plain binary. The htop-fit
# command loads the same wrapper; check it under /bin/sh with a stub htop
# that reports the config it was given.
X="${TMP}/xdg"
HTOP_CONFIG_DIR="${X}/htop" HTOP_BIN_DIR="${TMP}/runbin" bash "${SCRIPT}" install >/dev/null
mkdir -p "${TMP}/fakehtop"
# shellcheck disable=SC2016 # expanded by the stub at run time, not here
printf '#!/bin/sh
[ "$1" = --version ] && { echo "htop 3.4.1"; exit 0; }
echo "HTOPRC=${HTOPRC:-none}"
[ -n "$HTOPRC" ] && grep "^column_meters_0=" "$HTOPRC"
' >"${TMP}/fakehtop/htop"
chmod +x "${TMP}/fakehtop/htop"
if command -v script >/dev/null 2>&1; then
  out=$(XDG_CONFIG_HOME="${X}" PATH="${TMP}/fakehtop:${PATH}" LINES=30 \
    script -qec "/bin/sh ${TMP}/runbin/htop-fit" /dev/null 2>&1 | tr -d '\r')
  assert_eq "$(printf '%s\n' "${out}" | grep -c '^HTOPRC=/')" "1" "htop-fit runs htop with a fitted copy of htoprc"
  assert_eq "$(printf '%s\n' "${out}" | grep -c '^column_meters_0=.* CPU Memory Swap')" "1" "the copy has the history graphs added"
  rm -f "${X}/htop/htop-fit.sh"
  out=$(XDG_CONFIG_HOME="${X}" PATH="${TMP}/fakehtop:${PATH}" \
    script -qec "/bin/sh ${TMP}/runbin/htop-fit" /dev/null 2>&1 | tr -d '\r')
  assert_eq "$(printf '%s\n' "${out}" | head -1)" "HTOPRC=none" "without the wrapper htop-fit runs plain htop"
else
  echo "SKIP: htop-fit command run (no script(1) for a tty)"
fi

_test_report
