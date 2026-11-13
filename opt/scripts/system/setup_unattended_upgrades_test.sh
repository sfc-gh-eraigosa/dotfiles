#!/usr/bin/env bash
# Test driver for opt/scripts/system/setup_unattended_upgrades.sh (gff install.system.unattended-upgrades).
#
# What must hold:
#   * no apt-get                        -> SKIP, exit 0, nothing written (macOS, Fedora, ...);
#   * no running systemd                -> SKIP, exit 0 (containers, WSL without systemd);
#   * package absent                    -> apt-get install unattended-upgrades;
#   * apply                             -> 20auto-upgrades with Update-Package-Lists "1" and
#                                          Unattended-Upgrade "1"; the two apt timers enabled;
#   * apply again                       -> no apt-get, no write ("already");
#   * a different 20auto-upgrades       -> backed up, then replaced;
#   * --dry-run                         -> prints the plan, writes nothing, installs nothing;
#   * --status                          -> reads only.
#
# Nothing touches the host: UU_APT_CONF_DIR is a sandbox, UU_SUDO is empty, apt-get /
# dpkg-query / systemctl are recording fakes, UU_SYSTEMD_DIR points at a sandbox dir.
#
# Run: bash opt/scripts/system/setup_unattended_upgrades_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/setup_unattended_upgrades.sh"
assert_file_exists "${SCRIPT}" "setup_unattended_upgrades.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
assert_grep_negative "no hardcoded home path" '/home/[a-z]|/Users/[a-z]' "${SCRIPT}"
set +e

TMP="$(mktemp -d "${TMPDIR:-/tmp}/uu_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
LOG="${TMP}/calls.log"

