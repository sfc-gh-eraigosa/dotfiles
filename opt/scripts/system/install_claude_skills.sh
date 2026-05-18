#!/bin/bash
# install_claude_skills.sh - Idempotently install Claude Code skills, commands, and settings.
#
# Mirrors install_gemini_skills.sh. Reuses the same SKILL.md skill directories
# under src/*/skill/ (the SKILL.md format is shared between Claude and Gemini),
# so a single source of truth drives both assistants.

set -e

BASE_DIR="$(cd "$(dirname "$0")/../../.." && pwd)"

echo "Configuring Claude Code..."

CLAUDE_HOME="${HOME}/.claude"
mkdir -p "$CLAUDE_HOME/skills" "$CLAUDE_HOME/commands"

cleanup_broken_links() {
    local dir="$1"
    if [ -d "$dir" ]; then
        find "$dir" -type l ! -exec test -e {} \; -delete 2>/dev/null || true
    fi
}

# --- settings.json (back up real file if present, then symlink) ---
if [ -f "$BASE_DIR/ai/claude/settings.json" ]; then
    if [ -f "$CLAUDE_HOME/settings.json" ] && [ ! -L "$CLAUDE_HOME/settings.json" ]; then
        echo "  Backing up existing settings.json -> settings.json.bak"
        mv "$CLAUDE_HOME/settings.json" "$CLAUDE_HOME/settings.json.bak"
    fi
    ln -sf "$BASE_DIR/ai/claude/settings.json" "$CLAUDE_HOME/settings.json"
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
echo "  Linking skills..."

link_skill() {
    local src="$1"
    local dest="$2"

    if [ ! -d "$src" ]; then
        echo "    Skipping missing skill src: $src"
        return
    fi
    src="${src%/}"

    if [ -L "$dest" ]; then
        if [ "$(readlink "$dest")" = "$src" ]; then
            return
        fi
        rm "$dest"
    elif [ -e "$dest" ]; then
        echo "    Warning: $dest exists but is not a symlink. Moving to $dest.bak"
        mv "$dest" "$dest.bak"
    fi

    echo "    Linking $(basename "$dest") -> $src"
    ln -s "$src" "$dest"
}

# Reuse the same skill directories Gemini uses (SKILL.md is the shared format)
link_skill "$BASE_DIR/src/ssh-host-finder"   "$CLAUDE_HOME/skills/ssh-host-finder"
link_skill "$BASE_DIR/src/tmux-mgr/skill"    "$CLAUDE_HOME/skills/tmux"
link_skill "$BASE_DIR/src/gss/skill"         "$CLAUDE_HOME/skills/git-safe-sync"
link_skill "$BASE_DIR/src/ssh-key-sync"      "$CLAUDE_HOME/skills/ssh-key-sync"

cleanup_broken_links "$CLAUDE_HOME/skills"

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

echo "Claude Code configuration complete."
