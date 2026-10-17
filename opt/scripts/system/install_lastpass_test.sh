#!/usr/bin/env bash
# Test driver for opt/scripts/system/install_lastpass.sh
#
# What must hold:
#   * a host where `lpass` already runs is a no-op (nothing privileged runs);
#   * Debian-likes install the `lastpass-cli` apt package non-interactively,
#     through sudo when not root, and retry once after `apt-get update` when
#     the first install fails on a stale index;
#   * macOS goes through brew;
#   * when sudo would need a password and there is no terminal, apt is never
#     touched: one WARNING, exit 0 (fleet update over ssh must not stall);
#   * install.sh gates it fail-OPEN (gff_on, boolDefault true) as a deps-phase
#     key.
#
# Nothing real is installed: uname, sudo, apt-get, brew and lpass are PATH stubs
# that record their argv.
#
# Run: bash opt/scripts/system/install_lastpass_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/install_lastpass.sh"
INSTALL_SH="${REPO_ROOT}/install.sh"
FEATURES="${REPO_ROOT}/.github/gff/features.yaml"

assert_file_exists "${SCRIPT}" "install_lastpass.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
set +e

# --- source-level ------------------------------------------------------------
assert_grep "installs the distro package, not a fetched binary" 'apt-get install[^|]*lastpass-cli' "${SCRIPT}"
assert_grep "apt runs non-interactively" 'DEBIAN_FRONTEND=noninteractive' "${SCRIPT}"
assert_grep_negative "never pipes a remote script into a shell" 'curl[^|]*\|[[:space:]]*(ba)?sh' "${SCRIPT}"
assert_grep "macOS uses brew" 'brew install lastpass-cli' "${SCRIPT}"

# --- stubs ------------------------------------------------------------------
TMP="$(mktemp -d "${TMPDIR:-/tmp}/install_lastpass_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
BIN="${TMP}/stubs"; mkdir -p "${BIN}"
LOG="${TMP}/calls.log"
# Where the "package manager" drops lpass once "installed".
PKGBIN="${TMP}/pkgbin"; mkdir -p "${PKGBIN}"
# lpass shadow, FIRST on PATH so a real lpass on this host never leaks in:
# STUB_LPASS_VERSION=x.y.z plays an already-installed lpass; otherwise it runs
# the package-manager stub's copy once that exists, and fails until then.
SHADOW="${TMP}/shadow"; mkdir -p "${SHADOW}"

# shellcheck disable=SC2016 # the stubs expand their own $@ when they run
{
    printf '#!/bin/sh\ncase "$1" in -s) echo "${STUB_OS:-Linux}" ;; -m) echo x86_64 ;; *) echo "${STUB_OS:-Linux}" ;; esac\n' > "${BIN}/uname"
    # sudo: `-n true` succeeds unless STUB_SUDO_NEEDS_PASSWORD=1; otherwise runs the command.
    printf '#!/bin/sh\necho "sudo $*" >> "$STUB_LOG"\nif [ "$1" = "-n" ]; then [ "${STUB_SUDO_NEEDS_PASSWORD:-0}" != "1" ]; exit $?; fi\nexec "$@"\n' > "${BIN}/sudo"
    # apt-get: records argv; `install` fails STUB_APT_FAIL_FIRST times, then drops an lpass stub.
    cat > "${BIN}/apt-get" <<'EOF'
#!/bin/sh
echo "apt-get $*" >> "$STUB_LOG"
case "$1" in
    update) exit 0 ;;
    install)
        n=0; [ -f "$STUB_LOG.fails" ] && n=$(cat "$STUB_LOG.fails")
        if [ "$n" -lt "${STUB_APT_FAIL_FIRST:-0}" ]; then echo $((n + 1)) > "$STUB_LOG.fails"; echo "E: Unable to locate package lastpass-cli" >&2; exit 100; fi
        printf '#!/bin/sh\necho "LastPass CLI v1.3.7 (stub)"\n' > "$STUB_PKGBIN/lpass"; chmod +x "$STUB_PKGBIN/lpass"
        exit 0 ;;
