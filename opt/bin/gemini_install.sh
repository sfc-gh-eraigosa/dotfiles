#!/bin/bash

# Gemini CLI Installation Script
# This script updates Node.js via nvm and installs the Gemini CLI

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

# 3. Create environment profile
GEMINI_PROFILE="$HOME/.gemini.profile"
echo "Creating environment profile at $GEMINI_PROFILE..."

cat << 'EOF' > "$GEMINI_PROFILE"
# Gemini CLI Environment Setup
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh" # Load nvm
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion" # Load nvm bash_completion

# Ensure npm global bin is in PATH
if command -v npm &> /dev/null; then
    NPM_BIN=$(npm config get prefix)/bin
    if [[ ":$PATH:" != *":$NPM_BIN:"* ]]; then
        export PATH="$PATH:$NPM_BIN"
    fi
fi
EOF

# 4. Ensure the profile is sourced in .zshrc and .profile
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

# 5. Verify installation
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
