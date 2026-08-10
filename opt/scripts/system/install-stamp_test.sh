#!/usr/bin/env bash
# Test driver for opt/scripts/system/install-stamp.sh
#
# Unlike install.sh (which mutates the host and is therefore only
# source-checked), this script is safe to execute: it writes exactly one
# file under $HOME/.local/state/dotfiles. Every case below runs it for real
# against a throwaway $HOME.
#
# Run: bash opt/scripts/system/install-stamp_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/install-stamp.sh"
STAMP_REL=".local/state/dotfiles/install-stamp"

# === 1. Syntax check ===
assert_exit_code 0 "install-stamp.sh parses with bash -n" bash -n "$SCRIPT"

# Helper: run the script with a throwaway HOME against this repo as BASE_DIR.
# Echoes the temp HOME so the caller can inspect the result.
_run_stamp() {
    local phase="$1" base="${2:-$REPO_ROOT}" tmp
    tmp="$(mktemp -d)"
    HOME="$tmp" INSTALL_PHASE="$phase" bash "$SCRIPT" "$base" >/dev/null 2>&1
    echo "$tmp"
}

# === 2. INSTALL_PHASE=all writes a well-formed stamp ===
TMP_ALL="$(_run_stamp all)"
assert_file_exists "${TMP_ALL}/${STAMP_REL}" "phase=all writes the stamp"
assert_grep "stamp records a 40-char commit sha" \
    '^commit=[0-9a-f]{40}$' "${TMP_ALL}/${STAMP_REL}"
assert_grep "stamp records a unix epoch installed_at" \
    '^installed_at=[0-9]{10,}$' "${TMP_ALL}/${STAMP_REL}"
assert_grep "stamp records the branch" '^branch=.+$' "${TMP_ALL}/${STAMP_REL}"
assert_grep "stamp records the hostname" '^hostname=.+$' "${TMP_ALL}/${STAMP_REL}"

# === 3. Build phases must NEVER stamp (Docker layer cache correctness) ===
TMP_DEPS="$(_run_stamp deps)"
assert_exit_code 1 "phase=deps writes no stamp" test -f "${TMP_DEPS}/${STAMP_REL}"

TMP_CONFIG="$(_run_stamp config)"
assert_exit_code 1 "phase=config writes no stamp" test -f "${TMP_CONFIG}/${STAMP_REL}"

# === 4. Unset INSTALL_PHASE defaults to a full run (stamps) ===
TMP_UNSET="$(mktemp -d)"
( unset INSTALL_PHASE; HOME="$TMP_UNSET" bash "$SCRIPT" "$REPO_ROOT" >/dev/null 2>&1 )
assert_file_exists "${TMP_UNSET}/${STAMP_REL}" "unset INSTALL_PHASE defaults to stamping"

# === 5. A non-git BASE_DIR is not an error and writes nothing ===
NON_GIT="$(mktemp -d)"
TMP_NOGIT="$(mktemp -d)"
set +e
HOME="$TMP_NOGIT" INSTALL_PHASE=all bash "$SCRIPT" "$NON_GIT" >/dev/null 2>&1
RC=$?
set -e
assert_eq "$RC" "0" "non-git BASE_DIR exits 0 (never fails the install)"
assert_exit_code 1 "non-git BASE_DIR writes no stamp" test -f "${TMP_NOGIT}/${STAMP_REL}"

# === 6. Re-running overwrites rather than appending ===
TMP_TWICE="$(mktemp -d)"
HOME="$TMP_TWICE" INSTALL_PHASE=all bash "$SCRIPT" "$REPO_ROOT" >/dev/null 2>&1
HOME="$TMP_TWICE" INSTALL_PHASE=all bash "$SCRIPT" "$REPO_ROOT" >/dev/null 2>&1
LINES="$(grep -c '^commit=' "${TMP_TWICE}/${STAMP_REL}")"
assert_eq "$LINES" "1" "re-running overwrites the stamp (one commit line)"

_test_report
