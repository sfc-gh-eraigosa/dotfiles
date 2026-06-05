#!/usr/bin/env bash
# install_nerd_font_macos.sh — gsl-packaged macOS Nerd Font installer.
# 1. Preflight base requirements (brew, python3, curl, unzip).
# 2. Install the pinned MesloLGS NF faces into ~/Library/Fonts.
# 3. Write an iTerm2 Dynamic Profile (loads live, survives quit) and
#    best-effort Terminal.app config.
# Safe to re-run (idempotent).
set -euo pipefail

NERD_FONTS_VERSION="v3.4.0"
NERD_FONTS_ASSET="Meslo.zip"
NERD_FONTS_URL_BASE="https://github.com/ryanoasis/nerd-fonts/releases/download/${NERD_FONTS_VERSION}"
FONT_FAMILY="MesloLGS Nerd Font"
FONT_SIZE=13

# ── Preflight ────────────────────────────────────────────────────────────────
missing=0
for tool in brew python3 curl unzip; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "ERROR: required tool '$tool' not found."
    case "$tool" in
      brew) echo "  Install Homebrew from https://brew.sh/ then re-run." ;;
      *)    echo "  Install '$tool' (e.g. 'brew install $tool') then re-run." ;;
    esac
    missing=1
  fi
done
[ "$missing" -eq 0 ] || exit 1

# ── Install font faces from the pinned release ───────────────────────────────
font_dir="${HOME}/Library/Fonts"
mkdir -p "$font_dir"
if system_profiler SPFontsDataType 2>/dev/null | grep -qi "MesloLGS Nerd Font"; then
  echo "MesloLGS Nerd Font already present in system fonts; skipping download."
else
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  echo "Downloading ${NERD_FONTS_ASSET} (${NERD_FONTS_VERSION})..."
  curl -fsSL -o "${tmp}/${NERD_FONTS_ASSET}" "${NERD_FONTS_URL_BASE}/${NERD_FONTS_ASSET}"
  unzip -o -q "${tmp}/${NERD_FONTS_ASSET}" 'MesloLGSNerdFont-*.ttf' -d "$tmp"
  cp "${tmp}"/MesloLGSNerdFont-*.ttf "$font_dir"/
  echo "Installed MesloLGS Nerd Font faces into ${font_dir}."
fi

# ── iTerm2 Dynamic Profile (loads live; NOT overwritten on quit) ─────────────
# Replaces the old com.googlecode.iterm2.plist patching that lost writes when
# iTerm2 was running. DynamicProfiles are read live by a running iTerm2.
dyn_dir="${HOME}/Library/Application Support/iTerm2/DynamicProfiles"
mkdir -p "$dyn_dir"
cat > "${dyn_dir}/gsl-nerd-font.json" <<JSON
{
  "Profiles": [
    {
      "Name": "gsl Nerd Font",
      "Guid": "gsl-nerd-font",
      "Normal Font": "${FONT_FAMILY} ${FONT_SIZE}",
      "Non Ascii Font": "${FONT_FAMILY} ${FONT_SIZE}",
      "Use Non-ASCII Font": true
    }
  ]
}
JSON
echo "Wrote iTerm2 Dynamic Profile: ${dyn_dir}/gsl-nerd-font.json"
echo "  In iTerm2: Preferences → Profiles → select 'gsl Nerd Font' (or set it as Default)."

# ── Terminal.app (best effort; weaker PUA fallback than iTerm2) ──────────────
# Terminal.app renders fewer Nerd Font PUA glyphs than iTerm2 (no broad PUA
# fallback). iTerm2 is the recommended terminal for full gsl rendering.
osascript <<OSASCRIPT 2>/dev/null && echo "Terminal.app default font set." || \
  echo "NOTE: Terminal.app font update skipped (Terminal not scriptable here)."
tell application "Terminal"
  set font name of settings set "Basic" to "${FONT_FAMILY}"
  set font size of settings set "Basic" to ${FONT_SIZE}
end tell
OSASCRIPT

echo ""
echo "Done. Restart iTerm2 (or open a new window/tab) to apply the new font."
echo "Verify: system_profiler SPFontsDataType | grep -i meslo"
