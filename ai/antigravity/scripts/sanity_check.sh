#!/bin/bash
# Sanity check for dotfiles installation inside the container
set -e

echo "Starting Sanity Check..."

# 1. Path Discovery
echo "Verifying PATH discovery..."
source ~/.antigravity.profile

# Check subdirectories of opt/scripts are in PATH
for tool in git_add.sh antigravity_install.sh docker_up.sh rclone_sync.sh; do
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
for file in .zshrc .profile .antigravity.profile .gitools.sh .tmux.conf; do
    if [ ! -f "$HOME/$file" ]; then
        echo "FAIL: $file missing from HOME"
        exit 1
    fi
done
echo "PASS: Configurations present"

# 4. Git shortcuts (Zsh check) — git-help is a shell function from
# ~/.gitools.sh (not an alias) since the ~/.gitenv generator retirement, so
# probe with `type`, which accepts aliases and functions alike.
echo "Verifying git shortcuts in Zsh..."
zsh -c "source ~/.zshrc; type git-help" > /dev/null
echo "PASS: Git shortcuts functional"

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
# Resolve the Claude sanity check from THIS script's own location — never a
# hardcoded ~/git/dotfiles path, which breaks on worktrees / alternate clones /
# CI (F7 in docs/mbo/designs/2026-06-02-ai-config-home-provisioning.md).
AGY_SANITY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
REPO_ROOT="$(cd "$AGY_SANITY_DIR/../../.." && pwd)"
CLAUDE_SANITY="$REPO_ROOT/ai/claude/scripts/sanity_check.sh"
if command -v claude > /dev/null 2>&1 && [ -x "$CLAUDE_SANITY" ]; then
    "$CLAUDE_SANITY"
else
    echo "SKIP: claude CLI not installed in this environment"
fi

# 7. Antigravity's OWN configured hook wiring resolves + is exercised (D3).
# Uses the event-agnostic validator, which handles agy's named-hook
# hooks.json layout (and drives the antigravity_adapter.sh dialect bridge).
echo "Verifying Antigravity hook wiring..."
VALIDATE_HOOKS="$REPO_ROOT/ai/claude/scripts/validate_hooks.sh"
if [ -x "$VALIDATE_HOOKS" ] && [ -f "$HOME/.gemini/config/hooks.json" ]; then
    if ! "$VALIDATE_HOOKS" "$HOME/.gemini/config/hooks.json"; then
        echo "FAIL: configured hook wiring in ~/.gemini/config/hooks.json is broken (see above)"
        exit 1
    fi
else
    echo "SKIP: ~/.gemini/config/hooks.json or validator not present"
fi

# 8. Antigravity's configured statusLine command resolves (D3).
echo "Verifying Antigravity statusLine command..."
AGY_SETTINGS="$HOME/.gemini/antigravity-cli/settings.json"
if [ -f "$AGY_SETTINGS" ]; then
    STATUS_CMD="$(jq -r '.statusLine.command // empty' "$AGY_SETTINGS" 2>/dev/null)"
    if [ -n "$STATUS_CMD" ]; then
        # Expand leading ~ and $HOME
        sl_path="${STATUS_CMD##* }"
        sl_path="${sl_path/#\~/$HOME}"
        sl_path="${sl_path//\$HOME/$HOME}"
        if [ ! -r "$sl_path" ]; then
            echo "FAIL: statusLine script not readable: '$STATUS_CMD' (resolved: $sl_path)"
            exit 1
        fi
        echo "PASS: statusLine command resolves ($sl_path)"
    else
        echo "SKIP: statusLine command not configured"
    fi
else
    echo "SKIP: ~/.gemini/antigravity-cli/settings.json not present"
fi

echo "--------------------------------------------------"
echo "SANITY CHECK PASSED SUCCESSFULLY"
echo "--------------------------------------------------"
