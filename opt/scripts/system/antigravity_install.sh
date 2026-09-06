#!/usr/bin/env bash

# Antigravity CLI Installation Script
# Installs Google's Antigravity CLI (agy) — the successor to the retired
# Gemini CLI (EOL June 18, 2026) — via Google's checksummed bootstrapper,
# cleans up legacy Gemini CLI artifacts, and provisions the environment
# profile.

set -u

# 1. Install the Antigravity CLI (native binary, no Node.js required).
# The bootstrapper downloads the platform build, verifies its SHA-512
# against the release manifest, installs to ~/.local/bin/agy, and runs
# `agy install` for shell PATH setup (a no-op here: our profiles already
# export ~/.local/bin).
if command -v agy &> /dev/null; then
    echo "Antigravity CLI already installed: $(command -v agy)"
    echo "Checking for updates..."
    agy update || true
else
    echo "Installing Antigravity CLI (agy)..."
    curl -fsSL https://antigravity.google/cli/install.sh | bash
fi

# 2. Leftover Gemini CLI artifacts (the retired npm package, the old
# gemini() aliases, ~/.gemini.profile) are handled by the consent-based
# gemini_teardown.sh, which install.sh runs right after this script.

# 3. Create environment profile
ANTIGRAVITY_PROFILE="$HOME/.antigravity.profile"
echo "Creating environment profile at $ANTIGRAVITY_PROFILE..."

cat << 'PROFOF' > "$ANTIGRAVITY_PROFILE"
# Antigravity CLI Environment Setup

# Ensure the agy binary location is on PATH
case ":$PATH:" in
    *":$HOME/.local/bin:"*) ;;
    *) export PATH="$HOME/.local/bin:$PATH" ;;
esac

# Add opt/scripts subdirectories to PATH
if [ -d "$HOME/opt/scripts" ]; then
    for dir in "$HOME"/opt/scripts/*/; do
        if [ -d "$dir" ]; then
            # Remove trailing slash and add to PATH if not already there
            dir_path="${dir%/}"
            case ":$PATH:" in
                *":$dir_path:"*) ;;
                *) export PATH="$PATH:$dir_path" ;;
            esac
        fi
    done
fi

# Load NVM (still used by other Node tooling, e.g. gws)
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"

# Add tmux to PATH (for Nix systems)
if [ -d "/nix/store/mlyqvaa6lcwjfbp1dvzxkd9g46fksdnj-tmux-3.6a/bin" ]; then
    export PATH="/nix/store/mlyqvaa6lcwjfbp1dvzxkd9g46fksdnj-tmux-3.6a/bin:$PATH"
fi

# Antigravity tmux configuration
export TMUX_DEFAULT_SESSION="antigravity"

# Use underscores for functions to maintain POSIX/dash compatibility
tmux_start() {
    local session_name="$1"
    if [ -z "$session_name" ]; then
        # Find the first unused number from 0 to 9
        for i in 0 1 2 3 4 5 6 7 8 9; do
            if ! tmux has-session -t "$i" 2>/dev/null; then
                session_name="$i"
                break
            fi
        done
    fi
    session_name="${session_name:-$TMUX_DEFAULT_SESSION}"
    tmux new-session -A -s "$session_name"
}

tmux_a() {
    local session_name="${1:-$TMUX_DEFAULT_SESSION}"
    tmux attach-session -t "$session_name"
}

# Aliases are safer for hyphenated names across different shells
alias tmux-start='tmux_start'
alias tmux-a='tmux_a'
alias tmux-ls="tmux ls 2>/dev/null || echo 'No tmux sessions running'"

PROFOF

# 4. Ensure the profile is sourced in .zshrc and .profile (only when they
# are real files; the repo-managed symlinked profiles already source it).
for shell_config in "$HOME/.zshrc" "$HOME/.profile"; do
    if [ -f "$shell_config" ] && [ ! -L "$shell_config" ]; then
        if ! grep -q '\.antigravity\.profile' "$shell_config"; then
            echo "Adding source entry to $shell_config..."
            if [[ "$shell_config" == *".zshrc" ]]; then
                echo -e "\n# Load Antigravity CLI environment\n[[ -f \"\$HOME/.antigravity.profile\" ]] && source \"\$HOME/.antigravity.profile\"" >> "$shell_config"
            else
                echo -e "\n# Load Antigravity CLI environment\n[ -f \"\$HOME/.antigravity.profile\" ] && . \"\$HOME/.antigravity.profile\"" >> "$shell_config"
            fi
        else
            echo "Source entry already exists in $shell_config"
        fi
    fi
done

# 5. The bootstrapper's `agy install` step appends a hardcoded-$HOME PATH
# export to every rc file it can find — through our repo-managed symlinks,
# dirtying the repo with an unportable path. Strip it from symlinked rc
# files (the repo profiles already export ~/.local/bin portably). Rerunnable
# any time an agy self-update re-appends it.
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)"
if [ -x "$SCRIPT_DIR/strip-agy-rc-appends.sh" ]; then
    "$SCRIPT_DIR/strip-agy-rc-appends.sh"
fi

# 6. Verify installation
if command -v agy &> /dev/null; then
    echo "Antigravity CLI installed successfully!"
    agy changelog 2>/dev/null | head -n 1 || true
    echo "Run 'agy' to authenticate and get started."
else
    # Try the well-known install location if not yet on PATH
    if [ -x "$HOME/.local/bin/agy" ]; then
        echo "Antigravity CLI installed at ~/.local/bin/agy (restart your shell to pick up PATH)."
    else
        echo "Installation failed. Please check the installer output above."
        exit 1
    fi
fi
