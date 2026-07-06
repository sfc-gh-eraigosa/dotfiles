#!/bin/bash
# Test driver for ai/antigravity/aliases.sh
#
# Why: parallels ai/claude/aliases_test.sh — both files are sourced by
# the user's interactive shell and define the wrapper functions the user
# types directly (`agy`, `agy-yolo`). The same failure mode that
# bit `claude()` (an underscore-prefixed helper getting stripped by a
# CLI's shell-snapshot mechanism, leaving the public function broken)
# could equally bite agy if a refactor introduces one — so the
# `no underscore-prefixed helper functions` guard is precautionary even
# though Antigravity CLI does not (today) maintain a shell snapshot.
#
# Run: bash ai/antigravity/aliases_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=../_test_helpers.sh
. "${SELF_DIR}/../_test_helpers.sh"

ALIASES="${SELF_DIR}/aliases.sh"

# === Source-level checks ===
assert_file_exists "$ALIASES" "ai/antigravity/aliases.sh exists"
assert_exit_code 0 "ai/antigravity/aliases.sh parses with bash -n" bash -n "$ALIASES"

# Precautionary guard: no underscore-prefixed helper functions. Claude
# Code's shell-snapshot strips these; Antigravity CLI doesn't snapshot
# today, but if it ever adds one, an underscore helper would silently
# vanish. Keep the same shape both files use.
assert_grep_negative "no underscore-prefixed helper functions" \
    '^_[a-zA-Z_]+\(\)[[:space:]]*\{' "$ALIASES"

# `agy` and `agy-yolo` must remain defined at the top level (catch
# accidental removal during refactors).
assert_grep_negative "agy() not removed by accident" \
    '^# REMOVED agy\(\)' "$ALIASES"
assert_grep "agy() defined" '^agy\(\)[[:space:]]*\{' "$ALIASES"
assert_grep "agy-yolo() defined" '^agy-yolo\(\)[[:space:]]*\{' "$ALIASES"

# === Runtime checks (source the file in a subshell) ===
assert_in_subshell "agy function defined after sourcing" \
    ". '$ALIASES' && type agy >/dev/null 2>&1"
assert_in_subshell "agy-yolo function defined after sourcing" \
    ". '$ALIASES' && type agy-yolo >/dev/null 2>&1"

# `agy` body sanity: must call `command agy` (NOT a recursive
# `agy "$@"` which would loop). The source-level grep above already
# guards underscore-prefixed helpers — no body-level check needed (the
# `$TMUX_PANE` env var would yield a false positive on `_PANE`).
assert_in_subshell "agy body calls 'command agy' (avoids recursion)" \
    ". '$ALIASES' && declare -f agy | grep -q 'command agy'"
assert_in_subshell "agy-yolo body calls 'command agy --dangerously-skip-permissions'" \
    ". '$ALIASES' && declare -f agy-yolo | grep -q 'command agy --dangerously-skip-permissions'"

_test_report
