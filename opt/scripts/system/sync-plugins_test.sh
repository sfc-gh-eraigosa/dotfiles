#!/usr/bin/env bash
# Test driver for sync-plugins.sh --dry-run. Mirrors safety_guard_test.sh style.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SYNC="${SCRIPT_DIR}/sync-plugins.sh"

PASS=0
FAIL=0

assert_contains() {
    local haystack="$1" needle="$2" desc="$3"
    if printf '%s' "$haystack" | grep -qF -- "$needle"; then
        echo "PASS: $desc"; PASS=$((PASS+1))
    else
        echo "FAIL: $desc (missing: $needle)"; FAIL=$((FAIL+1))
    fi
}

assert_eq() {
    local got="$1" want="$2" desc="$3"
    if [ "$got" = "$want" ]; then
        echo "PASS: $desc"; PASS=$((PASS+1))
    else
        echo "FAIL: $desc (got '$got' want '$want')"; FAIL=$((FAIL+1))
    fi
}

OUT="$(bash "$SYNC" --dry-run 2>&1)"

assert_contains "$OUT" "DRY-RUN: claude plugin marketplace add anthropics/claude-plugins-official" "adds the official marketplace"
assert_contains "$OUT" "DRY-RUN: claude plugin install superpowers@claude-plugins-official" "installs superpowers"
assert_contains "$OUT" "DRY-RUN: claude plugin install mcp-apps@claude-plugins-official" "installs mcp-apps"

INSTALL_COUNT="$(printf '%s' "$OUT" | grep -c 'DRY-RUN: claude plugin install ')"
assert_eq "$INSTALL_COUNT" "12" "plans install for all 12 plugins"

ENABLE_COUNT="$(printf '%s' "$OUT" | grep -c 'DRY-RUN: claude plugin enable ')"
assert_eq "$ENABLE_COUNT" "12" "plans enable for all 12 plugins"

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
