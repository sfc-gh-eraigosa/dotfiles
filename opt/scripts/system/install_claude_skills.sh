#!/bin/bash
# install_claude_skills.sh - Idempotently install Claude Code's assistant-specific
# config: settings.json, slash commands, hooks, and shell aliases.
#
# Mirrors install_gemini_skills.sh (which handles Gemini's policies/commands/aliases).
# Skill linking is NOT done here — sync-skills.sh is the single canonical linker that
# discovers every SKILL.md and links it into BOTH ~/.agents/skills (Gemini) and
# ~/.claude/skills (Claude). Run sync-skills.sh (or the `sync-skills` alias) to refresh
# skills; this script only owns the Claude-specific config below.

set -e

BASE_DIR="$(cd "$(dirname "$0")/../../.." && pwd)"

echo "Configuring Claude Code..."

CLAUDE_HOME="${HOME}/.claude"
mkdir -p "$CLAUDE_HOME/commands"

cleanup_broken_links() {
    local dir="$1"
    if [ -d "$dir" ]; then
        find "$dir" -type l ! -exec test -e {} \; -delete 2>/dev/null || true
    fi
}

# --- settings.json (seed from template if absent, then symlink) ---
# settings.json is gitignored so each host can hold its own customizations
# (apiKeyHelper paths, ANTHROPIC_BASE_URL, enabledPlugins, etc.) without
# leaking them into the repo. The .template file is the tracked baseline.
SETTINGS_SRC="$BASE_DIR/ai/claude/settings.json"
SETTINGS_TEMPLATE="$BASE_DIR/ai/claude/settings.json.template"
if [ ! -f "$SETTINGS_SRC" ] && [ -f "$SETTINGS_TEMPLATE" ]; then
    echo "  Seeding ai/claude/settings.json from template (first run)"
    cp "$SETTINGS_TEMPLATE" "$SETTINGS_SRC"
fi
if [ -f "$SETTINGS_SRC" ]; then
    if [ -f "$CLAUDE_HOME/settings.json" ] && [ ! -L "$CLAUDE_HOME/settings.json" ]; then
        echo "  Backing up existing settings.json -> settings.json.bak"
        mv "$CLAUDE_HOME/settings.json" "$CLAUDE_HOME/settings.json.bak"
    fi
    ln -sf "$SETTINGS_SRC" "$CLAUDE_HOME/settings.json"
fi

# --- Commands (.md slash commands) ---
if [ -d "$BASE_DIR/ai/claude/commands" ]; then
    echo "  Setting up custom slash commands..."
    for cmd in "$BASE_DIR/ai/claude/commands"/*.md; do
        [ -e "$cmd" ] || continue
        cmd_name=$(basename "$cmd")
        echo "    Linking $cmd_name"
        ln -sf "$cmd" "$CLAUDE_HOME/commands/$cmd_name"
    done
fi

cleanup_broken_links "$CLAUDE_HOME/commands"

# --- Skills ---
# Handled by sync-skills.sh, which links every discovered SKILL.md into
# ~/.claude/skills (and ~/.agents/skills for Gemini). Nothing to do here.

# --- Hooks (referenced by absolute path from settings.json) ---
# We don't symlink hooks — settings.json points at them in-place. Just ensure
# they remain executable in case a checkout dropped the bit.
if [ -d "$BASE_DIR/ai/claude/hooks" ]; then
    chmod +x "$BASE_DIR/ai/claude/hooks"/*.sh 2>/dev/null || true
fi

# --- Shell aliases (~/.config/claude/aliases.sh, sourced by .zshrc and .bashrc) ---
# Mirrors the tmux-mgr convention of ~/.config/<tool>/aliases.sh. The state
# file (yolo.enabled) lives in the same directory.
if [ -f "$BASE_DIR/ai/claude/aliases.sh" ]; then
    CLAUDE_XDG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/claude"
    mkdir -p "$CLAUDE_XDG_DIR"
    if [ -e "$CLAUDE_XDG_DIR/aliases.sh" ] && [ ! -L "$CLAUDE_XDG_DIR/aliases.sh" ]; then
        echo "    Backing up existing aliases.sh -> aliases.sh.bak"
        mv "$CLAUDE_XDG_DIR/aliases.sh" "$CLAUDE_XDG_DIR/aliases.sh.bak"
    fi
    ln -sf "$BASE_DIR/ai/claude/aliases.sh" "$CLAUDE_XDG_DIR/aliases.sh"
fi

# --- statusline-command.sh (shim for gsl status line) ---
# The settings.json template points statusLine.command at ~/.claude/statusline-command.sh.
# This block symlinks the repo shim into place, backing up any existing plain file.
if [ -f "$BASE_DIR/ai/claude/statusline-command.sh" ]; then
    if [ -e "$CLAUDE_HOME/statusline-command.sh" ] && [ ! -L "$CLAUDE_HOME/statusline-command.sh" ]; then
        echo "  Backing up existing statusline-command.sh -> statusline-command.sh.bak"
        mv "$CLAUDE_HOME/statusline-command.sh" "$CLAUDE_HOME/statusline-command.sh.bak"
    fi
    ln -sf "$BASE_DIR/ai/claude/statusline-command.sh" "$CLAUDE_HOME/statusline-command.sh"
    chmod +x "$BASE_DIR/ai/claude/statusline-command.sh"
fi

echo "Claude Code configuration complete."
