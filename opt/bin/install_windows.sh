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
