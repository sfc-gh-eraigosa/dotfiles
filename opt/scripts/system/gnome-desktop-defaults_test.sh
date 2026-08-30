#!/usr/bin/env bash
# Test driver for gnome-desktop-defaults.sh — macOS-style Super+C/Super+V binds
# for gnome-terminal. Drives a stub `gsettings` on PATH so the assertions cover
# the real decision logic without touching the host's dconf.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
# shellcheck source=/dev/null
. "$REPO_ROOT/ai/_test_helpers.sh"

DEFAULTS="$SCRIPT_DIR/gnome-desktop-defaults.sh"

H="$(mktemp -d)"
trap 'rm -rf "$H"' EXIT
mkdir -p "$H/bin" "$H/run"

GT="org.gnome.Terminal.Legacy.Keybindings:/org/gnome/terminal/legacy/keybindings/"

# Stub gsettings: STATE is a "schema<TAB>key<TAB>value" table, SETLOG records
# every `set` so we can assert on writes (and on their absence).
cat > "$H/bin/gsettings" <<'STUB'
#!/usr/bin/env bash
case "$1" in
  # Real gsettings splits these: list-schemas omits relocatable schemas, which
  # is exactly the trap gnome-desktop-defaults.sh has to avoid.
  list-schemas) cat "$GS_SCHEMAS"; exit 0 ;;
  list-relocatable-schemas) cat "$GS_RELOC"; exit 0 ;;
  get)
    line="$(awk -F'\t' -v s="$2" -v k="$3" '$1==s && $2==k {print $3}' "$GS_STATE")"
    [ -n "$line" ] || exit 1
    printf '%s\n' "$line"; exit 0 ;;
  set)
    printf '%s %s %s\n' "$2" "$3" "$4" >> "$GS_SETLOG"
    tmp="$(mktemp)"
    awk -F'\t' -v s="$2" -v k="$3" '!($1==s && $2==k)' "$GS_STATE" > "$tmp"
    printf '%s\t%s\t%s\n' "$2" "$3" "$4" >> "$tmp"
    mv "$tmp" "$GS_STATE"; exit 0 ;;
esac
exit 1
STUB
chmod +x "$H/bin/gsettings"

reset_state() { # $1 = tray value, $2 = "with-gt" | "no-gt"
  : > "$H/setlog"
  {
    printf 'org.gnome.shell.keybindings\ttoggle-message-tray\t%s\n' "$1"
    printf '%s\tcopy\t%s\n'  "$GT" "'<Control><Shift>c'"
    printf '%s\tpaste\t%s\n' "$GT" "'<Control><Shift>v'"
  } > "$H/state"
  printf 'org.gnome.shell.keybindings\n' > "$H/schemas"
  if [ "$2" = "with-gt" ]; then
    printf 'org.gnome.Terminal.Legacy.Keybindings\norg.gnome.Terminal.Legacy.Profile\n' > "$H/reloc"
  else
    : > "$H/reloc"
  fi
}

# run_gnome [extra env assignments...] — a fully-formed writable GNOME session
run_gnome() {
  env PATH="$H/bin:$PATH" \
      GS_STATE="$H/state" GS_SETLOG="$H/setlog" GS_SCHEMAS="$H/schemas" GS_RELOC="$H/reloc" \
      XDG_CURRENT_DESKTOP="ubuntu:GNOME" \
      DBUS_SESSION_BUS_ADDRESS="unix:path=$H/run/bus" \
      XDG_RUNTIME_DIR="$H/run" \
      "$@" bash "$DEFAULTS"
}

# --- guards: must no-op off a real session -----------------------------------
reset_state "['<Super>v', '<Super>m']" with-gt
out="$(env PATH="$H/bin:$PATH" GS_STATE="$H/state" GS_SETLOG="$H/setlog" \
       GS_SCHEMAS="$H/schemas" GS_RELOC="$H/reloc" XDG_CURRENT_DESKTOP="ubuntu:GNOME" \
       XDG_RUNTIME_DIR="$H/run" DBUS_SESSION_BUS_ADDRESS= \
       bash "$DEFAULTS" 2>&1)"
assert_eq "$?" "0" "no session bus: exits clean"
case "$out" in *"skipping GNOME desktop defaults"*) r=0 ;; *) r=1 ;; esac
assert_eq "$r" "0" "no session bus: reports the skip"
assert_eq "$(wc -l < "$H/setlog" | tr -d ' ')" "0" "no session bus: writes nothing"

reset_state "['<Super>v', '<Super>m']" with-gt
run_gnome XDG_CURRENT_DESKTOP="KDE" > /dev/null 2>&1
assert_eq "$(wc -l < "$H/setlog" | tr -d ' ')" "0" "non-GNOME desktop: writes nothing"

reset_state "['<Super>v', '<Super>m']" with-gt
run_gnome XDG_RUNTIME_DIR="$H/run/does-not-exist" > /dev/null 2>&1
assert_eq "$(wc -l < "$H/setlog" | tr -d ' ')" "0" "unwritable runtime dir: writes nothing"

# --- happy path ---------------------------------------------------------------
reset_state "['<Super>v', '<Super>m']" with-gt
assert_exit_code 0 "applies cleanly on a GNOME session" run_gnome
assert_grep "binds copy to Super+C"  "copy '<Super>c'"  "$H/setlog"
assert_grep "binds paste to Super+V" "paste '<Super>v'" "$H/setlog"
assert_grep "frees Super+V from the message tray" \
    "toggle-message-tray \['<Super>m'\]" "$H/setlog"
assert_grep_negative "leaves Ctrl+C alone (SIGINT preserved)" \
    "<Control>c" "$H/setlog"

# --- idempotency ---------------------------------------------------------------
: > "$H/setlog"
run_gnome > /dev/null 2>&1
assert_eq "$(wc -l < "$H/setlog" | tr -d ' ')" "0" "second run is a no-op"

# --- gnome-terminal absent ------------------------------------------------------
reset_state "['<Super>v', '<Super>m']" no-gt
out="$(run_gnome 2>&1)"
assert_exit_code 0 "no gnome-terminal schema: still exits clean" run_gnome
assert_grep_negative "no gnome-terminal schema: binds no copy key" \
    "copy" "$H/setlog"
assert_grep "no gnome-terminal schema: still frees the tray key" \
    "toggle-message-tray" "$H/setlog"

# --- respects a user who already customized the tray ----------------------------
reset_state "['<Super>m']" with-gt
run_gnome > /dev/null 2>&1
assert_grep_negative "tray already free of Super+V: not rewritten" \
    "toggle-message-tray" "$H/setlog"

_test_report
