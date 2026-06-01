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

# Per-run marker: set only when the interactive Windows setup actually runs, so
# install.sh can print the Wispr Flow reminder banner at the very end. Cleared on
# every invocation (even non-WSL) so a stale marker never triggers a false banner.
WIN_SETUP_MARKER="${HOME}/.config/dotfiles/.windows-setup-just-ran"
rm -f "$WIN_SETUP_MARKER" 2>/dev/null || true

# Only run inside WSL (Windows Subsystem for Linux).
if ! grep -qi microsoft /proc/version 2>/dev/null; then
  exit 0
fi

# ---------------------------------------------------------------------------
# Ensure Windows interop is live. Without the WSLInterop binfmt handler, every
# Windows .exe (powershell.exe, winget, the wslpath targets below) fails with
# "exec format error" and this entire Windows setup silently no-ops. WSL normally
# registers the handler at boot; if it has gone missing (interop disabled, or a
# flaky restart), re-register it at runtime. binfmt_misc REFUSES a duplicate name,
# so only register when BOTH known handler names are absent. Session-only; the
# persistent fix is `[interop] enabled=true` in /etc/wsl.conf.
# ---------------------------------------------------------------------------
if [ ! -e /proc/sys/fs/binfmt_misc/WSLInterop ] && [ ! -e /proc/sys/fs/binfmt_misc/WSLInterop-late ]; then
  if [ -e /proc/sys/fs/binfmt_misc/register ]; then
    echo "WSL interop handler not registered; enabling it (may prompt for sudo)..."
    # install.sh caches sudo up front, so this is normally non-interactive.
    sudo bash -c 'echo ":WSLInterop:M::MZ::/init:PF" > /proc/sys/fs/binfmt_misc/register' 2>/dev/null \
      && echo "  WSL interop registered." \
      || echo "  WARNING: could not register WSL interop (need root?)." >&2
  else
    echo "WARNING: binfmt_misc not mounted; cannot enable WSL interop." >&2
  fi
fi

# ---------------------------------------------------------------------------
# Locate powershell.exe: prefer PATH, fall back to the standard System32 path.
# (Windows exes are not always on the WSL PATH, e.g. appendWindowsPath=false.)
# ---------------------------------------------------------------------------
ps_exe="$(command -v powershell.exe 2>/dev/null || true)"
if [ -z "$ps_exe" ]; then
  _ps_fallback="$(wslpath -u 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe' 2>/dev/null)"
  [ -n "$_ps_fallback" ] && [ -x "$_ps_fallback" ] && ps_exe="$_ps_fallback"
fi

if [ -z "$ps_exe" ]; then
  echo "NOTE: powershell.exe not found; skipping Windows Desktop deploy."
  exit 0
fi

# ---------------------------------------------------------------------------
# Resolve the real Desktop path (may be OneDrive-redirected).
# ---------------------------------------------------------------------------
# NOTE: </dev/null is load-bearing. powershell.exe (and Windows console exes in
# general) consume the parent shell's stdin under WSL interop. Without this, the
# Desktop lookup drains stdin and the interactive "Choice [y/n/s]" read below gets
# EOF -> empty -> silently skips Windows setup on a freshly-detected Windows box.
win_desktop_raw="$("$ps_exe" -NoProfile -Command "[Environment]::GetFolderPath('Desktop')" </dev/null 2>/dev/null | tr -d '\r')"
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
# Prompt on the controlling terminal, NOT stdin. Even with the </dev/null guards
# above, stdin can still be non-interactive here: piped installs (curl | bash) have
# no tty on stdin at all. /dev/tty reaches the real terminal in that case; if there
# is genuinely no terminal (CI), we say so rather than silently skipping.
if [ -r /dev/tty ]; then
    read -rp "Choice [y/n/s]: " choice < /dev/tty
else
    choice="__notty__"
fi

case "$choice" in
    y|Y)
        echo "Starting Windows customization... (this may take a few minutes)"
        # setup-apps.ps1 does the non-elevated app installs, then fires ONE
        # Start-Process -Verb RunAs (setup-elevated.ps1) that performs all admin work
        # in a single elevated child: the macOS-hotkeys logon task, the iTunes Win32
        # MSI, the Wispr Flow MSI, and the PowerToys Copilot-key remap. A single UAC
        # prompt appears during the run — approve it.
        # </dev/null is load-bearing: powershell.exe consumes the parent shell's stdin
        # under WSL interop (see the Desktop-path lookup above for the same guard).
        "$ps_exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "${BASE_DIR}/opt/Desktop/Apps/scripts/setup-apps.ps1" </dev/null > /tmp/setup_apps.log 2>&1
        cat /tmp/setup_apps.log

        # The app/MSI/task elevation all happens inside that single batch. Only Flow's
        # one-time ACCOUNT setup can't be scripted (sign-in, mic, shortcuts off the Win
        # key, start-at-login) — point at the runbook for it.
        wispr_doc_w="$(wslpath -w "${win_desktop}/Apps/scripts/WISPR-FLOW.md" 2>/dev/null)"
        echo ""
        echo "Wispr Flow one-time manual setup (sign-in, mic, shortcuts off Win, start-at-login):"
        echo "    ${wispr_doc_w:-%USERPROFILE%\\Desktop\\Apps\\scripts\\WISPR-FLOW.md}"
        echo ""

        # Mark that the Windows setup ran so install.sh prints the Wispr Flow
        # shortcut reminder banner at the very end (after all other output).
        mkdir -p "$(dirname "$WIN_SETUP_MARKER")"
        : > "$WIN_SETUP_MARKER"
        ;;
    s|S)
        echo "Creating sentinel file at $SENTINEL_FILE. Will not ask again."
        mkdir -p "$SENTINEL_DIR"
        echo "User chose to skip Windows setup on $(date)" > "$SENTINEL_FILE"
        ;;
    __notty__)
        # No controlling terminal (CI / fully non-interactive). Windows WAS detected,
        # so don't pretend we asked and don't silently skip — point at the exact
        # command to finish setup interactively.
        echo "No terminal available to prompt; Windows customization not run."
        echo "Windows detected — to configure it, run from an interactive shell:"
        echo "    bash \"${BASE_DIR}/opt/bin/install_windows.sh\" \"${BASE_DIR}\""
        ;;
    *)
        echo "Skipping Windows customization for now."
        ;;
esac
