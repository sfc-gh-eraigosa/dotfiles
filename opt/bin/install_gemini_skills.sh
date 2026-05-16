#!/bin/bash
# install_gemini_skills.sh - Idempotently install Gemini CLI skills and policies

set -e

# Determine the root of the dotfiles repository
BASE_DIR="$(cd "$(dirname "$0")/../.." && pwd)"

echo "Configuring Gemini CLI..."

# --- Policies ---
# 1. New standard policies in opt/conf/gemini/policies
if [ -d "$BASE_DIR/opt/conf/gemini/policies" ]; then
    echo "  Setting up system policies..."
    POLICIES_DEST="${HOME}/.gemini/policies"
    mkdir -p "$POLICIES_DEST"
    
    for policy in "$BASE_DIR/opt/conf/gemini/policies"/*.toml; do
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
if [ -d "$BASE_DIR/opt/conf/gemini/commands" ]; then
    echo "  Setting up custom commands..."
    COMMANDS_DEST="${HOME}/.gemini/commands"
    mkdir -p "$COMMANDS_DEST"
    
    for cmd in "$BASE_DIR/opt/conf/gemini/commands"/*.toml; do
        [ -e "$cmd" ] || continue
        cmd_name=$(basename "$cmd")
        dest_cmd="$COMMANDS_DEST/$cmd_name"
        echo "    Linking $cmd_name"
        ln -sf "$cmd" "$dest_cmd"
    done
fi

# --- Skills ---
echo "  Linking skills..."
SKILLS_DEST="${HOME}/.agents/skills"
mkdir -p "$SKILLS_DEST"

# Function to safely create a symlink for a skill
link_skill() {
    local src="$1"
    local dest="$2"
    
    if [ ! -d "$src" ]; then
        return
    fi
    
    # Remove trailing slash if any
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

# 1. Repo-wide skills from .gemini/skills
if [ -d "$BASE_DIR/.gemini/skills" ]; then
    for skill_dir in "$BASE_DIR/.gemini/skills"/*/; do
        skill_name=$(basename "$skill_dir")
        if [ -f "${skill_dir}SKILL.md" ]; then
            link_skill "$skill_dir" "$SKILLS_DEST/$skill_name"
        fi
    done
fi

# 2. Specific source skills
link_skill "$BASE_DIR/src/ssh-host-finder" "$SKILLS_DEST/ssh-host-finder"
link_skill "$BASE_DIR/src/tmux-mgr/skill" "$SKILLS_DEST/tmux"
link_skill "$BASE_DIR/src/gss/skill" "$SKILLS_DEST/git-safe-sync"
link_skill "$BASE_DIR/src/ssh-key-sync" "$SKILLS_DEST/ssh-key-sync"

echo "Gemini CLI Configuration complete."
