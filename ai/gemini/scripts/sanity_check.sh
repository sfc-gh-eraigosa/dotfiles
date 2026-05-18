#!/bin/bash
# Sanity check for dotfiles installation inside the container
set -e

echo "Starting Sanity Check..."

# 1. Path Discovery
echo "Verifying PATH discovery..."
source ~/.gemini.profile

# Check subdirectories of opt/scripts are in PATH
for tool in git_add.sh gemini_install.sh docker_up.sh rclone_sync.sh; do
    if ! command -v "$tool" > /dev/null; then
        echo "FAIL: $tool not found in PATH"
        exit 1
    fi
done
echo "PASS: Script discovery working"

# 2. Binaries
echo "Verifying binaries..."
for tool in gss tmux-mgr wol; do
    if ! "$HOME/opt/bin/$tool" version > /dev/null; then
        echo "FAIL: $tool version command failed"
        exit 1
    fi
done
echo "PASS: Binaries functional"

# 3. Environment Files
echo "Verifying configuration files..."
for file in .zshrc .profile .gemini.profile .gitenv .tmux.conf; do
    if [ ! -f "$HOME/$file" ]; then
        echo "FAIL: $file missing from HOME"
        exit 1
    fi
done
echo "PASS: Configurations present"

# 4. Aliases (Zsh check)
echo "Verifying aliases in Zsh..."
zsh -c "source ~/.zshrc && alias git-help" > /dev/null
echo "PASS: Aliases functional"

# 5. Homebrew/Tools (Mock check)
echo "Verifying tools installed by install.sh..."
for cmd in jq htop zsh; do
    if ! command -v "$cmd" > /dev/null; then
        echo "FAIL: $cmd missing"
        exit 1
    fi
done
echo "PASS: System tools present"

# 6. Claude Code integration (skips if claude not installed in the test image)
echo "Verifying Claude Code integration links..."
CLAUDE_SANITY="$HOME/git/dotfiles/ai/claude/scripts/sanity_check.sh"
if command -v claude > /dev/null 2>&1 && [ -x "$CLAUDE_SANITY" ]; then
    "$CLAUDE_SANITY"
else
    echo "SKIP: claude CLI not installed in this environment"
fi

echo "--------------------------------------------------"
echo "SANITY CHECK PASSED SUCCESSFULLY"
echo "--------------------------------------------------"
