#!/usr/bin/env bash
# Test driver for macos-keys-linux.sh — the keyd half of the macOS key layout.
#
# Drives stubbed sudo/systemctl/keyd/python3/... on PATH so the assertions cover the
# real decision logic without compiling keyd, touching /etc, or grabbing the
# keyboard. KEYD_ETC redirects the system config into a temp dir.
#
# The cases that matter most are the FAIL-CLOSED ones: a machine where keyd runs
# but the application mapper does not is worse than a stock machine, because
# Cmd+C in a terminal becomes SIGINT instead of copy.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
# shellcheck source=/dev/null
. "$REPO_ROOT/ai/_test_helpers.sh"

SUT="$SCRIPT_DIR/macos-keys-linux.sh"
CONF_DIR="$REPO_ROOT/opt/etc/keyd"

H="$(mktemp -d)"
trap 'rm -rf "$H"' EXIT
mkdir -p "$H/bin" "$H/etc" "$H/home"

# --- stubs -------------------------------------------------------------------
# sudo just runs the command; every path it touches is redirected into $H.
cat > "$H/bin/sudo" <<'STUB'
#!/usr/bin/env bash
while [ "${1#-}" != "$1" ] || case "$1" in *=*) true ;; *) false ;; esac; do shift; done
exec "$@"
STUB

cat > "$H/bin/systemctl" <<'STUB'
#!/usr/bin/env bash
echo "systemctl $*" >> "$STUB_LOG"
[ "$1" = "is-active" ] && { echo "${FAKE_SVC_STATE:-active}"; exit 0; }
exit 0
STUB

cat > "$H/bin/keyd" <<'STUB'
#!/usr/bin/env bash
[ "$1" = "--version" ] && { echo "keyd ${FAKE_KEYD_VERSION:-v2.6.0} (test)"; exit 0; }
exit 0
STUB

# The Xlib import check and the X11 focused-window probe both run through python3.
cat > "$H/bin/python3" <<'STUB'
#!/usr/bin/env bash
echo "python3 $*" >> "$STUB_LOG"
[ "${FAKE_XLIB_OK:-1}" = "1" ] || exit 1
exit 0
STUB

cat > "$H/bin/keyd-application-mapper" <<'STUB'
#!/usr/bin/env bash
echo "mapper started" >> "$STUB_LOG"
[ "${FAKE_MAPPER_SOCKET_OK:-1}" = "1" ] ||
  echo 'ERROR: Failed to connect to "/var/run/keyd.socket"' >> "$HOME/.config/keyd/app.log"
exit 0
STUB

cat > "$H/bin/pgrep" <<'STUB'
#!/usr/bin/env bash
[ "${FAKE_MAPPER_RUNNING:-1}" = "1" ] && { echo 4242; exit 0; }
exit 1
STUB

cat > "$H/bin/getent" <<'STUB'
#!/usr/bin/env bash
[ "$2" = "keyd" ] && { echo "keyd:x:1001:${USER}"; exit 0; }
exit 2
STUB

cat > "$H/bin/sg" <<'STUB'
#!/usr/bin/env bash
# sg keyd -c "<cmd>" — run the command string.
shift 2; exec /usr/bin/env bash -c "$1"
STUB

cat > "$H/bin/usermod" <<'STUB'
#!/usr/bin/env bash
echo "usermod $*" >> "$STUB_LOG"; exit 0
STUB

# Unstubbed, this finds the BUILD MACHINE's real desktop session and the
# no-graphical-session guard never fires.
cat > "$H/bin/loginctl" <<'STUB'
#!/usr/bin/env bash
[ "${FAKE_LOGINCTL_GUI:-0}" = "1" ] || exit 0
case "$1" in
  list-sessions) echo "  7 1000 tester seat0 tty2" ;;
  show-session)  case "$*" in *Type*) echo x11 ;; *User*) id -u ;; *) echo "" ;; esac ;;
esac
exit 0
STUB

cat > "$H/bin/gnome-extensions" <<'STUB'
#!/usr/bin/env bash
[ "${FAKE_GNOME_EXT_OK:-0}" = "1" ] || exit 1
[ "$1" = "list" ] && echo keyd
exit 0
STUB

