#!/usr/bin/env bash
# setup_unattended_upgrades.sh — unattended security upgrades on apt hosts
# (gff install.system.unattended-upgrades, default on).
#
# On a Debian/Ubuntu/Raspberry Pi OS host running systemd it:
#   1. installs the `unattended-upgrades` package when absent;
#   2. writes /etc/apt/apt.conf.d/20auto-upgrades turning on APT::Periodic
#      (a daily index refresh and a daily unattended-upgrade run);
#   3. enables the apt-daily.timer and apt-daily-upgrade.timer units.
# WHICH packages get upgraded stays the distro's default in 50unattended-upgrades:
# the release's security origin (plus ESM on Ubuntu Pro). Third-party repos (NVIDIA,
# Docker, nodesource, GitHub CLI) are not in it, so L4T and CUDA never move on their own.
#
# SKIPs (exit 0) without apt-get (macOS, Fedora) or without a running systemd
# (containers, WSL without systemd). Idempotent: a host already set up is a no-op.
# A 20auto-upgrades that is not ours is backed up (.<timestamp>.bak) first.
#
# Usage: setup_unattended_upgrades.sh [--dry-run | --status]
# Env (tests): UU_APT_CONF_DIR (/etc/apt/apt.conf.d), UU_SYSTEMD_DIR (/run/systemd/system),
#   UU_APT_GET, UU_DPKG_QUERY, UU_SYSTEMCTL, UU_SUDO ("sudo -n" when not root; empty = none).
# Companion test: setup_unattended_upgrades_test.sh. Docs: docs/sshd-hardening.md.
set -u

MODE=apply
for arg in "$@"; do
  case "${arg}" in
    --dry-run) MODE=dry ;;
    --status) MODE=status ;;
    -h|--help) sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "setup_unattended_upgrades.sh: unknown argument '${arg}' (use --dry-run | --status)" >&2; exit 64 ;;
  esac
done

APT_GET="${UU_APT_GET:-apt-get}"
DPKG_QUERY="${UU_DPKG_QUERY:-dpkg-query}"
SYSTEMCTL="${UU_SYSTEMCTL:-systemctl}"
CONF_DIR="${UU_APT_CONF_DIR:-/etc/apt/apt.conf.d}"
SYSTEMD_DIR="${UU_SYSTEMD_DIR:-/run/systemd/system}"
AUTO="${CONF_DIR}/20auto-upgrades"
PKG=unattended-upgrades
TIMERS="apt-daily.timer apt-daily-upgrade.timer"
if [ -n "${UU_SUDO+x}" ]; then SUDO="${UU_SUDO}"
elif [ "$(id -u)" -eq 0 ]; then SUDO=""
else SUDO="sudo -n"; fi

skip() { echo "SKIP: unattended-upgrades: $*"; exit 0; }
command -v "${APT_GET}" >/dev/null 2>&1 || skip "no apt-get (not an apt host)"
[ -d "${SYSTEMD_DIR}" ] || skip "systemd is not running here (no ${SYSTEMD_DIR}); the apt timers need it"

# shellcheck disable=SC2016 # ${Status} is dpkg-query's own format token, not a shell variable
installed() { "${DPKG_QUERY}" -W -f='${Status}' "${PKG}" 2>/dev/null | grep -q 'install ok installed'; }
timer_enabled() { "${SYSTEMCTL}" is-enabled --quiet "$1" 2>/dev/null; }
render() {
  cat <<RENDER
// Managed by dotfiles: opt/scripts/system/setup_unattended_upgrades.sh (gff install.system.unattended-upgrades).
// Which origins upgrade is the distro's default in 50unattended-upgrades (security only).
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
RENDER
}
file_current() { [ -f "${AUTO}" ] && render | cmp -s - "${AUTO}"; }

if [ "${MODE}" = status ]; then
  if installed; then echo "package ${PKG}: installed"; else echo "package ${PKG}: absent"; fi
  if file_current; then echo "${AUTO}: current"; elif [ -f "${AUTO}" ]; then echo "${AUTO}: present, differs from the dotfiles render"; else echo "${AUTO}: absent"; fi
  for t in ${TIMERS}; do if timer_enabled "${t}"; then echo "${t}: enabled"; else echo "${t}: not enabled"; fi; done
  exit 0
fi

if [ "${MODE}" = dry ]; then
  echo "unattended-upgrades --dry-run: plan"
  if installed; then echo "  package ${PKG}: already installed"; else echo "  would: ${APT_GET} install -y ${PKG}"; fi
  if file_current; then echo "  ${AUTO}: already current"; else echo "  would write ${AUTO}:"; render | sed 's/^/    /'; fi
  for t in ${TIMERS}; do if timer_enabled "${t}"; then echo "  ${t}: already enabled"; else echo "  would: systemctl enable --now ${t}"; fi; done
  exit 0
fi

if [ -n "${SUDO}" ] && ! ${SUDO} true 2>/dev/null; then
  echo "WARNING: unattended-upgrades setup needs sudo and it wants a password; run 'sudo -v' then '$0' interactively."
  exit 0
fi

# Backups end in .bak: apt skips only that suffix silently, and any other name in
# apt.conf.d prints "N: Ignoring file ... invalid filename extension" on every apt
# run. Earlier versions wrote .bak-<timestamp>; rename those once.
for old in "${AUTO}".bak-*; do
  [ -e "${old}" ] || continue
  ${SUDO} mv "${old}" "${AUTO}.${old##*.bak-}.bak" && echo "renamed ${old##*/} to ${AUTO##*/}.${old##*.bak-}.bak (apt ignores it silently)"
done

changed=0
if ! installed; then
  echo "installing ${PKG}..."
  if ! ${SUDO} env DEBIAN_FRONTEND=noninteractive "${APT_GET}" install -y -q "${PKG}"; then
    echo "install failed; refreshing the package index and retrying once..."
    ${SUDO} "${APT_GET}" update -q
    ${SUDO} env DEBIAN_FRONTEND=noninteractive "${APT_GET}" install -y -q "${PKG}" || { echo "ERROR: could not install ${PKG}" >&2; exit 1; }
  fi
  changed=1
fi

if ! file_current; then
  TMP="$(mktemp "${TMPDIR:-/tmp}/20auto-upgrades.XXXXXX")"
  trap 'rm -f "${TMP}"' EXIT
  render > "${TMP}"
  if [ -f "${AUTO}" ]; then
    BACKUP="${AUTO}.$(date +%Y%m%d%H%M%S).bak"
    ${SUDO} cp -p "${AUTO}" "${BACKUP}" && echo "kept the previous file as ${BACKUP}"
  fi
  ${SUDO} mkdir -p "${CONF_DIR}"
  ${SUDO} install -m 0644 "${TMP}" "${AUTO}" || { echo "ERROR: could not write ${AUTO}" >&2; exit 1; }
  echo "wrote ${AUTO}"
  changed=1
fi

for t in ${TIMERS}; do
  if ! timer_enabled "${t}"; then
    ${SUDO} "${SYSTEMCTL}" enable --now "${t}" && echo "enabled ${t}"
    changed=1
  fi
done

if [ "${changed}" -eq 0 ]; then
  echo "unattended-upgrades: already set up (package installed, ${AUTO##*/} current, timers enabled)"
else
  echo "unattended-upgrades: set up; security updates install daily (log: /var/log/unattended-upgrades/)"
fi
