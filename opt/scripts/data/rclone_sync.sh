#!/bin/bash

# Rclone Automatic Synchronization Script
# Monitors a local directory and syncs changes to a remote rclone destination.
# Supports Linux (inotifywait) and macOS (fswatch).

# --- Configuration ---
SOURCE_DIR="$HOME/GitHub"
REMOTE_NAME="gdrive"                # Name of the rclone remote
REMOTE_PATH="RaspberryPi/GitHub"    # Path on Google Drive
DEBOUNCE_TIME=5                     # Seconds to wait after a change before syncing
# ---------------------

OS="$(uname)"
WATCH_MODE=false
RESTORE_MODE=false
SKIP_LIST="RosettaCommons"

show_help() {
    cat << EOF
Usage: $(basename "$0") [OPTIONS]

Options:
  --watch           Continuous monitoring mode (triggers sync on changes).
  --restore         Reverse sync: Remote -> Local. (WARNING: Overwrites local files!)
  --skip FOLDERS    Comma-separated list of folders to skip (relative to source).
  --help            Show this help message.

Default:
  Performs a one-time sync from Local to Remote.

Examples:
  $(basename "$0") --watch
  $(basename "$0") --skip "node_modules,temp,.venv"
  $(basename "$0") --restore
EOF
}

# Argument parsing
while [[ $# -gt 0 ]]; do
    case "$1" in
        --watch)
            WATCH_MODE=true
            shift
            ;;
        --restore)
            RESTORE_MODE=true
            shift
            ;;
        --skip)
            SKIP_LIST="$2"
            shift 2
            ;;
        --help)
            show_help
            exit 0
            ;;
        *)
            echo "Error: Unknown option $1"
            show_help
            exit 1
            ;;
    esac
done

# Ensure requirements are met
if [[ "$OS" == "Linux" ]]; then
    if [[ "$WATCH_MODE" == "true" ]] && ! command -v inotifywait >/dev/null 2>&1; then
        echo "Error: inotifywait not found. Please install inotify-tools (e.g., sudo apt install inotify-tools)."
        exit 1
    fi
elif [[ "$OS" == "Darwin" ]]; then
    if [[ "$WATCH_MODE" == "true" ]] && ! command -v fswatch >/dev/null 2>&1; then
        echo "Error: fswatch not found. Please install it (e.g., brew install fswatch)."
        exit 1
    fi
else
    echo "Error: Unsupported OS: $OS"
    exit 1
fi

# Ensure rclone is installed
if ! command -v rclone >/dev/null 2>&1; then
    echo "Error: rclone not found."
    exit 1
fi

# Function to perform the sync
perform_sync() {
    local source="$1"
    local dest="$2"
    local mode_name="$3"
    local extra_args=()

    # Handle skip list
    if [[ -n "$SKIP_LIST" ]]; then
        IFS=',' read -ra ADDR <<< "$SKIP_LIST"
        for i in "${ADDR[@]}"; do
            extra_args+=(--exclude "$i/**")
        done
    fi

    # Simple logic: just run the sync if it's not already running
    if ! pgrep -x "rclone" > /dev/null; then
        echo "Starting $mode_name sync..."
        rclone sync "$source" "$dest" \
            --verbose \
            --transfers 4 \
            --checkers 8 \
            --contimeout 60s \
            --timeout 300s \
            --retries 3 \
            --low-level-retries 10 \
            --exclude ".git/**" \
            --exclude "node_modules/**" \
            --exclude ".venv/**" \
            "${extra_args[@]}"
        echo "Sync complete."
    else
        echo "Sync already in progress, skipping this trigger."
    fi
}

if [[ "$RESTORE_MODE" == "true" ]]; then
    echo "WARNING: This will overwrite files in $SOURCE_DIR with contents from $REMOTE_NAME:$REMOTE_PATH"
    read -p "Are you sure you want to proceed? (y/N): " -n 1 -r
    echo 
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Restoration cancelled."
        exit 0
    fi
    perform_sync "$REMOTE_NAME:$REMOTE_PATH" "$SOURCE_DIR" "RESTORE (Remote -> Local)"
elif [[ "$WATCH_MODE" == "true" ]]; then
    echo "Starting rclone sync monitor for $SOURCE_DIR..."
    echo "Syncing to $REMOTE_NAME:$REMOTE_PATH (OS: $OS, Mode: WATCH)"

    # Define the monitor command based on OS
    if [[ "$OS" == "Linux" ]]; then
        MONITOR_CMD="inotifywait -m -r -e modify,create,delete,move \"$SOURCE_DIR\""
    else
        MONITOR_CMD="fswatch -r \"$SOURCE_DIR\""
    fi

    # Main loop
    eval "$MONITOR_CMD" | while read -r line; do
        echo "Change detected: $line"
        
        # Wait for things to settle down (debounce)
        sleep "$DEBOUNCE_TIME"
        
        perform_sync "$SOURCE_DIR" "$REMOTE_NAME:$REMOTE_PATH" "WATCH (Local -> Remote)"
    done
else
    echo "Starting one-time rclone sync for $SOURCE_DIR..."
    echo "Syncing to $REMOTE_NAME:$REMOTE_PATH (OS: $OS, Mode: MANUAL)"
    perform_sync "$SOURCE_DIR" "$REMOTE_NAME:$REMOTE_PATH" "MANUAL (Local -> Remote)"
fi
