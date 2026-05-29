#!/usr/bin/env bash
# check-font-glyphs.sh — prove the installed MesloLGS NF covers every codepoint
# gsl's powerline style emits. Locates the font per-OS, builds + runs the
# glyphcheck Go helper. Citable from CI.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Locate a MesloLGS NF Regular face per platform.
case "$(uname -s)" in
  Darwin) font="$(/usr/bin/find "${HOME}/Library/Fonts" /Library/Fonts \
            -iname 'MesloLGS NF Regular.ttf' 2>/dev/null | head -1)" ;;
  *)      font="$(fc-list 2>/dev/null | grep -i 'MesloLGS NF Regular' \
            | head -1 | cut -d: -f1)" ;;
esac
if [ -z "${font:-}" ] || [ ! -f "$font" ]; then
  echo "FAIL: MesloLGS NF Regular not found. Run the OS font installer first."
  exit 1
fi

echo "== glyph-coverage check against: $font =="
( cd "$MODULE_DIR" && go run ./cmd/glyphcheck "$font" )
