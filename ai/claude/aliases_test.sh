#!/bin/bash
# Test driver for ai/claude/aliases.sh
#
# Why: the `claude` / `claude-toggle` functions historically depended on a
# private helper `_claude_yolo_enabled`. Claude Code's shell-snapshot
# mechanism strips functions whose names start with `_`, so the snapshot kept
# `claude()` but dropped `_claude_yolo_enabled` — producing
#   "claude:8: command not found: _claude_yolo_enabled"
# on every Claude Code launch. The inline-check fix removes that dependency;
# this test driver guards against any reintroduction of an underscore helper.
#
# Run: bash ai/claude/aliases_test.sh
set -u

ALIASES="$(cd "$(dirname "$0")" && pwd)/aliases.sh"
PASS=0
FAIL=0

# assert_in_subshell <label> <code...>
# Sources aliases.sh in a child bash and asserts <code> returns 0.
assert_in_subshell() {
    local label="$1"; shift
    if bash -c ". '$ALIASES' && $*" >/dev/null 2>&1; then
        echo "PASS: $label"
        PASS=$((PASS+1))
    else
        echo "FAIL: $label"
        FAIL=$((FAIL+1))
    fi
}

# assert_grep_negative <label> <pattern>
# Asserts the aliases.sh source does NOT contain <pattern>.
assert_grep_negative() {
    local label="$1" pattern="$2"
    if grep -qE "$pattern" "$ALIASES"; then
        echo "FAIL: $label (matched: $pattern)"
        FAIL=$((FAIL+1))
    else
        echo "PASS: $label"
        PASS=$((PASS+1))
    fi
}

# === Source-level checks ===
# No underscore-prefixed helper functions should reappear. Claude Code's
# snapshot strips them and dependent callers break silently.
assert_grep_negative "no underscore-prefixed helper functions" \
    '^_[a-zA-Z_]+\(\)[[:space:]]*\{'

# `claude` and `claude-toggle` must be defined at the top level.
assert_grep_negative "claude() not removed by accident" \
    '^# REMOVED claude\(\)'

# === Runtime checks (source the file in a subshell) ===
assert_in_subshell "claude function defined after sourcing" \
    "type claude >/dev/null 2>&1"
assert_in_subshell "claude-toggle function defined after sourcing" \
    "type claude-toggle >/dev/null 2>&1"
# `claude` must NOT reference any undefined command at parse time. We can't
# easily invoke claude() (it would shell out to the real binary), but we can
# inspect its body for forbidden references.
assert_in_subshell "claude body does not call _claude_yolo_enabled" \
    "! declare -f claude | grep -q '_claude_yolo_enabled'"
assert_in_subshell "claude-toggle body does not call _claude_yolo_enabled" \
    "! declare -f claude-toggle | grep -q '_claude_yolo_enabled'"

# === Variable setup ===
assert_in_subshell "CLAUDE_YOLO_DIR exported" \
    "[ -n \"\$CLAUDE_YOLO_DIR\" ]"
assert_in_subshell "CLAUDE_YOLO_FILE exported" \
    "[ -n \"\$CLAUDE_YOLO_FILE\" ]"

# === Summary ===
echo
echo "Passed: $PASS"
echo "Failed: $FAIL"
[ "$FAIL" -eq 0 ]
