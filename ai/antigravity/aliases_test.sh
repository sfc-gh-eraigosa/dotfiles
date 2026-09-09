#!/bin/bash
# Test driver for ai/antigravity/aliases.sh
#
# Why: parallels ai/claude/aliases_test.sh — both files are sourced by
# the user's interactive shell and define the wrapper functions the user
# types directly (`agy`, `agy-config`). The same failure mode that
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

# `agy` must remain defined at the top level (catch
# accidental removal during refactors).
assert_grep_negative "agy() not removed by accident" \
    '^# REMOVED agy\(\)' "$ALIASES"
assert_grep "agy() defined" '^agy\(\)[[:space:]]*\{' "$ALIASES"

# === Runtime checks (source the file in a subshell) ===
assert_in_subshell "agy function defined after sourcing" \
    ". '$ALIASES' && type agy >/dev/null 2>&1"

# `agy` body sanity: must call `command agy` (NOT a recursive
# `agy "$@"` which would loop). The source-level grep above already
# guards underscore-prefixed helpers — no body-level check needed (the
# `$TMUX_PANE` env var would yield a false positive on `_PANE`).
assert_in_subshell "agy body calls 'command agy' (avoids recursion)" \
    ". '$ALIASES' && declare -f agy | grep -q 'command agy'"

# === agy-parity (F1): sentinel launch config, ported from ai/claude/aliases_test.sh ===
# One canonical alias per workflow: agy-yolo is gone; YOLO is `agy-config yolo on`.
assert_grep_negative "agy-yolo removed (one canonical alias)" '^agy-yolo\(\)' "$ALIASES"
assert_in_subshell "agy-config function defined after sourcing" \
    ". '$ALIASES' && type agy-config >/dev/null 2>&1"
assert_in_subshell "agy_launch_flags function defined after sourcing" \
    ". '$ALIASES' && type agy_launch_flags >/dev/null 2>&1"
assert_in_subshell "AGY_CONFIG_DIR set" ". '$ALIASES' && [ -n \"\$AGY_CONFIG_DIR\" ]"
assert_in_subshell "AGY_YOLO_FILE set" ". '$ALIASES' && [ -n \"\$AGY_YOLO_FILE\" ]"
assert_in_subshell "agy body calls agy_launch_flags" \
    ". '$ALIASES' && declare -f agy | grep -q 'agy_launch_flags'"

# --- agy-config (isolated XDG dir per case) ---
assert_in_subshell "yolo defaults OFF (no sentinel)" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && [ ! -f \"\$AGY_YOLO_FILE\" ]"
assert_in_subshell "agy-config yolo on creates sentinel" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && agy-config yolo on >/dev/null && [ -f \"\$AGY_YOLO_FILE\" ]"
assert_in_subshell "agy-config yolo off removes sentinel" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && agy-config yolo on >/dev/null && agy-config yolo off >/dev/null && [ ! -f \"\$AGY_YOLO_FILE\" ]"
assert_in_subshell "agy-config status reports yolo OFF by default" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && agy-config | grep -q 'yolo    OFF'"
assert_in_subshell "agy-config status reports yolo ON after opt-in" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && agy-config yolo on >/dev/null && agy-config status | grep -q 'yolo    ON'"
assert_in_subshell "agy-config rejects unknown setting (exit 2)" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; agy-config bogus >/dev/null 2>&1; [ \$? -eq 2 ]"
assert_in_subshell "agy-config yolo rejects bad value (exit 2)" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; agy-config yolo maybe >/dev/null 2>&1; [ \$? -eq 2 ]"

# --- agy-config doctor (single-binary diagnostic) ---
assert_in_subshell "doctor prints the resolved binary line" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; agy-config doctor 2>&1 | grep -q 'command agy ->'"
assert_in_subshell "doctor: single binary => no warning, exit 0" \
    "H=\$(mktemp -d); mkdir -p \"\$H/b1\"; : > \"\$H/b1/agy\"; chmod +x \"\$H/b1/agy\"; export PATH=\"\$H/b1:/usr/bin:/bin\"; . '$ALIASES'; out=\$(agy-config doctor 2>&1); rc=\$?; [ \$rc -eq 0 ] && printf '%s' \"\$out\" | grep -q \"\$H/b1/agy\" && ! printf '%s' \"\$out\" | grep -q WARNING"
assert_in_subshell "doctor: multiple binaries => WARNING + nonzero" \
    "H=\$(mktemp -d); mkdir -p \"\$H/b1\" \"\$H/b2\"; : > \"\$H/b1/agy\"; : > \"\$H/b2/agy\"; chmod +x \"\$H/b1/agy\" \"\$H/b2/agy\"; export PATH=\"\$H/b1:\$H/b2:/usr/bin:/bin\"; . '$ALIASES'; agy-config doctor >/dev/null 2>&1; [ \$? -ne 0 ]"
assert_in_subshell "doctor: dedups repeated PATH entries (no false warning)" \
    "H=\$(mktemp -d); mkdir -p \"\$H/b1\"; : > \"\$H/b1/agy\"; chmod +x \"\$H/b1/agy\"; export PATH=\"\$H/b1:\$H/b1:/usr/bin:/bin\"; . '$ALIASES'; agy-config doctor >/dev/null 2>&1; [ \$? -eq 0 ]"
assert_in_subshell "doctor: resolves the binary under zsh (PATH-split regression)" \
    "command -v zsh >/dev/null 2>&1 || exit 0; H=\$(mktemp -d); mkdir -p \"\$H/b1\"; : > \"\$H/b1/agy\"; chmod +x \"\$H/b1/agy\"; export H ALIASES='$ALIASES'; zsh -c 'export PATH=\"\$H/b1:/usr/bin:/bin\"; . \"\$ALIASES\"; agy-config doctor 2>&1 | grep -q \"\$H/b1/agy\"'"

# --- Flag injection (exercised directly, TTY state passed in) ---
assert_in_subshell "default: no flags injected" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; agy_launch_flags tty 'fix bug'; [ \${#AGY_LAUNCH_FLAGS[@]} -eq 0 ]"
assert_in_subshell "yolo on: injects --dangerously-skip-permissions" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; agy-config yolo on >/dev/null; agy_launch_flags other 'x'; printf '%s\n' \"\${AGY_LAUNCH_FLAGS[@]}\" | grep -q -- '--dangerously-skip-permissions'"
assert_in_subshell "yolo on: prompt not captured into flags" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; agy-config yolo on >/dev/null; agy_launch_flags tty 'fix bug'; ! printf '%s\n' \"\${AGY_LAUNCH_FLAGS[@]}\" | grep -qx 'fix bug'"
assert_in_subshell "yolo on + print mode: flag still injected (applies to every mode)" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; agy-config yolo on >/dev/null; agy_launch_flags other -p 'x'; printf '%s\n' \"\${AGY_LAUNCH_FLAGS[@]}\" | grep -q -- '--dangerously-skip-permissions'"
assert_in_subshell "yolo off: no --dangerously-skip-permissions" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; agy_launch_flags tty 'x'; ! printf '%s\n' \"\${AGY_LAUNCH_FLAGS[@]:-}\" | grep -q -- '--dangerously-skip-permissions'"

_test_report
