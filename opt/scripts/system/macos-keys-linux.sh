#!/usr/bin/env bash
# macOS-style keyboard parity for the Linux desktop, via keyd.
#
# The Linux counterpart to opt/Desktop/Apps/scripts/macos.ahk on Windows: Super
# acts as Command, so Cmd+C/V/X/A/Z/S/F/T/W and the macOS navigation chords work
# in EVERY GUI application, not just the terminal. gnome-desktop-defaults.sh is
# the gsettings half of the same feature (desktop-level actions); this script is
# the evdev half (in-application editing keys).
#
# Both halves are gated by the cross-OS gff flag `keyboard.macos.enabled` — see
# docs/macos-keys.md for the full key table, the panic sequence, and recovery.
#
# Usage:
#   macos-keys-linux.sh              install / re-apply (idempotent)
#   macos-keys-linux.sh --doctor     report state, change nothing
#   macos-keys-linux.sh --uninstall  remove the config and stop the daemon
#
# No-ops on anything that is not a Linux machine with a graphical session
# (CI, docker, WSL, plain SSH with no desktop, macOS), so install.sh stays safe
# headless.

set -o nounset

KEYD_VERSION="v2.6.0"
KEYD_REPO="https://github.com/rvaiya/keyd.git"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
CONF_SRC_DIR="${REPO_ROOT}/opt/etc/keyd"

# Overridable so the test driver can point the whole install at a temp dir
# instead of the real /etc. Never set in normal use.
KEYD_ETC="${KEYD_ETC:-/etc/keyd}"
# Where evdev keyboards appear. Overridable for the same reason as KEYD_ETC: a CI
# container has no /sys/class/input, which would silently no-op every test.
INPUT_DEVICES_DIR="${INPUT_DEVICES_DIR:-/sys/class/input}"
APP_CONF="${HOME}/.config/keyd/app.conf"
KEYD_DROPIN="${KEYD_DROPIN:-/etc/systemd/system/keyd.service.d/10-dotfiles-restart.conf}"
UNIT="${HOME}/.config/systemd/user/keyd-application-mapper.service"
BUILD_CACHE="${HOME}/.cache/dotfiles/keyd"

# Populated by detect_graphical_session().
GUI_SESSION_TYPE=""
GUI_DISPLAY=""

log()  { echo "$*"; }
warn() { echo "WARNING: $*" >&2; }

# --- Guards -------------------------------------------------------------------

# A Linux host that could plausibly run a desktop. WSL is excluded on purpose:
# it has no seat, no evdev keyboard of its own, and the Windows side is already
# covered by macos.ahk.
host_supported() {
	[ "$(uname -s)" = "Linux" ] || return 1
	case "$(uname -r)" in
		*-[Mm]icrosoft* | *microsoft*) return 1 ;;
	esac
	command -v systemctl > /dev/null 2>&1 || return 1
	[ -d "${INPUT_DEVICES_DIR}" ] || return 1
	return 0
}

# Find the user's graphical session, which is NOT necessarily the session this
# script is running in — install.sh is frequently run over SSH while the desktop
# is logged in on the console. Sets GUI_SESSION_TYPE and GUI_DISPLAY.
detect_graphical_session() {
	# The easy case: we are already inside the desktop session.
	case "${XDG_SESSION_TYPE:-}" in
		x11 | wayland)
			GUI_SESSION_TYPE="${XDG_SESSION_TYPE}"
			GUI_DISPLAY="${DISPLAY:-}"
			return 0
			;;
	esac

	command -v loginctl > /dev/null 2>&1 || return 1

	_uid="$(id -u)"
	_sessions="$(loginctl list-sessions --no-legend 2> /dev/null | awk '{print $1}')"
	for _s in ${_sessions}; do
		_type="$(loginctl show-session "${_s}" -p Type --value 2> /dev/null)"
		_suid="$(loginctl show-session "${_s}" -p User --value 2> /dev/null)"
		[ "${_suid}" = "${_uid}" ] || continue
		case "${_type}" in
			x11 | wayland)
				GUI_SESSION_TYPE="${_type}"
				GUI_DISPLAY="$(loginctl show-session "${_s}" -p Display --value 2> /dev/null)"
				# loginctl's Display is often empty on X11; fall back to the
				# socket the running Xorg actually created.
				if [ -z "${GUI_DISPLAY}" ] && [ "${_type}" = "x11" ]; then
					_sock="$(ls /tmp/.X11-unix/ 2> /dev/null | head -n1)"
					[ -n "${_sock}" ] && GUI_DISPLAY=":${_sock#X}"
				fi
				return 0
				;;
		esac
	done
	return 1
}

