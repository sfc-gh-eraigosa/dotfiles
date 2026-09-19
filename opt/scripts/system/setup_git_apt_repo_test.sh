#!/usr/bin/env bash
# Test driver for opt/scripts/system/setup_git_apt_repo.sh
#
# The script's purpose is "git on Ubuntu hosts is CURRENT, not the release the
# distro froze at". Ubuntu 22.04's git 2.34.1 segfaults on claude's
# partial-clone checkout (dotfiles#343); the PPA's git does not. The script
# needs sudo and network, so — like setup_gh_apt_repo_test.sh — we assert on
# its source: the repo it registers, the key it trusts, the ESM-beating pin,
# the Ubuntu-only guard, and that a host provisioned earlier gets its pin and
# key refreshed on the "already configured" path rather than skipped.
#
# Run: bash opt/scripts/system/setup_git_apt_repo_test.sh
set -u
SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/setup_git_apt_repo.sh"
INSTALLER="${REPO_ROOT}/opt/bin/pkg-install-apt"

assert_file_exists "${SCRIPT}" "setup_git_apt_repo.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"

# --- what it registers ----------------------------------------------------
assert_grep "points at the git-core PPA" \
    'REPO_URL=https://ppa\.launchpadcontent\.net/git-core/ppa/ubuntu' "${SCRIPT}"
assert_grep "trusts the PPA by its full signing fingerprint, not a short id" \
    'FINGERPRINT=[0-9A-F]{40}$' "${SCRIPT}"
assert_grep "the sources entry is signed-by the dedicated keyring" \
    'signed-by=\$\{KEYRING\}' "${SCRIPT}"
assert_grep "uses the host codename, never a hardcoded release" \
    'VERSION_CODENAME' "${SCRIPT}"

# --- Ubuntu only ----------------------------------------------------------
# The PPA has no Debian builds; a Debian host pointed at it fails every update.
assert_grep "guards on ID=ubuntu from os-release" 'OS_ID" != "ubuntu"' "${SCRIPT}"
assert_grep "skips (exit 0) rather than fails on a non-Ubuntu apt host" \
    'has no builds here, skipping.*>&2' "${SCRIPT}"

# --- the ESM-beating pin (same trap as gh, issue #255) ---------------------
assert_grep "writes an apt preferences file" 'PREFS=/etc/apt/preferences.d/git-core-ppa' "${SCRIPT}"
assert_grep "pins by origin ppa.launchpadcontent.net" 'Pin: origin ppa\.launchpadcontent\.net' "${SCRIPT}"
assert_grep "pin priority is 600 (> ESM's 510)" 'Pin-Priority: 600' "${SCRIPT}"
assert_grep "pin is scoped to git and git-man only" 'Package: git git-man[[:space:]]*$' "${SCRIPT}"
assert_grep_negative "pin does NOT use a wildcard package" 'Package: \*' "${SCRIPT}"

# --- healing existing hosts ----------------------------------------------
assert_grep "write_prefs is defined" '^write_prefs\(\)' "${SCRIPT}"
assert_grep "refresh_key is defined" '^refresh_key\(\)' "${SCRIPT}"
early_exit_line=$(grep -n 'git-core PPA already configured' "${SCRIPT}" | head -1 | cut -d: -f1)
prefs_call_line=$(grep -n '^  write_prefs || {' "${SCRIPT}" | head -1 | cut -d: -f1)
key_call_line=$(grep -n '^  refresh_key || {' "${SCRIPT}" | head -1 | cut -d: -f1)
assert_exit_code 0 "already-configured path refreshes the key BEFORE its early exit" \
    test "${key_call_line:-999}" -lt "${early_exit_line:-0}"
assert_exit_code 0 "already-configured path writes the pin BEFORE its early exit" \
    test "${prefs_call_line:-999}" -lt "${early_exit_line:-0}"

# --- wired into the installer, next to gh ---------------------------------
assert_grep "pkg-install-apt runs it before apt-get update" \
    'setup_git_apt_repo\.sh' "${INSTALLER}"
git_hook_line=$(grep -n 'setup_git_apt_repo\.sh' "${INSTALLER}" | head -1 | cut -d: -f1)
update_line=$(grep -n 'as_root apt-get update' "${INSTALLER}" | head -1 | cut -d: -f1)
assert_exit_code 0 "the hook precedes apt-get update, so the manifest's git resolves to the PPA" \
    test "${git_hook_line:-999}" -lt "${update_line:-0}"

# --- the reason, kept honest ---------------------------------------------
assert_grep "the header names the real cause: git 2.34.1 segfaults on the partial-clone checkout" \
    'SEGFAULTS on that checkout' "${SCRIPT}"

_test_report
