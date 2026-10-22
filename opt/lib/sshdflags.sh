# shellcheck shell=sh
# sshdflags.sh — one-time migration of the legacy ~/.sshd.env switch into the
# install.network.sshd gff flag (POSIX/dash-safe). Needs opt/lib/gff.sh
# (gff_cli, gff_user_set) sourced first. Test seam: SSHD_ENV_FILE.

# sshd_migrate - always returns 0. Idempotent: the legacy file is removed once
# its meaning is recorded (or found moot), so a later run is a no-op.
sshd_migrate() {
  _sm_file="${SSHD_ENV_FILE:-${HOME}/.sshd.env}"
  [ -f "${_sm_file}" ] || return 0
  if gff_user_set install.network.sshd; then
    rm -f "${_sm_file}"
    echo "Removed stale ~/.sshd.env (install.network.sshd is already set on this machine)."
    return 0
  fi
  # Read the flag textually instead of sourcing a user-edited file.
  if ! grep -Eq "^[[:space:]]*(export[[:space:]]+)?SSHD_LOGIN=[\"']?true[\"']?[[:space:]]*(#.*)?$" "${_sm_file}"; then
    rm -f "${_sm_file}"
    echo "Removed ~/.sshd.env (it did not enable sshd)."
    return 0
  fi
  if gff_cli set install.network.sshd true >/dev/null 2>&1; then
    rm -f "${_sm_file}"
    GFF_INSTALL_NETWORK_SSHD=true
    export GFF_INSTALL_NETWORK_SSHD
    echo "Migrated ~/.sshd.env to gff: install.network.sshd=true (file removed)."
  else
    echo "WARNING: could not record install.network.sshd in gff; kept ~/.sshd.env (retrying next run)."
  fi
  return 0
}
