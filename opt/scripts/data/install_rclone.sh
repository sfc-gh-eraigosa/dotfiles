#!/bin/bash

# install_rclone.sh - Cross-platform installation script for rclone and requirements
# Supports: macOS (via Homebrew) and Linux (via apt)

set -e

echo "Starting installation for rclone and requirements..."

OS="$(uname)"

case "$OS" in
    "Darwin")
        echo "Detected macOS."
        if ! command -v brew >/dev/null 2>&1; then
            echo "Error: Homebrew not found. Please install Homebrew first: https://brew.sh/"
            exit 1
        fi
        
        echo "Installing rclone..."
        brew install rclone

        # inotify-tools is Linux-only. fswatch is a common alternative on macOS.
        echo "Note: inotify-tools is Linux-only. Installing fswatch as an alternative..."
        brew install fswatch
        ;;
    "Linux")
        echo "Detected Linux."
        if command -v apt-get >/dev/null 2>&1; then
            echo "Updating package list..."
            sudo apt-get update
            echo "Installing rclone and inotify-tools..."
            # noninteractive: a debconf prompt would hang non-tty contexts (see PR #182's tzdata hang)
            sudo DEBIAN_FRONTEND=noninteractive apt-get install -y rclone inotify-tools
        elif command -v snap >/dev/null 2>&1; then
            echo "apt-get not found, attempting installation via snap..."
            sudo snap install rclone
            echo "Warning: inotify-tools may need to be installed via your distribution's package manager."
        else
            echo "Error: Could not find a supported package manager (apt-get). Please install rclone and inotify-tools manually."
            exit 1
        fi
        ;;
    *)
        echo "Error: Unsupported OS: $OS"
        exit 1
        ;;
esac

echo "--------------------------------------------------"
echo "Installation complete!"
echo "rclone version: $(rclone version | head -n 1)"
if command -v inotifywait >/dev/null 2>&1; then
    echo "inotify-tools is installed."
elif command -v fswatch >/dev/null 2>&1; then
    echo "fswatch is installed (macOS substitute for inotify-tools)."
fi
echo "--------------------------------------------------"
echo "Next steps:"
echo "1. Run 'rclone config' to set up your cloud storage remote."
echo "2. Check the documentation in docs/rclone_setup.md for syncing instructions."
