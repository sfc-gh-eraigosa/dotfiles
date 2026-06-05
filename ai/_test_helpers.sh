# shellcheck shell=bash
# ai/_test_helpers.sh — shared assertion helpers for shell test drivers.
#
# Why a custom mini-framework (and not bats-core)?
#   Three existing drivers (safety_guard_test.sh, sync-plugins_test.sh,
#   aliases_test.sh) already use a simple POSIX-ish bash assertion pattern
#   with zero external deps. We standardize that pattern here so new
#   drivers don't reinvent it. Revisit bats if test count exceeds ~20 files.
#
# How to write a new test driver in 5 lines:
#   #!/usr/bin/env bash
#   set -u
#   . "$(cd "$(dirname "$0")" && pwd)/path/to/ai/_test_helpers.sh"
#   assert_eq "$(echo hi)" "hi" "echo prints hi"
#   _test_report   # prints "PASS=N FAIL=M" and exits 0 / 1
#
# Drivers this helper INTENTIONALLY does NOT cover:
#   - ai/claude/hooks/safety_guard_test.sh — uses a specialized
#     assert_exit(expected, tool, command, label) that feeds JSON
#     payloads to the hook on stdin. Leaving its inline helper alone.
#   - opt/scripts/system/sync-plugins_test.sh — uses an
#     assert_contains/assert_eq pair scoped to a single captured-output
#     buffer. Could be migrated later but the cost/benefit is low.
#
# Public functions (all increment PASS / FAIL counters):
#   assert_eq           <got> <want> <label>
#   assert_grep         <label> <pattern> <file>     # asserts file CONTAINS pattern (ERE)
#   assert_grep_negative <label> <pattern> <file>    # asserts file does NOT contain pattern
#   assert_in_subshell  <label> <code...>            # runs code in `bash -c`, expects exit 0
#   assert_file_exists  <path> <label>
#   assert_exit_code    <expected> <label> <cmd...>  # runs cmd, asserts exit code matches
#   _test_report                                     # prints summary; exits 1 if any FAIL
#
# Counters PASS and FAIL are exposed as plain shell variables; tests can
# read them if they need conditional cleanup logic.

# Guard against double-sourcing (helper sourced by both a driver and a
# nested subshell would reset counters).
if [ -n "${_TEST_HELPERS_LOADED:-}" ]; then
    return 0
fi
_TEST_HELPERS_LOADED=1

PASS=0
FAIL=0

assert_eq() {
    local got="$1" want="$2" label="$3"
    if [ "$got" = "$want" ]; then
        echo "PASS: $label"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $label (got '$got' want '$want')"
        FAIL=$((FAIL + 1))
    fi
}

assert_grep() {
    local label="$1" pattern="$2" file="$3"
    if [ ! -e "$file" ]; then
        echo "FAIL: $label (file not found: $file)"
        FAIL=$((FAIL + 1))
        return
    fi
    if grep -qE -- "$pattern" "$file"; then
        echo "PASS: $label"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $label (pattern not found: $pattern in $file)"
        FAIL=$((FAIL + 1))
    fi
}

assert_grep_negative() {
    local label="$1" pattern="$2" file="$3"
    if [ ! -e "$file" ]; then
        echo "FAIL: $label (file not found: $file)"
        FAIL=$((FAIL + 1))
        return
    fi
    if grep -qE -- "$pattern" "$file"; then
        echo "FAIL: $label (unexpected match: $pattern in $file)"
        FAIL=$((FAIL + 1))
    else
        echo "PASS: $label"
        PASS=$((PASS + 1))
    fi
}

assert_in_subshell() {
    local label="$1"
    shift
    if bash -c "$*" >/dev/null 2>&1; then
        echo "PASS: $label"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $label (cmd: $*)"
        FAIL=$((FAIL + 1))
    fi
}

assert_file_exists() {
    local path="$1" label="$2"
    if [ -e "$path" ]; then
        echo "PASS: $label"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $label (missing: $path)"
        FAIL=$((FAIL + 1))
    fi
}

assert_exit_code() {
    local expected="$1" label="$2"
    shift 2
    set +e
    "$@" >/dev/null 2>&1
    local rc=$?
    set -e
    if [ "$rc" = "$expected" ]; then
        echo "PASS: $label (exit $rc)"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $label (expected exit $expected, got $rc)"
        FAIL=$((FAIL + 1))
    fi
}

_test_report() {
    echo "----"
    echo "PASS=$PASS FAIL=$FAIL"
    [ "$FAIL" -eq 0 ]
}
