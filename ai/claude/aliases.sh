#!/bin/sh
# Shell helpers for the Claude Code CLI.
# Sourced from opt/profiles/.zshrc and opt/profiles/.bashrc.
#
# Two commands:
#
#   claude          honors the YOLO toggle. Pass any args through normally:
#                     claude "fix bug"               -> claude [--dangerously-skip-permissions] "fix bug"
#                     claude --model haiku "..."     -> claude [--dangerously-skip-permissions] --model haiku "..."
#                     claude --continue              -> claude [--dangerously-skip-permissions] --continue
#
#   claude-toggle   flip YOLO on/off and report the new state. Running it
#                   without arguments tells you the current state via the
#                   message it prints after toggling.
#
# State is a single sentinel file. Present => YOLO ON, absent => OFF.
# Path: $XDG_CONFIG_HOME/claude/yolo.enabled (defaults to ~/.config/claude/).
# Kept under ~/.config/claude/ so it doesn't pollute ~/.claude/ (owned by Claude Code).

CLAUDE_YOLO_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/claude"
CLAUDE_YOLO_FILE="$CLAUDE_YOLO_DIR/yolo.enabled"

_claude_yolo_enabled() {
    [ -f "$CLAUDE_YOLO_FILE" ]
}

claude() {
    if _claude_yolo_enabled; then
        command claude --dangerously-skip-permissions "$@"
    else
        command claude "$@"
    fi
}

claude-toggle() {
    mkdir -p "$CLAUDE_YOLO_DIR"
    if _claude_yolo_enabled; then
        rm -f "$CLAUDE_YOLO_FILE"
        echo "Claude YOLO mode: OFF — claude will prompt for permissions."
    else
        : > "$CLAUDE_YOLO_FILE"
        echo "Claude YOLO mode: ON  — claude will run with --dangerously-skip-permissions."
    fi
}
