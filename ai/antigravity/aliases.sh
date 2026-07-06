#!/usr/bin/env bash
# shellcheck shell=bash
# Shell helpers for the Antigravity CLI (agy).
# Sourced from opt/profiles/.zshrc and opt/profiles/.bashrc after .antigravity.profile.
#
# Commands:
#
#   agy             Auto-anchors in tmux when possible, then runs agy.
#                     - If running inside a tmux pane: anchors the pane as "antigravity"
#                       so AI-driven 'tmux-mgr window split' targets it correctly.
#                     - If not in tmux but a server is running: prints a nudge.
#
#   agy-yolo        Run agy with --dangerously-skip-permissions (auto-approve)
#                   for the current invocation.

agy() {
    # Auto-anchor in tmux so AI-driven pane splits target this pane.
    if [ -n "$TMUX_PANE" ]; then
        command -v tmux-mgr >/dev/null 2>&1 && tmux-mgr pane anchor "antigravity" 2>/dev/null || true
    elif tmux info >/dev/null 2>&1; then
        echo "Tip: run inside a tmux pane for AI pane-split support." \
             "Use 'tmux-start' or 'tmux new-session -A -s main' first." >&2
    fi
    command agy "$@"
}

agy-yolo() {
    if [ -n "$TMUX_PANE" ]; then
        command -v tmux-mgr >/dev/null 2>&1 && tmux-mgr pane anchor "antigravity" 2>/dev/null || true
    fi
    command agy --dangerously-skip-permissions "$@"
}

alias sync-skills="bash \$HOME/opt/scripts/system/sync-skills.sh"
alias sync-teams="bash \$HOME/opt/scripts/system/install_ai_teams.sh"
alias sync-forks="bash \$HOME/opt/scripts/../../ai/skills/sync-forks/scripts/check_and_sync_forks.sh"