# --- Dependencies -------------------------------------------------------------

apt_install() {
	command -v apt-get > /dev/null 2>&1 || {
		warn "no apt-get; install these by hand: $*"
		return 1
	}
	sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "$@" > /dev/null 2>&1 ||
		{
			warn "could not install: $*"
			return 1
		}
	return 0
}

# python3-xlib backs the application mapper's X11 backend. Without it the mapper
# cannot tell a terminal from a browser, and Cmd+C in a terminal becomes SIGINT.
ensure_mapper_deps() {
	python3 -c 'import Xlib' > /dev/null 2>&1 && return 0
	log "Installing python3-xlib (needed to detect the focused window)..."
	apt_install python3-xlib || return 1
	python3 -c 'import Xlib' > /dev/null 2>&1
}

ensure_build_deps() {
	_missing=""
	for _t in gcc make git; do
		command -v "${_t}" > /dev/null 2>&1 || _missing="${_missing} ${_t}"
	done
	[ -z "${_missing}" ] && return 0
	log "Installing build dependencies:${_missing}"
	# shellcheck disable=SC2086
	apt_install ${_missing}
}

# --- keyd ---------------------------------------------------------------------

keyd_installed_version() {
	command -v keyd > /dev/null 2>&1 || return 1
	keyd --version 2> /dev/null | awk '{print $2}'
}

# keyd is not packaged for Ubuntu noble (checked on 24.04 arm64), so build the
# pinned tag from source. It is plain C with no dependencies beyond libc and
# builds in a couple of seconds even on the Spark's aarch64 cores.
ensure_keyd() {
	_have="$(keyd_installed_version || true)"
	if [ "${_have}" = "${KEYD_VERSION}" ]; then
		log "keyd ${KEYD_VERSION} already installed."
		return 0
	fi

	ensure_build_deps || return 1

	log "Building keyd ${KEYD_VERSION} from source..."
	mkdir -p "$(dirname "${BUILD_CACHE}")" || return 1
	if [ -d "${BUILD_CACHE}/.git" ]; then
		git -C "${BUILD_CACHE}" fetch --quiet --tags --depth 1 origin "${KEYD_VERSION}" 2> /dev/null
	else
		rm -rf "${BUILD_CACHE}"
		git -c advice.detachedHead=false clone --quiet --depth 1 \
			--branch "${KEYD_VERSION}" "${KEYD_REPO}" "${BUILD_CACHE}" ||
			{
				warn "could not clone keyd"
				return 1
			}
	fi
	git -C "${BUILD_CACHE}" checkout --quiet "${KEYD_VERSION}" 2> /dev/null

	make -C "${BUILD_CACHE}" -j"$(nproc 2> /dev/null || echo 2)" > /dev/null 2>&1 ||
		{
			warn "keyd build failed"
			return 1
		}
	sudo make -C "${BUILD_CACHE}" install > /dev/null 2>&1 ||
		{
			warn "keyd install failed"
			return 1
		}
	log "Installed keyd ${KEYD_VERSION}."
}

# Find running mapper processes WITHOUT pkill/pgrep -f.
#
# `pkill -f keyd-application-mapper` matches any process whose command line merely
# CONTAINS that string -- an editor, a grep, a shell running a script that mentions
# it -- and kills it. That is not hypothetical: it killed the shell running this
# repo's own test suite. Matching on the installed BINARY PATH, and skipping our
# own pid, is precise.
mapper_pids() {
	for _d in /proc/[0-9]*; do
		_p="${_d#/proc/}"
		[ "${_p}" = "$$" ] && continue
		# A process can exit between the glob and the read, so the redirect itself
		# must not be what fails -- guard the file rather than the pipeline.
		[ -r "${_d}/cmdline" ] || continue
		tr '\0' ' ' 2> /dev/null < "${_d}/cmdline" |
			grep -q 'bin/keyd-application-mapper' || continue
		echo "${_p}"
	done
}

mapper_running() {
	[ -n "$(mapper_pids)" ]
}

