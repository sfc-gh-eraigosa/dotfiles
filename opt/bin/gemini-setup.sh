#!/usr/bin/env bash
# ==============================================================================
# Gemini CLI Setup & Configuration Tool
# ==============================================================================
set -e

# --- Configuration ---
GEMINI_DIR="${HOME}/.gemini"
DOTFILES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
GEMINI_DOT_DIR="${DOTFILES_DIR}/system/gemini"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

show_status() {
    echo -e "${BOLD}--- Gemini CLI Status ---${NC}"
    if command -v gemini >/dev/null 2>&1; then
        echo -e "${GREEN}[OK]${NC} Gemini CLI is installed."
    else
        echo -e "${RED}[MISSING]${NC} Gemini CLI (npm install -g @google/gemini-cli)"
    fi

    if [ -f "${GEMINI_DIR}/oauth_creds.json" ]; then
        echo -e "${GREEN}[OK]${NC} Authentication (oauth_creds.json) found."
    else
        echo -e "${RED}[MISSING]${NC} Authentication (Please run 'gemini auth login')"
    fi

    if [ -L "${GEMINI_DIR}/settings.json" ]; then
        echo -e "${GREEN}[OK]${NC} Settings are symlinked to dotfiles."
    else
        echo -e "${BLUE}[LOCAL]${NC} Settings are local (Not managed by dotfiles)."
    fi
}

init_gemini() {
    echo -e "${BLUE}Initializing Gemini Configuration...${NC}"
    
    # 1. Ensure ~/.gemini exists
    mkdir -p "${GEMINI_DIR}"

    # 2. Backup and symlink settings.json if it exists in dotfiles
    if [ -f "${GEMINI_DOT_DIR}/settings.json" ]; then
        if [ -f "${GEMINI_DIR}/settings.json" ] && [ ! -L "${GEMINI_DIR}/settings.json" ]; then
            echo -e "Backing up existing settings.json..."
            mv "${GEMINI_DIR}/settings.json" "${GEMINI_DIR}/settings.json.bak"
        fi
        ln -sf "${GEMINI_DOT_DIR}/settings.json" "${GEMINI_DIR}/settings.json"
        echo -e "${GREEN}Symlinked settings.json from dotfiles.${NC}"
    else
        echo -e "${RED}Warning:${NC} No settings.json found in dotfiles system/gemini/"
    fi

    # 3. Create skills directory if not present
    mkdir -p "${GEMINI_DIR}/skills"
    
    echo -e "${GREEN}Success: Gemini CLI initialized.${NC}"
}

backup_gemini() {
    echo -e "${BLUE}Backing up Gemini settings to dotfiles...${NC}"
    mkdir -p "${GEMINI_DOT_DIR}"
    
    # We DON'T backup oauth_creds.json or google_accounts.json (SECURITY)
    if [ -f "${GEMINI_DIR}/settings.json" ] && [ ! -L "${GEMINI_DIR}/settings.json" ]; then
        cp "${GEMINI_DIR}/settings.json" "${GEMINI_DOT_DIR}/settings.json"
        echo -e "${GREEN}Backup complete (settings.json only).${NC}"
    else
        echo -e "No local settings.json to backup (already symlinked or missing)."
    fi
}

case "$1" in
    init)
        init_gemini
        ;;
    backup)
        backup_gemini
        ;;
    status)
        show_status
        ;;
    *)
        echo "Usage: $0 {init|backup|status}"
        exit 1
        ;;
esac
