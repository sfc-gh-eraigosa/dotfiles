#!/usr/bin/env bash
# setup_system_tuning.sh — the standard disk tuning for Linux nodes
# (gff install.system.tuning, default on).
#
# Small SD/eMMC-booted nodes (Raspberry Pi, Jetson) fill their root disk with logs
# and package caches nobody reads. On a Linux host running systemd it:
#   1. writes /etc/systemd/journald.conf.d/10-dotfiles-tuning.conf capping the
#      persistent journal at SystemMaxUse=2G, restarts systemd-journald when the
#      file changed, and vacuums the journal down to the cap;
#   2. on an apt host, writes /etc/apt/apt.conf.d/52dotfiles-tuning with
#      APT::Periodic::CleanInterval "7", so apt-daily empties /var/cache/apt
#      (downloaded .debs) once a week.
#
# SKIPs (exit 0) off Linux (macOS) or without a running systemd (containers, WSL
# without systemd); the apt step is skipped alone without apt-get. Idempotent: a
# host already tuned is a no-op. A differing file is backed up (.<timestamp>.bak).
#
# Usage: setup_system_tuning.sh [--dry-run | --status]
# Env: ST_JOURNAL_MAX (2G) widens or narrows the journal cap.
# Env (tests): ST_UNAME, ST_SYSTEMD_DIR (/run/systemd/system), ST_JOURNALD_DIR
#   (/etc/systemd/journald.conf.d), ST_APT_CONF_DIR (/etc/apt/apt.conf.d), ST_APT_GET,
#   ST_SYSTEMCTL, ST_JOURNALCTL, ST_SUDO ("sudo -n" when not root; empty = none).
# Companion test: setup_system_tuning_test.sh. Docs: docs/system-tuning.md.
set -u

MODE=apply
for arg in "$@"; do
  case "${arg}" in
    --dry-run) MODE=dry ;;
    --status) MODE=status ;;
    -h|--help) sed -n '2,24p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "setup_system_tuning.sh: unknown argument '${arg}' (use --dry-run | --status)" >&2; exit 64 ;;
  esac
done

UNAME="${ST_UNAME:-$(uname -s)}"
SYSTEMD_DIR="${ST_SYSTEMD_DIR:-/run/systemd/system}"
JD_DIR="${ST_JOURNALD_DIR:-/etc/systemd/journald.conf.d}"
APT_DIR="${ST_APT_CONF_DIR:-/etc/apt/apt.conf.d}"
APT_GET="${ST_APT_GET:-apt-get}"
SYSTEMCTL="${ST_SYSTEMCTL:-systemctl}"
JOURNALCTL="${ST_JOURNALCTL:-journalctl}"
MAX="${ST_JOURNAL_MAX:-2G}"
JD_FILE="${JD_DIR}/10-dotfiles-tuning.conf"
APT_FILE="${APT_DIR}/52dotfiles-tuning"
if [ -n "${ST_SUDO+x}" ]; then SUDO="${ST_SUDO}"
elif [ "$(id -u)" -eq 0 ]; then SUDO=""
else SUDO="sudo -n"; fi

skip() { echo "SKIP: system tuning: $*"; exit 0; }
[ "${UNAME}" = Linux ] || skip "not Linux (${UNAME})"
[ -d "${SYSTEMD_DIR}" ] || skip "systemd is not running here (no ${SYSTEMD_DIR})"
case "${MAX}" in *[!0-9KMGT]*|'') echo "ERROR: ST_JOURNAL_MAX='${MAX}' is not a size like 2G or 500M" >&2; exit 64 ;; esac
HAVE_APT=0; command -v "${APT_GET}" >/dev/null 2>&1 && HAVE_APT=1

render_jd() {
  cat <<RENDER
# Managed by dotfiles: opt/scripts/system/setup_system_tuning.sh (gff install.system.tuning).
[Journal]
SystemMaxUse=${MAX}
RENDER
}
render_apt() {
  cat <<RENDER
// Managed by dotfiles: opt/scripts/system/setup_system_tuning.sh (gff install.system.tuning).
// apt-daily runs apt-get clean every 7 days: downloaded .debs leave /var/cache/apt.
APT::Periodic::CleanInterval "7";
RENDER
}
current() { [ -f "$1" ] && "$2" | cmp -s - "$1"; }  # current <file> <render fn>
state() { if current "$1" "$2"; then echo current; elif [ -f "$1" ]; then echo "present, differs from the dotfiles render"; else echo absent; fi; }

if [ "${MODE}" = status ]; then
  echo "${JD_FILE}: $(state "${JD_FILE}" render_jd)"
  if [ "${HAVE_APT}" -eq 1 ]; then echo "${APT_FILE}: $(state "${APT_FILE}" render_apt)"; else echo "apt: not an apt host"; fi
  "${JOURNALCTL}" --disk-usage 2>/dev/null | tail -1
  exit 0
fi

if [ "${MODE}" = dry ]; then
  echo "system tuning --dry-run: plan"
  if current "${JD_FILE}" render_jd; then echo "  ${JD_FILE}: already current"
  else echo "  would write ${JD_FILE}:"; render_jd | sed 's/^/    /'; echo "  would: systemctl restart systemd-journald"; fi
  echo "  would: journalctl --vacuum-size=${MAX}"
  if [ "${HAVE_APT}" -eq 0 ]; then echo "  apt: not an apt host, skipped"
  elif current "${APT_FILE}" render_apt; then echo "  ${APT_FILE}: already current"
  else echo "  would write ${APT_FILE}:"; render_apt | sed 's/^/    /'; fi
  exit 0
fi

if [ -n "${SUDO}" ] && ! ${SUDO} true 2>/dev/null; then
  echo "WARNING: system tuning needs sudo and it wants a password; run 'sudo -v' then '$0' interactively."
  exit 0
fi

TMP="$(mktemp -d "${TMPDIR:-/tmp}/system-tuning.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
place() {  # place <file> <render fn>; returns 0 when it wrote
  current "$1" "$2" && return 1
  "$2" > "${TMP}/new"
  if [ -f "$1" ]; then
    b="$1.$(date +%Y%m%d%H%M%S).bak"
    ${SUDO} cp -p "$1" "${b}" && echo "kept the previous file as ${b}"
  fi
  ${SUDO} mkdir -p "$(dirname "$1")"
  ${SUDO} install -m 0644 "${TMP}/new" "$1" || { echo "ERROR: could not write $1" >&2; exit 1; }
  echo "wrote $1"
}

changed=0
if place "${JD_FILE}" render_jd; then
  changed=1
  ${SUDO} "${SYSTEMCTL}" restart systemd-journald || echo "WARNING: could not restart systemd-journald; the cap applies at its next start."
fi
# Vacuum every run: cheap when already under the cap, and it catches a journal
# that grew while the cap was larger.
${SUDO} "${JOURNALCTL}" --vacuum-size="${MAX}" >/dev/null 2>&1 || echo "WARNING: journalctl --vacuum-size=${MAX} failed."

if [ "${HAVE_APT}" -eq 1 ]; then
  place "${APT_FILE}" render_apt && changed=1
else
  echo "apt: not an apt host, skipped"
fi

if [ "${changed}" -eq 0 ]; then
  echo "system tuning: already set (journal capped at ${MAX}, weekly apt clean)"
else
  echo "system tuning: set; journal capped at ${MAX}$([ "${HAVE_APT}" -eq 1 ] && echo ', apt cache cleaned weekly')"
fi
