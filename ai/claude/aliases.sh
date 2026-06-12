#!/bin/sh
# shellcheck shell=bash
# shellcheck disable=SC2139  # the sync-* aliases expand $HOME at definition time on purpose ($HOME is stable for the shell's lifetime)
# Shell helpers for the Claude Code CLI.
# Sourced from opt/profiles/.zshrc and opt/profiles/.bashrc (so the runtime
# shell is always bash or zsh — both support arrays/`local`, which the flag
# injection below relies on; this file is never executed by a bare POSIX sh).
#
# Commands:
#
#   claude          Honors the on-disk config and auto-anchors in tmux when possible.
#                     - If running inside a tmux pane: anchors the pane as "claude"
#                       so AI-driven 'tmux-mgr window split' targets it correctly.
#                     - If not in tmux but a server is running: prints a nudge.
#                     - Injects flags based on the config sentinels below (both
#                       OFF by default — opt in with `claude-config`):
#                     claude "fix bug"           -> claude [--remote-control <dir>] [--dangerously-skip-permissions] "fix bug"
#                     claude --model haiku "..." -> claude [--remote-control <dir>] [--dangerously-skip-permissions] --model haiku "..."
#                     claude -p "..."            -> claude [--dangerously-skip-permissions] -p "..."   (never --remote-control)
#
#   claude-config   Show or change the launch config:
#                     claude-config                 # show current state
#                     claude-config yolo   on|off   # --dangerously-skip-permissions
#                     claude-config remote on|off   # --remote-control on interactive sessions
#                     claude-config doctor          # report the installed claude binary,
#                                                   # warn if more than one is on PATH
#
# Config is expressed as sentinel files under $XDG_CONFIG_HOME/claude
# (defaults to ~/.config/claude/), kept out of ~/.claude/ (owned by Claude Code).
# Both sentinels share the same polarity — present => ON, absent => OFF — so the
# safe, no-config default for both is OFF (you opt in explicitly):
#
#   yolo.enabled     present => YOLO ON   (default OFF — opt in).
#   remote.enabled   present => remote ON (default OFF — opt in).
#
# --remote-control is added ONLY for interactive sessions: it is skipped when
# stdin is not a TTY or when print mode (-p / --print) is requested, so
# scripted/headless `claude -p "..."` and piped runs are left untouched.
#
# Security note (remote-control): --remote-control opens a remote-control
# surface for the interactive session. It is OPT-IN (default OFF) and gated to
# interactive TTYs, and access is still mediated by your normal authenticated
# SSH + tmux. Because this aliases.sh is symlinked onto every machine `install`
# runs on, the opt-in default keeps fresh/shared/remote hosts quiet until you
# run `claude-config remote on` there. Turn it off again with
# `claude-config remote off`. See docs/machine-local-overrides.md.

CLAUDE_CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/claude"
# CROSS-REPO CONTRACT — DO NOT RENAME OR INLINE.
# The shell variable CLAUDE_YOLO_FILE (name + value semantics: absolute path to
# the YOLO sentinel) is read by the playground project's nano-carried
# `claude-local` helper (~/.config/nano/ollama-client.sh), which sources into the
# same interactive shell, reads ${CLAUDE_YOLO_FILE:-}, and re-applies the YOLO
# policy itself before calling `command claude`. Renaming this var, changing its
# meaning, or moving the sentinel path silently breaks `claude-local`. The
# yolo.enabled FILENAME and the claude-config tool name are NOT part of the
# contract — only this variable. Coordinate cross-repo before changing it.
CLAUDE_YOLO_FILE="$CLAUDE_CONFIG_DIR/yolo.enabled"
CLAUDE_REMOTE_ENABLED_FILE="$CLAUDE_CONFIG_DIR/remote.enabled"

# Note: the config logic lives in top-level functions (NOT underscore-prefixed)
# because Claude Code's shell-snapshot mechanism strips functions whose names
# start with '_'. A `_claude_*` helper would survive in the live shell but
# vanish from the snapshot, causing "command not found" errors whenever the
# snapshot is sourced.

# Decide which launch flags to inject. Populates the global array
# CLAUDE_LAUNCH_FLAGS. TTY state is PASSED IN (not probed) so the test driver
# can exercise this deterministically. Top-level (no leading '_') so the
# shell-snapshot keeps it.
#   $1     : "tty" if stdin is an interactive terminal, anything else otherwise
#   $2..   : the user's claude args
claude_launch_flags() {
    CLAUDE_LAUNCH_FLAGS=()
    claude_tty="$1"
    shift

    # YOLO (opt-in): present sentinel => skip permission prompts.
    [ -f "$CLAUDE_YOLO_FILE" ] && CLAUDE_LAUNCH_FLAGS+=(--dangerously-skip-permissions)

    # Remote control (opt-in): only for interactive sessions, never print mode.
    if [ -f "$CLAUDE_REMOTE_ENABLED_FILE" ] && [ "$claude_tty" = "tty" ]; then
        claude_remote_ok=1
        # Scan args positionally (not a substring match on the joined string) so
        # a prompt that merely contains the token "-p" doesn't suppress remote.
        for claude_arg in "$@"; do
            case "$claude_arg" in
                -p|--print) claude_remote_ok=0; break ;;
            esac
        done
        # --remote-control takes an OPTIONAL [name]; pass an explicit name (the
        # current dir basename) so it can never consume the user's positional
        # prompt as the session name. The array carries a name-with-spaces safely.
        [ "$claude_remote_ok" = 1 ] && CLAUDE_LAUNCH_FLAGS+=(--remote-control "$(basename "$PWD")")
    fi
}