# --- The fail-closed gate -----------------------------------------------------
#
# THIS IS THE MOST IMPORTANT FUNCTION IN THE SCRIPT.
#
# /etc/keyd/default.conf makes Cmd+C emit Ctrl+C. In a terminal that is SIGINT,
# not copy. Only the application mapper turns it back into Ctrl+Shift+C. So a
# machine where keyd runs but the mapper does not is strictly WORSE than a
# machine with no customization at all: the user loses copy AND gains a stray
# interrupt. We therefore refuse to install the keyd config unless the mapper's
# window-detection backend is proven to work first.
mapper_backend_ok() {
	case "${GUI_SESSION_TYPE}" in
		x11)
			# Pure-Xlib backend: no shell extension, no GNOME restart needed.
			DISPLAY="${GUI_DISPLAY}" python3 - <<-'PY' > /dev/null 2>&1
				import Xlib.display, Xlib.Xatom
				d = Xlib.display.Display()
				d.screen().root.get_full_property(
				    d.intern_atom('_NET_ACTIVE_WINDOW'), Xlib.Xatom.WINDOW)
			PY
			return $?
			;;
		wayland)
			# Xlib cannot see native Wayland windows, so GNOME's shell extension
			# is the only route. Require it to be present AND enabled.
			gnome-extensions info keyd > /dev/null 2>&1 || return 1
			gnome-extensions list --enabled 2> /dev/null | grep -q '^keyd' || return 1
			return 0
			;;
	esac
	return 1
}

# The mapper picks a backend in the order wlroots, cosmic, Gnome, X — and its
# GNOME backend needs a shell extension plus a GNOME restart. On an X11 session
# we would rather have the dependency-free Xlib backend, and hiding
# XDG_CURRENT_DESKTOP/GNOME_SETUP_DISPLAY is what makes the mapper skip GNOME and
# fall through to it.
mapper_command() {
	if [ "${GUI_SESSION_TYPE}" = "x11" ]; then
		echo "env -u GNOME_SETUP_DISPLAY XDG_CURRENT_DESKTOP= DISPLAY=${GUI_DISPLAY} keyd-application-mapper -d"
	else
		echo "keyd-application-mapper -d"
	fi
}

# Same command without -d. systemd supervises the process itself, so the unit has
# to run it in the foreground -- a daemonizing ExecStart looks like an instant
# exit and systemd would restart it forever.
mapper_command_fg() {
	mapper_command | sed 's/ -d$//'
}

# ExecStart for the user unit.
#
# The `sg keyd -c` wrapper is load-bearing and NOT redundant with usermod. A
# systemd USER unit inherits its groups from user@<uid>.service, which was started
# at login -- before the installer added this user to the keyd group. So the unit
# would run but fail with EACCES on /var/run/keyd.socket until the next full
# logout, silently leaving Cmd+C as SIGINT in terminals for the rest of the
# session. User units cannot use SupplementaryGroups= (a system-unit privilege),
# and `sg` needs no password for a user already listed in the group.
mapper_exec_start() {
	_sg="$(command -v sg 2> /dev/null)"
	if [ -n "${_sg}" ]; then
		echo "${_sg} keyd -c \"$(mapper_command_fg)\""
	else
		mapper_command_fg
	fi
}

# True when this shell's credentials already include the keyd group. Membership
# recorded by usermod does NOT apply to sessions that were already open.
keyd_group_active() {
	id -nG 2> /dev/null | tr ' ' '\n' | grep -qx keyd
}

# --- Install steps ------------------------------------------------------------

install_configs() {
	sudo mkdir -p "${KEYD_ETC}" || return 1
	# Preserve anything the user (or another tool) put there first. Once.
	if [ -f "${KEYD_ETC}/default.conf" ] &&
		! sudo cmp -s "${CONF_SRC_DIR}/default.conf" "${KEYD_ETC}/default.conf" &&
		[ ! -f "${KEYD_ETC}/default.conf.pre-dotfiles" ]; then
		log "Backing up existing ${KEYD_ETC}/default.conf -> default.conf.pre-dotfiles"
		sudo cp -a "${KEYD_ETC}/default.conf" "${KEYD_ETC}/default.conf.pre-dotfiles" || return 1
	fi
	sudo install -m 0644 "${CONF_SRC_DIR}/default.conf" "${KEYD_ETC}/default.conf" || return 1

	mkdir -p "$(dirname "${APP_CONF}")" || return 1
	install -m 0644 "${CONF_SRC_DIR}/app.conf" "${APP_CONF}" || return 1
}

