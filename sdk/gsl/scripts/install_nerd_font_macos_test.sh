#!/usr/bin/env bash
# Self-test for install_nerd_font_macos.sh. Drives the REAL script with a
# stubbed toolchain (brew/python3/osascript no-ops, curl logs its calls, unzip
# writes fake font faces carrying a "Nerd Fonts X.Y.Z" version string like the
# real ones, system_profiler reports the family on demand) under a temp HOME,
# so the install decision is covered without a Mac, the network, or iTerm2.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALLER="$SCRIPT_DIR/install_nerd_font_macos.sh"
pass=0
fail=0

ok()   { echo "ok   - $1"; pass=$((pass + 1)); }
notok(){ echo "FAIL - $1"; fail=$((fail + 1)); }

assert_contains() { # desc file needle
  if grep -qF -- "$3" "$2"; then ok "$1"; else notok "$1 (missing: $3)"; fi
}
assert_absent() { # desc file needle
  if grep -qF -- "$3" "$2"; then notok "$1 (unexpectedly present: $3)"; else ok "$1"; fi
}

H="$(mktemp -d)"
trap 'rm -rf "$H"' EXIT
mkdir -p "$H/bin"
FONT_DIR="$H/Library/Fonts"
FACES="MesloLGSNerdFont-Regular.ttf MesloLGSNerdFont-Bold.ttf MesloLGSNerdFont-Italic.ttf MesloLGSNerdFont-BoldItalic.ttf"

for t in brew python3 osascript; do printf '#!/bin/sh\nexit 0\n' > "$H/bin/$t"; done
cat > "$H/bin/curl" <<'STUB'
#!/bin/sh
printf '%s\n' "$*" >> "$CURL_LOG"
exit 0
STUB
cat > "$H/bin/unzip" <<'STUB'
#!/bin/sh
dest=.
while [ $# -gt 0 ]; do
  if [ "$1" = "-d" ]; then dest="$2"; shift; fi
  shift
done
mkdir -p "$dest"
for face in MesloLGSNerdFont-Regular MesloLGSNerdFont-Bold MesloLGSNerdFont-Italic MesloLGSNerdFont-BoldItalic; do
  printf 'OTTO\000\001binary\000Version 1.210;Nerd Fonts %s\000MesloLGS NF\n' "$FAKE_NF_VERSION" > "$dest/$face.ttf"
done
exit 0
STUB
# system_profiler lists the family only when SP_SEES_FONT=1 (default 0): a font
# installed somewhere other than ~/Library/Fonts (e.g. a brew cask).
cat > "$H/bin/system_profiler" <<'STUB'
#!/bin/sh
[ "${SP_SEES_FONT:-0}" = "1" ] || exit 0
echo "    MesloLGS Nerd Font:"
STUB
chmod +x "$H"/bin/*

seed_font() { # $1 = installed Nerd Fonts version, or "none" for no files on disk
  rm -rf "$FONT_DIR"; : > "$H/curllog"
  [ "$1" = "none" ] && return 0
  mkdir -p "$FONT_DIR"
  for face in $FACES; do
    printf 'OTTO\000\001binary\000Version 1.210;Nerd Fonts %s\000MesloLGS NF\n' "$1" > "$FONT_DIR/$face"
  done
}

installed_version() {
  LC_ALL=C tr -c '[:print:]' '\n' < "$FONT_DIR/MesloLGSNerdFont-Regular.ttf" 2>/dev/null |
    grep -o 'Nerd Fonts [0-9][0-9.]*' | head -1
}

run() { # extra env assignments as args
  env PATH="$H/bin:/usr/bin:/bin" HOME="$H" \
      NERD_FONTS_VERSION=3.5.1 FAKE_NF_VERSION=3.5.1 CURL_LOG="$H/curllog" \
      "$@" bash "$INSTALLER" > "$H/out" 2>&1
}

# --- pinned version already in ~/Library/Fonts: skip, name the version -------
seed_font 3.5.1
if run; then ok "installer exits clean"; else notok "installer exits clean"; fi
assert_contains "pinned version on disk: skips the download" "$H/out" "already installed"
assert_contains "pinned version on disk: names the version it found" "$H/out" "3.5.1"
if [ -s "$H/curllog" ]; then notok "pinned version on disk: curl never runs"
else ok "pinned version on disk: curl never runs"; fi
assert_absent "pinned version on disk: no replaced-files notice" "$H/out" "replaced"
assert_contains "still writes the iTerm2 dynamic profile" "$H/out" "Wrote iTerm2 Dynamic Profile"

# --- pin bumped past what is installed: replace the faces AND explain --------
seed_font 3.4.0
run
assert_contains "pin bump: downloads the new release" "$H/curllog" "v3.5.1/Meslo.zip"
if [ "$(installed_version)" = "Nerd Fonts 3.5.1" ]; then ok "pin bump: faces on disk are now 3.5.1"
else notok "pin bump: faces on disk are now 3.5.1 (got: $(installed_version))"; fi
assert_contains "pin bump: says the files were replaced" "$H/out" "replaced"
assert_contains "pin bump: says what changed (old version)" "$H/out" "3.4.0"
assert_contains "pin bump: tells the user to restart the terminal" "$H/out" "estart"

# --- family installed elsewhere (not ~/Library/Fonts): leave it alone --------
seed_font none
run SP_SEES_FONT=1
assert_contains "installed elsewhere: skips" "$H/out" "skipping download"
if [ -s "$H/curllog" ]; then notok "installed elsewhere: curl never runs"
else ok "installed elsewhere: curl never runs"; fi

# --- nothing anywhere: fresh install, no replaced-files notice ---------------
seed_font none
run
assert_contains "fresh install: downloads" "$H/curllog" "v3.5.1/Meslo.zip"
assert_contains "fresh install: reports the install" "$H/out" "Installed MesloLGS Nerd Font"
assert_absent "fresh install: no replaced-files notice" "$H/out" "replaced"

echo "----"
echo "install_nerd_font_macos_test: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
