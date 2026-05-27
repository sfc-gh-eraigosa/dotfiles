#!/bin/bash
# =============================================================================
# install_windows.sh — Windows/WSL-only deploy step
#
# Copies opt/Desktop/* onto the real Windows Desktop. The Desktop folder is
# often OneDrive-redirected, so we ask Windows for its actual path via
# PowerShell and translate it with wslpath.
#
# Usage (called from install.sh):
#   bash "${BASE_DIR}/opt/bin/install_windows.sh" "<BASE_DIR>"
#
# No-op when not running inside WSL / Windows.
# =============================================================================

set -euo pipefail

BASE_DIR="${1:-}"
if [ -z "$BASE_DIR" ]; then
  echo "ERROR: install_windows.sh requires BASE_DIR as the first argument." >&2
  exit 1
fi

# Only run inside WSL (Windows Subsystem for Linux).
if ! grep -qi microsoft /proc/version 2>/dev/null; then
  exit 0
fi

# ---------------------------------------------------------------------------
# Locate powershell.exe: prefer PATH, fall back to the standard System32 path.
# (Windows exes are not always on the WSL PATH, e.g. appendWindowsPath=false.)
# ---------------------------------------------------------------------------
ps_exe="$(command -v powershell.exe 2>/dev/null || true)"
if [ -z "$ps_exe" ] && [ -x "/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe" ]; then
  ps_exe="/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe"
fi

if [ -z "$ps_exe" ]; then
  echo "NOTE: powershell.exe not found; skipping Windows Desktop deploy."
  exit 0
fi

# ---------------------------------------------------------------------------
# Resolve the real Desktop path (may be OneDrive-redirected).
# ---------------------------------------------------------------------------
win_desktop_raw="$("$ps_exe" -NoProfile -Command "[Environment]::GetFolderPath('Desktop')" 2>/dev/null | tr -d '\r')"
win_desktop="$(wslpath -u "$win_desktop_raw" 2>/dev/null)"

if [ -z "$win_desktop" ] || [ ! -d "$win_desktop" ]; then
  echo "WARNING: could not resolve Windows Desktop (${win_desktop_raw}); skipping desktop deploy."
  exit 0
fi

# ---------------------------------------------------------------------------
# Deploy opt/Desktop/* onto the Windows Desktop.
# ---------------------------------------------------------------------------
echo "Deploying opt/Desktop -> ${win_desktop}"
cp -r "${BASE_DIR}/opt/Desktop/." "${win_desktop}/"

# ---------------------------------------------------------------------------
# Interactive Windows Customization (WSL only)
# ---------------------------------------------------------------------------
SENTINEL_DIR="${HOME}/.config/dotfiles"
SENTINEL_FILE="${SENTINEL_DIR}/.skip_windows_setup"

if [ -f "$SENTINEL_FILE" ]; then
    exit 0
fi

echo ""
echo "Windows Desktop Customization detected."
echo "Would you like to run the PowerShell setup scripts to configure Windows Terminal,"
echo "install desktop apps (Discord, Slack, AutoHotkey, etc.), and set up macOS-style hotkeys?"
echo ""
echo "Options:"
echo "  [y] Yes, run setup now."
echo "  [n] No, skip for now (will ask again next time)."
echo "  [s] Skip and never ask again (creates sentinel file)."
echo ""
read -rp "Choice [y/n/s]: " choice

case "$choice" in
    y|Y)
        echo "Starting Windows customization... (this may take a few minutes)"
        # Use the absolute path and redirection to avoid the pipe-hang issue discovered earlier.
        # Run setup-apps.ps1 first to ensure all apps (including AutoHotkey) are installed.
        "$ps_exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "${BASE_DIR}/opt/Desktop/Apps/scripts/setup-apps.ps1" > /tmp/setup_apps.log 2>&1
        cat /tmp/setup_apps.log
        
        # Then setup the macOS-style hotkeys (AutoHotkey)
        echo "Registering macOS-style hotkeys (may trigger a Windows UAC prompt)..."
        "$ps_exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "${BASE_DIR}/opt/Desktop/Apps/scripts/setup-autostart.ps1" > /tmp/setup_autostart.log 2>&1
        cat /tmp/setup_autostart.log
        ;;
    s|S)
        echo "Creating sentinel file at $SENTINEL_FILE. Will not ask again."
        mkdir -p "$SENTINEL_DIR"
        echo "User chose to skip Windows setup on $(date)" > "$SENTINEL_FILE"
        ;;
    *)
        echo "Skipping Windows customization for now."
        ;;
esac
