#!/usr/bin/env bash
# sshd_harden.sh — the dotfiles sshd hardening drop-in (gff install.network.sshd-hardening).
#
# Writes /etc/ssh/sshd_config.d/10-dotfiles-hardening.conf:
#   PasswordAuthentication no, KbdInteractiveAuthentication no, PermitEmptyPasswords no,
#   PermitRootLogin no, PubkeyAuthentication yes, MaxAuthTries 3, LoginGraceTime 30,
#   AllowUsers <this user>   (SSHD_ALLOW_USERS="a b" widens it; SSHD_ALLOW_USERS=- omits it)
# then validates with `sshd -t`, reloads sshd, and reports the effective values from
# `sshd -T`. Named 10- so it sorts before the distro's 50-cloud-init.conf: in
# sshd_config the FIRST value a keyword gets wins.
#
# Safety:
#   * REFUSES (exit 2, nothing written) while this user has no key in
#     ~/.ssh/authorized_keys: passwords go off only when a key can still get in.
#   * `sshd -t` runs before the reload; a failing check rolls the file back (exit 1).
#   * SKIPs (exit 0) where there is no sshd, no sshd_config, or no
#     `Include sshd_config.d` line (a drop-in would be ignored), and off Linux.
#   * Idempotent: an unchanged drop-in means no write and no reload.
#   * A file at the drop-in path that is not ours is backed up (.bak-<timestamp>) first.
#
# Usage: sshd_harden.sh [--dry-run | --status | --uninstall]
# Env (tests and odd hosts): SSHD_HARDEN_ETC (/etc/ssh), SSHD_HARDEN_SSHD (/usr/sbin/sshd),
#   SSHD_HARDEN_SYSTEMCTL (systemctl), SSHD_HARDEN_KEYS (~/.ssh/authorized_keys),
#   SSHD_HARDEN_SUDO ("sudo -n" when not root; set it empty to run without sudo).
# Companion test: sshd_harden_test.sh. Docs: docs/sshd-hardening.md.
set -u

MODE=apply
for arg in "$@"; do
  case "${arg}" in
    --dry-run) MODE=dry ;;
    --status) MODE=status ;;
    --uninstall) MODE=uninstall ;;
    -h|--help) sed -n '2,25p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "sshd_harden.sh: unknown argument '${arg}' (use --dry-run | --status | --uninstall)" >&2; exit 64 ;;
  esac
done

ETC="${SSHD_HARDEN_ETC:-/etc/ssh}"
CONF="${ETC}/sshd_config"
DROPIN_DIR="${ETC}/sshd_config.d"
DROPIN="${DROPIN_DIR}/10-dotfiles-hardening.conf"
SSHD="${SSHD_HARDEN_SSHD:-/usr/sbin/sshd}"
SYSTEMCTL="${SSHD_HARDEN_SYSTEMCTL:-systemctl}"
KEYS="${SSHD_HARDEN_KEYS:-${HOME}/.ssh/authorized_keys}"
ALLOW_USERS="${SSHD_ALLOW_USERS:-$(id -un)}"
if [ -n "${SSHD_HARDEN_SUDO+x}" ]; then SUDO="${SSHD_HARDEN_SUDO}"
elif [ "$(id -u)" -eq 0 ]; then SUDO=""
else SUDO="sudo -n"; fi

skip() { echo "SKIP: sshd hardening: $*"; exit 0; }

# --- preconditions -----------------------------------------------------------
[ "$(uname -s)" = "Linux" ] || skip "only Linux sshd is managed here ($(uname -s))"
[ -f "${CONF}" ] || skip "no ${CONF} (sshd is not installed)"
[ -x "${SSHD}" ] || skip "no sshd binary at ${SSHD}"
grep -qE '^[[:space:]]*Include[[:space:]]+.*sshd_config\.d/' "${CONF}" ||
  skip "${CONF} has no Include of sshd_config.d, so a drop-in would be ignored; add 'Include ${DROPIN_DIR}/*.conf' at its top first"
if [ -n "${SUDO}" ] && ! ${SUDO} true 2>/dev/null; then
  echo "WARNING: sshd hardening needs sudo and it wants a password; run 'sudo -v' then '$0' interactively."
  exit 0
fi

# --- the rendered drop-in ----------------------------------------------------
render() {
  cat <<RENDER
# Managed by dotfiles: opt/scripts/network/sshd_harden.sh (gff install.network.sshd-hardening).
# Edits here are overwritten on the next install.sh. To remove it:
#   gff set install.network.sshd-hardening false && sshd_harden.sh --uninstall
# Named 10- so it sorts before the distro's 50-cloud-init.conf: the first value wins.
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitEmptyPasswords no
PermitRootLogin no
PubkeyAuthentication yes
MaxAuthTries 3
LoginGraceTime 30
RENDER
  [ "${ALLOW_USERS}" = "-" ] || echo "AllowUsers ${ALLOW_USERS}"
}

