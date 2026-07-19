#!/usr/bin/env bash
# Test driver for opt/bin/sshd-setup. Mocks external commands via PATH
# shadowing (same pattern as pkg-install_test.sh). Run: bash opt/bin/sshd-setup_test.sh
set -u
SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"
SSHD_SETUP="${SELF_DIR}/sshd-setup"
TMPDIR_TEST="$(mktemp -d)"; trap 'rm -rf "$TMPDIR_TEST"' EXIT

# Sandbox PATH: essentials only, so `command -v gh` / `git` / `ufw` genuinely
# miss when a test needs them absent (real ones live in /usr/bin).
SANDBOX_PATH="${TMPDIR_TEST}/sandbox"
mkdir -p "$SANDBOX_PATH" "${TMPDIR_TEST}/mocks"
for tool in bash sh env grep sed awk cat wc tr ls mkdir rm chmod touch head tail cut printf echo uname pgrep dirname basename readlink find sort; do
    if command -v "$tool" >/dev/null 2>&1; then
        ln -sf "$(command -v "$tool")" "${SANDBOX_PATH}/${tool}"
    fi
done

assert_exit_code 0 "sshd-setup parses with bash -n" bash -n "$SSHD_SETUP"

# usage on no args -> exit 1, mentions subcommands
set +e; OUT=$(bash "$SSHD_SETUP" 2>&1); RC=$?; set -e
assert_eq "$RC" "1" "no args exits 1"
case "$OUT" in *"status|enable|keys"*) echo "PASS: usage lists subcommands"; PASS=$((PASS+1));;
  *) echo "FAIL: usage missing subcommand list (got: $OUT)"; FAIL=$((FAIL+1));; esac

# platform detection: wsl via fake /proc/version
echo "Linux version 6.6 (Microsoft@Microsoft.com)" > "${TMPDIR_TEST}/proc_version"
OUT=$(SSHD_SETUP_PROC_VERSION="${TMPDIR_TEST}/proc_version" bash "$SSHD_SETUP" _detect 2>&1)
assert_eq "$OUT" "wsl" "detects wsl from microsoft /proc/version"
echo "Linux version 6.6 (gcc)" > "${TMPDIR_TEST}/proc_version"
OUT=$(SSHD_SETUP_PROC_VERSION="${TMPDIR_TEST}/proc_version" bash "$SSHD_SETUP" _detect 2>&1)
assert_eq "$OUT" "linux" "detects plain linux"

_test_report
