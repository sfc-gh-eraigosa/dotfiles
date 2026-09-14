#!/bin/bash
# setup_jtop.sh - Install and patch jtop for JetPack 6.2.1 support

set -e

BASE_DIR="$(cd "$(dirname "$0")/../../.." && pwd)"
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

# Install or update jetson-stats. It must go in as root (jtop runs as a root
# service), so pip's root-user and new-version notices are expected, not news;
# silence them through pip's env vars, which a pip too old to know them ignores
# (the equivalent flags would be an error there).
if ! command -v jtop &> /dev/null; then
    echo "Installing jetson-stats..."
else
    echo "Updating jetson-stats..."
fi
sudo PIP_ROOT_USER_ACTION=ignore PIP_DISABLE_PIP_VERSION_CHECK=1 pip3 install -U jetson-stats

# Path to jetson_variables.py
JTOP_VARS="/usr/local/lib/python3.10/dist-packages/jtop/core/jetson_variables.py"

if [ -f "$JTOP_VARS" ]; then
    # L4T 36.4.7 -> JetPack 6.2.1 mapping
    TARGET_MAPPING='"36.4.7": "6.2.1",'
    
    if ! grep -q "$TARGET_MAPPING" "$JTOP_VARS"; then
        echo "Patching $JTOP_VARS for JetPack 6.2.1 support..."
        # Insert after JP6 comment or first mapping
        # sed -i.bak (suffix attached) is portable in-place on GNU and BSD; drop the backup.
        sudo sed -i.bak '/# -------- JP6 --------/a \    "36.4.7": "6.2.1",' "$JTOP_VARS" && sudo rm -f "${JTOP_VARS}.bak"
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