# keyd ships no Restart= policy, so a crash leaves the keyboard stock until someone
# notices and restarts it by hand. It CAN crash: a config that makes it emit its own
# trigger modifier recursively took it down with a SIGSEGV during development. The
# burst limit matters as much as the restart -- a genuinely crash-looping keyd
# should give up and leave a working stock keyboard rather than thrash forever.
install_keyd_dropin() {
	sudo mkdir -p "$(dirname "${KEYD_DROPIN}")" || return 1
	printf '%s\n' \
		'[Unit]' \
		'StartLimitIntervalSec=60' \
		'StartLimitBurst=5' \
		'' \
		'[Service]' \
		'Restart=on-failure' \
		'RestartSec=2' |
		sudo tee "${KEYD_DROPIN}" > /dev/null || return 1
	sudo systemctl daemon-reload > /dev/null 2>&1
}

enable_keyd() {
	install_keyd_dropin || warn "could not install the keyd restart drop-in"
	sudo systemctl enable keyd > /dev/null 2>&1 ||
		warn "could not enable the keyd service"
	sudo systemctl restart keyd > /dev/null 2>&1 || {
		warn "keyd failed to start; run 'sudo systemctl status keyd' and 'sudo keyd check'"
		return 1
	}
	# The mapper talks to keyd over its socket, which is group-owned by `keyd`.
	# Check the group FILE, not this shell's credentials: an already-open session
	# never picks up a new group, so `id -nG` would re-report "missing" forever and
	# usermod would run on every install.
	if getent group keyd > /dev/null 2>&1 &&
		! getent group keyd | cut -d: -f4 | tr ',' '\n' | grep -qx "${USER}"; then
		log "Adding ${USER} to the 'keyd' group (log out and back in for it to take effect)."
		sudo usermod -aG keyd "${USER}" > /dev/null 2>&1 ||
			warn "could not add ${USER} to the keyd group"
	fi
}

# A systemd USER unit rather than an XDG autostart .desktop entry, which is what
# keyd's own README suggests. The difference is supervision: if the mapper dies,
# an autostart entry does nothing until the next login, and the machine sits in
# the one state this feature must never be in -- keyd remapping Cmd+C to Ctrl+C
# with nothing left to turn it back into copy inside a terminal. Restart=always
# closes that window.
install_unit() {
	mkdir -p "$(dirname "${UNIT}")" || return 1
	cat > "${UNIT}" <<-EOF
		[Unit]
		Description=keyd application mapper (dotfiles: macOS-style per-app key overrides)
		Documentation=https://github.com/sfc-gh-eraigosa/dotfiles/blob/main/docs/macos-keys.md
		PartOf=graphical-session.target
		After=graphical-session.target

		[Service]
		Type=simple
		ExecStart=$(mapper_exec_start)
		Restart=always
		RestartSec=2

		[Install]
		WantedBy=graphical-session.target
	EOF
	systemctl --user daemon-reload > /dev/null 2>&1
	systemctl --user enable keyd-application-mapper.service > /dev/null 2>&1 ||
		warn "could not enable the mapper user unit"
}

start_mapper() {
	_log="${HOME}/.config/keyd/app.log"

	if ! mapper_running; then
		# Clear the log so the health check below only judges this run.
		rm -f "${_log}"
		# Prefer the supervised unit. It only works once the user manager itself has
		# the keyd group, which an already-open login session does not -- so fall
		# back to `sg`, which applies the group to a single command immediately.
		systemctl --user start keyd-application-mapper.service > /dev/null 2>&1
		sleep 2
		if ! mapper_running ||
			grep -q 'Failed to connect' "${_log}" 2> /dev/null; then
			systemctl --user stop keyd-application-mapper.service > /dev/null 2>&1
			rm -f "${_log}"
			if keyd_group_active; then
				# shellcheck disable=SC2091
				$(mapper_command) > /dev/null 2>&1 &
			else
				sg keyd -c "$(mapper_command)" > /dev/null 2>&1 &
			fi
			sleep 2
		fi
		mapper_running || return 1
	fi

	# A live process is NOT proof of success: the mapper daemonizes FIRST and only
	# then discovers it cannot open /var/run/keyd.socket, so it sits there running
	# and applying nothing. That is precisely the state that leaves Cmd+C as SIGINT
	# in a terminal. Check the log unconditionally -- including for a mapper that
	# was already running when we got here, which may be a stale broken one from a
	# previous boot.
	if grep -q 'Failed to connect' "${_log}" 2> /dev/null; then
		return 1
	fi
	return 0
}

# --- Doctor / uninstall -------------------------------------------------------

