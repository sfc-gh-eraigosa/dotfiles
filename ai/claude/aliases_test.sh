#!/bin/bash
# Test driver for ai/claude/aliases.sh
#
# Why: the `claude` / `claude-config` functions must stay free of private
# helpers. Claude Code's shell-snapshot mechanism strips functions whose names
# start with `_`, so a `_claude_yolo_enabled` helper would survive in the live
# shell but vanish from the snapshot, producing
#   "claude:8: command not found: _claude_yolo_enabled"
# on every Claude Code launch. The top-level-function design avoids that; this
# driver guards against any reintroduction of an underscore helper, and covers
# the claude-config tool plus the actual flag-injection behavior of
# claude_launch_flags (TTY/print guards, the opt-in sentinels, and the
# regression guard for the --remote-control prompt-swallow bug).
#
# claude_launch_flags(tty_state, args...) populates the global array
# CLAUDE_LAUNCH_FLAGS, so we can exercise the real decision logic deterministically
# without shelling out to the claude binary (TTY state is passed in, not probed).
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

# `claude` must NOT be accidentally removed.
assert_grep_negative "claude() not removed by accident" \
    '^# REMOVED claude\(\)' "$ALIASES"

# CROSS-REPO CONTRACT GUARD: the literal `CLAUDE_YOLO_FILE=` assignment must
# survive. Playground's claude-local reads this exact variable; a rename here
# silently breaks it, so trip a red test if it ever disappears.
assert_grep "CLAUDE_YOLO_FILE cross-repo contract var preserved" \
    '^CLAUDE_YOLO_FILE=' "$ALIASES"

# The remote-control injection MUST pass an explicit name so the optional
# --remote-control [name] arg cannot swallow the user's positional prompt.
assert_grep "remote-control passes an explicit session name (anti prompt-swallow)" \
    '\-\-remote-control "\$\(basename' "$ALIASES"

# === Runtime checks (source the file in a subshell) ===
assert_in_subshell "claude function defined after sourcing" \
    ". '$ALIASES' && type claude >/dev/null 2>&1"
assert_in_subshell "claude-config function defined after sourcing" \
    ". '$ALIASES' && type claude-config >/dev/null 2>&1"
assert_in_subshell "claude_launch_flags function defined after sourcing" \
    ". '$ALIASES' && type claude_launch_flags >/dev/null 2>&1"
# `claude` must NOT reference any undefined command at parse time.
assert_in_subshell "claude body does not call _claude_yolo_enabled" \
    ". '$ALIASES' && ! declare -f claude | grep -q '_claude_yolo_enabled'"
assert_in_subshell "claude-config body does not call _claude_yolo_enabled" \
    ". '$ALIASES' && ! declare -f claude-config | grep -q '_claude_yolo_enabled'"

# === Variable setup ===
assert_in_subshell "CLAUDE_CONFIG_DIR set" \
    ". '$ALIASES' && [ -n \"\$CLAUDE_CONFIG_DIR\" ]"
assert_in_subshell "CLAUDE_YOLO_FILE set" \
    ". '$ALIASES' && [ -n \"\$CLAUDE_YOLO_FILE\" ]"
assert_in_subshell "CLAUDE_REMOTE_ENABLED_FILE set" \
    ". '$ALIASES' && [ -n \"\$CLAUDE_REMOTE_ENABLED_FILE\" ]"

