#!/usr/bin/env bash
# Test driver for opt/bin/claude-rc-boot. Exercises the name-generation and
# arg handling without launching Claude. Run: bash opt/bin/claude-rc-boot_test.sh
set -u
SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"
T="${SELF_DIR}/claude-rc-boot"

assert_exit_code 0 "claude-rc-boot parses with bash -n" bash -n "$T"

# --help exits 0 and mentions Remote Control
assert_exit_code 0 "--help exits 0" bash "$T" --help
OUT=$(bash "$T" --help 2>&1)
case "$OUT" in *"Remote Control"*) echo "PASS: help mentions Remote Control"; PASS=$((PASS+1));;
  *) echo "FAIL: help text missing 'Remote Control' (got: $OUT)"; FAIL=$((FAIL+1));; esac

# --print-name with explicit prefix -> prefix-YYYY-MM-DD_HHMM (no launch)
OUT=$(bash "$T" --print-name myhost 2>&1)
case "$OUT" in
  myhost-[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]_[0-9][0-9][0-9][0-9]) echo "PASS: name format with prefix"; PASS=$((PASS+1));;
  *) echo "FAIL: name format (got: $OUT)"; FAIL=$((FAIL+1));;
esac

# CLAUDE_RC_PREFIX env is honored when no arg given
OUT=$(CLAUDE_RC_PREFIX=envhost bash "$T" --print-name 2>&1)
case "$OUT" in envhost-2*) echo "PASS: env prefix honored"; PASS=$((PASS+1));;
  *) echo "FAIL: env prefix (got: $OUT)"; FAIL=$((FAIL+1));; esac

# prefix sanitization: spaces/specials collapse to single dashes, trimmed
OUT=$(bash "$T" --print-name 'My Host!!' 2>&1)
case "$OUT" in My-Host-2[0-9][0-9][0-9]-*) echo "PASS: prefix sanitized"; PASS=$((PASS+1));;
  *) echo "FAIL: prefix sanitize (got: $OUT)"; FAIL=$((FAIL+1));; esac

_test_report
