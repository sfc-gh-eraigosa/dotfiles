#!/bin/bash
# Setup docker permissions for the current user
# Works on Linux, Jetson, and handles macOS checks

OS_TYPE="$(uname -s)"

if ! command -v docker &> /dev/null; then
    echo "Docker is not installed. Skipping permission setup."
    exit 0
fi

echo "Setting up Docker permissions..."

# DOCKER_SOCK overrides the socket path (tests point it at a temp socket).
DOCKER_SOCK="${DOCKER_SOCK:-/var/run/docker.sock}"

# need_root <what>: true when a root-only change can run in this session. A
# fleet background run has no terminal and may have no cached credential, so
# every bare `sudo` there printed "sudo: a terminal is required" and failed;
# say once what was skipped instead. A terminal means sudo can prompt.
need_root() {
    if [ "$(id -u)" = 0 ] || sudo -n true 2>/dev/null || [ -t 0 ]; then
        return 0
    fi
    echo "Docker permissions: $1 needs sudo, which this session cannot use without a terminal; skipping."
    return 1
}

if [[ "$OS_TYPE" == "Linux" ]]; then
    # Create docker group if it doesn't exist
    if ! getent group docker > /dev/null; then
        if need_root "creating the docker group"; then
            echo "Creating docker group..."
            sudo groupadd docker
        fi
    fi

    # Resolve the target user even when $USER is unset (non-login shells, CI,
    # or root-driven automation) — otherwise groups/usermod get an empty arg.
    TARGET_USER="${USER:-$(id -un)}"

    # Add the target user to the docker group if not already a member.
    # root already has full docker access, so there is nothing to do.
    if [ "$TARGET_USER" != "root" ] && ! groups "$TARGET_USER" | grep &>/dev/null "\bdocker\b"; then
        if need_root "adding $TARGET_USER to the docker group"; then
            echo "Adding user $TARGET_USER to docker group..."
            sudo usermod -aG docker "$TARGET_USER"
            echo "NOTE: You may need to log out and back in for group changes to take effect."
            echo "Alternatively, run: newgrp docker"
        fi
    fi

    # Check if the socket exists and set permissions if needed for immediate
    # use — only when they are not already right, so an up-to-date host never
    # needs sudo here. 666 exists for a session not yet in the docker group
    # (usermod only takes effect at the next login); a session that already has
    # the group is served by a group-writable root:docker socket as it is.
    if [ -S "$DOCKER_SOCK" ]; then
        # -L: judge the socket that chown/chmod change, not a symlink to it
        # (Docker Desktop's WSL integration links /var/run/docker.sock; the
        # link itself reads root:root 777, so it never looked right).
        _sock_state="$(stat -L -c '%U:%G %a' "$DOCKER_SOCK" 2>/dev/null)" # portability-ok: Linux-only branch (GNU stat)
        _sock_ok=no
        case "$_sock_state" in
            "root:docker 666") _sock_ok=yes ;;
            "root:docker "[0-7][67][0-7])
                id -Gn 2>/dev/null | tr ' ' '\n' | grep -qx docker && _sock_ok=yes ;;
        esac
        if [ "$_sock_ok" = no ] && need_root "fixing $DOCKER_SOCK ownership/mode (is: ${_sock_state:-unknown})"; then
            echo "Ensuring $DOCKER_SOCK has correct permissions..."
            sudo chown root:docker "$DOCKER_SOCK"
            sudo chmod 666 "$DOCKER_SOCK"
        fi
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