claude() {
    # Auto-anchor in tmux so AI-driven pane splits target this pane.
    if [ -n "$TMUX_PANE" ]; then
        command -v tmux-mgr >/dev/null 2>&1 && tmux-mgr pane anchor "claude" 2>/dev/null || true
    elif tmux info >/dev/null 2>&1; then
        echo "Tip: run inside a tmux pane for AI pane-split support." \
             "Use 'tmux-start' or 'tmux new-session -A -s main' first." >&2
    fi

    claude_tty="other"
    [ -t 0 ] && claude_tty="tty"
    claude_launch_flags "$claude_tty" "$@"
    command claude "${CLAUDE_LAUNCH_FLAGS[@]}" "$@"
}

claude-config() {
    mkdir -p "$CLAUDE_CONFIG_DIR"
    case "$1" in
        yolo)
            case "$2" in
                on)  : > "$CLAUDE_YOLO_FILE"
                     echo "Claude YOLO: ON  — claude runs with --dangerously-skip-permissions." ;;
                off) rm -f "$CLAUDE_YOLO_FILE"
                     echo "Claude YOLO: OFF — claude prompts for permissions." ;;
                *)   echo "usage: claude-config yolo on|off" >&2; return 2 ;;
            esac
            ;;
        remote)
            case "$2" in
                on)  : > "$CLAUDE_REMOTE_ENABLED_FILE"
                     echo "Claude Remote Control: ON  — interactive sessions start with --remote-control." ;;
                off) rm -f "$CLAUDE_REMOTE_ENABLED_FILE"
                     echo "Claude Remote Control: OFF — interactive sessions start normally." ;;
                *)   echo "usage: claude-config remote on|off" >&2; return 2 ;;
            esac
            ;;
        doctor)
            # Report which claude binary the wrapper actually runs (via PATH —
            # `command claude` bypasses this function but still does a PATH
            # lookup) and warn if more than one is installed. The canonical
            # binary is the one installed by claude_install.sh (run by
            # install.sh): npm on Linux/WSL, brew cask on macOS. Extra copies
            # (e.g. the native `curl claude.ai/install.sh` install under
            # ~/.local) are consolidated away by re-running install.sh.
            # Split PATH via tr, not `for d in $PATH`: zsh does not word-split
            # unquoted variables, so the for-loop form sees PATH as ONE word
            # and doctor reports <not found> in the user's login shell.
            claude_all="$(
                printf '%s\n' "$PATH" | tr ':' '\n' | while IFS= read -r claude_d; do
                    [ -x "$claude_d/claude" ] && printf '%s\n' "$claude_d/claude"
                done | awk '!seen[$0]++'
            )"
            claude_resolved="$(printf '%s\n' "$claude_all" | sed '/^$/d' | head -1)"
            claude_n="$(printf '%s\n' "$claude_all" | grep -c .)"
            echo "Claude binary diagnostics:"
            echo "  command claude -> ${claude_resolved:-<not found>}"
            echo "  on PATH ($claude_n):"
            printf '%s\n' "$claude_all" | sed '/^$/d; s/^/    /'
            if [ "$claude_n" -gt 1 ]; then
                echo "  WARNING: more than one claude binary is installed. The dotfiles"
                echo "  installer (claude_install.sh, run by install.sh) keeps ONE canonical"
                echo "  binary (npm on Linux/WSL, brew cask on macOS) and removes the"
                echo "  non-orchestrated native install. Re-run install.sh to consolidate."
                return 1
            fi
            ;;
        ""|status)
            [ -f "$CLAUDE_YOLO_FILE" ] && yolo_state="ON" || yolo_state="OFF"
            [ -f "$CLAUDE_REMOTE_ENABLED_FILE" ] && remote_state="ON" || remote_state="OFF"
            echo "Claude launch config:"
            echo "  yolo    $yolo_state   (claude-config yolo on|off)"
            echo "  remote  $remote_state   (claude-config remote on|off)"
            echo "  (run 'claude-config doctor' to check the installed binary)"
            ;;
        *)
            echo "usage: claude-config [status]" >&2
            echo "       claude-config yolo on|off" >&2
            echo "       claude-config remote on|off" >&2
            echo "       claude-config doctor" >&2
            return 2
            ;;
    esac
}

alias sync-skills="bash $HOME/opt/scripts/system/sync-skills.sh"
alias sync-plugins="bash $HOME/opt/scripts/system/sync-plugins.sh"
alias sync-teams="bash $HOME/opt/scripts/system/install_ai_teams.sh"
alias sync-forks="bash $HOME/opt/scripts/../../ai/skills/sync-forks/scripts/check_and_sync_forks.sh"
