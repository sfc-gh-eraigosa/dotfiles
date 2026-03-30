#!/bin/bash
# Check if sshd is running as a system service
# Supports: systemctl (Linux), launchctl (macOS), and pgrep (fallback)

if [ -f ~/.sshd.env ] ; then
    . ~/.sshd.env
fi

if [ "${SSHD_LOGIN}" != "true" ] ; then
    echo "SSHD_LOGIN is not set to true in ~/.sshd.env. Skipping."
    exit 0
fi

# 1. Check systemctl (Modern Linux)
if command -v systemctl &> /dev/null; then
    if systemctl is-active --quiet sshd || systemctl is-active --quiet ssh; then
        echo "sshd is already running as a system service."
        exit 0
    fi
fi

# 2. Check launchctl (macOS)
if command -v launchctl &> /dev/null; then
    # Check if the service is loaded and running
    if sudo launchctl list com.openssh.sshd &> /dev/null; then
        echo "sshd is already running via launchctl."
        exit 0
    fi
fi

# 3. Check pgrep (fallback for all)
if pgrep -x "sshd" > /dev/null; then
    echo "sshd process is already running."
    exit 0
fi

# If we get here, try to start it
echo "Starting sshd in the background..."
if [ ! -d /var/run/sshd ] ; then
    sudo mkdir -p /var/run/sshd
fi

# macOS might need a different path or setup, but /usr/sbin/sshd is standard
sudo /usr/sbin/sshd -D &
