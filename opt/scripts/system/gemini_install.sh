#!/bin/bash

# Gemini CLI Installation Script
# This script updates Node.js via nvm, installs the Gemini CLI,
# and configures safety policies.

# 1. Update Node.js to the latest LTS version using nvm
echo "Updating Node.js to the latest LTS version..."
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh" # Load nvm

if command -v nvm &> /dev/null; then
    nvm install --lts
    nvm alias default 'lts/*'
    nvm use default
else
    echo "Error: nvm is not installed. Please install nvm first."
    exit 1
fi

# 2. Install the Gemini CLI globally
echo "Installing Gemini CLI..."
npm install -g @google/gemini-cli

# 3. Configure Safety Policies
echo "Configuring Gemini safety policies..."
POLICIES_DIR="$HOME/.gemini/policies"
mkdir -p "$POLICIES_DIR"

# Identify the base directory of the dotfiles repo
# We use a literal path or relative calculation that doesn't trigger injection
# Since we are in opt/scripts/system/, the root is ../../..
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)
BASE_DIR=$(cd -- "$SCRIPT_DIR/../../.." &> /dev/null && pwd)
SAFETY_POLICY_SRC="$BASE_DIR/opt/conf/gemini/policies/safety.toml"

if [ -f "$SAFETY_POLICY_SRC" ]; then
    echo "Linking safety policy from $SAFETY_POLICY_SRC..."
    ln -sf "$SAFETY_POLICY_SRC" "$POLICIES_DIR/safety.toml"
else
    echo "Warning: Safety policy template not found at $SAFETY_POLICY_SRC"
fi

# 4. Create environment profile
GEMINI_PROFILE="$HOME/.gemini.profile"
echo "Creating environment profile at $GEMINI_PROFILE..."

cat << 'PROFOF' > "$GEMINI_PROFILE"
# Gemini CLI Environment Setup

# Load NVM
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"

# Add tmux to PATH (for Nix systems)
if [ -d "/nix/store/mlyqvaa6lcwjfbp1dvzxkd9g46fksdnj-tmux-3.6a/bin" ]; then
    export PATH="/nix/store/mlyqvaa6lcwjfbp1dvzxkd9g46fksdnj-tmux-3.6a/bin:$PATH"
fi

# Gemini tmux configuration
export TMUX_DEFAULT_SESSION="gemini"

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

# 5. Ensure the profile is sourced in .zshrc and .profile
for shell_config in "$HOME/.zshrc" "$HOME/.profile"; do
    if [ -f "$shell_config" ]; then
        if ! grep -q "source.*\.gemini\.profile" "$shell_config" && ! grep -q "\. .*\.gemini\.profile" "$shell_config"; then
            echo "Adding source entry to $shell_config..."
            # Use portable '.' for .profile and check for zsh/bash for [[
            if [[ "$shell_config" == *".zshrc" ]]; then
                echo -e "\n# Load Gemini CLI environment\n[[ -f \"\$HOME/.gemini.profile\" ]] && source \"\$HOME/.gemini.profile\"" >> "$shell_config"
            else
                echo -e "\n# Load Gemini CLI environment\n[ -f \"\$HOME/.gemini.profile\" ] && . \"\$HOME/.gemini.profile\"" >> "$shell_config"
            fi
        else
            echo "Source entry already exists in $shell_config"
        fi
    fi
done

# 6. Verify installation
if command -v gemini &> /dev/null; then
    echo "Gemini CLI installed successfully!"
    gemini --version
    echo "Run 'gemini' to authenticate and get started."
else
    # Try sourcing the new profile if not yet in path
    source "$GEMINI_PROFILE"
    if command -v gemini &> /dev/null; then
        echo "Gemini CLI installed successfully (via manual source)!"
        gemini --version
    else
        echo "Installation failed. Please check your npm permissions or manually source $GEMINI_PROFILE"
        exit 1
    fi
fi
