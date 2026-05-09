#!/bin/bash
# install_gemini_skills.sh - Idempotently install Gemini CLI skills and policies

set -e

# Determine the root of the dotfiles repository
BASE_DIR="$(cd "$(dirname "$0")/../.." && pwd)"

echo "Configuring Gemini CLI..."

# --- Policies ---
if [ -d "${BASE_DIR}/.gemini/policies" ]; then
    echo "  Setting up policies..."
    POLICIES_DEST="${HOME}/.gemini/policies"
    mkdir -p "$POLICIES_DEST"
    
    for policy in "${BASE_DIR}/.gemini/policies"/*.toml; do
        [ -e "$policy" ] || continue
        policy_name="$(basename "$policy")"
        dest_policy="${POLICIES_DEST}/${policy_name}"
        
        if [ -L "$dest_policy" ] && [ "$(readlink "$dest_policy")" = "$policy" ]; then
            continue
        fi
        
        ln -sf "$policy" "$dest_policy"
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
if [ -d "${BASE_DIR}/.gemini/skills" ]; then
    for skill_dir in "${BASE_DIR}/.gemini/skills"/*/; do
        skill_name="$(basename "${skill_dir}")"
        if [ -f "${skill_dir}SKILL.md" ]; then
            link_skill "${skill_dir}" "${SKILLS_DEST}/${skill_name}"
        fi
    done
fi

# 2. Specific source skills
link_skill "${BASE_DIR}/src/ssh-host-finder" "${SKILLS_DEST}/ssh-host-finder"
link_skill "${BASE_DIR}/src/tmux-mgr/skill" "${SKILLS_DEST}/tmux"

echo "Gemini CLI Configuration complete."