doctor() {
	log "macos-keys (Linux) status"
	log "  host supported:      $(host_supported && echo yes || echo "no — skipping")"
	if detect_graphical_session; then
		log "  graphical session:   ${GUI_SESSION_TYPE} (DISPLAY=${GUI_DISPLAY:-unset})"
	else
		log "  graphical session:   none found"
	fi
	log "  keyd binary:         $(keyd_installed_version || echo "not installed")"
	_svc="$(systemctl is-active keyd 2> /dev/null)"
	log "  keyd service:        ${_svc:-inactive}"
	log "  /etc/keyd/default.conf: $([ -f "${KEYD_ETC}/default.conf" ] && echo present || echo absent)"
	log "  ~/.config/keyd/app.conf: $([ -f "${APP_CONF}" ] && echo present || echo absent)"
	log "  window-detect backend: $(mapper_backend_ok && echo ok || echo "NOT WORKING")"
	log "  mapper running:      $(mapper_running && echo yes || echo no)"
	log "  mapper unit:         $([ -f "${UNIT}" ] && echo present || echo absent)"
	_us="$(systemctl --user is-enabled keyd-application-mapper.service 2> /dev/null)"
	log "  mapper unit enabled: ${_us:-no}"
}

uninstall() {
	log "Removing macOS-style key mappings..."
	sudo systemctl disable --now keyd > /dev/null 2>&1
	sudo rm -f "${KEYD_DROPIN}"
	sudo rmdir "$(dirname "${KEYD_DROPIN}")" 2> /dev/null
	sudo systemctl daemon-reload > /dev/null 2>&1
	for _mp in $(mapper_pids); do kill "${_mp}" 2> /dev/null; done
	systemctl --user disable --now keyd-application-mapper.service > /dev/null 2>&1
	rm -f "${UNIT}" "${APP_CONF}"
	systemctl --user daemon-reload > /dev/null 2>&1
	if [ -f "${KEYD_ETC}/default.conf.pre-dotfiles" ]; then
		sudo mv "${KEYD_ETC}/default.conf.pre-dotfiles" "${KEYD_ETC}/default.conf"
		log "Restored the pre-dotfiles ${KEYD_ETC}/default.conf."
	else
		sudo rm -f "${KEYD_ETC}/default.conf"
	fi
	log "Done. The keyboard is back to stock."
}

# --- Main ---------------------------------------------------------------------

main() {
	case "${1:-}" in
		--doctor)
			doctor
			return 0
			;;
		--uninstall)
			uninstall
			return 0
			;;
	esac

	if ! host_supported; then
		log "Not a Linux desktop host; skipping macOS-style key mappings."
		return 0
	fi
	if ! detect_graphical_session; then
		log "No graphical session found; skipping macOS-style key mappings."
		return 0
	fi

	log "Applying macOS-style key mappings (Super acts as Command)..."

	# Order is deliberate: everything that can fail SAFELY runs before the config
	# that changes what the keyboard does.
	ensure_mapper_deps || {
		warn "python3-xlib unavailable; refusing to remap the keyboard (see docs/macos-keys.md)."
		return 0
	}
	ensure_keyd || {
		warn "keyd unavailable; skipping."
		return 0
	}
	if ! mapper_backend_ok; then
		warn "cannot detect the focused window on this ${GUI_SESSION_TYPE} session."
		if [ "${GUI_SESSION_TYPE}" = "wayland" ]; then
			warn "Wayland needs the keyd GNOME extension:"
			warn "  ln -s /usr/local/share/keyd/gnome-extension-45 \\"
			warn "        ~/.local/share/gnome-shell/extensions/keyd"
			warn "  gnome-extensions enable keyd   # then log out and back in"
		fi
		warn "REFUSING to install the keyd config: without per-app overrides,"
		warn "Cmd+C in a terminal would send SIGINT instead of copying."
		return 0
	fi

	install_configs || {
		warn "could not install keyd configs; skipping."
		return 0
	}
	enable_keyd || return 0
	install_unit || warn "could not install the mapper user unit"
	if ! start_mapper; then
		warn "the application mapper did not come up; rolling back so the keyboard"
		warn "is not left with Cmd+C mapped to SIGINT inside terminals."
		uninstall
		return 0
	fi

	log "macOS-style keys are live. 'macos-keys-linux.sh --doctor' reports state;"
	log "hold Backspace+Esc+Enter together to panic-quit keyd if anything misbehaves."
}

main "$@"
