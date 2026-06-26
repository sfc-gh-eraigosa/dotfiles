#!/usr/bin/env bash
# ==============================================================================
# Jetson Remote Hybrid Setup Tool (Tailscale + NoMachine)
# ==============================================================================
set -e

# --- Configuration & Styling ---
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# --- 1. Tailscale Installation (Headless Connectivity) ---
install_tailscale() {
    echo -e "${BLUE}${BOLD}[1/2] Installing Tailscale (Secure Connectivity)...${NC}"
    if command -v tailscale >/dev/null 2>&1; then
        echo -e "${GREEN}Tailscale is already installed.${NC}"
    else
        echo -e "${BLUE}Running official Tailscale install script...${NC}"
        curl -fsSL https://tailscale.com/install.sh | sh
    fi

    echo -e "${GREEN}${BOLD}Tailscale installed! Next steps:${NC}"
    echo -e "1. Run: ${BOLD}sudo tailscale up${NC}"
    echo -e "2. Copy the URL provided and visit it in your macOS browser."
    echo -e "3. Once linked, you can SSH into this Jetson from macOS using its Tailscale IP."
}

# --- 2. NoMachine Download (High-Performance GUI) ---
install_nomachine() {
    echo -e "${BLUE}${BOLD}[2/2] Preparing NoMachine (GUI Desktop)...${NC}"
    # grep -P/\K is GNU-only (absent on macOS BSD grep); use -oE + sed (both portable).
    NOMACHINE_VER=$(curl -s https://www.nomachine.com/download/download&id=116 | grep -oE "nomachine_[0-9]+\.[0-9]+" | sed -E 's/^nomachine_//' | head -n1 || echo "8.16")
    NOMACHINE_SUB=$(curl -s https://www.nomachine.com/download/download&id=116 | grep -oE "nomachine_[0-9]+\.[0-9]+\._[0-9]+" | sed -E 's/.*\._//' | head -n1 || echo "1")
    DEB_FILE="nomachine_${NOMACHINE_VER}.${NOMACHINE_SUB}_arm64.deb"
    
    if command -v nxserver >/dev/null 2>&1; then
        echo -e "${GREEN}NoMachine is already installed.${NC}"
    else
        echo -e "${BLUE}Downloading NoMachine for ARM64 (Jetson Native)...${NC}"
        wget -q "https://download.nomachine.com/download/${NOMACHINE_VER}/Arm/${DEB_FILE}" -O "/tmp/${DEB_FILE}"
        echo -e "${BLUE}Installing NoMachine (requires sudo)...${NC}"
        sudo dpkg -i "/tmp/${DEB_FILE}"
        rm "/tmp/${DEB_FILE}"
    fi

    echo -e "${GREEN}${BOLD}NoMachine installed! Remote Desktop Tips:${NC}"
    echo -e "- Use NoMachine on your macOS computer to connect."
    echo -e "- Connect to the Tailscale IP of this Jetson for secure remote GUI access."
    echo -e "- ${RED}${BOLD}IMPORTANT:${NC} Headless Jetson requires an HDMI Dummy Plug for GPU acceleration."
}

# --- Help / Instructions ---
show_usage() {
    echo -e "${BOLD}Jetson Hybrid Remote Tool Usage:${NC}"
    echo -e "  $0 all       - Install both Tailscale and NoMachine"
    echo -e "  $0 tailscale - Install Tailscale only"
    echo -e "  $0 nomachine - Install NoMachine only"
    echo -e "  $0 status    - Check status of both services"
}

case "$1" in
    all)
        install_tailscale
        install_nomachine
        ;;
    tailscale)
        install_tailscale
        ;;
    nomachine)
        install_nomachine
        ;;
    status)
        echo -e "${BOLD}--- System Status ---${NC}"
        if command -v tailscale >/dev/null 2>&1; then
            echo -e "${GREEN}[OK]${NC} Tailscale installed. (IP: $(tailscale ip -4 2>/dev/null || echo 'Not running'))"
        else
            echo -e "${RED}[MISSING]${NC} Tailscale"
        fi

        if command -v nxserver >/dev/null 2>&1; then
            echo -e "${GREEN}[OK]${NC} NoMachine Server installed."
        else
            echo -e "${RED}[MISSING]${NC} NoMachine"
        fi
        ;;
    *)
        show_usage
        ;;
esac
