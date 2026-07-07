#!/usr/bin/env bash
# ==============================================================================
# Google CLI Setup & Configuration Tool (Antigravity & Workspace)
# ==============================================================================
set -e

# --- Configuration ---
# Antigravity CLI reuses ~/.gemini: CLI settings live in
# ~/.gemini/antigravity-cli/, the global customization root in ~/.gemini/config/.
AGY_CONFIG_ROOT="${HOME}/.gemini/config"
GWS_DIR="${HOME}/.gws"
GWS_CONFIG_DIR="${HOME}/.config/gws"
DOTFILES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
GWS_DOT_DIR="${DOTFILES_DIR}/ai/gws"
INSTALL_LOG="/tmp/google-cli-setup.log"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# --- Logic ---

# 1. Environment Sourcing
load_node_env() {
    # Respect the user's already-active node manager first. Only fall back to
    # discovery (fnm -> nodenv -> nvm-latest) if no npm is on PATH. Crucial:
    # do NOT pre-pend nvm-latest unconditionally — that shadowed fnm's current
    # node and caused globals (e.g. gws) to land in an unused node version.
    if ! command -v npm >/dev/null 2>&1; then
        if [ -x "${HOME}/.local/share/fnm/fnm" ]; then
            eval "$("${HOME}/.local/share/fnm/fnm" env)"
        fi
        if ! command -v npm >/dev/null 2>&1 && [ -d "${HOME}/.nodenv" ]; then
            export PATH="${HOME}/.nodenv/shims:${PATH}"
        fi
        if ! command -v npm >/dev/null 2>&1 && [ -d "${HOME}/.nvm/versions/node" ]; then
            local latest_nvm
            latest_nvm=$(ls -1 "${HOME}/.nvm/versions/node" 2>/dev/null | tail -1)
            [ -n "$latest_nvm" ] && export PATH="${HOME}/.nvm/versions/node/${latest_nvm}/bin:${PATH}"
        fi
    fi

    # Always make ~/opt/bin reachable for locally-installed tools.
    export PATH="${HOME}/opt/bin:${PATH}"

    # Last resort on a fresh machine: install.sh has installed nvm but no
    # Node version yet (the retired gemini_install.sh used to run this).
    if ! command -v npm >/dev/null 2>&1 && [ -s "${HOME}/.nvm/nvm.sh" ]; then
        echo -e "${BLUE}npm not found — installing Node LTS via nvm...${NC}"
        # shellcheck source=/dev/null
        \. "${HOME}/.nvm/nvm.sh"
        nvm install --lts >> "$INSTALL_LOG" 2>&1 && nvm use --lts >> "$INSTALL_LOG" 2>&1
    fi

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
        $SUDO npm install -g "$package_name@latest" >> "$INSTALL_LOG" 2>&1
    else
        echo -e "${BLUE}Installing ${cmd_name} (${package_name})...${NC}"
        $SUDO npm install -g "$package_name" >> "$INSTALL_LOG" 2>&1
    fi
}

install_agy() {
    # Antigravity CLI (agy) — native binary via Google's checksummed
    # bootstrapper; no npm involved (successor to the retired Gemini CLI).
    if command -v agy >/dev/null 2>&1; then
        echo -e "${BLUE}Updating Antigravity CLI (agy)...${NC}"
        agy update >> "$INSTALL_LOG" 2>&1 || true
    else
        echo -e "${BLUE}Installing Antigravity CLI (agy)... (log: ${INSTALL_LOG})${NC}"
        curl -fsSL https://antigravity.google/cli/install.sh | bash >> "$INSTALL_LOG" 2>&1
        export PATH="${HOME}/.local/bin:${PATH}"
    fi
}

install_gcloud() {
    # Check if gcloud is in PATH or in the standard install location
    if command -v gcloud >/dev/null 2>&1 || [ -f "${HOME}/opt/google-cloud-sdk/bin/gcloud" ]; then
        echo -e "${GREEN}gcloud CLI is already installed.${NC}"
        # Ensure it's in the current PATH if it was just found in the standard location
        if ! command -v gcloud >/dev/null 2>&1; then
            export PATH="${HOME}/opt/google-cloud-sdk/bin:${PATH}"
        fi
        return
    fi

    echo -e "${BLUE}Installing Google Cloud SDK (gcloud)... (log: ${INSTALL_LOG})${NC}"
    # Redirecting to log for a cleaner install experience as requested
    curl -fsSL https://sdk.cloud.google.com | bash -s -- --disable-prompts --install-dir="${HOME}/opt" >> "$INSTALL_LOG" 2>&1
    
    # Add to path for current session
    export PATH="${HOME}/opt/google-cloud-sdk/bin:${PATH}"
}

