#!/usr/bin/env bash
# ==============================================================================
# Jetson Performance & Headless Stability Toggle
# ==============================================================================
set -e

# --- Configuration Paths ---
DOTFILES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
JETSON_CONFIG_DIR="${DOTFILES_DIR}/system/jetson"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

show_status() {
    echo -e "${BOLD}--- Jetson Performance Status ---${NC}"
    echo -n "Power Mode: "
    nvpmodel -q | grep "NV Power Mode" || echo "Unknown"
    
    echo -n "Sleep/Suspend: "
    if systemctl is-enabled sleep.target >/dev/null 2>&1; then
        echo -e "${GREEN}Enabled${NC} (System can sleep)"
    else
        echo -e "${RED}Masked/Disabled${NC} (Always awake)"
    fi

    echo -n "Jetson Clocks Service: "
    if systemctl is-active jetson-clocks.service >/dev/null 2>&1; then
        echo -e "${GREEN}Active${NC} (Max clocks)"
    else
        echo -e "${RED}Inactive${NC} (Dynamic clocks)"
    fi
}

init_jetson() {
    echo -e "${BLUE}Initializing Jetson-Specific System Configs...${NC}"
    
    # 1. Setup jetson-clocks service
    if [ -f "${JETSON_CONFIG_DIR}/etc/systemd/system/jetson-clocks.service" ]; then
        sudo cp "${JETSON_CONFIG_DIR}/etc/systemd/system/jetson-clocks.service" /etc/systemd/system/
        sudo systemctl daemon-reload
        sudo systemctl enable jetson-clocks.service
    fi

    # 2. Setup Docker NVME config
    if [ -f "${JETSON_CONFIG_DIR}/etc/docker/daemon.json" ]; then
        sudo mkdir -p /etc/docker
        sudo cp "${JETSON_CONFIG_DIR}/etc/docker/daemon.json" /etc/docker/
        sudo systemctl restart docker || echo "Docker restart failed (expected if data-root not yet ready)"
    fi

    echo -e "${GREEN}Success: Jetson system files initialized from dotfiles.${NC}"
}

enable_perf() {
    echo -e "${BLUE}Enabling Max Performance & Headless Stability...${NC}"
    sudo nvpmodel -m 0
    sudo jetson_clocks
    sudo systemctl mask sleep.target suspend.target hibernate.target hybrid-sleep.target
    sudo systemctl start jetson-clocks.service
    echo -e "${GREEN}Success: Jetson is now locked to MAX performance and will not sleep.${NC}"
}

disable_perf() {
    echo -e "${BLUE}Restoring Default Power & Sleep Settings...${NC}"
    sudo systemctl unmask sleep.target suspend.target hibernate.target hybrid-sleep.target
    sudo systemctl stop jetson-clocks.service
    echo -e "${GREEN}Success: Sleep targets restored. Performance service stopped.${NC}"
}

case "$1" in
    init)
        init_jetson
        ;;
    on)
        enable_perf
        ;;
    off)
        disable_perf
        ;;
    status)
        show_status
        ;;
    *)
        echo "Usage: $0 {init|on|off|status}"
        exit 1
        ;;
esac
