#!/bin/bash
# Test driver for ai/gemini/aliases.sh
#
# Why: parallels ai/claude/aliases_test.sh — both files are sourced by
# the user's interactive shell and define the wrapper functions the user
# types directly (`gemini`, `gemini-yolo`). The same failure mode that
# bit `claude()` (an underscore-prefixed helper getting stripped by a
# CLI's shell-snapshot mechanism, leaving the public function broken)
# could equally bite gemini if a refactor introduces one — so the
# `no underscore-prefixed helper functions` guard is precautionary even
# though Gemini CLI does not (today) maintain a shell snapshot.
#
# Run: bash ai/gemini/aliases_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=../_test_helpers.sh
. "${SELF_DIR}/../_test_helpers.sh"

ALIASES="${SELF_DIR}/aliases.sh"

# === Source-level checks ===
assert_file_exists "$ALIASES" "ai/gemini/aliases.sh exists"
assert_exit_code 0 "ai/gemini/aliases.sh parses with bash -n" bash -n "$ALIASES"

# Precautionary guard: no underscore-prefixed helper functions. Claude
# Code's shell-snapshot strips these; Gemini CLI doesn't snapshot today,
# but if it ever adds one, an underscore helper would silently vanish.
# Keep the same shape both files use.
assert_grep_negative "no underscore-prefixed helper functions" \
    '^_[a-zA-Z_]+\(\)[[:space:]]*\{' "$ALIASES"

# `gemini` and `gemini-yolo` must remain defined at the top level (catch
# accidental removal during refactors).
assert_grep_negative "gemini() not removed by accident" \
    '^# REMOVED gemini\(\)' "$ALIASES"
assert_grep "gemini() defined" '^gemini\(\)[[:space:]]*\{' "$ALIASES"
assert_grep "gemini-yolo() defined" '^gemini-yolo\(\)[[:space:]]*\{' "$ALIASES"

# === Runtime checks (source the file in a subshell) ===
assert_in_subshell "gemini function defined after sourcing" \
    ". '$ALIASES' && type gemini >/dev/null 2>&1"
assert_in_subshell "gemini-yolo function defined after sourcing" \
    ". '$ALIASES' && type gemini-yolo >/dev/null 2>&1"

# `gemini` body sanity: must call `command gemini` (NOT a recursive
# `gemini "$@"` which would loop). The source-level grep above already
# guards underscore-prefixed helpers — no body-level check needed (the
# `$TMUX_PANE` env var would yield a false positive on `_PANE`).
assert_in_subshell "gemini body calls 'command gemini' (avoids recursion)" \
    ". '$ALIASES' && declare -f gemini | grep -q 'command gemini'"
assert_in_subshell "gemini-yolo body calls 'command gemini -y'" \
    ". '$ALIASES' && declare -f gemini-yolo | grep -q 'command gemini -y'"

_test_report
