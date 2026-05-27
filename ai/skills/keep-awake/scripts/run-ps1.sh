#!/usr/bin/env bash
#
# run-ps1.sh — Launch one of this skill's PowerShell scripts from inside a WSL
# shell on a Windows host. Mirrors the convention in opt/bin/install_windows.sh:
# powershell.exe is NOT always on the WSL PATH (e.g. appendWindowsPath=false in
# /etc/wsl.conf), so we locate it explicitly and translate the script path with
# wslpath before invoking it.
#
# Usage:
#   ./run-ps1.sh <script.ps1> [args...]      # script.ps1 is relative to this dir
#
# Examples:
#   ./run-ps1.sh keep-awake.ps1 -SleepMonitors
#   ./run-ps1.sh keep-awake.ps1 -RemindAt 06:30 -IncludeBattery
#   ./run-ps1.sh revert-keepawake.ps1 -Minutes 5 -Quiet
#
set -euo pipefail

# Only meaningful inside WSL. On native Windows, run the .ps1 directly in
# PowerShell; on macOS/Linux, use the .sh scripts instead.
if ! grep -qi microsoft /proc/version 2>/dev/null; then
    echo "ERROR: run-ps1.sh is for WSL on a Windows host." >&2
    echo "  - Native Windows: run the .ps1 directly with powershell.exe." >&2
    echo "  - macOS:          use keep-awake.sh / revert-keepawake.sh." >&2
    exit 1
fi

# Locate powershell.exe: prefer PATH, fall back to the standard System32 path.
ps_exe="$(command -v powershell.exe 2>/dev/null || true)"
if [ -z "$ps_exe" ] && [ -x "/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe" ]; then
    ps_exe="/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe"
fi
if [ -z "$ps_exe" ]; then
    echo "ERROR: powershell.exe not found on PATH or in System32." >&2
    exit 1
fi

script_name="${1:-}"
[ -n "$script_name" ] || { echo "Usage: run-ps1.sh <script.ps1> [args...]" >&2; exit 1; }
shift

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script_path="${here}/${script_name}"
[ -f "$script_path" ] || { echo "ERROR: no such script: ${script_path}" >&2; exit 1; }

# Translate the WSL path to a Windows path (e.g. \\wsl.localhost\Ubuntu\...).
win_script="$(wslpath -w "$script_path")"

exec "$ps_exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$win_script" "$@"
