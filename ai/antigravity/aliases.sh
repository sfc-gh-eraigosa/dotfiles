#!/usr/bin/env bash
# shellcheck shell=bash
# shellcheck disable=SC2139  # the sync-* aliases expand $HOME at definition time on purpose ($HOME is stable for the shell's lifetime)
# Shell helpers for the Antigravity CLI (agy).
# Sourced from opt/profiles/.zshrc and opt/profiles/.bashrc after .antigravity.profile
# (so the runtime shell is always bash or zsh — both support arrays/`local`,
# which the flag injection below relies on; never executed by a bare POSIX sh).
#
# Mirrors ai/claude/aliases.sh (the `claude` / `claude-config` pair) so both
# assistants start the same way — see docs/mbo/designs/agy-parity.md.
#
# Commands:
#
#   agy             Honors the on-disk launch config and auto-anchors in tmux when possible.
#                     - If running inside a tmux pane: anchors the pane as "antigravity"
#                       so AI-driven 'tmux-mgr window split' targets it correctly.
#                     - If not in tmux but a server is running: prints a nudge.
#                     - Injects flags based on the config sentinel below (OFF by
#                       default — opt in with `agy-config`):
#                     agy "fix bug"        -> agy [--dangerously-skip-permissions] "fix bug"
#                     agy -p "..."         -> agy [--dangerously-skip-permissions] -p "..."
#
#   agy-config      Show or change the launch config:
#                     agy-config              # show current state
#                     agy-config yolo on|off  # --dangerously-skip-permissions
#                     agy-config doctor       # report the installed agy binary,
#                                             # warn if more than one is on PATH
#
# Config is expressed as a sentinel file under $XDG_CONFIG_HOME/antigravity
# (defaults to ~/.config/antigravity/ — the same dir this file is copied into),
# kept out of ~/.gemini/ (owned by agy). Present => ON, absent => OFF, so the
# safe, no-config default is OFF (you opt in explicitly, per machine):
#
#   yolo.enabled     present => YOLO ON   (default OFF — opt in).
#
# There is no remote-control sentinel: agy has no --remote-control flag
# (remote presence is agy's own remoteControlHostname setting).
#
# Note: the config logic lives in top-level functions (NOT underscore-prefixed).
# Claude Code's shell-snapshot mechanism strips functions whose names start
# with '_'; agy does not snapshot today, but the same shape keeps both files
# safe if it ever does (ai/antigravity/aliases_test.sh guards it).

AGY_CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/antigravity"
AGY_YOLO_FILE="$AGY_CONFIG_DIR/yolo.enabled"

# Decide which launch flags to inject. Populates the global array
# AGY_LAUNCH_FLAGS. TTY state is PASSED IN (not probed) so the test driver can
# exercise this deterministically; YOLO applies to every mode (interactive and
# print), so today the TTY state is informational only.
#   $1     : "tty" if stdin is an interactive terminal, anything else otherwise
#   $2..   : the user's agy args
agy_launch_flags() {
    AGY_LAUNCH_FLAGS=()
    agy_tty="$1"
    shift

    # YOLO (opt-in): present sentinel => auto-approve tool permission requests.
    [ -f "$AGY_YOLO_FILE" ] && AGY_LAUNCH_FLAGS+=(--dangerously-skip-permissions)

    : "$agy_tty"
}

agy() {
    # Auto-anchor in tmux so AI-driven pane splits target this pane.
    if [ -n "$TMUX_PANE" ]; then
        command -v tmux-mgr >/dev/null 2>&1 && tmux-mgr pane anchor "antigravity" 2>/dev/null || true
    elif tmux info >/dev/null 2>&1; then
        echo "Tip: run inside a tmux pane for AI pane-split support." \
             "Use 'tmux-start' or 'tmux new-session -A -s main' first." >&2
    fi

    agy_tty="other"
    [ -t 0 ] && agy_tty="tty"
    agy_launch_flags "$agy_tty" "$@"
    command agy "${AGY_LAUNCH_FLAGS[@]}" "$@"
}

agy-config() {
    mkdir -p "$AGY_CONFIG_DIR"
    case "$1" in
        yolo)
            case "$2" in
                on)  : > "$AGY_YOLO_FILE"
                     echo "agy YOLO: ON  — agy runs with --dangerously-skip-permissions." ;;
                off) rm -f "$AGY_YOLO_FILE"
                     echo "agy YOLO: OFF — agy prompts for permissions." ;;
                *)   echo "usage: agy-config yolo on|off" >&2; return 2 ;;
            esac
            ;;
        doctor)
            # Report which agy binary the wrapper actually runs (via PATH —
            # `command agy` bypasses this function but still does a PATH
            # lookup) and warn if more than one is installed. The canonical
            # binary is ~/.local/bin/agy, installed by antigravity_install.sh
            # (Google's checksummed bootstrapper). Split PATH via tr, not
            # `for d in $PATH`: zsh does not word-split unquoted variables.
            agy_all="$(
                printf '%s\n' "$PATH" | tr ':' '\n' | while IFS= read -r agy_d; do
                    [ -x "$agy_d/agy" ] && printf '%s\n' "$agy_d/agy"
                done | awk '!seen[$0]++'
            )"
            agy_resolved="$(printf '%s\n' "$agy_all" | sed '/^$/d' | head -1)"
            agy_n="$(printf '%s\n' "$agy_all" | grep -c .)"
            echo "agy binary diagnostics:"
            echo "  command agy -> ${agy_resolved:-<not found>}"
            echo "  on PATH ($agy_n):"
            printf '%s\n' "$agy_all" | sed '/^$/d; s/^/    /'
            if [ "$agy_n" -gt 1 ]; then
                echo "  WARNING: more than one agy binary is installed. The dotfiles"
                echo "  installer (antigravity_install.sh, run by install.sh) keeps ONE"
                echo "  canonical binary at ~/.local/bin/agy. Remove the extra copy."
                return 1
            fi
            ;;
        ""|status)
            [ -f "$AGY_YOLO_FILE" ] && agy_yolo_state="ON" || agy_yolo_state="OFF"
            echo "agy launch config:"
            echo "  yolo    $agy_yolo_state   (agy-config yolo on|off)"
            echo "  (run 'agy-config doctor' to check the installed binary)"
            ;;
        *)
            echo "usage: agy-config [status]" >&2
            echo "       agy-config yolo on|off" >&2
            echo "       agy-config doctor" >&2
            return 2
            ;;
    esac
}

alias sync-skills="bash \$HOME/opt/scripts/system/sync-skills.sh"
alias sync-teams="bash \$HOME/opt/scripts/system/install_ai_teams.sh"
alias sync-forks="bash \$HOME/opt/scripts/../../ai/skills/sync-forks/scripts/check_and_sync_forks.sh"