chmod +x "$H"/bin/*

run_sut() { # run_sut [VAR=val ...]
  : > "$H/stublog"
  rm -rf "$H/etc/keyd" "$H/etc/systemd" "$H/home/.config"
  if [ "${PRESEED_BROKEN_LOG:-0}" = "1" ]; then
    mkdir -p "$H/home/.config/keyd"
    echo 'ERROR: Failed to connect to "/var/run/keyd.socket"' > "$H/home/.config/keyd/app.log"
  fi
  env PATH="$H/bin:$PATH" HOME="$H/home" KEYD_ETC="$H/etc/keyd" \
      STUB_LOG="$H/stublog" XDG_SESSION_TYPE=x11 DISPLAY=:99 \
      KEYD_DROPIN="$H/etc/systemd/keyd.service.d/10-dotfiles-restart.conf" \
      "$@" bash "$SUT"
}

# --- guards: must no-op where there is no desktop ------------------------------
out="$(run_sut XDG_SESSION_TYPE=tty PATH="$H/bin:$PATH" 2>&1)"
assert_eq "$?" "0" "no graphical session: exits clean"
case "$out" in *"No graphical session"*) r=0 ;; *) r=1 ;; esac
assert_eq "$r" "0" "no graphical session: reports the skip"
assert_eq "$([ -f "$H/etc/keyd/default.conf" ] && echo yes || echo no)" "no" \
    "no graphical session: installs no keyd config"

# --- FAIL-CLOSED: no window detection => refuse to remap ------------------------
# Without python3-xlib the mapper cannot tell a terminal from a browser, so the
# script must NOT install a config that turns Cmd+C into Ctrl+C.
out="$(run_sut FAKE_XLIB_OK=0 2>&1)"
assert_eq "$?" "0" "no python3-xlib: exits clean"
assert_eq "$([ -f "$H/etc/keyd/default.conf" ] && echo yes || echo no)" "no" \
    "no python3-xlib: refuses to install the keyd config"

# A Wayland session with no keyd GNOME extension is the same hazard.
out="$(run_sut XDG_SESSION_TYPE=wayland FAKE_GNOME_EXT_OK=0 2>&1)"
assert_eq "$([ -f "$H/etc/keyd/default.conf" ] && echo yes || echo no)" "no" \
    "wayland without the GNOME extension: refuses to install"
case "$out" in *"SIGINT"*) r=0 ;; *) r=1 ;; esac
assert_eq "$r" "0" "refusal explains the SIGINT hazard"

# --- ROLLBACK: mapper starts but cannot reach the keyd socket -------------------
# A live process is not proof of success; it daemonizes and only then fails to
# open the socket. Leaving that state configured is the worst outcome.
out="$(PRESEED_BROKEN_LOG=1 run_sut FAKE_MAPPER_RUNNING=1 2>&1)"
assert_eq "$([ -f "$H/etc/keyd/default.conf" ] && echo yes || echo no)" "no" \
    "already-running mapper that never reached the socket: config rolled back"
case "$out" in *"rolling back"*) r=0 ;; *) r=1 ;; esac
assert_eq "$r" "0" "rollback is announced"

# --- happy path ----------------------------------------------------------------
out="$(run_sut 2>&1)"
assert_eq "$?" "0" "happy path: exits clean"
assert_eq "$([ -f "$H/etc/keyd/default.conf" ] && echo yes || echo no)" "yes" \
    "happy path: installs /etc/keyd/default.conf"
assert_eq "$([ -f "$H/home/.config/keyd/app.conf" ] && echo yes || echo no)" "yes" \
    "happy path: installs ~/.config/keyd/app.conf"
assert_eq "$([ -f "$H/home/.config/systemd/user/keyd-application-mapper.service" ] && echo yes || echo no)" "yes" \
    "happy path: installs the supervised mapper user unit"
assert_grep "happy path: enables the keyd service" "systemctl enable keyd" "$H/stublog"

# keyd ships no Restart= policy and it CAN segfault, which would silently drop the
# whole layout until someone restarts it by hand.
DROPIN="$H/etc/systemd/keyd.service.d/10-dotfiles-restart.conf"
assert_grep "happy path: keyd restarts itself after a crash" "Restart=on-failure" "$DROPIN"
assert_grep "happy path: a crash LOOP gives up instead of thrashing" \
    "StartLimitBurst=5" "$DROPIN"

# On X11 the mapper must use the dependency-free Xlib backend, which means hiding
# GNOME from its backend probe — otherwise it picks the GNOME backend and needs a
# shell extension plus a session restart.
UNIT_F="$H/home/.config/systemd/user/keyd-application-mapper.service"
assert_grep "x11 unit forces the Xlib backend" "XDG_CURRENT_DESKTOP=" "$UNIT_F"
# systemd supervises the process, so it must NOT daemonize itself.
assert_grep_negative "unit does not run the mapper with -d" "keyd-application-mapper -d" "$UNIT_F"
assert_grep "unit restarts the mapper if it dies" "Restart=always" "$UNIT_F"
# A user unit inherits the login session's groups, which predate the usermod, so
# without sg it fails with EACCES on keyd's socket for the rest of the session.
assert_grep "unit wraps ExecStart in sg to get the keyd group" \
    "ExecStart=.*sg keyd -c" "$UNIT_F"

# --- config content: regressions that would break the keyboard -----------------
# keyd's parser rejects a trailing comment on a binding line ("invalid key or
# action") and silently drops that binding, so guard the whole file.
bad="$(grep -nE '^[^#[[:space:]][^#]*=[^#]*#' "$CONF_DIR/default.conf" || true)"
assert_eq "$bad" "" "default.conf: no trailing comments on binding lines"

assert_grep "default.conf: Super is the Command key" "leftmeta = layer\(cmd\)" "$CONF_DIR/default.conf"
# ":M", never ":C". A ":C" layer consumes the physical Super, so Cmd+Tab can only
# emit a discrete chord and GNOME's switcher will not hold open to cycle.
assert_grep "default.conf: the cmd layer keeps Meta held" "\[cmd:M\]" "$CONF_DIR/default.conf"
assert_grep_negative "default.conf: does not use the hold-breaking :C layer" \
    "\[cmd:C\]" "$CONF_DIR/default.conf"
assert_grep "default.conf: Cmd+C is an explicit Ctrl+C" "^c = C-c" "$CONF_DIR/default.conf"
# tab/space/m/h must stay UNMAPPED so they reach GNOME as real held Super chords.
for k in tab space m h l; do
  assert_grep_negative "default.conf: $k is left for GNOME (not remapped)" \
      "^$k *=" "$CONF_DIR/default.conf"
done
assert_grep "default.conf: Cmd+Left is start-of-line" "^left  = home" "$CONF_DIR/default.conf"

# Alt+Left/Right must stay the browser's Back/Forward -- word navigation is not
# worth taking them, and Ctrl+arrows already does it natively.
alt_arrows="$(sed -n '/^\[alt\]/,/^\[/p' "$CONF_DIR/default.conf" | grep -nE '^(left|right) *=' || true)"
assert_eq "$alt_arrows" "" "default.conf: [alt] layer leaves Alt+arrows to the browser"

# CRASH REGRESSION: emitting M-<key> from a layer that Meta itself activates makes
# keyd re-enter that layer recursively until it dies. Observed as a SIGSEGV with a
# 147MB memory peak on keyd v2.6.0. Nothing in `keyd check` catches it -- the config
# parses perfectly and only crashes once a key is actually pressed.
meta_emit="$(sed -n '/^\[cmd/,$p' "$CONF_DIR/default.conf" | grep -nE '^[a-z0-9]+ *= *M-' || true)"
assert_eq "$meta_emit" "" "default.conf: no Meta-emitting binding inside a Meta-triggered layer"

# The single most important line in the feature: Cmd+C inside a terminal must be
# copy, never SIGINT.
assert_grep "app.conf: gnome-terminal is covered" "\[gnome-terminal\*\]" "$CONF_DIR/app.conf"
assert_grep "app.conf: Cmd+C in a terminal copies instead of sending SIGINT" \
    "cmd.c = C-S-c" "$CONF_DIR/app.conf"
assert_grep "app.conf: Cmd+S cannot freeze the terminal with XOFF" \
    "cmd.s = noop" "$CONF_DIR/app.conf"

_test_report
