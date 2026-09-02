#!/bin/bash
# GNOME desktop defaults — the Linux counterpart to the macOS-style hotkeys that
# opt/Desktop/Apps/scripts/macos.ahk provides on Windows.
#
# THE OTHER HALF: macos-keys-linux.sh applies the evdev half of this feature via
# keyd, which is what makes Cmd+C work inside Firefox/VS Code/Nautilus rather than
# only in gnome-terminal. The split is deliberate:
#   - keyd    -> in-application editing keys (Cmd+C/V/X/A/Z/S/F...)
#   - gsettings (here) -> desktop-level ACTIONS that keyd deliberately hands back
#                         to GNOME as a real <Super> chord (Cmd+Tab/Space/M/H)
# Both halves are gated by the cross-OS gff flag `keyboard.macos.enabled`.
#
# WHY gnome-terminal MUST STAY ON STOCK Ctrl+Shift+C/V: keyd's app.conf turns
# Cmd+C/V into Ctrl+Shift+C/V while a terminal is focused (Ctrl+C has to stay
# SIGINT). Those chords only do anything if the terminal's own copy/paste
# accelerators are still the stock ones. Before keyd existed (#247) this script
# bound them to <Super>c/<Super>v instead; with keyd consuming Super at the evdev
# layer that chord never arrives, and VTE, seeing Ctrl+Shift+V with no accelerator
# to match, writes the raw control code to the shell -- Cmd+V printed ^V and
# Cmd+C was one keystroke from an interrupt. apply_terminal_copy_paste now UNDOES
# that stale bind on machines that received it. Never bind these to Super again.
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

# greset_if SCHEMA[:PATH] KEY STALE — restore the schema default, but only when
# the key still holds the exact value an older version of this script wrote, so a
# deliberate user customization survives. Never fatal.
greset_if() {
	_gr_have="$(gsettings get "$1" "$2" 2> /dev/null)" || return 0
	[ "$_gr_have" = "$3" ] || return 0
	gsettings reset "$1" "$2" 2> /dev/null ||
		echo "WARNING: could not reset $1 $2"
}

apply_terminal_copy_paste() {
	# gnome-terminal may not be installed (Ptyxis-only or headless GNOME).
	# NOTE: the Keybindings schema is RELOCATABLE (it is bound to a dconf path),
	# and `gsettings list-schemas` omits relocatable schemas entirely — it must
	# be looked up with list-relocatable-schemas or this always reports "absent".
	if ! gsettings list-relocatable-schemas 2> /dev/null |
		grep -qx 'org.gnome.Terminal.Legacy.Keybindings'; then
		echo "gnome-terminal keybinding schema absent; skipping copy/paste check."
		return 0
	fi
	# Migration: put copy/paste back on stock Ctrl+Shift+C/V where the pre-keyd
	# script had moved them to Super (see the header). keyd's app.conf owns
	# Cmd+C/V in the terminal from here on; gsettings must not bind them at all.
	greset_if "$GT_KEYS" copy "'<Super>c'"
	greset_if "$GT_KEYS" paste "'<Super>v'"
}

free_message_tray() {
	# GNOME binds the message tray to <Super>v (swallowing Cmd+V) and, once this
	# script has run once, to <Super>m -- which now has to go too, because Cmd+M
	# is Minimize. <Super>n keeps the tray reachable and collides with nothing.
	# Only rewrite when one of those keys is actually still bound, so a deliberate
	# user customization survives a re-run.
	_tray="$(gsettings get org.gnome.shell.keybindings toggle-message-tray 2> /dev/null)" || return 0
	case "$_tray" in
		*"<Super>v"* | *"<Super>m"*)
			gset org.gnome.shell.keybindings toggle-message-tray "['<Super>n']"
			;;
	esac
}

apply_macos_desktop_keys() {
	# The chords keyd forwards to GNOME untouched. Each one is a macos.ahk
	# behaviour that only the desktop can perform, not the focused application.

	# Cmd+Space = Spotlight. GNOME's search lives on panel-main-menu; <Super>space
	# has to be pried off the input-source switcher first or it wins.
	_src="$(gsettings get org.gnome.desktop.wm.keybindings switch-input-source 2> /dev/null)"
	case "$_src" in
		*"<Super>space"*)
			gset org.gnome.desktop.wm.keybindings switch-input-source "['XF86Keyboard']"
			;;
	esac
	gset org.gnome.desktop.wm.keybindings panel-main-menu "['<Super>space']"

	# Cmd+M and Cmd+H both minimize, exactly as macos.ahk does on Windows.
	gset org.gnome.desktop.wm.keybindings minimize "['<Super>m', '<Super>h']"

	# A lone Cmd tap does nothing on macOS. Left alone, GNOME opens the Activities
	# overview every time the user reaches for Cmd and changes their mind -- which
	# is jarring enough that macos.ahk spends a dedicated block suppressing the
	# same behaviour on Windows.
	gset org.gnome.mutter overlay-key "''"

	# Cmd+Option+arrows tile. keyd emits these as plain Ctrl+Alt chords -- it must
	# NOT synthesize <Super> from inside the Meta layer, which crashes it (see the
	# warning in opt/etc/keyd/default.conf) -- so GNOME has to be pointed at
	# Ctrl+Alt to receive them. Cmd+L (lock) needs nothing here: it is left out of
	# the keyd layer entirely, so it arrives as GNOME's stock <Super>l.
	gset org.gnome.mutter.keybindings toggle-tiled-left "['<Control><Alt>Left']"
	gset org.gnome.mutter.keybindings toggle-tiled-right "['<Control><Alt>Right']"
	gset org.gnome.desktop.wm.keybindings maximize "['<Control><Alt>Up']"
	gset org.gnome.desktop.wm.keybindings unmaximize "['<Control><Alt>Down']"
}

if gnome_session_available; then
	echo "Applying GNOME desktop defaults (macOS-style Cmd+Space/M/H, stock terminal copy/paste)..."
	apply_terminal_copy_paste
	free_message_tray
	apply_macos_desktop_keys
else
	echo "Not a writable GNOME session; skipping GNOME desktop defaults."
fi
