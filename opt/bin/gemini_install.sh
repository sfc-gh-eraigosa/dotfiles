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

# 3. Verify installation
if command -v gemini &> /dev/null; then
    echo "Gemini CLI installed successfully!"
    gemini --version
    echo "Run 'gemini' to authenticate and get started."
else
    echo "Installation failed. Please check your npm permissions."
    exit 1
fi
