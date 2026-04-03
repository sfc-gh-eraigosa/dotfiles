#!/usr/bin/env bash
# ==============================================================================
# Vault Binary Setup Tool (Pre-compiled for macOS)
# ==============================================================================
set -e

# --- Configuration ---
VAULT_VERSION="1.21.4"
INSTALL_DIR="${HOME}/opt/bin"
TMP_DIR="/tmp/vault_install"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# --- Architecture Detection ---
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  VAULT_ARCH="amd64" ;;
    arm64)   VAULT_ARCH="arm64" ;;
    *)       echo -e "${RED}Error: Unsupported architecture $ARCH${NC}"; exit 1 ;;
esac

# --- Logic ---
echo -e "${BLUE}Setting up Vault ${VAULT_VERSION} for macOS (${VAULT_ARCH})...${NC}"

# Check current version
if command -v vault >/dev/null 2>&1; then
    CURRENT_V=$(vault --version | awk '{print $2}' | sed 's/v//')
    if [ "$CURRENT_V" == "$VAULT_VERSION" ]; then
        echo -e "${GREEN}Vault ${VAULT_VERSION} is already installed.${NC}"
        exit 0
    fi
    echo -e "Upgrading Vault $CURRENT_V -> $VAULT_VERSION..."
fi

# Ensure directory exists
mkdir -p "${INSTALL_DIR}"
mkdir -p "${TMP_DIR}"

# Download
URL="https://releases.hashicorp.com/vault/${VAULT_VERSION}/vault_${VAULT_VERSION}_darwin_${VAULT_ARCH}.zip"
echo -e "Downloading from ${URL}..."
curl -fsSL "$URL" -o "${TMP_DIR}/vault.zip"

# Unzip and Install
echo -e "Installing to ${INSTALL_DIR}/vault..."
unzip -o "${TMP_DIR}/vault.zip" -d "${TMP_DIR}"
mv "${TMP_DIR}/vault" "${INSTALL_DIR}/vault"
chmod +x "${INSTALL_DIR}/vault"

# Cleanup
rm -rf "${TMP_DIR}"

# Verification
echo -e "\n${GREEN}${BOLD}Success! Vault installed.${NC}"
vault --version
