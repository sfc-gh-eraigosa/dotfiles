#!/usr/bin/env bash
# wsl_interop_binfmt.sh — keep the WSLInterop binfmt registration alive on WSL2.
#
# Problem: the WSLInterop entry in /proc/sys/fs/binfmt_misc can get wiped after
# boot, and then EVERY Windows .exe fails with "exec format error". WSL ships a
# self-heal for this (a generated systemd-binfmt override that re-registers the
# entry), but that unit carries ConditionVirtualization=!container, which fails
# under WSL — so the self-heal never runs and /etc/binfmt.d/ is never applied.
#
# Fix: install a small systemd unit of our own (no virtualization condition)
# that re-registers WSLInterop at boot if it is missing, and start it once now.
# The registration string matches WSL's own generator (flags :P).
#
# Idempotent and non-fatal: safe to re-run from install.sh; exits 0 with a
# warning when it cannot act (not WSL, no systemd, no root/sudo).
set -u

UNIT_NAME="wsl-interop-binfmt.service"
UNIT_PATH="/etc/systemd/system/${UNIT_NAME}"
BINFMT_ENTRY=':WSLInterop:M::MZ::/init:P'
REGISTER_CMD="[ -f /proc/sys/fs/binfmt_misc/WSLInterop ] || echo \"${BINFMT_ENTRY}\" > /proc/sys/fs/binfmt_misc/register"

# WSL only
if ! grep -qi microsoft /proc/version 2>/dev/null; then
    exit 0
fi

# systemd only (persistence needs it; without systemd WSL re-registers on boot itself)
if [ ! -d /run/systemd/system ]; then
    echo "WSL interop: no systemd; skipping persistent binfmt unit." >&2
    exit 0
fi

interop_registered() {
    [ -f /proc/sys/fs/binfmt_misc/WSLInterop ]
}

# Pick a privilege escalation path: root, interactive sudo (tty), or sudo -n.
run_root() {
    if [ "$(id -u)" = "0" ]; then
        "$@"
    elif [ -t 0 ]; then
        sudo "$@"
    else
        sudo -n "$@"
    fi
}

if interop_registered && [ -f "${UNIT_PATH}" ]; then
    echo "WSL interop: WSLInterop binfmt registered and ${UNIT_NAME} installed."
    exit 0
fi

unit_content() {
    cat <<EOF
[Unit]
Description=Restore WSLInterop binfmt registration (dotfiles)
Documentation=https://www.kernel.org/doc/html/latest/admin-guide/binfmt-misc.html
DefaultDependencies=no
After=local-fs.target
ConditionPathExists=/init

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/sh -c '${REGISTER_CMD}'

[Install]
WantedBy=multi-user.target
EOF
}

if ! unit_content | run_root tee "${UNIT_PATH}" > /dev/null 2>&1; then
    echo "WSL interop: WARNING — need root to install ${UNIT_PATH}." >&2
    echo "WSL interop: run manually: sudo sh -c 'echo \"${BINFMT_ENTRY}\" > /proc/sys/fs/binfmt_misc/register'" >&2
    exit 0
fi

run_root systemctl daemon-reload
run_root systemctl enable "${UNIT_NAME}" > /dev/null 2>&1
run_root systemctl restart "${UNIT_NAME}"

if interop_registered; then
    echo "WSL interop: WSLInterop binfmt registered; ${UNIT_NAME} will restore it on boot."
else
    echo "WSL interop: WARNING — registration still missing after ${UNIT_NAME} ran." >&2
    exit 1
fi