init_configs() {
    echo -e "${BLUE}Initializing configurations...${NC}"
    
    # Antigravity: the global customization root (skills/, hooks/, hooks.json)
    # is provisioned by install_antigravity_skills.sh + sync-skills.sh; just
    # make sure the skills root exists so a fresh agy sees it.
    mkdir -p "${AGY_CONFIG_ROOT}/skills"

    # GWS
    mkdir -p "${GWS_DIR}"
    mkdir -p "${GWS_CONFIG_DIR}"
    
    # Seed config.json from template if it doesn't exist
    if [ ! -f "${GWS_DOT_DIR}/config.json" ] && [ -f "${GWS_DOT_DIR}/config.json.template" ]; then
        echo -e "${BLUE}Seeding GWS configuration from template...${NC}"
        cp "${GWS_DOT_DIR}/config.json.template" "${GWS_DOT_DIR}/config.json"
    fi

    if [ -f "${GWS_DOT_DIR}/config.json" ]; then
        ln -sf "${GWS_DOT_DIR}/config.json" "${GWS_DIR}/config.json"
        echo -e "${GREEN}Symlinked GWS configuration.${NC}"
    fi

    # Seed client_secret.json from template if it doesn't exist (optional path)
    if [ ! -f "${GWS_CONFIG_DIR}/client_secret.json" ] && [ -f "${GWS_DOT_DIR}/client_secret.json.template" ]; then
        echo -e "${BLUE}Seeding GWS client_secret.json from template...${NC}"
        cp "${GWS_DOT_DIR}/client_secret.json.template" "${GWS_CONFIG_DIR}/client_secret.json"
    fi
}

get_gcloud_version() {
    if command -v gcloud >/dev/null 2>&1; then
        gcloud --version | head -n 1
    elif [ -f "${HOME}/opt/google-cloud-sdk/bin/gcloud" ]; then
        "${HOME}/opt/google-cloud-sdk/bin/gcloud" --version | head -n 1
    else
        echo "Missing"
    fi
}

show_status() {
    echo -e "\n${BOLD}--- Google CLI Status ---${NC}"
    
    # gcloud
    local gcloud_ver; gcloud_ver=$(get_gcloud_version)
    if [[ "$gcloud_ver" != "Missing" ]]; then
        echo -e "${GREEN}[OK]${NC} gcloud CLI: $gcloud_ver"
    else
        echo -e "${RED}[MISSING]${NC} gcloud CLI"
    fi

    # Antigravity
    if command -v agy >/dev/null 2>&1 || [ -x "${HOME}/.local/bin/agy" ]; then
        echo -e "${GREEN}[OK]${NC} Antigravity CLI (agy)"
    else
        echo -e "${RED}[MISSING]${NC} Antigravity CLI (agy)"
    fi

    # GWS
    if command -v gws >/dev/null 2>&1; then
        echo -e "${GREEN}[OK]${NC} GWS CLI: $(gws --version 2>/dev/null | head -n 1 || echo 'Installed')"
    else
        echo -e "${RED}[MISSING]${NC} GWS CLI (@googleworkspace/cli)"
    fi

    # Auth checks
    [ -d "${HOME}/.gemini/antigravity-cli" ] && echo -e "${GREEN}[OK]${NC} Antigravity CLI initialized." || echo -e "${RED}[!]${NC} Antigravity CLI not initialized (run 'agy' to complete the OAuth flow)"
    
    if command -v gws >/dev/null 2>&1; then
        if [ -f "${GWS_CONFIG_DIR}/client_secret.json" ] && ! grep -q "YOUR_CLIENT_ID" "${GWS_CONFIG_DIR}/client_secret.json"; then
             echo -e "${GREEN}[OK]${NC} GWS Client Secret found."
        elif [ -f "${GWS_CONFIG_DIR}/tokens.json" ]; then
             echo -e "${GREEN}[OK]${NC} GWS Auth Token found."
        else
            echo -e "${RED}[!]${NC} GWS Auth missing."
            echo -e "    Recommended path: run 'gws auth setup'"
        fi
    fi
    
    # Final PATH warning if gcloud is installed but not in PATH
    if [[ "$gcloud_ver" != "Missing" ]] && ! command -v gcloud >/dev/null 2>&1; then
        echo -e "\n${BLUE}${BOLD}NOTE: gcloud is installed but not in your current PATH.${NC}"
        echo -e "${BLUE}Please restart your shell or run: source ~/.bashrc (or ~/.zshrc)${NC}"
    fi
}

# Main Execution Flow
setup_all() {
    echo -e "${BLUE}Starting Google CLI setup... Full log at ${INSTALL_LOG}${NC}"
    load_node_env
    install_gcloud
    # agy itself is installed/updated by antigravity_install.sh (install.sh
    # runs it right before this script); install only when absent so a full
    # bootstrap doesn't hit Google's updater twice. `google-cli-setup.sh agy`
    # forces the install/update path explicitly.
    command -v agy >/dev/null 2>&1 || install_agy
    install_or_upgrade "gws" "@googleworkspace/cli"
    init_configs
    show_status
    echo -e "\n${GREEN}${BOLD}Success: Google CLI setup complete!${NC}"
}

# Only dispatch when executed directly. Sourcing this file (e.g. from a
# test driver) loads helpers like `load_node_env` without firing the
# install flow. Required by opt/scripts/system/google-cli-setup_test.sh.
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
    case "${1:-}" in
        status)
            show_status
            ;;
        gcloud)
            install_gcloud
            ;;
        agy)
            install_agy
            ;;
        *)
            setup_all
            ;;
    esac
fi
