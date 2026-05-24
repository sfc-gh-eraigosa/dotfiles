#!/bin/sh
# Shell helpers for the Claude Code CLI.
# Sourced from opt/profiles/.zshrc and opt/profiles/.bashrc.
#
# Commands:
#
#   claude          Honors the YOLO toggle and auto-anchors in tmux when possible.
#                     - If running inside a tmux pane: anchors the pane as "claude"
#                       so AI-driven 'tmux-mgr window split' targets it correctly.
#                     - If not in tmux but a server is running: prints a nudge.
#                     claude "fix bug"               -> claude [--dangerously-skip-permissions] "fix bug"
#                     claude --model haiku "..."     -> claude [--dangerously-skip-permissions] --model haiku "..."
#                     claude --continue              -> claude [--dangerously-skip-permissions] --continue
#
#   claude-toggle   flip YOLO on/off and report the new state.
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
    # Auto-anchor in tmux so AI-driven pane splits target this pane.
    if [ -n "$TMUX_PANE" ]; then
        command -v tmux-mgr >/dev/null 2>&1 && tmux-mgr pane anchor "claude" 2>/dev/null || true
    elif tmux info >/dev/null 2>&1; then
        echo "Tip: run inside a tmux pane for AI pane-split support." \
             "Use 'tmux-start' or 'tmux new-session -A -s main' first." >&2
    fi
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

alias sync-skills="bash $HOME/git/dotfiles/opt/scripts/system/sync-skills.sh"
