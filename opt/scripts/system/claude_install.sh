#!/bin/bash
#
# Claude Code CLI Installation Script
#
# Cross-platform installer for Claude Code (https://claude.com/claude-code):
#   - macOS:       installed via Homebrew cask 'claude-code' (see opt/profiles/Brewfile)
#                  This script verifies presence and updates if needed.
#   - Linux:       Debian/Ubuntu, Raspberry Pi (arm64/armv7l), and NVIDIA Jetson — installed via npm.
#   - Windows WSL: Treated as Linux — installed via npm.
#
# Re-running this script is safe and idempotent: it updates an existing install
# rather than re-installing from scratch.

set -e

NPM_PKG="@anthropic-ai/claude-code"

# Detect platform
OS_KIND="unknown"
case "$(uname -s)" in
    Darwin*) OS_KIND="macos" ;;
    Linux*)
        if grep -qi microsoft /proc/version 2>/dev/null; then
            OS_KIND="wsl"
        else
            OS_KIND="linux"
        fi
        ;;
esac

echo "Detected platform: $OS_KIND"

install_via_brew() {
    if ! command -v brew &> /dev/null; then
        echo "WARNING: Homebrew not found on macOS. Falling back to npm install."
        install_via_npm
        return
    fi

    if brew list --cask claude-code &> /dev/null; then
        echo "claude-code cask already installed. Checking for updates..."
        brew upgrade --cask claude-code 2>/dev/null || true
    else
        echo "Installing claude-code cask..."
        brew install --cask claude-code || {
            echo "WARNING: brew install failed. Falling back to npm install."
            install_via_npm
        }
    fi
}

install_via_npm() {
    # Ensure node is available via nvm
    export NVM_DIR="$HOME/.nvm"
    # shellcheck disable=SC1091
    [ -s "$NVM_DIR/nvm.sh" ] && . "$NVM_DIR/nvm.sh"

    if ! command -v npm &> /dev/null; then
        if command -v nvm &> /dev/null; then
            echo "npm not found; installing latest LTS node via nvm..."
            nvm install --lts
            nvm alias default 'lts/*'
            nvm use default
        else
            echo "ERROR: npm/node not available and nvm not initialized. Install nvm first."
            exit 1
        fi
    fi

    if npm ls -g --depth=0 "$NPM_PKG" &> /dev/null; then
        echo "$NPM_PKG already installed globally. Updating..."
        npm update -g "$NPM_PKG"
    else
        echo "Installing $NPM_PKG globally via npm..."
        npm install -g "$NPM_PKG"
    fi
}

case "$OS_KIND" in
    macos) install_via_brew ;;
    linux|wsl) install_via_npm ;;
    *)
        echo "ERROR: Unsupported platform: $(uname -s)"
        exit 1
        ;;
esac

# Verify
if command -v claude &> /dev/null; then
    echo "Claude Code installed successfully!"
    claude --version || true
    echo "Run 'claude' to authenticate and get started."
else
    echo "WARNING: 'claude' is not on PATH after install. You may need to open a new shell."
    echo "         npm global bin: $(npm config get prefix 2>/dev/null)/bin"
fi
