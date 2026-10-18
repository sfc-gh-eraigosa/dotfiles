#!/usr/bin/env bash
# Self-test for install_nerd_font_linux.sh. Drives the REAL script with a
# stubbed toolchain (curl logs its calls, unzip writes fake font faces that
# carry a "Nerd Fonts X.Y.Z" version string like the real ones) and a stub
# gsettings backed by a state table, so the assertions cover the actual
# decision logic without touching the host's dconf, fonts, or the network.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALLER="$SCRIPT_DIR/install_nerd_font_linux.sh"
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
mkdir -p "$H/bin" "$H/run"
FONT_DIR="$H/.local/share/fonts/MesloLGS"
FACES="MesloLGSNerdFont-Regular.ttf MesloLGSNerdFont-Bold.ttf MesloLGSNerdFont-Italic.ttf MesloLGSNerdFont-BoldItalic.ttf"

# curl and fc-cache record every invocation; unzip "extracts" fake faces into
# the -d target, stamped with the version the download would have carried.
cat > "$H/bin/curl" <<'STUB'
#!/bin/sh
printf '%s\n' "$*" >> "$CURL_LOG"
exit 0
STUB
cat > "$H/bin/fc-cache" <<'STUB'
#!/bin/sh
printf '%s\n' "$*" >> "$FCCACHE_LOG"
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
# fc-list reports the family only when FC_LIST_SEES_FONT=1 (default), so a
# case can model the fontconfig miss seen in the 2026-10-16 fleet run.
cat > "$H/bin/fc-list" <<'STUB'
#!/bin/sh
[ "${FC_LIST_SEES_FONT:-1}" = "1" ] || exit 0
echo "/f/MesloLGSNerdFont-Regular.ttf: MesloLGS Nerd Font:style=Regular"
STUB

cat > "$H/bin/gsettings" <<'STUB'
#!/usr/bin/env bash
case "$1" in
  # Real gsettings splits these: list-schemas omits RELOCATABLE schemas, which
  # is the trap the installer has to avoid for the Profile schema.
  list-schemas)             printf 'org.gnome.desktop.interface\n'; exit 0 ;;
  list-relocatable-schemas) cat "$GS_RELOC"; exit 0 ;;
  get)
    v="$(awk -F'\t' -v s="$2" -v k="$3" '$1==s && $2==k {print $3}' "$GS_STATE")"
    [ -n "$v" ] || exit 1
    printf '%s\n' "$v"; exit 0 ;;
  set)
    printf '%s %s %s\n' "$2" "$3" "$4" >> "$GS_SETLOG"
    tmp="$(mktemp)"
    awk -F'\t' -v s="$2" -v k="$3" '!($1==s && $2==k)' "$GS_STATE" > "$tmp"
    printf '%s\t%s\t%s\n' "$2" "$3" "$4" >> "$tmp"
    mv "$tmp" "$GS_STATE"; exit 0 ;;
