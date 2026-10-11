#!/usr/bin/env bash
# ==============================================================================
# SOPS Binary Setup — install the `sops` secrets-management tool into ~/opt/bin
# ==============================================================================
# Why this exists:
#   * macOS installs sops from opt/profiles/Brewfile (brew 'sops').
#   * Linux/WSL has no usable apt package (Ubuntu ships only the unrelated
#     `orca-sops`), so the cross-platform packages.tsv path can't provide it.
#   * sops is a single static Go binary, so fetching the official release binary
#     is the most portable option — and it survives goenv Go-version bumps that
#     previously orphaned a `go install`-based symlink in ~/opt/bin.
#
# Converges (objective `pins`): the version comes from opt/profiles/versions.tsv
# (pins_want sops); a host whose ~/opt/bin/sops differs is re-downloaded, a host
# already on it is a no-op. A sops that lives OUTSIDE ~/opt/bin (brew/apt) is
# never touched — another package manager owns it.
#
# Environment:
#   SOPS_VERSION       version to install (default: versions.tsv, else 3.13.1)
#   SOPS_FORCE=1       reinstall even when already at SOPS_VERSION
#   SOPS_INSTALL_DIR   where the binary goes (default ~/opt/bin)
#   SOPS_RELEASE_BASE  release URL base (tests point it at a file:// fixture)
set -e

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PINS_LIB="${SELF_DIR}/../../lib/pins.sh"
if [ -f "${PINS_LIB}" ]; then
    # shellcheck source=../../lib/pins.sh
    . "${PINS_LIB}"
fi
# Fail-open: without the lib, the hardcoded default below still applies.
command -v pins_want >/dev/null 2>&1 || pins_want() { printf '%s\n' "${2:-}"; }
command -v pins_latest_gh >/dev/null 2>&1 || pins_latest_gh() { return 1; }

SOPS_VERSION="${SOPS_VERSION:-$(pins_want sops 3.13.1)}"
SOPS_VERSION="${SOPS_VERSION#v}"
INSTALL_DIR="${SOPS_INSTALL_DIR:-${HOME}/opt/bin}"
RELEASE_BASE="${SOPS_RELEASE_BASE:-https://github.com/getsops/sops/releases/download}"
OWN="${INSTALL_DIR}/sops"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}$*${NC}"; }
ok()   { echo -e "${GREEN}$*${NC}"; }
die()  { echo -e "${RED}install_sops: $*${NC}" >&2; exit 1; }

# "sops 3.13.1 (latest)" -> 3.13.1
# --disable-version-check: plain `sops --version` asks GitHub for the latest
# release on every call (and waits on it offline).
version_of() { "$1" --version --disable-version-check 2>/dev/null | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -n 1 || true; }

# Ownership: a runnable sops on PATH that is NOT ours belongs to brew/apt —
# report it and leave it alone (SOPS_FORCE=1 installs ours anyway).
FOUND="$(command -v sops 2>/dev/null || true)"
if [ "${SOPS_FORCE:-0}" != "1" ] && [ ! -x "${OWN}" ] && [ -n "${FOUND}" ] \
        && [ "${FOUND}" != "${OWN}" ] && "${FOUND}" --version >/dev/null 2>&1; then
    info "sops at ${FOUND} ($(version_of "${FOUND}")) is managed outside ${INSTALL_DIR}; leaving it alone"
    exit 0
fi

# --- OS / arch detection (matches the getsops release asset naming) ---
case "$(uname -s)" in
    Linux)  SOPS_OS="linux"  ;;
    Darwin) SOPS_OS="darwin" ;;
    *) die "unsupported OS $(uname -s)" ;;
esac
case "$(uname -m)" in
    x86_64)        SOPS_ARCH="amd64" ;;
    arm64|aarch64) SOPS_ARCH="arm64" ;;
    *) die "unsupported arch $(uname -m)" ;;
esac

if [ "${SOPS_VERSION}" = "latest" ]; then
    SOPS_VERSION="$(pins_latest_gh getsops/sops)" || die "could not resolve the latest sops release"
fi
case "${SOPS_VERSION}" in
    *[!0-9.]*|"") die "SOPS_VERSION must look like 3.13.1 (got '${SOPS_VERSION}')" ;;
esac

# Probing with `--version` (not just -x) also means a dangling symlink — e.g. an
# old `go install` link into a now-removed goenv Go version — reads as "not
# installed" and gets repaired below.
HAVE=""
if [ -x "${OWN}" ]; then HAVE="$(version_of "${OWN}")"; fi
if [ "${HAVE}" = "${SOPS_VERSION}" ] && [ "${SOPS_FORCE:-0}" != "1" ]; then
    ok "sops already at ${HAVE}: ${OWN}"
    exit 0
fi

command -v curl >/dev/null 2>&1 || die "curl is required to download sops"
mkdir -p "${INSTALL_DIR}"
URL="${RELEASE_BASE}/v${SOPS_VERSION}/sops-v${SOPS_VERSION}.${SOPS_OS}.${SOPS_ARCH}"
if [ -n "${HAVE}" ]; then
    info "Updating sops ${HAVE} -> ${SOPS_VERSION} (${SOPS_OS}/${SOPS_ARCH}) in ${INSTALL_DIR}..."
else
    info "Installing sops ${SOPS_VERSION} (${SOPS_OS}/${SOPS_ARCH}) to ${OWN}..."
fi
TMP_BIN="$(mktemp "${INSTALL_DIR}/.sops.XXXXXX")"
curl -fsSL "${URL}" -o "${TMP_BIN}" || { rm -f "${TMP_BIN}"; die "download failed: ${URL}"; }
chmod +x "${TMP_BIN}"
"${TMP_BIN}" --version --disable-version-check >/dev/null 2>&1 || { rm -f "${TMP_BIN}"; die "downloaded binary does not run: ${URL}"; }

# Clear any stale entry (including a dangling symlink) before moving the new
# binary in. `-e` is false for a dangling symlink, so test `-L` too.
if [ -e "${OWN}" ] || [ -L "${OWN}" ]; then
    rm -f "${OWN}"
fi
mv "${TMP_BIN}" "${OWN}"

ok "Success! $("${OWN}" --version --disable-version-check | head -1)"
