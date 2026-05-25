#!/bin/bash
# Setup docker permissions for the current user
# Works on Linux, Jetson, and handles macOS checks

OS_TYPE="$(uname -s)"

if ! command -v docker &> /dev/null; then
    echo "Docker is not installed. Skipping permission setup."
    exit 0
fi

echo "Setting up Docker permissions..."

if [[ "$OS_TYPE" == "Linux" ]]; then
    # Create docker group if it doesn't exist
    if ! getent group docker > /dev/null; then
        echo "Creating docker group..."
        sudo groupadd docker
    fi

    # Resolve the target user even when $USER is unset (non-login shells, CI,
    # or root-driven automation) — otherwise groups/usermod get an empty arg.
    TARGET_USER="${USER:-$(id -un)}"

    # Add the target user to the docker group if not already a member.
    # root already has full docker access, so there is nothing to do.
    if [ "$TARGET_USER" != "root" ] && ! groups "$TARGET_USER" | grep &>/dev/null "\bdocker\b"; then
        echo "Adding user $TARGET_USER to docker group..."
        sudo usermod -aG docker "$TARGET_USER"
        echo "NOTE: You may need to log out and back in for group changes to take effect."
        echo "Alternatively, run: newgrp docker"
    fi

    # Check if the socket exists and set permissions if needed for immediate use
    if [ -S /var/run/docker.sock ]; then
        echo "Ensuring /var/run/docker.sock has correct permissions..."
        sudo chown root:docker /var/run/docker.sock
        sudo chmod 666 /var/run/docker.sock
    fi

    # Final verification for current shell session
    if ! groups | grep -q "\bdocker\b"; then
        # Detect the shell that called this script
        CALLER_SHELL=$(ps -p $PPID -o comm= 2>/dev/null | sed 's/^-//')
        [ -z "$CALLER_SHELL" ] && CALLER_SHELL="$SHELL"
        [ -z "$CALLER_SHELL" ] && CALLER_SHELL="bash"

        echo "----------------------------------------------------------------------"
        echo "WARNING: Your current shell session does NOT yet recognize the 'docker' group."
        echo "To apply group changes without logging out, run this command now:"
        echo ""
        echo "    exec sg docker \"$CALLER_SHELL\""
        echo ""
        echo "Or simply log out and back in."
        echo "----------------------------------------------------------------------"
    fi

elif [[ "$OS_TYPE" == "Darwin" ]]; then
    echo "On macOS, Docker permissions are usually managed by Docker Desktop."
    # Use gtimeout on macOS (coreutils), timeout on Linux
    _timeout="timeout"
    command -v timeout &>/dev/null || _timeout="gtimeout"
    if command -v "$_timeout" &>/dev/null && ! ("$_timeout" 5 docker system info &>/dev/null) 2>/dev/null; then
        echo "Docker Desktop does not appear to be running. Skipping Docker checks."
    elif ! command -v "$_timeout" &>/dev/null; then
        echo "NOTE: 'timeout' command not found. Skipping Docker daemon check (install coreutils)."
    else
        echo "Docker is responsive."
    fi
fi

echo "Docker permission setup complete."
