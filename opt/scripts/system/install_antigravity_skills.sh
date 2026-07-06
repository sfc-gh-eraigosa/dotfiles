#!/bin/bash
# install_antigravity_skills.sh - Idempotently install Antigravity CLI (agy)
# assistant-specific config: lifecycle hooks, shell aliases, and legacy
# Gemini CLI cleanup. Skill links are handled by sync-skills.sh (the
# canonical linker for BOTH assistants); this script only does
# antigravity-specific wiring — mirroring install_claude_skills.sh.
#
# Antigravity CLI layout (verified against agy 1.0.16):
#   ~/.gemini/antigravity-cli/settings.json   CLI-owned settings (not managed here)
#   ~/.gemini/config/                         global customization root
#     ├── skills/                             (populated by sync-skills.sh)
#     ├── hooks/                              guard scripts (copied below)
#     └── hooks.json                          hook wiring (rendered below)

set -e

# Determine the root of the dotfiles repository
BASE_DIR="$(cd "$(dirname "$0")/../../.." && pwd)"

echo "Configuring Antigravity CLI..."

AGY_CONFIG_ROOT="${HOME}/.gemini/config"

# Function to clean up broken symlinks in a directory
cleanup_broken_links() {
    local dir="$1"
    if [ -d "$dir" ]; then
        echo "  Cleaning up broken links in $dir..."
        # Portable way to find and delete broken symlinks
        find "$dir" -type l ! -exec test -e {} \; -delete
    fi
}

# --- Hooks (COPIED into ~/.gemini/config/hooks; hooks.json references the
# well-known $HOME path). Copy, not symlink, per the provisioning directive
# (CLAUDE.md -> Shell & Dotfiles Conventions). safety_guard.sh loads
# strip_heredocs.awk from its own dir, so the .awk sibling is copied too.
# antigravity_adapter.sh translates agy's hook dialect to the shared guard
# contract. Test drivers (*_test.sh) stay in the repo and are not copied. ---
if [ -d "$BASE_DIR/ai/hooks" ]; then
    echo "  Copying hooks into ~/.gemini/config/hooks..."
    HOOKS_DEST="${AGY_CONFIG_ROOT}/hooks"
    mkdir -p "$HOOKS_DEST"
    for f in "$BASE_DIR/ai/hooks"/*.sh "$BASE_DIR/ai/hooks"/*.awk; do
        [ -e "$f" ] || continue
        case "$(basename "$f")" in
            *_test.sh) continue ;;
        esac
        cp "$f" "$HOOKS_DEST/$(basename "$f")"
    done
    chmod +x "$HOOKS_DEST"/*.sh 2>/dev/null || true
fi

# --- hooks.json (rendered from the repo template; repo-owned wiring) ---
# Unlike the retired Gemini CLI (hooks lived inside settings.json and needed
# a forced-subset merge), agy keeps hook wiring in a dedicated hooks.json —
# so the whole file is ours to render. __HOME__ is substituted because agy
# does not expand env vars in hook command strings.
HOOKS_JSON_SRC="$BASE_DIR/ai/antigravity/hooks.json.template"
if [ -f "$HOOKS_JSON_SRC" ]; then
    echo "  Rendering ~/.gemini/config/hooks.json..."
    mkdir -p "$AGY_CONFIG_ROOT"
    sed "s|__HOME__|${HOME}|g" "$HOOKS_JSON_SRC" > "$AGY_CONFIG_ROOT/hooks.json"
fi

# --- Shell aliases (~/.config/antigravity/aliases.sh, sourced by .zshrc and
# .bashrc). Provides the agy() wrapper with tmux auto-anchor support. ---
AGY_XDG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/antigravity"
if [ -f "$BASE_DIR/ai/antigravity/aliases.sh" ]; then
    echo "  Linking antigravity aliases.sh..."
    mkdir -p "$AGY_XDG_DIR"
    if [ -e "$AGY_XDG_DIR/aliases.sh" ] && [ ! -L "$AGY_XDG_DIR/aliases.sh" ]; then
        echo "    Backing up existing aliases.sh -> aliases.sh.bak"
        mv "$AGY_XDG_DIR/aliases.sh" "$AGY_XDG_DIR/aliases.sh.bak"
    fi
    ln -sf "$BASE_DIR/ai/antigravity/aliases.sh" "$AGY_XDG_DIR/aliases.sh"
fi

# --- Legacy Gemini CLI cleanup (one-time migration, idempotent) ---
# Gemini CLI was retired 2026-06-18. Remove the repo-managed artifacts it
# left behind; leave host-owned files (settings.json, oauth creds, history)
# alone — Antigravity reuses ~/.gemini and they don't conflict.
echo "  Cleaning up retired Gemini CLI artifacts..."
# 1. Policy/command symlinks that pointed into this repo
for legacy_dir in "${HOME}/.gemini/policies" "${HOME}/.gemini/commands"; do
    if [ -d "$legacy_dir" ]; then
        find "$legacy_dir" -maxdepth 1 -type l | while read -r link; do
            case "$(readlink "$link")" in
                "$BASE_DIR"/*) rm -f "$link" ;;
            esac
        done
        rmdir "$legacy_dir" 2>/dev/null || true
    fi
done
# 2. Gemini's hook copies (agy reads ~/.gemini/config/hooks instead)
if [ -d "${HOME}/.gemini/hooks" ]; then
    rm -rf "${HOME}/.gemini/hooks"
fi
# 3. Gemini aliases dir (the agy() wrapper lives in ~/.config/antigravity now)
LEGACY_GEMINI_XDG="${XDG_CONFIG_HOME:-$HOME/.config}/gemini"
if [ -L "$LEGACY_GEMINI_XDG/aliases.sh" ]; then
    rm -f "$LEGACY_GEMINI_XDG/aliases.sh"
    rmdir "$LEGACY_GEMINI_XDG" 2>/dev/null || true
fi
# 4. Gemini CLI's global skills root (~/.agents/skills): remove only links
#    that point into this repo; sync-skills.sh targets ~/.gemini/config/skills.
if [ -d "${HOME}/.agents/skills" ]; then
    find "${HOME}/.agents/skills" -maxdepth 1 -type l | while read -r link; do
        case "$(readlink "$link")" in
            "$BASE_DIR"/*) rm -f "$link" ;;
        esac
    done
    rmdir "${HOME}/.agents/skills" "${HOME}/.agents" 2>/dev/null || true
fi

cleanup_broken_links "${AGY_CONFIG_ROOT}/hooks"
cleanup_broken_links "${AGY_CONFIG_ROOT}/skills"

echo "Antigravity CLI Configuration complete."