# === claude-config behavior (isolated XDG dir per case) ===
# Both settings default OFF (opt-in): no sentinels present after a fresh source.
assert_in_subshell "remote defaults OFF (no enabled sentinel)" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && [ ! -f \"\$CLAUDE_REMOTE_ENABLED_FILE\" ]"
assert_in_subshell "yolo defaults OFF (no enabled sentinel)" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && [ ! -f \"\$CLAUDE_YOLO_FILE\" ]"
assert_in_subshell "claude-config remote on creates enabled sentinel" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && claude-config remote on >/dev/null && [ -f \"\$CLAUDE_REMOTE_ENABLED_FILE\" ]"
assert_in_subshell "claude-config remote off removes enabled sentinel" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && claude-config remote on >/dev/null && claude-config remote off >/dev/null && [ ! -f \"\$CLAUDE_REMOTE_ENABLED_FILE\" ]"
assert_in_subshell "claude-config yolo on creates yolo sentinel" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && claude-config yolo on >/dev/null && [ -f \"\$CLAUDE_YOLO_FILE\" ]"
assert_in_subshell "claude-config yolo off removes yolo sentinel" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && claude-config yolo on >/dev/null && claude-config yolo off >/dev/null && [ ! -f \"\$CLAUDE_YOLO_FILE\" ]"
assert_in_subshell "claude-config status reports remote OFF by default" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && claude-config | grep -q 'remote  OFF'"
assert_in_subshell "claude-config status reports yolo OFF by default" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && claude-config | grep -q 'yolo    OFF'"
assert_in_subshell "claude-config rejects unknown setting (exit 2)" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; claude-config bogus >/dev/null 2>&1; [ \$? -eq 2 ]"

# === claude-config doctor (single-binary diagnostic) ===
# Header line always printed.
assert_in_subshell "doctor prints the resolved binary line" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; claude-config doctor 2>&1 | grep -q 'command claude ->'"
# Exactly one claude on a controlled PATH: no warning, exit 0, resolves to it.
assert_in_subshell "doctor: single binary => no warning, exit 0" \
    "H=\$(mktemp -d); mkdir -p \"\$H/b1\"; : > \"\$H/b1/claude\"; chmod +x \"\$H/b1/claude\"; export PATH=\"\$H/b1:/usr/bin:/bin\"; . '$ALIASES'; out=\$(claude-config doctor 2>&1); rc=\$?; [ \$rc -eq 0 ] && printf '%s' \"\$out\" | grep -q \"\$H/b1/claude\" && ! printf '%s' \"\$out\" | grep -q WARNING"
# Two claude binaries on PATH: warns and returns non-zero.
assert_in_subshell "doctor: multiple binaries => WARNING + nonzero" \
    "H=\$(mktemp -d); mkdir -p \"\$H/b1\" \"\$H/b2\"; : > \"\$H/b1/claude\"; : > \"\$H/b2/claude\"; chmod +x \"\$H/b1/claude\" \"\$H/b2/claude\"; export PATH=\"\$H/b1:\$H/b2:/usr/bin:/bin\"; . '$ALIASES'; claude-config doctor >/dev/null 2>&1; [ \$? -ne 0 ]"
# De-dups a binary that appears under repeated PATH entries (one logical claude).
assert_in_subshell "doctor: dedups repeated PATH entries (no false warning)" \
    "H=\$(mktemp -d); mkdir -p \"\$H/b1\"; : > \"\$H/b1/claude\"; chmod +x \"\$H/b1/claude\"; export PATH=\"\$H/b1:\$H/b1:/usr/bin:/bin\"; . '$ALIASES'; claude-config doctor >/dev/null 2>&1; [ \$? -eq 0 ]"
# Cross-shell regression: zsh does not word-split unquoted variables, so a
# 'for d in \$PATH' scan sees PATH as ONE word and doctor reports <not found>
# in the user's login shell. Skips (passes) when zsh is not installed.
assert_in_subshell "doctor: resolves the binary under zsh (PATH-split regression)" \
    "command -v zsh >/dev/null 2>&1 || exit 0; H=\$(mktemp -d); mkdir -p \"\$H/b1\"; : > \"\$H/b1/claude\"; chmod +x \"\$H/b1/claude\"; export H ALIASES='$ALIASES'; zsh -c 'export PATH=\"\$H/b1:/usr/bin:/bin\"; . \"\$ALIASES\"; claude-config doctor 2>&1 | grep -q \"\$H/b1/claude\"'"

# === Flag-injection behavior (the headline feature — exercised directly) ===
# Default (nothing opted in), interactive, with a prompt: NO flags injected.
assert_in_subshell "default opt-out: no flags injected for interactive prompt" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; claude_launch_flags tty 'fix bug'; [ \${#CLAUDE_LAUNCH_FLAGS[@]} -eq 0 ]"

# Remote ON + interactive + prompt: injects exactly --remote-control + a name
# (2 elements), and the user's prompt is NOT in the flags array (the blocker
# regression guard — proves --remote-control cannot swallow the prompt).
assert_in_subshell "remote on: injects --remote-control with explicit name" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; claude-config remote on >/dev/null; claude_launch_flags tty 'fix bug'; [ \"\${CLAUDE_LAUNCH_FLAGS[0]}\" = '--remote-control' ] && [ \${#CLAUDE_LAUNCH_FLAGS[@]} -eq 2 ]"
assert_in_subshell "remote on: user prompt is NOT captured into the flags (anti prompt-swallow)" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; claude-config remote on >/dev/null; claude_launch_flags tty 'fix bug'; ! printf '%s\n' \"\${CLAUDE_LAUNCH_FLAGS[@]}\" | grep -qx 'fix bug'"

# Remote ON but NON-interactive (no TTY): suppressed.
assert_in_subshell "remote on but non-tty: no --remote-control" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; claude-config remote on >/dev/null; claude_launch_flags other 'fix bug'; ! printf '%s\n' \"\${CLAUDE_LAUNCH_FLAGS[@]:-}\" | grep -q -- '--remote-control'"

# Remote ON + interactive + print mode (-p / --print): suppressed.
assert_in_subshell "remote on but -p print mode: no --remote-control" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; claude-config remote on >/dev/null; claude_launch_flags tty -p 'do x'; ! printf '%s\n' \"\${CLAUDE_LAUNCH_FLAGS[@]:-}\" | grep -q -- '--remote-control'"
assert_in_subshell "remote on but --print mode: no --remote-control" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; claude-config remote on >/dev/null; claude_launch_flags tty --print 'do x'; ! printf '%s\n' \"\${CLAUDE_LAUNCH_FLAGS[@]:-}\" | grep -q -- '--remote-control'"

# False-positive guard: a prompt that merely CONTAINS the token '-p' must NOT
# be treated as print mode (positional scan, not substring match).
assert_in_subshell "prompt containing '-p' is not mistaken for print mode" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; claude-config remote on >/dev/null; claude_launch_flags tty 'tell me about the -p flag'; printf '%s\n' \"\${CLAUDE_LAUNCH_FLAGS[@]}\" | grep -q -- '--remote-control'"

# YOLO ON: injects --dangerously-skip-permissions regardless of TTY.
assert_in_subshell "yolo on: injects --dangerously-skip-permissions" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; claude-config yolo on >/dev/null; claude_launch_flags other 'x'; printf '%s\n' \"\${CLAUDE_LAUNCH_FLAGS[@]}\" | grep -q -- '--dangerously-skip-permissions'"
assert_in_subshell "yolo off: no --dangerously-skip-permissions" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; claude_launch_flags tty 'x'; ! printf '%s\n' \"\${CLAUDE_LAUNCH_FLAGS[@]:-}\" | grep -q -- '--dangerously-skip-permissions'"

# Both ON + interactive: both flags present, prompt still not swallowed.
assert_in_subshell "yolo+remote on: both flags injected, prompt preserved" \
    "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; claude-config yolo on >/dev/null; claude-config remote on >/dev/null; claude_launch_flags tty 'fix bug'; printf '%s\n' \"\${CLAUDE_LAUNCH_FLAGS[@]}\" | grep -q -- '--remote-control' && printf '%s\n' \"\${CLAUDE_LAUNCH_FLAGS[@]}\" | grep -q -- '--dangerously-skip-permissions' && ! printf '%s\n' \"\${CLAUDE_LAUNCH_FLAGS[@]}\" | grep -qx 'fix bug'"

_test_report
