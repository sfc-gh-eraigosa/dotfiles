#!/bin/bash
# GNOME desktop defaults — the Linux counterpart to the macOS-style hotkeys that
# opt/Desktop/Apps/scripts/macos.ahk provides on Windows.
#
# WHY <Super> AND NOT <Control>: on macOS, Terminal.app copies with Cmd+C, and
# Super is the Linux analogue of Cmd — so <Super>c/<Super>v deliver the same
# muscle memory while leaving Ctrl+C as SIGINT. Binding Ctrl+C to copy would
# break interrupts outright: gnome-terminal consumes a matched accelerator
# unconditionally and has no copy-if-selection-else-pass-through mode (Ptyxis
# and kitty have one; VTE 0.76 / gnome-terminal 3.52 do not).
#
# Idempotent and safe to re-run by hand:
#   ~/opt/scripts/system/gnome-desktop-defaults.sh
# No-ops on anything that is not a live GNOME session (CI, docker, WSL, plain
# SSH, macOS), so install.sh stays safe headless.

set -o nounset

GT_KEYS="org.gnome.Terminal.Legacy.Keybindings:/org/gnome/terminal/legacy/keybindings/"

# A GNOME session we can actually write settings into. Without a session bus or
# a writable XDG_RUNTIME_DIR, dconf has nowhere to persist and `gsettings set`
# degrades to a silent no-op — better to skip loudly than to pretend it worked.
gnome_session_available() {
	command -v gsettings > /dev/null 2>&1 || return 1
	[ -n "${DBUS_SESSION_BUS_ADDRESS:-}" ] || return 1
	[ -n "${XDG_RUNTIME_DIR:-}" ] && [ -w "${XDG_RUNTIME_DIR}" ] || return 1
	case "${XDG_CURRENT_DESKTOP:-}" in
		*GNOME*) return 0 ;;
		*) return 1 ;;
	esac
}

# gset SCHEMA[:PATH] KEY VALUE — idempotent, never fatal. Reads first so a
# re-run touches nothing, and so a missing key exits 0 instead of failing.
gset() {
	_gs_have="$(gsettings get "$1" "$2" 2> /dev/null)" || return 0
	[ "$_gs_have" = "$3" ] && return 0
	gsettings set "$1" "$2" "$3" 2> /dev/null ||
		echo "WARNING: could not set $1 $2"
}

apply_terminal_copy_paste() {
	# gnome-terminal may not be installed (Ptyxis-only or headless GNOME).
	# NOTE: the Keybindings schema is RELOCATABLE (it is bound to a dconf path),
	# and `gsettings list-schemas` omits relocatable schemas entirely — it must
	# be looked up with list-relocatable-schemas or this always reports "absent".
	if ! gsettings list-relocatable-schemas 2> /dev/null |
		grep -qx 'org.gnome.Terminal.Legacy.Keybindings'; then
		echo "gnome-terminal keybinding schema absent; skipping copy/paste binds."
		return 0
	fi
	gset "$GT_KEYS" copy "'<Super>c'"
	gset "$GT_KEYS" paste "'<Super>v'"
}

free_super_v() {
	# GNOME binds <Super>v to the message tray, which would swallow the paste
	# before gnome-terminal ever sees it. <Super>m stays as the tray toggle, so
	# nothing is lost. Only rewrite when <Super>v is actually still bound, to
	# avoid stomping a deliberate user customization on a re-run.
	_tray="$(gsettings get org.gnome.shell.keybindings toggle-message-tray 2> /dev/null)" || return 0
	case "$_tray" in
		*"<Super>v"*)
			gset org.gnome.shell.keybindings toggle-message-tray "['<Super>m']"
			;;
	esac
}

if gnome_session_available; then
	echo "Applying GNOME desktop defaults (macOS-style Super+C / Super+V)..."
	apply_terminal_copy_paste
	free_super_v
else
	echo "Not a writable GNOME session; skipping GNOME desktop defaults."
fi
