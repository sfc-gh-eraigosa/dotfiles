#!/bin/sh
# Shell helpers for the Gemini CLI.
# Sourced from opt/profiles/.zshrc and opt/profiles/.bashrc after .gemini.profile.
#
# Commands:
#
#   gemini          Auto-anchors in tmux when possible, then runs gemini.
#                     - If running inside a tmux pane: anchors the pane as "gemini"
#                       so AI-driven 'tmux-mgr window split' targets it correctly.
#                     - If not in tmux but a server is running: prints a nudge.
#
#   gemini-yolo     Run gemini with -y (auto-approve) for the current invocation.

gemini() {
    # Auto-anchor in tmux so AI-driven pane splits target this pane.
    if [ -n "$TMUX_PANE" ]; then
        command -v tmux-mgr >/dev/null 2>&1 && tmux-mgr pane anchor "gemini" 2>/dev/null || true
    elif tmux info >/dev/null 2>&1; then
        echo "Tip: run inside a tmux pane for AI pane-split support." \
             "Use 'tmux-start' or 'tmux new-session -A -s main' first." >&2
    fi
    command gemini "$@"
}

gemini-yolo() {
    if [ -n "$TMUX_PANE" ]; then
        command -v tmux-mgr >/dev/null 2>&1 && tmux-mgr pane anchor "gemini" 2>/dev/null || true
    fi
    command gemini -y "$@"
}

alias sync-skills="bash $HOME/git/dotfiles/opt/scripts/system/sync-skills.sh"
alias sync-forks="bash $HOME/git/dotfiles/ai/skills/sync-forks/scripts/check_and_sync_forks.sh"
