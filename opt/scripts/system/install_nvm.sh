#!/usr/bin/env bash
# install_nvm.sh — install or update nvm (Node Version Manager) to the pin in
# opt/profiles/versions.tsv.
#
# Converging, like the other pinned installers: an nvm already at the wanted
# version is a no-op (no download); an older one re-runs nvm's own installer
# for the wanted tag, which updates the ~/.nvm git checkout in place. This
# replaces the old install.sh block, which installed v0.39.7 once and never
# touched it again (pins design: the "frozen at first install" mode).
#
# PROFILE=/dev/null: nvm's installer otherwise appends a loader to the detected
# shell rc file — ours are symlinks into the dotfiles repo, which already loads
# nvm itself.
#
# Overrides: NVM_VERSION=x.y.z (or latest), NVM_FORCE=1, NVM_DIR (default
# ~/.nvm), NVM_INSTALL_BASE (release base, for tests).
set -e

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PINS_LIB="${SELF_DIR}/../../lib/pins.sh"
if [ -f "${PINS_LIB}" ]; then
    # shellcheck source=../../lib/pins.sh
    . "${PINS_LIB}"
fi
command -v pins_want >/dev/null 2>&1 || pins_want() { printf '%s\n' "${2:-}"; }
command -v pins_latest_gh >/dev/null 2>&1 || pins_latest_gh() { return 1; }

NVM_VERSION="${NVM_VERSION:-$(pins_want nvm 0.39.7)}"
NVM_DIR="${NVM_DIR:-${HOME}/.nvm}"
NVM_INSTALL_BASE="${NVM_INSTALL_BASE:-https://raw.githubusercontent.com/nvm-sh/nvm}"

BLUE='\033[0;34m'; GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
info() { echo -e "${BLUE}$*${NC}"; }
ok()   { echo -e "${GREEN}$*${NC}"; }
die()  { echo -e "${RED}install_nvm: $*${NC}" >&2; exit 1; }

if [ "${NVM_VERSION}" = "latest" ]; then
    NVM_VERSION="$(pins_latest_gh nvm-sh/nvm)" || die "could not resolve the latest nvm release"
fi
case "${NVM_VERSION}" in
    *[!0-9.]*|"") die "NVM_VERSION must look like 0.39.7 (got '${NVM_VERSION}')" ;;
esac

# installed_version: what this NVM_DIR's nvm reports, or "" when absent.
installed_version() {
    [ -s "${NVM_DIR}/nvm.sh" ] || return 0
    bash -c '. "$1/nvm.sh" >/dev/null 2>&1 && nvm --version' _ "${NVM_DIR}" 2>/dev/null || true
}

HAVE="$(installed_version)"
if [ "${HAVE}" = "${NVM_VERSION}" ] && [ "${NVM_FORCE:-0}" != "1" ]; then
    ok "nvm already at ${HAVE}: ${NVM_DIR}"
    exit 0
fi
command -v curl >/dev/null 2>&1 || die "curl is required"
if [ -n "${HAVE}" ]; then
    info "Updating nvm ${HAVE} -> ${NVM_VERSION} in ${NVM_DIR}..."
else
    info "Installing nvm ${NVM_VERSION} to ${NVM_DIR}..."
fi

TMP_INST="$(mktemp "${TMPDIR:-/tmp}/nvm-install.XXXXXX")"
trap 'rm -f "${TMP_INST}"' EXIT
curl -fsSL "${NVM_INSTALL_BASE}/v${NVM_VERSION}/install.sh" -o "${TMP_INST}" \
    || die "could not fetch ${NVM_INSTALL_BASE}/v${NVM_VERSION}/install.sh"
NVM_DIR="${NVM_DIR}" PROFILE=/dev/null bash "${TMP_INST}" >/dev/null 2>&1 \
    || die "nvm ${NVM_VERSION} installer failed"

GOT="$(installed_version)"
[ "${GOT}" = "${NVM_VERSION}" ] || die "nvm reports '${GOT:-nothing}' after installing ${NVM_VERSION}"
ok "nvm ${GOT} ready: ${NVM_DIR}"
