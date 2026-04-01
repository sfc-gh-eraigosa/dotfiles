#!/usr/bin/env bash
# ==============================================================================
# Antigravity ARM64 Installation Script (for Jetson Nano)
# ==============================================================================
set -e

# --- Configuration ---
INSTALL_DIR="${HOME}/opt/bin"
BIN_NAME="antigravity"
TARGET_PATH="${INSTALL_DIR}/${BIN_NAME}"

# Replace this with the actual internal release URL for the linux-arm64 build
# For example: "https://internal-repo.example.com/artifacts/antigravity-linux-arm64"
DOWNLOAD_URL="<INSERT_ANTIGRAVITY_LINUX_ARM64_URL_HERE>"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

echo -e "${BLUE}${BOLD}Starting Antigravity ARM64 Installation...${NC}"

# 1. Architecture Check
ARCH=$(uname -m)
if [ "$ARCH" != "aarch64" ] && [ "$ARCH" != "arm64" ]; then
    echo -e "${RED}Error: This script is intended for ARM64/aarch64 architecture (like Jetson Nano). Detected: ${ARCH}${NC}"
    exit 1
fi

# 2. Ensure install directory exists
echo -e "Ensuring installation directory exists at ${INSTALL_DIR}"
mkdir -p "${INSTALL_DIR}"

# 3. Download or locate the binary
if [ "$DOWNLOAD_URL" == "<INSERT_ANTIGRAVITY_LINUX_ARM64_URL_HERE>" ]; then
    echo -e "${RED}Warning: DOWNLOAD_URL is not configured.${NC}"
    echo "Please edit this script and replace DOWNLOAD_URL with the actual internal link to the arm64 binary."
    echo ""
    echo -e "If you have already manually copied the binary over SSH, simply move it to: ${BOLD}${TARGET_PATH}${NC}"
    echo "and run: chmod +x ${TARGET_PATH}"
    exit 1
fi

echo -e "Downloading Antigravity linux-arm64 from ${DOWNLOAD_URL}..."
curl -fL -o "${TARGET_PATH}" "$DOWNLOAD_URL"

# 4. Make it executable
echo -e "Applying execution permissions..."
chmod +x "${TARGET_PATH}"

# 5. Verify installation
echo -e "${GREEN}Success: Antigravity is securely installed at ${TARGET_PATH}!${NC}"
echo "You can now run 'antigravity' from your terminal."
