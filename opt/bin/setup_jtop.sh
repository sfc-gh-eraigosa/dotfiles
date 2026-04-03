#!/bin/bash
# setup_jtop.sh - Install and patch jtop for JetPack 6.2.1 support

set -e

BASE_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
HARDWARE_LIB="${BASE_DIR}/opt/lib/hardware.sh"

# Source hardware detection
if [ -f "$HARDWARE_LIB" ]; then
    . "$HARDWARE_LIB"
else
    echo "Error: Hardware library not found at $HARDWARE_LIB"
    exit 1
fi

# Check if on Jetson
if ! is_jetson; then
    echo "Not on NVIDIA Jetson hardware. Skipping jtop setup."
    exit 0
fi

echo "Setting up jtop and jetson-stats..."

# Install or update jetson-stats
if ! command -v jtop &> /dev/null; then
    echo "Installing jetson-stats..."
    sudo pip3 install -U jetson-stats
else
    echo "Updating jetson-stats..."
    sudo pip3 install -U jetson-stats
fi

# Path to jetson_variables.py
JTOP_VARS="/usr/local/lib/python3.10/dist-packages/jtop/core/jetson_variables.py"

if [ -f "$JTOP_VARS" ]; then
    # L4T 36.4.7 -> JetPack 6.2.1 mapping
    TARGET_MAPPING='"36.4.7": "6.2.1",'
    
    if ! grep -q "$TARGET_MAPPING" "$JTOP_VARS"; then
        echo "Patching $JTOP_VARS for JetPack 6.2.1 support..."
        # Insert after JP6 comment or first mapping
        sudo sed -i '/# -------- JP6 --------/a \    "36.4.7": "6.2.1",' "$JTOP_VARS"
        echo "Patch applied."
    else
        echo "Mapping for 36.4.7 already exists in $JTOP_VARS."
    fi
else
    echo "Warning: $JTOP_VARS not found. Cannot apply version patch."
fi

# Enable and restart the service
# Note: In version 4.x, the service is often named 'jtop.service'
# In older versions it was 'jetson_stats.service'
for service in "jtop" "jetson_stats"; do
    if systemctl list-unit-files | grep -q "${service}.service"; then
        echo "Ensuring ${service} service is active..."
        sudo systemctl enable "${service}"
        sudo systemctl restart "${service}"
        echo "${service} service restarted."
    fi
done

echo "jtop setup complete."
