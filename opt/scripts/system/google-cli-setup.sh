#!/usr/bin/env bash
# ==============================================================================
# Google CLI Setup & Configuration Tool (Gemini & Workspace)
# ==============================================================================
set -e

# --- Configuration ---
GEMINI_DIR="${HOME}/.gemini"
GWS_DIR="${HOME}/.gws"
DOTFILES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
GEMINI_DOT_DIR="${DOTFILES_DIR}/ai/gemini"
GWS_DOT_DIR="${DOTFILES_DIR}/ai/gws"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# --- Logic ---

# 1. Environment Sourcing
load_node_env() {
    # Prefer user-managed Node environments (fnm, nodenv, nvm)
    export PATH="${HOME}/.local/share/fnm:${HOME}/.nodenv/shims:${HOME}/.nvm/versions/node/$(ls -1 ${HOME}/.nvm/versions/node 2>/dev/null | tail -1)/bin:${HOME}/opt/bin:${PATH}"
    
    # Try to load fnm environment specifically if binary exists
    if [ -x "${HOME}/.local/share/fnm/fnm" ]; then
        eval "$("${HOME}/.local/share/fnm/fnm" env)"
    fi

    # Final check
    if ! command -v npm >/dev/null 2>&1; then
        echo -e "${RED}Error: npm not found. Please ensure Node.js is installed.${NC}"
        exit 1
    fi
    
    # Decide if sudo is needed
    NPM_PREFIX=$(npm config get prefix)
    if [[ "$NPM_PREFIX" == "/usr" ]] || [[ "$NPM_PREFIX" == "/usr/local" ]]; then
        SUDO="sudo"
        echo -e "${BLUE}Global npm prefix is system-wide (${NPM_PREFIX}). Will use sudo for installation.${NC}"
    else
        SUDO=""
    fi
}

install_or_upgrade() {
    local cmd_name="$1"
    local package_name="$2"
    
    if command -v "$cmd_name" >/dev/null 2>&1; then
        echo -e "${BLUE}Updating ${cmd_name} (${package_name})...${NC}"
        $SUDO npm install -g "$package_name@latest"
    else
        echo -e "${BLUE}Installing ${cmd_name} (${package_name})...${NC}"
        $SUDO npm install -g "$package_name"
    fi
}

init_configs() {
    echo -e "${BLUE}Initializing configurations...${NC}"
    
    # Gemini
    mkdir -p "${GEMINI_DIR}/skills"
    
    if [ -f "${GEMINI_DOT_DIR}/settings.json" ]; then
        ln -sf "${GEMINI_DOT_DIR}/settings.json" "${GEMINI_DIR}/settings.json"
        echo -e "${GREEN}Symlinked Gemini settings.${NC}"
    fi

    if [ -d "${GEMINI_DOT_DIR}/policies" ]; then
        mkdir -p "${GEMINI_DIR}/policies"
        # Link files inside policies rather than the directory itself for flexibility
        for f in "${GEMINI_DOT_DIR}/policies"/*; do
            [ -e "$f" ] || continue
            ln -sf "$f" "${GEMINI_DIR}/policies/$(basename "$f")"
        done
        echo -e "${GREEN}Symlinked Gemini policies.${NC}"
    fi

    # GWS
    mkdir -p "${GWS_DIR}"
    if [ -f "${GWS_DOT_DIR}/config.json" ]; then
        ln -sf "${GWS_DOT_DIR}/config.json" "${GWS_DIR}/config.json"
        echo -e "${GREEN}Symlinked GWS configuration.${NC}"
    fi
}

show_status() {
    echo -e "\n${BOLD}--- Google CLI Status ---${NC}"
    
    # Gemini
    if command -v gemini >/dev/null 2>&1; then
        echo -e "${GREEN}[OK]${NC} Gemini CLI: $(gemini --version 2>/dev/null | head -n 1 || echo 'Installed')"
    else
        echo -e "${RED}[MISSING]${NC} Gemini CLI (@google/gemini-cli)"
    fi

    # GWS
    if command -v gws >/dev/null 2>&1; then
        echo -e "${GREEN}[OK]${NC} GWS CLI: $(gws --version 2>/dev/null | head -n 1 || echo 'Installed')"
    else
        echo -e "${RED}[MISSING]${NC} GWS CLI (@googleworkspace/cli)"
    fi

    # Auth checks
    [ -f "${GEMINI_DIR}/oauth_creds.json" ] && echo -e "${GREEN}[OK]${NC} Gemini Auth found." || echo -e "${RED}[!]${NC} Gemini Auth missing (run 'gemini auth login')"
}

# Main Execution Flow
setup_all() {
    load_node_env
    install_or_upgrade "gemini" "@google/gemini-cli"
    install_or_upgrade "gws" "@googleworkspace/cli"
    init_configs
    show_status
    echo -e "\n${GREEN}${BOLD}Success: Google CLI setup complete!${NC}"
}

case "$1" in
    status)
        show_status
        ;;
    *)
        setup_all
        ;;
esac
