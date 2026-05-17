#!/usr/bin/env bash

# install_rclone_service.sh
#
# Helper script to install rclone_sync.sh as a persistent background service.
# - Linux: Creates a systemd user service.
# - macOS: Creates a LaunchAgent.

set -e

# Path to the sync script we want to run
SYNC_SCRIPT="$HOME/opt/scripts/data/rclone_sync.sh"
SERVICE_NAME="rclone-sync"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

function log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

function log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if the sync script exists
if [[ ! -f "$SYNC_SCRIPT" ]]; then
    log_error "Sync script not found at: $SYNC_SCRIPT"
    log_error "Please ensure the script is in place or adjust the SYNC_SCRIPT variable in this installer."
    exit 1
fi

# Ensure sync script is executable
if [[ ! -x "$SYNC_SCRIPT" ]]; then
    log_info "Making $SYNC_SCRIPT executable..."
    chmod +x "$SYNC_SCRIPT"
fi

function install_linux_service() {
    log_info "Detected Linux. Installing systemd user service..."

    # Create systemd user directory if it doesn't exist
    SYSTEMD_DIR="$HOME/.config/systemd/user"
    mkdir -p "$SYSTEMD_DIR"

    SERVICE_FILE="$SYSTEMD_DIR/${SERVICE_NAME}.service"

    log_info "Creating service file at: $SERVICE_FILE"

    cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Rclone Real-time Sync Service
After=network-online.target

[Service]
Type=simple
ExecStart=$SYNC_SCRIPT --watch
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
EOF

    # Reload systemd, enable and start service
    log_info "Reloading systemd daemon..."
    systemctl --user daemon-reload

    log_info "Enabling ${SERVICE_NAME}.service..."
    systemctl --user enable "${SERVICE_NAME}.service"

    log_info "Starting ${SERVICE_NAME}.service..."
    systemctl --user restart "${SERVICE_NAME}.service"

    log_info "Service installed and started successfully!"
    log_info "Check status with: systemctl --user status ${SERVICE_NAME}.service"
    log_info "View logs with: journalctl --user -u ${SERVICE_NAME}.service -f"
}

function install_macos_service() {
    log_info "Detected macOS. Installing LaunchAgent..."

    AGENTS_DIR="$HOME/Library/LaunchAgents"
    mkdir -p "$AGENTS_DIR"

    PLIST_NAME="com.$(whoami).${SERVICE_NAME}"
    PLIST_FILE="$AGENTS_DIR/${PLIST_NAME}.plist"

    log_info "Creating plist file at: $PLIST_FILE"

    cat > "$PLIST_FILE" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>${PLIST_NAME}</string>
    <key>ProgramArguments</key>
    <array>
        <string>${SYNC_SCRIPT}</string>
        <string>--watch</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/${SERVICE_NAME}.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/${SERVICE_NAME}.error.log</string>
</dict>
</plist>
EOF

    # Unload if already loaded to force update
    if launchctl list | grep -q "${PLIST_NAME}"; then
        log_info "Unloading existing service..."
        launchctl unload "$PLIST_FILE" || true
    fi

    log_info "Loading service..."
    launchctl load "$PLIST_FILE"

    log_info "Service installed and loaded successfully!"
    log_info "Logs available at: /tmp/${SERVICE_NAME}.log"
}


function show_help() {
    echo "Usage: $(basename "$0") [OPTIONS] [COMMAND]"
    echo ""
    echo "Commands:"
    echo "  install    Install the service (default)"
    echo "  uninstall  Uninstall the service"
    echo ""
    echo "Options:"
    echo "  -h, --help Show this help message"
}

function uninstall_linux_service() {
    log_info "Detected Linux. Uninstalling systemd user service..."
    
    # Define service file path corresponding to install logic
    SYSTEMD_DIR="$HOME/.config/systemd/user"
    SERVICE_FILE="$SYSTEMD_DIR/${SERVICE_NAME}.service"

    log_info "Stopping and disabling service..."
    systemctl --user stop "${SERVICE_NAME}.service" || true
    systemctl --user disable "${SERVICE_NAME}.service" || true
    
    if [[ -f "$SERVICE_FILE" ]]; then
        log_info "Removing service file: $SERVICE_FILE"
        rm "$SERVICE_FILE"
    else
        log_info "Service file not found at $SERVICE_FILE"
    fi
    
    log_info "Reloading systemd daemon..."
    systemctl --user daemon-reload
    log_info "Service uninstalled successfully."
}

function uninstall_macos_service() {
    log_info "Detected macOS. Uninstalling LaunchAgent..."
    
    AGENTS_DIR="$HOME/Library/LaunchAgents"
    PLIST_NAME="com.$(whoami).${SERVICE_NAME}"
    PLIST_FILE="$AGENTS_DIR/${PLIST_NAME}.plist"

    # Unload if loaded
    if launchctl list | grep -q "${PLIST_NAME}"; then
        log_info "Unloading service..."
        launchctl unload "$PLIST_FILE" || true
    fi

    if [[ -f "$PLIST_FILE" ]]; then
        log_info "Removing plist file: $PLIST_FILE"
        rm "$PLIST_FILE"
    else
        log_info "Plist file not found at $PLIST_FILE"
    fi
    
    log_info "Service uninstalled successfully."
}

# Main detection logic
OS_NAME="$(uname -s)"
COMMAND="install"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        install)
            COMMAND="install"
            shift
            ;;
        uninstall)
            COMMAND="uninstall"
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            log_error "Unknown argument: $1"
            show_help
            exit 1
            ;;
    esac
done

if [[ "$COMMAND" == "install" ]]; then
    case "$OS_NAME" in
        Linux*)     install_linux_service;;
        Darwin*)    install_macos_service;;
        *)          log_error "Unsupported operating system: $OS_NAME"; exit 1 ;;
    esac
elif [[ "$COMMAND" == "uninstall" ]]; then
    case "$OS_NAME" in
        Linux*)     uninstall_linux_service;;
        Darwin*)    uninstall_macos_service;;
        *)          log_error "Unsupported operating system: $OS_NAME"; exit 1 ;;
    esac
fi