esac
exit 1
STUB
chmod +x "$H"/bin/*

PROF="b1dcc9dd-0000-0000-0000-000000000000"
PKEY="org.gnome.Terminal.Legacy.Profile:/org/gnome/terminal/legacy/profiles:/:${PROF}/"

seed() { # $1 = use-system-font, $2 = "with-profile"|"no-profile"
  : > "$H/setlog"; : > "$H/curllog"; : > "$H/fccachelog"
  {
    printf 'org.gnome.Terminal.ProfilesList\tdefault\t%s\n' "'${PROF}'"
    printf 'org.gnome.desktop.interface\tmonospace-font-name\t%s\n' "'Ubuntu Sans Mono 13'"
    printf '%s\tuse-system-font\t%s\n' "$PKEY" "$1"
    printf '%s\tfont\t%s\n' "$PKEY" "'Monospace 12'"
  } > "$H/state"
  if [ "$2" = "with-profile" ]; then
    printf 'org.gnome.Terminal.Legacy.Profile\norg.gnome.Terminal.Legacy.Keybindings\n' > "$H/reloc"
  else
    : > "$H/reloc"
  fi
}

seed_font() { # $1 = installed Nerd Fonts version, or "none" for no files on disk
  rm -rf "$FONT_DIR"
  [ "$1" = "none" ] && return 0
  mkdir -p "$FONT_DIR"
  for face in $FACES; do
    printf 'OTTO\000\001binary\000Version 1.210;Nerd Fonts %s\000MesloLGS NF\n' "$1" > "$FONT_DIR/$face"
  done
}

installed_version() { # the version string the faces on disk carry now
  LC_ALL=C tr -c '[:print:]' '\n' < "$FONT_DIR/MesloLGSNerdFont-Regular.ttf" 2>/dev/null |
    grep -o 'Nerd Fonts [0-9][0-9.]*' | head -1
}

# The WSL branch keys off /proc/version; point it at a fake so the gnome cases
# never reach the Windows host installer, even when the suite runs ON WSL.
printf 'Linux version 6.8.0-generic (buildd@ubuntu)\n' > "$H/proc_version_linux"
printf 'Linux version 6.6.87.2-microsoft-standard-WSL2\n' > "$H/proc_version_wsl"

run() { # extra env assignments as args
  env PATH="$H/bin:/usr/bin:/bin" HOME="$H" \
      NERD_FONTS_VERSION=3.5.1 FAKE_NF_VERSION=3.5.1 \
      NERD_FONT_PROC_VERSION="$H/proc_version_linux" \
      CURL_LOG="$H/curllog" FCCACHE_LOG="$H/fccachelog" \
      GS_STATE="$H/state" GS_SETLOG="$H/setlog" GS_RELOC="$H/reloc" \
      DBUS_SESSION_BUS_ADDRESS="unix:path=$H/run/bus" XDG_RUNTIME_DIR="$H/run" \
      "$@" bash "$INSTALLER" > "$H/out" 2>&1
}

# --- pinned version already on disk: skip, and say which version ------------
seed "true" with-profile
seed_font 3.5.1
if run; then ok "installer exits clean"; else notok "installer exits clean"; fi
assert_contains "pinned version on disk: skips the download" "$H/out" "already installed"
assert_contains "pinned version on disk: names the version it found" "$H/out" "3.5.1"
if [ -s "$H/curllog" ]; then notok "pinned version on disk: curl never runs"
else ok "pinned version on disk: curl never runs"; fi
assert_absent "pinned version on disk: no restart notice" "$H/out" "estart"
assert_contains "sets use-system-font false" "$H/setlog" "use-system-font false"
assert_contains "carries over the size already in effect (13)" \
  "$H/setlog" "font MesloLGS Nerd Font 13"

# --- pin bumped past what is installed: replace the files AND say to restart --
# A running terminal keeps the old inodes mapped and draws PUA glyphs from a
# fallback font until it is restarted (what hit spark on 2026-10-16).
seed "false" with-profile
seed_font 3.4.0
run
assert_contains "pin bump: downloads the new release" "$H/curllog" "v3.5.1/Meslo.zip"
if [ "$(installed_version)" = "Nerd Fonts 3.5.1" ]; then ok "pin bump: faces on disk are now 3.5.1"
else notok "pin bump: faces on disk are now 3.5.1 (got: $(installed_version))"; fi
assert_contains "pin bump: tells the user to restart the terminal" "$H/out" "restart"
assert_contains "pin bump: says what changed (old version)" "$H/out" "3.4.0"
assert_contains "pin bump: says what changed (new version)" "$H/out" "3.5.1"
assert_contains "pin bump: refreshes the font cache" "$H/fccachelog" "$FONT_DIR"

# --- fontconfig misses the family but the pinned files ARE on disk -----------
# The 2026-10-16 fleet run re-downloaded a font that was already installed
# because `fc-list` came back empty. Presence on disk decides, not fc-list.
seed "false" with-profile
seed_font 3.5.1
run FC_LIST_SEES_FONT=0
assert_contains "fc-list miss, files present: still skips" "$H/out" "already installed"
if [ -s "$H/curllog" ]; then notok "fc-list miss, files present: curl never runs"
else ok "fc-list miss, files present: curl never runs"; fi
assert_contains "fc-list miss, files present: re-registers the dir with fc-cache" \
  "$H/fccachelog" "$FONT_DIR"

# --- nothing on disk: fresh install, nothing was replaced, no restart nag -----
seed "false" with-profile
seed_font none
run
assert_contains "fresh install: downloads" "$H/curllog" "v3.5.1/Meslo.zip"
assert_contains "fresh install: reports the install" "$H/out" "Installed MesloLGS Nerd Font"
assert_absent "fresh install: no restart notice (nothing was replaced)" "$H/out" "estart"

# --- respects an explicit user font choice ------------------------------------
seed "false" with-profile
seed_font 3.5.1
run
assert_absent "leaves a custom font alone" "$H/setlog" "use-system-font"
assert_contains "says why it left the font alone" "$H/out" "already uses a custom font"

# --- guards --------------------------------------------------------------------
seed "true" no-profile
run
assert_absent "no gnome-terminal Profile schema: writes nothing" "$H/setlog" "use-system-font"

seed "true" with-profile
run DBUS_SESSION_BUS_ADDRESS=
assert_absent "no session bus: writes nothing" "$H/setlog" "use-system-font"

seed "true" with-profile
run XDG_RUNTIME_DIR="$H/run/nope"
assert_absent "unwritable runtime dir: writes nothing" "$H/setlog" "use-system-font"

# --- WSL over ssh: powershell.exe not on PATH, but present in System32 --------
# Fleet runs over ssh into WSL have no /mnt/c dirs on PATH, so the old bare
# `command -v powershell.exe` warned and skipped the Windows host install on
# every run. The shared find_powershell falls back to the System32 copy that
# wslpath locates. Stub wslpath maps into a fake Windows tree; the fake
# powershell.exe records its argv.
W="$H/win/System32/WindowsPowerShell/v1.0"
mkdir -p "$W"
cat > "$W/powershell.exe" <<'STUB'
#!/bin/sh
printf '%s\n' "$*" >> "$PS_LOG"
exit 0
STUB
cat > "$H/bin/wslpath" <<'STUB'
#!/bin/sh
case "$1" in
  -u) [ "$2" = 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe' ] || exit 1
      printf '%s\n' "$FAKE_WIN/System32/WindowsPowerShell/v1.0/powershell.exe" ;;
  -w) printf 'C:\\fake%s\n' "$2" | tr '/' '\\' ;;
  *) exit 1 ;;
esac
STUB
chmod +x "$W/powershell.exe" "$H/bin/wslpath"

seed "false" with-profile
: > "$H/pslog"
run NERD_FONT_PROC_VERSION="$H/proc_version_wsl" FAKE_WIN="$H/win" PS_LOG="$H/pslog"
assert_absent "WSL: no 'powershell.exe not in PATH' skip when System32 has it" \
  "$H/out" "not in PATH"
assert_contains "WSL: runs the Windows host installer" "$H/out" \
  "Installing MesloLGS NF on Windows host via PowerShell"
assert_contains "WSL: invokes the System32 powershell.exe with the .ps1" \
  "$H/pslog" "install_nerd_font_windows.ps1"

# ... and with neither a PATH copy nor a System32 copy it still skips cleanly.
seed "false" with-profile
: > "$H/pslog"
if run NERD_FONT_PROC_VERSION="$H/proc_version_wsl" FAKE_WIN="$H/nowhere" PS_LOG="$H/pslog"; then
  ok "WSL, no powershell.exe anywhere: installer still exits clean"
else
  notok "WSL, no powershell.exe anywhere: installer still exits clean"
fi
assert_contains "WSL, no powershell.exe anywhere: says it skipped" "$H/out" \
  "skipping Windows host install"
if [ -s "$H/pslog" ]; then notok "WSL, no powershell.exe anywhere: nothing invoked"
else ok "WSL, no powershell.exe anywhere: nothing invoked"; fi

# --- scripts copied out of the repo (no opt/lib helper): PATH lookup still works
SA="$H/standalone/scripts"
mkdir -p "$SA" "$H/pathps"
cp "$INSTALLER" "$SA/install_nerd_font_linux.sh"
: > "$SA/install_nerd_font_windows.ps1"
cp "$W/powershell.exe" "$H/pathps/powershell.exe"
seed "false" with-profile
: > "$H/pslog"
if env PATH="$H/pathps:$H/bin:/usr/bin:/bin" HOME="$H" \
     NERD_FONTS_VERSION=3.5.1 FAKE_NF_VERSION=3.5.1 \
     CURL_LOG="$H/curllog" FCCACHE_LOG="$H/fccachelog" \
     NERD_FONT_PROC_VERSION="$H/proc_version_wsl" FAKE_WIN="$H/nowhere" PS_LOG="$H/pslog" \
     GS_STATE="$H/state" GS_SETLOG="$H/setlog" GS_RELOC="$H/reloc" \
     bash "$SA/install_nerd_font_linux.sh" > "$H/out" 2>&1; then
  ok "no helper (standalone copy): installer exits clean"
else
  notok "no helper (standalone copy): installer exits clean"
fi
assert_contains "no helper (standalone copy): falls back to powershell.exe on PATH" \
  "$H/pslog" "install_nerd_font_windows.ps1"

echo "----"
echo "install_nerd_font_linux_test: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
