#!/usr/bin/env bash
# Self-test for install_nerd_font_linux.sh — specifically the gnome-terminal
# font wiring. Drives the REAL script with a stubbed toolchain (fc-list reports
# the font already present, so the download path is skipped) and a stub
# gsettings backed by a state table, so the assertions cover the actual
# decision logic without touching the host's dconf or the network.
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

for t in curl unzip fc-cache; do printf '#!/bin/sh\nexit 0\n' > "$H/bin/$t"; done
printf '#!/bin/sh\necho "/f/MesloLGSNerdFont-Regular.ttf: MesloLGS Nerd Font:style=Regular"\n' > "$H/bin/fc-list"

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
  : > "$H/setlog"
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

run() { # extra env assignments as args
  env PATH="$H/bin:/usr/bin:/bin" HOME="$H" \
      GS_STATE="$H/state" GS_SETLOG="$H/setlog" GS_RELOC="$H/reloc" \
      DBUS_SESSION_BUS_ADDRESS="unix:path=$H/run/bus" XDG_RUNTIME_DIR="$H/run" \
      "$@" bash "$INSTALLER" > "$H/out" 2>&1
}

# --- happy path: system font in use -> switch to the Nerd Font ----------------
seed "true" with-profile
if run; then ok "installer exits clean"; else notok "installer exits clean"; fi
assert_contains "skips the download when the font is already installed" \
  "$H/out" "already installed"
assert_contains "sets use-system-font false" "$H/setlog" "use-system-font false"
assert_contains "carries over the size already in effect (13)" \
  "$H/setlog" "font MesloLGS Nerd Font 13"

# --- respects an explicit user font choice ------------------------------------
seed "false" with-profile
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

echo "----"
echo "install_nerd_font_linux_test: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