esac
exit 0
EOF
    printf '#!/bin/sh\necho "brew $*" >> "$STUB_LOG"\nprintf "#!/bin/sh\\necho \\"LastPass CLI v1.6.1 (brew stub)\\"\\n" > "$STUB_PKGBIN/lpass"; chmod +x "$STUB_PKGBIN/lpass"\n' > "${BIN}/brew"
} 2>/dev/null
chmod +x "${BIN}"/*
# shellcheck disable=SC2016 # expanded when the stub runs
printf '#!/bin/sh\nif [ -n "${STUB_LPASS_VERSION:-}" ]; then echo "LastPass CLI v${STUB_LPASS_VERSION}"; exit 0; fi\nif [ -x "$STUB_PKGBIN/lpass" ]; then exec "$STUB_PKGBIN/lpass" "$@"; fi\nexit 1\n' > "${SHADOW}/lpass"
chmod +x "${SHADOW}/lpass"

run_lp() {  # run_lp [VAR=value ...]  (stdin is not a terminal)
    : > "${LOG}"; rm -f "${LOG}.fails" "${PKGBIN}/lpass"
    env -u LASTPASS_FORCE STUB_LOG="${LOG}" STUB_PKGBIN="${PKGBIN}" \
        PATH="${SHADOW}:${BIN}:${PKGBIN}:/usr/bin:/bin" "$@" bash "${SCRIPT}" 2>&1 </dev/null
}

# 1. Fresh Debian-like host, sudo cached: one non-interactive apt install through sudo.
out="$(run_lp)"; rc=$?
assert_eq "${rc}" "0" "fresh apt host exits 0"
assert_eq "$(grep -c '^apt-get install -y -qq --no-install-recommends lastpass-cli' "${LOG}")" "1" "installs lastpass-cli via apt exactly once"
assert_eq "$(grep -c '^sudo env DEBIAN_FRONTEND=noninteractive apt-get install' "${LOG}")" "1" "the install goes through sudo, non-interactively"
assert_eq "$(grep -c 'apt-get update' "${LOG}")" "0" "no apt-get update when the first install succeeds"
assert_eq "$(printf '%s' "${out}" | grep -c 'Success! lastpass-cli 1.3.7')" "1" "success names the installed version"

# 2. Stale index: the first install fails, then update + one retry.
out="$(run_lp STUB_APT_FAIL_FIRST=1)"; rc=$?
assert_eq "${rc}" "0" "stale-index host exits 0 after the retry"
assert_eq "$(grep -c '^apt-get update -qq' "${LOG}")" "1" "refreshes the index once after a failed install"
assert_eq "$(grep -c '^apt-get install' "${LOG}")" "2" "retries the install exactly once"

# 3. Already installed: no-op, nothing privileged runs.
out="$(run_lp STUB_LPASS_VERSION=1.3.7)"; rc=$?
assert_eq "${rc}" "0" "already-installed exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'already installed: .*(1.3.7)')" "1" "already-installed is reported"
assert_eq "$(wc -l < "${LOG}" | tr -d ' ')" "0" "already-installed touches neither sudo nor apt"

# 4. LASTPASS_FORCE=1 reinstalls even when present.
out="$(run_lp STUB_LPASS_VERSION=1.3.7 LASTPASS_FORCE=1)"; rc=$?
assert_eq "${rc}" "0" "forced reinstall exits 0"
assert_eq "$(grep -c '^apt-get install' "${LOG}")" "1" "LASTPASS_FORCE=1 runs the package manager again"

# 5. sudo needs a password and there is no terminal: skip with ONE warning, exit 0.
out="$(run_lp STUB_SUDO_NEEDS_PASSWORD=1)"; rc=$?
assert_eq "${rc}" "0" "no-sudo/no-tty exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'WARNING: install_lastpass: sudo needs a password and there is no terminal')" "1" "no-sudo/no-tty warns once"
assert_eq "$(grep -c 'apt-get' "${LOG}")" "0" "no-sudo/no-tty never touches apt"

# 6. macOS: brew, no sudo.
out="$(run_lp STUB_OS=Darwin)"; rc=$?
assert_eq "${rc}" "0" "macOS exits 0"
assert_eq "$(grep -c '^brew install lastpass-cli$' "${LOG}")" "1" "macOS installs the brew formula"
assert_eq "$(grep -c 'sudo' "${LOG}")" "0" "macOS never calls sudo"

# 7. Unsupported OS is refused.
out="$(run_lp STUB_OS=FreeBSD)"; rc=$?
assert_eq "${rc}" "1" "unsupported OS exits 1"

# --- gff wiring -------------------------------------------------------------
assert_grep "features.yaml declares install.tools.lastpass" 'path: install\.tools\.lastpass$' "${FEATURES}"
assert_eq "$(awk '$2 == "path:" { k = $3 } k == "install.tools.lastpass" && $1 == "boolDefault:" { print $2; exit }' "${FEATURES}")" \
    "true" "install.tools.lastpass is on by default (boolDefault true)"
assert_grep "install.sh gates lastpass fail-open with gff_on" 'gff_on install\.tools\.lastpass;' "${INSTALL_SH}"
assert_eq "$(grep -E '^_IP_DEPS_FLAGS=' "${INSTALL_SH}" | grep -c -w 'INSTALL_TOOLS_LASTPASS')" "1" \
    "INSTALL_TOOLS_LASTPASS is a deps-phase key"
assert_eq "$(grep -E '^_IP_CONFIG_FLAGS=' "${INSTALL_SH}" | grep -c -w 'INSTALL_TOOLS_LASTPASS')" "0" \
    "INSTALL_TOOLS_LASTPASS is NOT a config-phase key"

_test_report
