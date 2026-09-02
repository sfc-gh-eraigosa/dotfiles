#!/usr/bin/env bash
# Test driver for gnome-desktop-defaults.sh — the gsettings half of the macOS-style
# keyboard layout. Drives a stub `gsettings` on PATH so the assertions cover the
# real decision logic without touching the host's dconf.
#
# The gnome-terminal cases guard a regression that shipped between #247 and #252:
# an older version of the script bound gnome-terminal copy/paste to <Super>c/v.
# Once keyd took over Super (#252) those chords never reach the terminal, and the
# stock Ctrl+Shift+C/V accelerators keyd's app.conf emits were gone -- so Cmd+V
# printed ^V and Cmd+C was one accelerator away from SIGINT. The script now has
# to put those keys BACK to stock and never bind them to Super again.
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
WM="org.gnome.desktop.wm.keybindings"

# Stub gsettings: STATE is a "schema<TAB>key<TAB>value" table, SETLOG records
# every `set` and `reset` so we can assert on writes (and on their absence).
# `reset` restores the schema default from the DEFAULTS table, as the real one does.
cat > "$H/bin/gsettings" <<'STUB'
#!/usr/bin/env bash
replace_row() { # schema key value
  tmp="$(mktemp)"
  awk -F'\t' -v s="$1" -v k="$2" '!($1==s && $2==k)' "$GS_STATE" > "$tmp"
  printf '%s\t%s\t%s\n' "$1" "$2" "$3" >> "$tmp"
  mv "$tmp" "$GS_STATE"
}
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
    replace_row "$2" "$3" "$4"; exit 0 ;;
  reset)
    printf 'reset %s %s\n' "$2" "$3" >> "$GS_SETLOG"
    def="$(awk -F'\t' -v s="$2" -v k="$3" '$1==s && $2==k {print $3}' "$GS_DEFAULTS")"
    [ -n "$def" ] || exit 1
    replace_row "$2" "$3" "$def"; exit 0 ;;
esac
exit 1
STUB
chmod +x "$H/bin/gsettings"

# gnome-terminal 3.52 schema defaults, what `gsettings reset` restores.
GT_COPY_STOCK="'<Control><Shift>c'"
GT_PASTE_STOCK="'<Control><Shift>v'"
{
  printf '%s\tcopy\t%s\n'  "$GT" "$GT_COPY_STOCK"
  printf '%s\tpaste\t%s\n' "$GT" "$GT_PASTE_STOCK"
} > "$H/defaults"

# reset_state TRAY with-gt|no-gt [COPY PASTE]
# COPY/PASTE default to the STALE <Super> binds an older script version wrote,
# because that is the state every already-provisioned machine is in.
reset_state() {
  : > "$H/setlog"
  {
    printf 'org.gnome.shell.keybindings\ttoggle-message-tray\t%s\n' "$1"
    printf '%s\tcopy\t%s\n'  "$GT" "${3:-"'<Super>c'"}"
    printf '%s\tpaste\t%s\n' "$GT" "${4:-"'<Super>v'"}"
    # GNOME 46 stock values for the desktop-action keys.
    printf 'org.gnome.mutter\toverlay-key\t%s\n' "'Super_L'"
    printf '%s\tminimize\t%s\n' "$WM" "['<Super>h']"
    printf '%s\tpanel-main-menu\t%s\n' "$WM" "['<Alt>F1']"
    printf '%s\tswitch-input-source\t%s\n' "$WM" "['<Super>space', 'XF86Keyboard']"
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
      GS_DEFAULTS="$H/defaults" \
      XDG_CURRENT_DESKTOP="ubuntu:GNOME" \
      DBUS_SESSION_BUS_ADDRESS="unix:path=$H/run/bus" \
      XDG_RUNTIME_DIR="$H/run" \
      "$@" bash "$DEFAULTS"
}

# --- guards: must no-op off a real session -----------------------------------
reset_state "['<Super>v', '<Super>m']" with-gt
out="$(env PATH="$H/bin:$PATH" GS_STATE="$H/state" GS_SETLOG="$H/setlog" \
       GS_SCHEMAS="$H/schemas" GS_RELOC="$H/reloc" GS_DEFAULTS="$H/defaults" \
       XDG_CURRENT_DESKTOP="ubuntu:GNOME" \
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
# keyd's app.conf turns Cmd+C/V into Ctrl+Shift+C/V inside gnome-terminal, so the
# terminal must be on its STOCK accelerators. The stale <Super> binds are undone
# with `reset`, never overwritten with a hardcoded chord.
assert_grep "resets a stale Super+C copy bind to stock"  "reset $GT copy"  "$H/setlog"
assert_grep "resets a stale Super+V paste bind to stock" "reset $GT paste" "$H/setlog"
assert_eq "$(awk -F'\t' -v s="$GT" '$1==s && $2=="paste" {print $3}' "$H/state")" \
    "$GT_PASTE_STOCK" "paste ends up on Ctrl+Shift+V"
assert_grep_negative "never binds gnome-terminal copy/paste to Super" \
    "<Super>[cv]'" "$H/setlog"
assert_grep "frees Super+V from the message tray" \
    "toggle-message-tray \['<Super>n'\]" "$H/setlog"

# --- the macOS desktop actions keyd hands back to GNOME -------------------------
assert_grep "Cmd+Space opens the Activities search" \
    "panel-main-menu \['<Super>space'\]" "$H/setlog"
assert_grep "frees Super+Space from the input-source switcher" \
    "switch-input-source \['XF86Keyboard'\]" "$H/setlog"
assert_grep "Cmd+M and Cmd+H both minimize" \
    "minimize \['<Super>m', '<Super>h'\]" "$H/setlog"
assert_grep "a lone Cmd tap does nothing (overlay-key cleared)" \
    "overlay-key ''" "$H/setlog"
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
assert_grep_negative "no gnome-terminal schema: touches no copy key" \
    "copy" "$H/setlog"
assert_grep "no gnome-terminal schema: still frees the tray key" \
    "toggle-message-tray" "$H/setlog"

# --- gnome-terminal already on stock or user-customized copy/paste ----------------
# A fresh machine (stock Ctrl+Shift+C/V) needs nothing written.
reset_state "['<Super>n']" with-gt "$GT_COPY_STOCK" "$GT_PASTE_STOCK"
run_gnome > /dev/null 2>&1
assert_grep_negative "stock terminal copy/paste: not touched" \
    "$GT" "$H/setlog"

# A user's own chord is not the stale value this script wrote, so it survives.
reset_state "['<Super>n']" with-gt "'<Alt>c'" "'<Alt>v'"
run_gnome > /dev/null 2>&1
assert_grep_negative "user-customized terminal copy/paste: not touched" \
    "$GT" "$H/setlog"

# --- respects a user who already customized the tray ----------------------------
# <Super>n holds neither of the keys the macOS layout needs (V for paste, M for
# minimize), so a deliberate customization must survive untouched.
reset_state "['<Super>n']" with-gt
run_gnome > /dev/null 2>&1
assert_grep_negative "tray already free of Super+V and Super+M: not rewritten" \
    "toggle-message-tray" "$H/setlog"

# A tray still on <Super>m DOES have to move, or Cmd+M would open the tray
# instead of minimizing the window.
reset_state "['<Super>m']" with-gt
run_gnome > /dev/null 2>&1
assert_grep "tray on Super+M is moved aside for Cmd+M minimize" \
    "toggle-message-tray \['<Super>n'\]" "$H/setlog"

_test_report
