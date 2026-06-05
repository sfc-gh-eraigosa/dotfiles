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

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=../_test_helpers.sh
. "${SELF_DIR}/../_test_helpers.sh"

ALIASES="${SELF_DIR}/aliases.sh"

# === Source-level checks ===
# No underscore-prefixed helper functions should reappear. Claude Code's
# snapshot strips them and dependent callers break silently.
assert_grep_negative "no underscore-prefixed helper functions" \
    '^_[a-zA-Z_]+\(\)[[:space:]]*\{' "$ALIASES"

# `claude` and `claude-toggle` must be defined at the top level.
assert_grep_negative "claude() not removed by accident" \
    '^# REMOVED claude\(\)' "$ALIASES"

# === Runtime checks (source the file in a subshell) ===
assert_in_subshell "claude function defined after sourcing" \
    ". '$ALIASES' && type claude >/dev/null 2>&1"
assert_in_subshell "claude-toggle function defined after sourcing" \
    ". '$ALIASES' && type claude-toggle >/dev/null 2>&1"
# `claude` must NOT reference any undefined command at parse time. We can't
# easily invoke claude() (it would shell out to the real binary), but we can
# inspect its body for forbidden references.
assert_in_subshell "claude body does not call _claude_yolo_enabled" \
    ". '$ALIASES' && ! declare -f claude | grep -q '_claude_yolo_enabled'"
assert_in_subshell "claude-toggle body does not call _claude_yolo_enabled" \
    ". '$ALIASES' && ! declare -f claude-toggle | grep -q '_claude_yolo_enabled'"

# === Variable setup ===
assert_in_subshell "CLAUDE_YOLO_DIR exported" \
    ". '$ALIASES' && [ -n \"\$CLAUDE_YOLO_DIR\" ]"
assert_in_subshell "CLAUDE_YOLO_FILE exported" \
    ". '$ALIASES' && [ -n \"\$CLAUDE_YOLO_FILE\" ]"

_test_report