EFFECTIVE_KEYS='passwordauthentication|kbdinteractiveauthentication|permitemptypasswords|permitrootlogin|pubkeyauthentication|maxauthtries|logingracetime|allowusers'
effective() {  # prints the effective values sshd will use; empty when sshd -T cannot run
  ${SUDO} "${SSHD}" -T 2>/dev/null | grep -E "^(${EFFECTIVE_KEYS}) "
}
report_effective() {  # report_effective [check]: with "check", warn when the drop-in is not what wins
  _check="${1:-}"
  _eff="$(effective)"
  if [ -z "${_eff}" ]; then
    echo "note: 'sshd -T' gave no answer here (it needs root for the host keys); effective values not checked"
    return 0
  fi
  echo "effective (sshd -T):"; printf '%s\n' "${_eff}" | sed 's/^/  /'
  [ "${_check}" = check ] || return 0
  for want in "passwordauthentication no" "kbdinteractiveauthentication no" "permitrootlogin no"; do
    _key="${want% *}"
    _got="$(printf '%s\n' "${_eff}" | awk -v k="${_key}" '$1 == k { print $2; exit }')"
    [ -n "${_got}" ] && [ "${_got}" != "${want#* }" ] &&
      echo "WARNING: effective ${_key} is '${_got}', not '${want#* }': an earlier directive in ${CONF} or a drop-in that sorts before ${DROPIN##*/} wins; fix that file."
  done
  return 0
}

key_count() {  # authorized keys this user can log in with
  [ -r "${KEYS}" ] || { echo 0; return; }
  # grep -c prints the count even when it is 0 (and exits 1), so no fallback echo here.
  _n="$(grep -cE '^[[:space:]]*(ssh-(rsa|ed25519|dss)|ecdsa-sha2-[a-z0-9-]+|sk-(ssh-ed25519|ecdsa-sha2-[a-z0-9-]+)@openssh\.com)[[:space:]]' "${KEYS}" 2>/dev/null)"
  echo "${_n:-0}"
}

reload_sshd() {
  if ! command -v "${SYSTEMCTL%% *}" >/dev/null 2>&1 && [ ! -x "${SYSTEMCTL}" ]; then
    echo "WARNING: no systemctl; reload sshd by hand (kill -HUP \$(pgrep -o -x sshd))"
    return 0
  fi
  _svc=sshd
  ${SUDO} "${SYSTEMCTL}" is-active --quiet ssh 2>/dev/null && _svc=ssh
  if ${SUDO} "${SYSTEMCTL}" reload "${_svc}" 2>/dev/null; then
    echo "reloaded ${_svc}"
  elif ${SUDO} "${SYSTEMCTL}" try-restart "${_svc}" 2>/dev/null; then
    echo "restarted ${_svc} (reload was refused)"
  else
    echo "WARNING: could not reload ${_svc}; the drop-in applies at the next sshd restart"
  fi
}

# --- modes -------------------------------------------------------------------
case "${MODE}" in
  status)
    _applied=""
    if [ -f "${DROPIN}" ]; then
      _applied=check
      if render | cmp -s - "${DROPIN}"; then echo "sshd hardening: applied (${DROPIN})"
      else echo "sshd hardening: applied, but ${DROPIN} differs from the current render (re-run to update)"; fi
    else
      echo "sshd hardening: not applied (no ${DROPIN})"
    fi
    echo "authorized keys for $(id -un): $(key_count) in ${KEYS}"
    report_effective ${_applied}
    exit 0 ;;
  dry)
    echo "sshd hardening --dry-run: would write ${DROPIN}:"
    render
    if [ "$(key_count)" -eq 0 ]; then
      echo "note: $(id -un) has no key in ${KEYS}; a real run would REFUSE (exit 2) until one is added (sshd-setup keys)."
    fi
    if [ -f "${DROPIN}" ] && render | cmp -s - "${DROPIN}"; then echo "(already in place, unchanged)"; fi
    exit 0 ;;
  uninstall)
    if [ -f "${DROPIN}" ]; then
      ${SUDO} rm -f "${DROPIN}" && echo "removed ${DROPIN}"
      reload_sshd
    else
      echo "sshd hardening: nothing to remove (no ${DROPIN})"
    fi
    exit 0 ;;
esac

# --- apply -------------------------------------------------------------------
if [ "$(key_count)" -eq 0 ]; then
  echo "REFUSED: sshd hardening would turn password login off, but $(id -un) has no key in ${KEYS}."
  echo "         Add one first (opt/bin/sshd-setup keys, or paste a public key), then re-run."
  exit 2
fi

TMP="$(mktemp "${TMPDIR:-/tmp}/sshd-harden.XXXXXX")"
trap 'rm -f "${TMP}"' EXIT
render > "${TMP}"

if [ -f "${DROPIN}" ] && cmp -s "${TMP}" "${DROPIN}"; then
  echo "sshd hardening: already applied (${DROPIN} unchanged, no reload)"
  report_effective check
  exit 0
fi

BACKUP=""
if [ -f "${DROPIN}" ]; then
  BACKUP="${DROPIN}.bak-$(date +%Y%m%d%H%M%S)"
  ${SUDO} cp -p "${DROPIN}" "${BACKUP}" && echo "kept the previous file as ${BACKUP}"
fi
${SUDO} mkdir -p "${DROPIN_DIR}"
if ! ${SUDO} install -m 0644 "${TMP}" "${DROPIN}"; then
  echo "ERROR: could not write ${DROPIN}" >&2; exit 1
fi

if ! ${SUDO} "${SSHD}" -t; then
  echo "ERROR: 'sshd -t' rejected the configuration with ${DROPIN##*/} in place; rolling it back, sshd untouched." >&2
  if [ -n "${BACKUP}" ]; then ${SUDO} mv -f "${BACKUP}" "${DROPIN}"; else ${SUDO} rm -f "${DROPIN}"; fi
  exit 1
fi

echo "wrote ${DROPIN} (AllowUsers: ${ALLOW_USERS})"
reload_sshd
report_effective check
