#!/usr/bin/env bash
# Self-test for check-font-glyphs.sh and the glyphcheck binary.
# Network-free: skips coverage test if MesloLGS NF is not installed.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
pass=0
fail=0

assert_exit() {
  local want="$1"; shift
  local desc="$1"; shift
  "$@" >/dev/null 2>&1
  local got=$?
  if [ "$got" = "$want" ]; then
    echo "ok   - $desc (exit $got)"; pass=$((pass + 1))
  else
    echo "FAIL - $desc (want exit $want, got $got)"; fail=$((fail + 1))
  fi
}

# Build glyphcheck once for this test run.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
( cd "$MODULE_DIR" && go build -o "$tmp/glyphcheck" ./cmd/glyphcheck ) >/dev/null 2>&1 \
  || { echo "FAIL: could not build glyphcheck"; exit 1; }

# 1) Wrong arg count → exit 2
assert_exit 2 "no args → usage exit 2" "$tmp/glyphcheck"
assert_exit 2 "too many args → usage exit 2" "$tmp/glyphcheck" a b

# 2) Non-existent file → exit 2
assert_exit 2 "missing file → exit 2" "$tmp/glyphcheck" "/nonexistent/font.ttf"

# 3) A font without PUA glyphs → exit 1 (missing codepoints)
# Find any system font that does NOT have MesloLGS NF in its name.
stub_font=""
for candidate in \
    "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf" \
    "/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf" \
    "/System/Library/Fonts/Helvetica.ttc" \
    "/System/Library/Fonts/Arial.ttf"; do
  if [ -f "$candidate" ]; then
    stub_font="$candidate"
    break
  fi
done
if [ -n "$stub_font" ]; then
  assert_exit 1 "non-PUA font reports missing codepoints (exit 1)" "$tmp/glyphcheck" "$stub_font"
else
  echo "skip - no system font found for negative test"
fi

# 4) If MesloLGS Nerd Font is installed, check coverage (exit 0).
meslo_font=""
case "$(uname -s)" in
  Darwin) meslo_font="$(/usr/bin/find "${HOME}/Library/Fonts" /Library/Fonts \
            -iname 'MesloLGSNerdFont-Regular.ttf' 2>/dev/null | head -1)" ;;
  *)      raw="$(fc-list 2>/dev/null | grep -i 'MesloLGS Nerd Font' | grep -i 'Regular' | head -1 | cut -d: -f1)"; meslo_font="${raw%% }" ;;
esac
if [ -n "${meslo_font:-}" ] && [ -f "$meslo_font" ]; then
  assert_exit 0 "MesloLGS Nerd Font Regular covers all 17 codepoints (exit 0)" "$tmp/glyphcheck" "$meslo_font"
else
  echo "skip - MesloLGS Nerd Font not installed; run install_nerd_font_<os>.sh first"
fi

echo "----"
echo "check-font-glyphs_test: $pass passed, $fail failed"
[ "$fail" = "0" ]
