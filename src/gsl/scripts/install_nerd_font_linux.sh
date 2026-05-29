#!/usr/bin/env bash
# install_nerd_font_linux.sh — gsl-packaged Linux/WSL Nerd Font installer.
# Installs the pinned MesloLGS NF faces into ~/.local/share/fonts and runs
# fc-cache. In WSL, also invokes the Windows host installer so Windows Terminal
# can render the font. Safe to re-run (idempotent).
set -euo pipefail

NERD_FONTS_VERSION="v3.4.0"
NERD_FONTS_ASSET="Meslo.zip"
NERD_FONTS_URL_BASE="https://github.com/ryanoasis/nerd-fonts/releases/download/${NERD_FONTS_VERSION}"

# ── Preflight ────────────────────────────────────────────────────────────────
missing=0
for tool in curl unzip fc-cache; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "ERROR: required tool '$tool' not found."
    echo "  Install it: sudo apt-get install -y fontconfig unzip curl"
    missing=1
  fi
done
[ "$missing" -eq 0 ] || exit 1

# ── Install Linux-side fonts ─────────────────────────────────────────────────
font_dir="${HOME}/.local/share/fonts/MesloLGS"
mkdir -p "$font_dir"
# Capture fc-list output to avoid SIGPIPE breaking pipefail when grep exits early.
_fc_list="$(fc-list 2>/dev/null)"
if echo "$_fc_list" | grep -qi "MesloLGS Nerd Font"; then
  echo "MesloLGS Nerd Font already installed; skipping download."
else
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  echo "Downloading ${NERD_FONTS_ASSET} (${NERD_FONTS_VERSION})..."
  curl -fsSL -o "${tmp}/${NERD_FONTS_ASSET}" "${NERD_FONTS_URL_BASE}/${NERD_FONTS_ASSET}"
  # Extract the four MesloLGS NerdFont faces (Regular, Bold, Italic, BoldItalic).
  # The v3.x archive uses no-space names: MesloLGSNerdFont-{style}.ttf
  unzip -o -q "${tmp}/${NERD_FONTS_ASSET}" \
    'MesloLGSNerdFont-Regular.ttf' \
    'MesloLGSNerdFont-Bold.ttf' \
    'MesloLGSNerdFont-Italic.ttf' \
    'MesloLGSNerdFont-BoldItalic.ttf' \
    -d "$font_dir"
  fc-cache -f "$font_dir"
  echo "Installed MesloLGS Nerd Font into ${font_dir} and refreshed font cache."
fi

echo "Verify: fc-list | grep -i MesloLGS"

# ── WSL: also install on the Windows host for Windows Terminal ───────────────
# Windows Terminal renders WSL sessions using the host font stack; the font
# must exist on Windows even when gsl runs in the Linux layer.
if grep -qi microsoft /proc/version 2>/dev/null; then
  SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
  WIN_INSTALLER_LINUX="${SCRIPT_DIR}/install_nerd_font_windows.ps1"
  if [ ! -f "$WIN_INSTALLER_LINUX" ]; then
    echo "WARNING: Windows installer not found at ${WIN_INSTALLER_LINUX}; skipping Windows host install."
  elif ! command -v powershell.exe >/dev/null 2>&1; then
    echo "WARNING: powershell.exe not in PATH; skipping Windows host install."
  else
    WIN_INSTALLER_WIN="$(wslpath -w "$WIN_INSTALLER_LINUX")"
    echo "Installing MesloLGS NF on Windows host via PowerShell..."
    powershell.exe -ExecutionPolicy Bypass -NonInteractive -WindowStyle Hidden \
      -File "$WIN_INSTALLER_WIN"
    echo "Windows host font install complete. Restart Windows Terminal to apply."
  fi
fi
