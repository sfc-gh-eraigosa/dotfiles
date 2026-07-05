#!/bin/bash
# sync-skills.sh - Synchronizes all AI agent skills and builds associated binaries.

set -e

# Determine the root of the dotfiles repository
# Portable replacement for `readlink -f "$0"` (GNU-only; absent on macOS/BSD):
# `cd ... && pwd -P` resolves symlinks (like ~/opt) to the physical script dir.
SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" >/dev/null 2>&1 && pwd -P)"
BASE_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd -P)"

# Destinations that receive the synced skills. Gemini CLI reads ~/.agents/skills;
# Claude Code reads ~/.claude/skills. The SKILL.md format is shared between both
# assistants, so the single discovery pass below links every skill into each one.
# This is the canonical skill linker for BOTH tools — install_gemini_skills.sh
# and install_claude_skills.sh only handle their assistant-specific config now.
SKILLS_DESTS=("${HOME}/.agents/skills" "${HOME}/.claude/skills")

show_help() {
    echo "Usage: sync-skills [FLAGS]"
    echo ""
    echo "Synchronizes agent skills from the dotfiles repository into both"
    # shellcheck disable=SC2088 # literal tilde paths shown in help text, not meant to expand
    echo "~/.agents/skills (Gemini CLI) and ~/.claude/skills (Claude Code)."
    echo ""
    echo "Flags:"
    echo "  --build     Build associated binaries (gss, tmux-mgr, wol, gsl) while syncing."
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
for dest_root in "${SKILLS_DESTS[@]}"; do
    mkdir -p "$dest_root"
done

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

# Link a skill (by basename) into every destination in SKILLS_DESTS.
link_skill_all() {
    local src="$1"
    local name="$2"
    local dest_root
    for dest_root in "${SKILLS_DESTS[@]}"; do
        link_skill "$src" "$dest_root/$name"
    done
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
for component in "gss" "tmux-mgr" "wol" "gsl"; do
    # Go modules are migrating src/ -> sdk/; prefer sdk/, fall back to src/.
    for root in "sdk" "src"; do
        if [ -d "$BASE_DIR/$root/$component" ]; then
            build_component "$BASE_DIR/$root/$component" "$component"
            break
        fi
    done
done

# 2. Dynamic Skill Discovery
echo "Discovering and linking skills..."

# A. Skills in src/ and sdk/ (either a 'skill/' subdirectory or a top-level 'SKILL.md')
# We look for {src,sdk}/*/skill/ or {src,sdk}/*/SKILL.md (the sdk/ glob is inert if absent).
for dir in "$BASE_DIR/src"/*/ "$BASE_DIR/sdk"/*/; do
    [ -d "$dir" ] || continue
    name=$(basename "$dir")
    
    # Priority 1: 'skill/' subdirectory (like sdk/gss/skill)
    if [ -d "${dir}skill" ]; then
        # Map specific names if needed, otherwise use the directory name
        dest_name="$name"
        case "$name" in
            gss) dest_name="git-safe-sync" ;;
            tmux-mgr) dest_name="tmux" ;;
            gsl) dest_name="gsl-status" ;;
        esac
        link_skill_all "${dir}skill" "$dest_name"
    # Priority 2: 'SKILL.md' file in the directory (like src/ssh-host-finder/SKILL.md)
    elif [ -f "${dir}SKILL.md" ]; then
        link_skill_all "$dir" "$name"
    fi
done

# B. Generic AI skills from ai/skills/
if [ -d "$BASE_DIR/ai/skills" ]; then
    for skill_dir in "$BASE_DIR/ai/skills"/*/; do
        [ -d "$skill_dir" ] || continue
        skill_name=$(basename "$skill_dir")
        if [ -f "${skill_dir}SKILL.md" ]; then
            link_skill_all "$skill_dir" "$skill_name"
        fi
    done
fi

# 3. Repo-wide skills from .gemini/skills (if any)
if [ -d "$BASE_DIR/.gemini/skills" ]; then
    for skill_dir in "$BASE_DIR/.gemini/skills"/*/; do
        [ -d "$skill_dir" ] || continue
        skill_name=$(basename "$skill_dir")
        if [ -f "${skill_dir}SKILL.md" ]; then
            link_skill_all "$skill_dir" "$skill_name"
        fi
    done
fi

# Cleanup broken symlinks in every destination
for dest_root in "${SKILLS_DESTS[@]}"; do
    if [ -d "$dest_root" ]; then
        echo "  Cleaning up broken links in $dest_root..."
        find "$dest_root" -type l ! -exec test -e {} \; -delete
    fi
done

echo "Skill synchronization complete."