FAKES="${TMP}/fakes"; mkdir -p "${FAKES}"
cat > "${FAKES}/apt-get" <<'FAKE'
#!/bin/sh
echo "apt-get $*" >> "${FAKE_LOG}"
FAKE
cat > "${FAKES}/dpkg-query" <<'FAKE'
#!/bin/sh
echo "dpkg-query $*" >> "${FAKE_LOG}"
# installed iff the marker file exists
[ -e "${FAKE_PKG_INSTALLED}" ] && { echo "install ok installed"; exit 0; }
exit 1
FAKE
cat > "${FAKES}/systemctl" <<'FAKE'
#!/bin/sh
echo "systemctl $*" >> "${FAKE_LOG}"
case "$1" in is-enabled) [ -e "${FAKE_TIMERS_ENABLED}" ] && { echo enabled; exit 0; }; echo disabled; exit 1 ;; esac
exit 0
FAKE
chmod +x "${FAKES}"/*

fresh() {  # fresh <name> [installed]
    CONF="${TMP}/$1/apt.conf.d"; SYSD="${TMP}/$1/run-systemd"; PKG="${TMP}/$1/pkg-installed"; TIMERS="${TMP}/$1/timers-enabled"
    mkdir -p "${CONF}" "${SYSD}"
    [ "${2:-}" = installed ] && : > "${PKG}"
    : > "${LOG}"
}
AUTO() { echo "${CONF}/20auto-upgrades"; }

run() {
    env HOME="${TMP}/home" FAKE_LOG="${LOG}" FAKE_PKG_INSTALLED="${PKG}" FAKE_TIMERS_ENABLED="${TIMERS}" \
        UU_APT_CONF_DIR="${CONF}" UU_SYSTEMD_DIR="${SYSD}" UU_SUDO="" \
        UU_APT_GET="${FAKES}/apt-get" UU_DPKG_QUERY="${FAKES}/dpkg-query" UU_SYSTEMCTL="${FAKES}/systemctl" \
        "$@" 2>&1
}

# 1. No apt-get -> SKIP.
fresh noapt
out="$(run UU_APT_GET="${TMP}/does-not-exist" bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "no apt-get: exit 0"
assert_in_subshell "no apt-get: says SKIP" "printf '%s' '${out//\'/}' | grep -q 'SKIP'"
[ ! -e "$(AUTO)" ] && echo "PASS: no apt-get: nothing written" || echo "FAIL: no apt-get: wrote a file"

# 2. No systemd -> SKIP.
fresh nosystemd; rmdir "${SYSD}"
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "no systemd: exit 0"
assert_in_subshell "no systemd: says SKIP and names systemd" "printf '%s' '${out//\'/}' | grep -q 'SKIP.*systemd'"
assert_grep_negative "no systemd: no apt-get" 'apt-get' "${LOG}"

# 3. Apply on a host without the package -> install, write, enable timers.
fresh apply
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "apply: exit 0"
assert_grep "apply: installs the package" 'apt-get .*install.* unattended-upgrades' "${LOG}"
assert_file_exists "$(AUTO)" "apply: 20auto-upgrades written"
assert_grep "apply: periodic update" '^APT::Periodic::Update-Package-Lists "1";$' "$(AUTO)"
assert_grep "apply: periodic upgrade" '^APT::Periodic::Unattended-Upgrade "1";$' "$(AUTO)"
assert_grep "apply: enables apt-daily.timer" 'systemctl enable .*apt-daily.timer' "${LOG}"
assert_grep "apply: enables apt-daily-upgrade.timer" 'systemctl enable .*apt-daily-upgrade.timer' "${LOG}"

# 4. Apply again, package present, timers enabled -> no apt-get, no write.
fresh again installed; : > "${TIMERS}"; run bash "${SCRIPT}" >/dev/null; : > "${LOG}"
before="$(cat "$(AUTO)")"
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "re-apply: exit 0"
assert_in_subshell "re-apply: says already" "printf '%s' '${out//\'/}' | grep -qi 'already'"
assert_grep_negative "re-apply: no apt-get" 'apt-get' "${LOG}"
assert_grep_negative "re-apply: no enable" 'systemctl enable' "${LOG}"
assert_eq "$(cat "$(AUTO)")" "${before}" "re-apply: file unchanged"

# 5. A different 20auto-upgrades is backed up, then replaced.
fresh foreign installed; printf 'APT::Periodic::Unattended-Upgrade "0";\n' > "$(AUTO)"
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "foreign: exit 0"
assert_in_subshell "foreign: a .bak copy exists" "ls '${CONF}' | grep -qE '^20auto-upgrades\.[0-9]{14}\.bak$'"
# apt silently skips only names that END in .bak; anything else in apt.conf.d makes
# every apt run print "N: Ignoring file ... invalid filename extension".
assert_in_subshell "foreign: the copy ends in .bak (apt skips it silently)" "! ls '${CONF}' | grep -q '\.bak-'"
assert_grep "foreign: replaced" '^APT::Periodic::Unattended-Upgrade "1";$' "$(AUTO)"

# 5b. A .bak-<timestamp> copy left by an earlier version is renamed to <timestamp>.bak,
# even on a host that is otherwise current (found live 2026-11-13 on three hosts).
fresh legacy installed; : > "${TIMERS}"; run bash "${SCRIPT}" >/dev/null
printf 'old\n' > "$(AUTO).bak-20261110200219"
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "legacy backup: exit 0"
assert_file_exists "$(AUTO).20261110200219.bak" "legacy backup: renamed to <timestamp>.bak"
[ ! -e "$(AUTO).bak-20261110200219" ] && echo "PASS: legacy backup: old name gone" || echo "FAIL: legacy backup: old name still there"
assert_eq "$(cat "$(AUTO).20261110200219.bak")" "old" "legacy backup: content kept"

# 6. --dry-run writes and installs nothing; --status reads only.
fresh dry
out="$(run bash "${SCRIPT}" --dry-run)"; rc=$?
assert_eq "${rc}" "0" "dry-run: exit 0"
assert_in_subshell "dry-run: names the package" "printf '%s' '${out//\'/}' | grep -q 'unattended-upgrades'"
[ ! -e "$(AUTO)" ] && echo "PASS: dry-run: nothing written" || echo "FAIL: dry-run: wrote a file"
assert_grep_negative "dry-run: no apt-get" 'apt-get' "${LOG}"
assert_grep_negative "dry-run: no enable" 'systemctl enable' "${LOG}"
fresh status
out="$(run bash "${SCRIPT}" --status)"; rc=$?
assert_eq "${rc}" "0" "status: exit 0"
assert_in_subshell "status: reports the package state" "printf '%s' '${out//\'/}' | grep -qi 'package'"
[ ! -e "$(AUTO)" ] && echo "PASS: status: nothing written" || echo "FAIL: status: wrote a file"
assert_grep_negative "status: no apt-get" 'apt-get' "${LOG}"

_test_report
