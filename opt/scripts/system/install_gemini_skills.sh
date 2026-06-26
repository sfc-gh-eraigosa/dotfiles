#!/bin/bash
# install_gemini_skills.sh - Idempotently install Gemini CLI skills and policies

set -e

# Determine the root of the dotfiles repository
BASE_DIR="$(cd "$(dirname "$0")/../../.." && pwd)"

echo "Configuring Gemini CLI..."

# Function to clean up broken symlinks in a directory
cleanup_broken_links() {
    local dir="$1"
    if [ -d "$dir" ]; then
        echo "  Cleaning up broken links in $dir..."
        # Portable way to find and delete broken symlinks
        find "$dir" -type l ! -exec test -e {} \; -delete
    fi
}

# --- Policies ---
# 1. New standard policies in ai/gemini/policies
if [ -d "$BASE_DIR/ai/gemini/policies" ]; then
    echo "  Setting up system policies..."
    POLICIES_DEST="${HOME}/.gemini/policies"
    mkdir -p "$POLICIES_DEST"
    
    for policy in "$BASE_DIR/ai/gemini/policies"/*.toml; do
        [ -e "$policy" ] || continue
        policy_name=$(basename "$policy")
        dest_policy="$POLICIES_DEST/$policy_name"
        ln -sf "$policy" "$dest_policy"
    done
fi

# 2. Repo-wide policies in .gemini/policies (if any)
if [ -d "$BASE_DIR/.gemini/policies" ]; then
    echo "  Setting up repo-specific policies..."
    POLICIES_DEST="${HOME}/.gemini/policies"
    mkdir -p "$POLICIES_DEST"
    
    for policy in "$BASE_DIR/.gemini/policies"/*.toml; do
        [ -e "$policy" ] || continue
        policy_name=$(basename "$policy")
        dest_policy="$POLICIES_DEST/$policy_name"
        ln -sf "$policy" "$dest_policy"
    done
fi

# --- Commands ---
if [ -d "$BASE_DIR/ai/gemini/commands" ]; then
    echo "  Setting up custom commands..."
    COMMANDS_DEST="${HOME}/.gemini/commands"
    mkdir -p "$COMMANDS_DEST"
    
    for cmd in "$BASE_DIR/ai/gemini/commands"/*.toml; do
        [ -e "$cmd" ] || continue
        cmd_name=$(basename "$cmd")
        dest_cmd="$COMMANDS_DEST/$cmd_name"
        echo "    Linking $cmd_name"
        ln -sf "$cmd" "$dest_cmd"
    done
fi

# Cleanup obsolete/broken links from previous structures
cleanup_broken_links "${HOME}/.gemini/policies"
cleanup_broken_links "${HOME}/.gemini/commands"

# --- Shell aliases (~/.config/gemini/aliases.sh, sourced by .zshrc and .bashrc) ---
# Provides the gemini() wrapper with tmux auto-anchor support.
GEMINI_XDG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/gemini"
if [ -f "$BASE_DIR/ai/gemini/aliases.sh" ]; then
    echo "  Linking gemini aliases.sh..."
    mkdir -p "$GEMINI_XDG_DIR"
    if [ -e "$GEMINI_XDG_DIR/aliases.sh" ] && [ ! -L "$GEMINI_XDG_DIR/aliases.sh" ]; then
        echo "    Backing up existing aliases.sh -> aliases.sh.bak"
        mv "$GEMINI_XDG_DIR/aliases.sh" "$GEMINI_XDG_DIR/aliases.sh.bak"
    fi
    ln -sf "$BASE_DIR/ai/gemini/aliases.sh" "$GEMINI_XDG_DIR/aliases.sh"
fi

# --- Hooks (COPIED into ~/.gemini/hooks; settings.json references the
# well-known $HOME path). Copy, not symlink, per the provisioning directive
# (CLAUDE.md -> Shell & Dotfiles Conventions). safety_guard.sh loads
# strip_heredocs.awk from its own dir, so the .awk sibling is copied too.
# Test drivers (*_test.sh) stay in the repo and are not copied. ---
if [ -d "$BASE_DIR/ai/hooks" ]; then
    echo "  Copying hooks into ~/.gemini/hooks..."
    HOOKS_DEST="${HOME}/.gemini/hooks"
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

# --- settings.json (host-owned real file; forced subset merged on every run) ---
# Mirrors the Claude model: the host OWNS ~/.gemini/settings.json (theme, auth,
# vimMode, ...); the immutable hook wiring (settings.forced.json) is deep-merged
# over it on every run. ai/gemini/settings.json is the first-run seed. No
# symlink. See docs/mbo/designs/2026-06-02-ai-config-home-provisioning.md (D4).
GEMINI_HOME="${HOME}/.gemini"
GSETTINGS_DEST="$GEMINI_HOME/settings.json"
GSETTINGS_SEED="$BASE_DIR/ai/gemini/settings.json"
GSETTINGS_FORCED="$BASE_DIR/ai/gemini/settings.forced.json"
APPLY_FORCED="$BASE_DIR/opt/scripts/system/apply-forced-settings.sh"
mkdir -p "$GEMINI_HOME"
# Back up a pre-existing real host settings.json once before we first mutate it.
if [ -f "$GSETTINGS_DEST" ] && [ ! -L "$GSETTINGS_DEST" ] && [ ! -e "$GSETTINGS_DEST.bak" ]; then
    echo "  Backing up existing settings.json -> settings.json.bak"
    cp "$GSETTINGS_DEST" "$GSETTINGS_DEST.bak"
fi
if [ -L "$GSETTINGS_DEST" ]; then
    echo "  Migrating legacy settings.json symlink to a host-owned file"
    # Portable replacement for `readlink -f` (GNU-only; absent on macOS/BSD):
    # plain `readlink` (BSD+GNU) reads the link, then anchor a relative target
    # to the symlink's physical dir via `cd ... && pwd -P`.
    legacy_link="$(readlink "$GSETTINGS_DEST" 2>/dev/null || true)"
    case "$legacy_link" in
        "") legacy_target="" ;;
        /*) legacy_target="$legacy_link" ;;
        *)  legacy_target="$(cd -- "$(dirname -- "$GSETTINGS_DEST")" >/dev/null 2>&1 && pwd -P)/$legacy_link" ;;
    esac
    rm -f "$GSETTINGS_DEST"
    if [ -n "$legacy_target" ] && [ -f "$legacy_target" ]; then
        cp "$legacy_target" "$GSETTINGS_DEST"
    fi
fi
if [ ! -f "$GSETTINGS_DEST" ] && [ -f "$GSETTINGS_SEED" ]; then
    echo "  Seeding ~/.gemini/settings.json from repo seed (first run)"
    cp "$GSETTINGS_SEED" "$GSETTINGS_DEST"
fi
if [ -f "$GSETTINGS_DEST" ] && [ -f "$GSETTINGS_FORCED" ]; then
    if command -v jq > /dev/null 2>&1; then
        echo "  Applying forced Gemini settings subset (hooks)"
        bash "$APPLY_FORCED" "$GSETTINGS_DEST" "$GSETTINGS_FORCED"
    else
        echo "  WARNING: jq not installed — cannot merge forced Gemini settings; hook wiring may be stale" >&2
    fi
fi

echo "Gemini CLI Configuration complete."
