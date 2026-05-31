#!/usr/bin/env bash
# retire-ahk-voice-macro.sh — clean up the OLD AutoHotkey Copilot-key voice macro
# on a machine that was provisioned before dictation moved to Wispr Flow.
#
# The voice macro (Copilot key -> Windows Voice Typing into Claude Desktop) used
# to live inside opt/Desktop/Apps/scripts/macos.ahk and was deployed to the
# Windows Desktop. It has been retired: the canonical copy is preserved at
# archive/macos-copilot-claude-voice.ahk and macos.ahk no longer contains it.
#
# This script finds a DEPLOYED macos.ahk that still has the voice block, backs it
# up locally, re-deploys the cleaned repo copy over it, and restarts AutoHotkey so
# the change takes effect. The macOS-style Cmd shortcuts in macos.ahk are kept.
#
# Safe + idempotent: a no-op (with a clear message) when nothing old is found,
# when not running under WSL, or when PowerShell can't be reached.
#
# Usage:
#   retire-ahk-voice-macro.sh            clean if found
#   retire-ahk-voice-macro.sh --dry-run  report what it would do, change nothing
set -u

DRY_RUN=0
case "${1:-}" in
    --dry-run) DRY_RUN=1 ;;
    --help|-h)
        echo "Usage: retire-ahk-voice-macro.sh [--dry-run]"
        echo "Retire the old AHK Copilot-key voice macro from the Windows Desktop deploy."
        exit 0 ;;
    "") ;;
    *) echo "retire-ahk-voice-macro: unknown argument '$1'" >&2; exit 2 ;;
esac

# Repo root, resolved from this script's location (works via the ~/opt symlink).
SCRIPT_PATH="$(readlink -f "$0")"
BASE_DIR="$(cd "$(dirname "$SCRIPT_PATH")/../../.." && pwd)"
REPO_AHK="${BASE_DIR}/opt/Desktop/Apps/scripts/macos.ahk"

# Markers that identify the retired voice block in a deployed macos.ahk.
VOICE_MARKERS='ClaudeKey\(\)|Copilot key  ->  Claude Desktop voice assistant|\*F23::'

note() { echo "retire-ahk-voice-macro: $*"; }

# Only meaningful inside WSL (the deploy target is the Windows Desktop).
if ! grep -qi microsoft /proc/version 2>/dev/null; then
    note "not running under WSL; nothing to do."
    exit 0
fi

# Locate powershell.exe (PATH, then the standard System32 path via wslpath so
# custom automount.root configurations are respected).
ps_exe="$(command -v powershell.exe 2>/dev/null || true)"
if [ -z "$ps_exe" ]; then
    _ps_fallback="$(wslpath -u 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe' 2>/dev/null)"
    [ -n "$_ps_fallback" ] && [ -x "$_ps_fallback" ] && ps_exe="$_ps_fallback"
fi
if [ -z "$ps_exe" ]; then
    note "powershell.exe not found; cannot reach the Windows Desktop. Nothing to do."
    exit 0
fi

# Resolve the real Windows Desktop (may be OneDrive-redirected).
win_desktop_raw="$("$ps_exe" -NoProfile -Command "[Environment]::GetFolderPath('Desktop')" 2>/dev/null | tr -d '\r')"
win_desktop="$(wslpath -u "$win_desktop_raw" 2>/dev/null)"
if [ -z "$win_desktop" ] || [ ! -d "$win_desktop" ]; then
    note "could not resolve the Windows Desktop (${win_desktop_raw}); nothing to do."
    exit 0
fi

deployed="${win_desktop}/Apps/scripts/macos.ahk"
if [ ! -f "$deployed" ]; then
    note "no deployed macos.ahk at '${deployed}'; nothing to clean."
    exit 0
fi
if ! grep -qE "$VOICE_MARKERS" "$deployed" 2>/dev/null; then
    note "deployed macos.ahk is already free of the voice macro; nothing to clean."
    exit 0
fi

note "found the retired voice macro in: ${deployed}"
if [ "$DRY_RUN" = "1" ]; then
    note "[dry-run] would back it up, re-deploy the cleaned macos.ahk, and restart AutoHotkey."
    exit 0
fi

# Refuse to deploy a repo copy that itself still carries the voice block (e.g. an
# old checkout) — that would defeat the cleanup.
if [ ! -f "$REPO_AHK" ]; then
    note "ERROR: repo macos.ahk not found at '${REPO_AHK}'; aborting." >&2
    exit 1
fi
if grep -qE "$VOICE_MARKERS" "$REPO_AHK" 2>/dev/null; then
    note "ERROR: the repo's macos.ahk still contains the voice macro — update the repo first." >&2
    exit 1
fi

# Back up the deployed copy locally (outside the repo — it's machine state).
backup_dir="${HOME}/.config/dotfiles/ahk-voice-backups"
mkdir -p "$backup_dir"
ts="$(date +%Y%m%d-%H%M%S)"
backup="${backup_dir}/macos.ahk.${ts}"
cp "$deployed" "$backup"
note "backed up the old deployed copy to: ${backup}"

# Re-deploy the cleaned repo copy.
cp "$REPO_AHK" "$deployed"
note "re-deployed the cleaned macos.ahk (macOS shortcuts kept, voice macro removed)."

# Restart AutoHotkey so it reloads the cleaned script.
"$ps_exe" -NoProfile -Command \
    "Get-Process AutoHotkey* -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue; \
     try { Start-ScheduledTask -TaskName 'macOS Hotkeys' -ErrorAction Stop } catch { Write-Output 'NOTE: could not start the macOS Hotkeys task; it will start at next logon.' }" \
    2>/dev/null | tr -d '\r'

note "done. The Copilot key is now free for Wispr Flow — bind it in Flow (see WISPR-FLOW.md)."
