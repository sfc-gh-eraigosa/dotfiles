#!/usr/bin/env bash
# install_nerd_font_linux.sh — gsl-packaged Linux/WSL Nerd Font installer.
# Installs the pinned MesloLGS NF faces into ~/.local/share/fonts and runs
# fc-cache. In WSL, also invokes the Windows host installer so Windows Terminal
# can render the font. Safe to re-run (idempotent).
set -euo pipefail

# The release tag (v-prefixed) comes from the `nerd-fonts` row of
# opt/profiles/versions.tsv (fail-open to v3.4.0); NERD_FONTS_VERSION overrides it.
_nf_lib="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/../../../opt/lib/pins.sh"
if [ -z "${NERD_FONTS_VERSION:-}" ]; then
  NERD_FONTS_VERSION="3.4.0"
  if [ -f "${_nf_lib}" ]; then
    # shellcheck source=../../../opt/lib/pins.sh
    . "${_nf_lib}"
    NERD_FONTS_VERSION="$(pins_want nerd-fonts 3.4.0)"
    if [ "${NERD_FONTS_VERSION}" = "latest" ]; then
      NERD_FONTS_VERSION="$(pins_latest_gh ryanoasis/nerd-fonts || echo 3.4.0)"
    fi
  fi
fi
NERD_FONTS_VERSION="v${NERD_FONTS_VERSION#v}"
unset _nf_lib
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

# The Nerd Fonts patcher stamps "Nerd Fonts X.Y.Z" into every face's version
# string, so the files on disk say which release they came from. Presence on
# disk decides whether to (re)install — NOT `fc-list`: in the 2026-10-16 fleet
# run fc-list came back empty for a font that WAS installed, the installer
# re-downloaded it, and `unzip -o` replaced the faces under a running
# gnome-terminal, which then drew every powerline glyph from a fallback font.
installed_nerd_font_version() { # prints X.Y.Z; prints nothing when unreadable
  LC_ALL=C tr -c '[:print:]' '\n' < "$1" 2>/dev/null |
    grep -o 'Nerd Fonts [0-9][0-9.]*' | head -1 | awk '{print $3}'
}

_regular="${font_dir}/MesloLGSNerdFont-Regular.ttf"
want="${NERD_FONTS_VERSION#v}"
have=""
had_files=0
if [ -f "$_regular" ]; then
  had_files=1
  have="$(installed_nerd_font_version "$_regular")" || have=""
fi

if [ "$had_files" -eq 1 ] && [ "$have" = "$want" ]; then
  echo "MesloLGS Nerd Font v${have} already installed (pinned v${want}); skipping download."
  # fontconfig can still miss the directory (stale cache); re-register it
  # rather than re-downloading — registering does not touch the files.
  fc-list 2>/dev/null | grep -qi "MesloLGS Nerd Font" || fc-cache -f "$font_dir"
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
  echo "Installed MesloLGS Nerd Font v${want} into ${font_dir} and refreshed font cache."
  if [ "$had_files" -eq 1 ]; then
    echo "NOTE: replaced the MesloLGS Nerd Font files on disk (v${have:-unknown} -> v${want})."
    echo "      Terminals that are already running keep the OLD files mapped and"
    echo "      will draw powerline/icon glyphs from a fallback font until restarted."
    echo "      Close EVERY window of the terminal emulator (gnome-terminal runs one"
    echo "      server process for all windows) and open it again. A herdr session"
    echo "      survives that: run 'herdr' to reattach."
  fi
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
# NERD_FONT_PROC_VERSION is a test seam for the WSL check (default /proc/version).
if grep -qi microsoft "${NERD_FONT_PROC_VERSION:-/proc/version}" 2>/dev/null; then
  SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd -P)"
  WIN_INSTALLER_LINUX="${SCRIPT_DIR}/install_nerd_font_windows.ps1"

  # find_powershell comes from the dotfiles repo's shared helper, found from
  # this script's own location (sdk/gsl/scripts -> repo root). It checks PATH,
  # then the System32 copy via wslpath: ssh sessions (fleet runs) have no /mnt/c
  # dirs on PATH, but powershell.exe is still there and works.
  _wps_lib="${SCRIPT_DIR}/../../../opt/lib/winpowershell.sh"
  if [ -f "$_wps_lib" ]; then
    # shellcheck source=opt/lib/winpowershell.sh
    . "$_wps_lib"
  else
    # gsl scripts copied out of the repo: PATH lookup only (the old behavior).
    find_powershell() { command -v powershell.exe; }  # helper-absent fallback
  fi

  if [ ! -f "$WIN_INSTALLER_LINUX" ]; then
    echo "WARNING: Windows installer not found at ${WIN_INSTALLER_LINUX}; skipping Windows host install."
  elif ! ps_exe="$(find_powershell)"; then
    echo "WARNING: powershell.exe not found (PATH or System32); skipping Windows host install."
  else
    WIN_INSTALLER_WIN="$(wslpath -w "$WIN_INSTALLER_LINUX")"
    echo "Installing MesloLGS NF on Windows host via PowerShell..."
    # </dev/null: powershell.exe drains the parent's stdin under WSL interop.
    "$ps_exe" -ExecutionPolicy Bypass -NonInteractive -WindowStyle Hidden \
      -File "$WIN_INSTALLER_WIN" </dev/null
    echo "Windows host font install complete. Restart Windows Terminal to apply."
  fi
fi
