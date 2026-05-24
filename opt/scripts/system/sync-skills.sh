#!/bin/bash
# sync-skills.sh - Synchronizes all AI agent skills and builds associated binaries.

set -e

# Determine the root of the dotfiles repository
# Use readlink -f to handle symlinks (like ~/opt) and find the physical repo root
SCRIPT_PATH="$(readlink -f "$0")"
BASE_DIR="$(cd "$(dirname "$SCRIPT_PATH")/../../.." && pwd)"
SKILLS_DEST="${HOME}/.agents/skills"

show_help() {
    echo "Usage: sync-skills [FLAGS]"
    echo ""
    echo "Synchronizes agent skills from the dotfiles repository to ~/.agents/skills."
    echo ""
    echo "Flags:"
    echo "  --build     Build associated binaries (gss, tmux-mgr, wol) while syncing."
    echo "  --help      Show this help message."
    echo ""
}

BUILD=false
for arg in "$@"; do
  case $arg in
    --build)
      BUILD=true
      shift
      ;;
    --help)
      show_help
      exit 0
      ;;
  esac
done

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

# Function to build a component if build.sh exists
build_component() {
    local dir="$1"
    local name="$2"
    if [[ "$BUILD" == "true" ]] && [[ -f "$dir/build.sh" ]]; then
        echo "Building $name..."
        (cd "$dir" && bash build.sh)
    fi
}

# 1. Component Builds (for tools with source code)
# Explicitly build known binaries if --build is passed
for component in "gss" "tmux-mgr" "wol"; do
    build_component "$BASE_DIR/src/$component" "$component"
done

# 2. Dynamic Skill Discovery
echo "Discovering and linking skills..."

# A. Skills in src/ (either as a 'skill/' subdirectory or a top-level 'SKILL.md')
# We look for src/*/skill/ or src/*/SKILL.md
for dir in "$BASE_DIR/src"/*/; do
    [ -d "$dir" ] || continue
    name=$(basename "$dir")
    
    # Priority 1: 'skill/' subdirectory (like src/gss/skill)
    if [ -d "${dir}skill" ]; then
        # Map specific names if needed, otherwise use the directory name
        dest_name="$name"
        case "$name" in
            gss) dest_name="git-safe-sync" ;;
            tmux-mgr) dest_name="tmux" ;;
        esac
        link_skill "${dir}skill" "$SKILLS_DEST/$dest_name"
    # Priority 2: 'SKILL.md' file in the directory (like src/ssh-host-finder/SKILL.md)
    elif [ -f "${dir}SKILL.md" ]; then
        link_skill "$dir" "$SKILLS_DEST/$name"
    fi
done

# B. Generic AI skills from ai/skills/
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
