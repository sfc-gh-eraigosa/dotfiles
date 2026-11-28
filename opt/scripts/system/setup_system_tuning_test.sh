#!/usr/bin/env bash
# Test driver for opt/scripts/system/setup_system_tuning.sh (gff install.system.tuning).
#
# What must hold:
#   * not Linux                         -> SKIP, exit 0, nothing written (macOS);
#   * no running systemd                -> SKIP, exit 0 (containers, WSL without systemd);
#   * apply                             -> journald drop-in with SystemMaxUse=2G, journald
#                                          restarted, journal vacuumed; apt file with
#                                          APT::Periodic::CleanInterval "7";
#   * apply again                       -> no write, no restart ("already"), still vacuums;
#   * a different drop-in               -> backed up (.<timestamp>.bak), then replaced;
#   * no apt-get                        -> journald part only, no apt file;
#   * ST_JOURNAL_MAX=1G                 -> the cap follows; a bad size is refused (64);
#   * --dry-run / --status              -> write, restart and vacuum nothing.
#
# Nothing touches the host: every directory is a sandbox, ST_SUDO is empty, and
# systemctl / journalctl / apt-get are recording fakes.
#
# Run: bash opt/scripts/system/setup_system_tuning_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/setup_system_tuning.sh"
assert_file_exists "${SCRIPT}" "setup_system_tuning.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
assert_grep_negative "no hardcoded home path" '/home/[a-z]|/Users/[a-z]' "${SCRIPT}"
set +e

TMP="$(mktemp -d "${TMPDIR:-/tmp}/st_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
LOG="${TMP}/calls.log"

FAKES="${TMP}/fakes"; mkdir -p "${FAKES}"
for f in apt-get systemctl journalctl; do
  # shellcheck disable=SC2016 # $* and ${FAKE_LOG} expand inside the fake, not here
  printf '#!/bin/sh\necho "%s $*" >> "${FAKE_LOG}"\n' "${f}" > "${FAKES}/${f}"
done
chmod +x "${FAKES}"/*

fresh() {  # fresh <name>
  SYSD="${TMP}/$1/run-systemd"; JD="${TMP}/$1/journald.conf.d"; APT="${TMP}/$1/apt.conf.d"
  mkdir -p "${SYSD}"
  : > "${LOG}"
}
JDF() { echo "${JD}/10-dotfiles-tuning.conf"; }
APF() { echo "${APT}/52dotfiles-tuning"; }

run() {
  env HOME="${TMP}/home" FAKE_LOG="${LOG}" ST_UNAME=Linux ST_SUDO="" \
    ST_SYSTEMD_DIR="${SYSD}" ST_JOURNALD_DIR="${JD}" ST_APT_CONF_DIR="${APT}" \
    ST_APT_GET="${FAKES}/apt-get" ST_SYSTEMCTL="${FAKES}/systemctl" ST_JOURNALCTL="${FAKES}/journalctl" \
    "$@" 2>&1
}

# 1. Not Linux -> SKIP.
fresh mac
out="$(run ST_UNAME=Darwin bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "not Linux: exit 0"
assert_in_subshell "not Linux: says SKIP" "printf '%s' '${out//\'/}' | grep -q 'SKIP'"
[ ! -e "$(JDF)" ] && echo "PASS: not Linux: nothing written" || echo "FAIL: not Linux: wrote a file"

# 2. No systemd -> SKIP.
fresh nosystemd; rmdir "${SYSD}"
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "no systemd: exit 0"
assert_in_subshell "no systemd: says SKIP" "printf '%s' '${out//\'/}' | grep -q 'SKIP'"
[ ! -e "$(JDF)" ] && echo "PASS: no systemd: nothing written" || echo "FAIL: no systemd: wrote a file"

# 3. Apply writes both files, restarts journald, vacuums.
fresh apply
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "apply: exit 0"
assert_grep "apply: journald cap is 2G" '^SystemMaxUse=2G$' "$(JDF)"
assert_grep "apply: journald section" '^\[Journal\]$' "$(JDF)"
assert_grep "apply: weekly apt clean" '^APT::Periodic::CleanInterval "7";$' "$(APF)"
assert_grep "apply: journald restarted" '^systemctl restart systemd-journald$' "${LOG}"
assert_grep "apply: journal vacuumed" '^journalctl --vacuum-size=2G$' "${LOG}"

# 4. Apply again is a no-op apart from the vacuum.
: > "${LOG}"; before="$(cat "$(JDF)")"
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "re-apply: exit 0"
assert_in_subshell "re-apply: says already" "printf '%s' '${out//\'/}' | grep -q 'already'"
assert_grep_negative "re-apply: no restart" 'restart' "${LOG}"
assert_grep "re-apply: still vacuums" 'vacuum-size' "${LOG}"
assert_eq "$(cat "$(JDF)")" "${before}" "re-apply: drop-in unchanged"

# 5. A different drop-in is backed up, then replaced.
fresh foreign; mkdir -p "${JD}"; printf '[Journal]\nSystemMaxUse=8G\n' > "$(JDF)"
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "foreign: exit 0"
assert_in_subshell "foreign: a .bak copy exists" "ls '${JD}' | grep -qE '^10-dotfiles-tuning\.conf\.[0-9]{14}\.bak$'"
assert_grep "foreign: replaced" '^SystemMaxUse=2G$' "$(JDF)"

# 6. No apt-get -> journald part only.
fresh noapt
out="$(run ST_APT_GET="${TMP}/does-not-exist" bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "no apt: exit 0"
assert_file_exists "$(JDF)" "no apt: journald drop-in written"
[ ! -e "$(APF)" ] && echo "PASS: no apt: no apt file" || echo "FAIL: no apt: wrote the apt file"

# 7. ST_JOURNAL_MAX moves the cap; a bad size is refused.
fresh custom
out="$(run ST_JOURNAL_MAX=1G bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "custom cap: exit 0"
assert_grep "custom cap: SystemMaxUse=1G" '^SystemMaxUse=1G$' "$(JDF)"
fresh badsize
out="$(run ST_JOURNAL_MAX='2 GB; rm' bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "64" "bad size: exit 64"
[ ! -e "$(JDF)" ] && echo "PASS: bad size: nothing written" || echo "FAIL: bad size: wrote a file"

# 8. --dry-run and --status change nothing.
fresh dry
out="$(run bash "${SCRIPT}" --dry-run)"; rc=$?
assert_eq "${rc}" "0" "dry-run: exit 0"
assert_in_subshell "dry-run: shows the cap" "printf '%s' '${out//\'/}' | grep -q 'SystemMaxUse=2G'"
[ ! -e "$(JDF)" ] && echo "PASS: dry-run: nothing written" || echo "FAIL: dry-run: wrote a file"
assert_grep_negative "dry-run: no restart or vacuum" 'restart|vacuum' "${LOG}"
fresh status
out="$(run bash "${SCRIPT}" --status)"; rc=$?
assert_eq "${rc}" "0" "status: exit 0"
assert_in_subshell "status: reports the drop-in" "printf '%s' '${out//\'/}' | grep -q 'absent'"
[ ! -e "$(JDF)" ] && echo "PASS: status: nothing written" || echo "FAIL: status: wrote a file"
assert_grep_negative "status: no restart or vacuum" 'restart|vacuum' "${LOG}"

_test_report
