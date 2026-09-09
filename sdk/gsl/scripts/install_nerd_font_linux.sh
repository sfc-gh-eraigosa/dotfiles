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
# Required commands and their Debian/Ubuntu package names (fc-cache ships in
# fontconfig). On apt-based systems we auto-install whatever's missing so a fresh
# clone bootstraps cleanly; on other distros we fall back to a clear message.
pkg_for() { case "$1" in fc-cache) echo fontconfig ;; *) echo "$1" ;; esac; }

missing_pkgs=()
for tool in curl unzip fc-cache; do
  command -v "$tool" >/dev/null 2>&1 || missing_pkgs+=("$(pkg_for "$tool")")
done

if [ "${#missing_pkgs[@]}" -gt 0 ]; then
  if command -v apt-get >/dev/null 2>&1; then
    SUDO=""
    [ "$(id -u)" -ne 0 ] && command -v sudo >/dev/null 2>&1 && SUDO="sudo"
    echo "Installing missing font prerequisites: ${missing_pkgs[*]}"
    $SUDO apt-get update -y -qq >/dev/null 2>&1 || true
    $SUDO apt-get install -y -qq "${missing_pkgs[@]}" || true
  fi
  # Re-check; only abort if a tool is STILL missing after the install attempt.
  missing=0
  for tool in curl unzip fc-cache; do
    if ! command -v "$tool" >/dev/null 2>&1; then
      echo "ERROR: required tool '$tool' not found."
      missing=1
    fi
  done
  if [ "$missing" -ne 0 ]; then
    echo "  Install the prerequisites and re-run: sudo apt-get install -y fontconfig unzip curl"
    exit 1
  fi
fi

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

# ── Point gnome-terminal at the font ─────────────────────────────────────────
# Installing the font is not enough: gnome-terminal defaults to use-system-font,
# i.e. the GNOME monospace font (Ubuntu Sans Mono), which has no Nerd Font PUA
# glyphs — so gsl's powerline separators and icons render as tofu on a machine
# where the font IS correctly installed. check-font-glyphs.sh validates the font
# FILE, so it passes and never catches this. Symmetric with the WSL branch below,
# which installs on the Windows host precisely so Windows Terminal can render it.
#
# Only touches the DEFAULT profile, only when it is still on the system font, so
# a deliberate font choice is never overwritten.
configure_gnome_terminal_font() {
  command -v gsettings >/dev/null 2>&1 || return 0
  [ -n "${DBUS_SESSION_BUS_ADDRESS:-}" ] || return 0
  [ -n "${XDG_RUNTIME_DIR:-}" ] && [ -w "${XDG_RUNTIME_DIR}" ] || return 0

  # The Profile schema is RELOCATABLE; `gsettings list-schemas` omits those
  # entirely, so it must be looked up with list-relocatable-schemas.
  gsettings list-relocatable-schemas 2>/dev/null |
    grep -qx 'org.gnome.Terminal.Legacy.Profile' || return 0

  _prof="$(gsettings get org.gnome.Terminal.ProfilesList default 2>/dev/null | tr -d "'")"
  [ -n "$_prof" ] || return 0
  _key="org.gnome.Terminal.Legacy.Profile:/org/gnome/terminal/legacy/profiles:/:${_prof}/"

  # Respect an explicit user font choice.
  if [ "$(gsettings get "$_key" use-system-font 2>/dev/null)" != "true" ]; then
    echo "gnome-terminal already uses a custom font; leaving it alone."
    return 0
  fi

  # Carry over the size already in effect so only the glyphs change, not layout.
  _size="$(gsettings get org.gnome.desktop.interface monospace-font-name 2>/dev/null |
    tr -d "'" | awk '{print $NF}')"
  case "$_size" in ''|*[!0-9]*) _size=12 ;; esac

  gsettings set "$_key" use-system-font false 2>/dev/null &&
    gsettings set "$_key" font "MesloLGS Nerd Font ${_size}" 2>/dev/null &&
    echo "Set gnome-terminal font to 'MesloLGS Nerd Font ${_size}'." ||
    echo "WARNING: could not set the gnome-terminal font."
}

configure_gnome_terminal_font

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
