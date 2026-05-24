#!/bin/bash
# sync-skills.sh - Synchronizes all AI agent skills from the repository to ~/.agents/skills

set -e

# Determine the root of the dotfiles repository
BASE_DIR="$(cd "$(dirname "$0")/../../.." && pwd)"
SKILLS_DEST="${HOME}/.agents/skills"

echo "Synchronizing AI agent skills..."
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

# 1. Standard source-code skills
link_skill "$BASE_DIR/src/ssh-host-finder" "$SKILLS_DEST/ssh-host-finder"
link_skill "$BASE_DIR/src/tmux-mgr/skill" "$SKILLS_DEST/tmux"
link_skill "$BASE_DIR/src/gss/skill" "$SKILLS_DEST/git-safe-sync"
link_skill "$BASE_DIR/src/ssh-key-sync" "$SKILLS_DEST/ssh-key-sync"

# 2. Generic AI skills from ai/skills
if [ -d "$BASE_DIR/ai/skills" ]; then
    for skill_dir in "$BASE_DIR/ai/skills"/*/; do
        [ -d "$skill_dir" ] || continue
        skill_name=$(basename "$skill_dir")
        if [ -f "${skill_dir}SKILL.md" ]; then
            link_skill "$skill_dir" "$SKILLS_DEST/$skill_name"
        fi
    done
fi

# 3. Repo-wide skills from .gemini/skills (if any)
if [ -d "$BASE_DIR/.gemini/skills" ]; then
    for skill_dir in "$BASE_DIR/.gemini/skills"/*/; do
        [ -d "$skill_dir" ] || continue
        skill_name=$(basename "$skill_dir")
        if [ -f "${skill_dir}SKILL.md" ]; then
            link_skill "$skill_dir" "$SKILLS_DEST/$skill_name"
        fi
    done
fi

# Cleanup broken symlinks
if [ -d "$SKILLS_DEST" ]; then
    echo "  Cleaning up broken links in $SKILLS_DEST..."
    find "$SKILLS_DEST" -type l ! -exec test -e {} \; -delete
fi

echo "Skill synchronization complete."
