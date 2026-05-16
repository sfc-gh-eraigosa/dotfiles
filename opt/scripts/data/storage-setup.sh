#!/usr/bin/env bash
# ==============================================================================
# Jetson Storage Discovery & Setup Tool
# ==============================================================================
set -e

MOUNT_POINT="/mnt/data"
SWAP_FILE="${MOUNT_POINT}/swapfile"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

discover_nvme() {
    # Find the first NVMe block device
    local dev
    dev=$(lsblk -dpnno NAME,TRAN | grep nvme | awk '{print $1}' | head -n1)
    
    if [ -z "$dev" ]; then
        echo -e "${RED}Error: No NVMe drive detected via lsblk.${NC}" >&2
        return 1
    fi
    echo "$dev"
}

get_uuid() {
    local dev=$1
    local part="${dev}p1"
    local uuid
    uuid=$(sudo blkid -s UUID -o value "$part" || true)
    echo "$uuid"
}

show_status() {
    echo -e "${BOLD}--- Storage Discovery Status ---${NC}"
    local dev
    dev=$(discover_nvme || true)
    
    if [ -n "$dev" ]; then
        echo -e "Detected NVMe: ${GREEN}${dev}${NC}"
        local uuid
        uuid=$(get_uuid "$dev")
        if [ -n "$uuid" ]; then
            echo -e "Partition UUID: ${BLUE}${uuid}${NC}"
        else
            echo -e "Partition: ${RED}Not found or not formatted.${NC}"
        fi
    fi

    if mountpoint -q "$MOUNT_POINT"; then
        echo -e "Mount Point: ${GREEN}${MOUNT_POINT} (Mounted)${NC}"
    else
        echo -e "Mount Point: ${RED}${MOUNT_POINT} (Not Mounted)${NC}"
    fi

    if [ -f "$SWAP_FILE" ]; then
        echo -e "Swap File: ${GREEN}${SWAP_FILE}${NC}"
        swapon --show | grep -q "$SWAP_FILE" && echo -e "Swap Status: ${GREEN}Active${NC}" || echo -e "Swap Status: ${RED}Inactive${NC}"
    fi
}

init_storage() {
    local dev
    dev=$(discover_nvme)
    local part="${dev}p1"
    
    echo -e "${BLUE}Configuring storage for device: ${dev}...${NC}"

    # 1. Check if formatted
    local uuid
    uuid=$(get_uuid "$dev")
    if [ -z "$uuid" ]; then
        echo -e "${RED}Device ${part} is not formatted.${NC}"
        echo -e "Please format manually first: sudo mkfs.ext4 -L DATA ${part}"
        exit 1
    fi

    # 2. Ensure mount point exists
    sudo mkdir -p "$MOUNT_POINT"

    # 3. Update fstab dynamically
    if ! grep -q "$uuid" /etc/fstab; then
        echo -e "Adding UUID=${uuid} to /etc/fstab..."
        echo "UUID=${uuid} ${MOUNT_POINT} ext4 defaults,noatime,commit=60 0 2" | sudo tee -a /etc/fstab
    else
        echo -e "${GREEN}fstab already contains this UUID.${NC}"
    fi

    # 4. Mount if not mounted
    if ! mountpoint -q "$MOUNT_POINT"; then
        sudo mount -a
    fi

    # 5. Handle Swap
    if [ ! -f "$SWAP_FILE" ]; then
        echo -e "${BLUE}Creating 8GB Swap File on NVMe...${NC}"
        sudo fallocate -l 8G "$SWAP_FILE"
        sudo chmod 600 "$SWAP_FILE"
        sudo mkswap "$SWAP_FILE"
    fi

    if ! grep -q "$SWAP_FILE" /etc/fstab; then
        echo "${SWAP_FILE} none swap sw 0 0" | sudo tee -a /etc/fstab
    fi

    if ! swapon --show | grep -q "$SWAP_FILE"; then
        sudo swapon "$SWAP_FILE"
    fi

    echo -e "${GREEN}Success: Storage initialized and mounted at ${MOUNT_POINT}${NC}"
}

case "$1" in
    init)
        init_storage
        ;;
    status)
        show_status
        ;;
    *)
        echo "Usage: $0 {init|status}"
        exit 1
        ;;
esac
