#!/usr/bin/env bash
# ==============================================================================
# glow Setup — the charmbracelet markdown renderer into ~/opt/bin
# ==============================================================================
# Why this exists:
#   * herdr-file-viewer renders markdown through glow when it is on PATH and
#     falls back to plain text otherwise; `glow README.md` is also useful on
#     its own. Opt-in (gff install.tools.glow).
#   * Ubuntu 24.04 has no glow package and the Charm apt repo would add a
#     third-party apt source to every host, so we fetch the release tarball
#     (MIT; linux + macOS for x86_64, arm64 and 32-bit arm) and verify it
#     against the release's checksums.txt. Fail-closed: no listed checksum,
#     no install.
#   * Pinned (GLOW_VERSION) rather than latest, like install_yq.sh: bumping it
#     is a reviewed one-line change.
#
# Safe to re-run: a host already on GLOW_VERSION is a no-op with no download.
# Env overrides:
#   GLOW_VERSION       release to install (default: 3.0.0)
#   GLOW_INSTALL_DIR   target dir (default: ~/opt/bin)
#   GLOW_FORCE=1       reinstall even when the wanted version is present
#   GLOW_RELEASE_BASE  release download base (default: https://github.com/charmbracelet/glow/releases/download)
set -e

GLOW_VERSION="${GLOW_VERSION:-3.0.0}"
INSTALL_DIR="${GLOW_INSTALL_DIR:-${HOME}/opt/bin}"
RELEASE_BASE="${GLOW_RELEASE_BASE:-https://github.com/charmbracelet/glow/releases/download}"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}$*${NC}"; }
ok()   { echo -e "${GREEN}$*${NC}"; }
die()  { echo -e "${RED}install_glow: $*${NC}" >&2; exit 1; }

TMP_DIR=""
# `if`, not `[ … ] &&`: under set -e a false && in the EXIT trap would turn a
# successful run (nothing to clean) into exit status 1.
cleanup() { if [ -n "${TMP_DIR}" ]; then rm -rf "${TMP_DIR}"; fi; }
trap cleanup EXIT

case "${GLOW_VERSION}" in
    *[!0-9.]*|"") die "GLOW_VERSION must look like 3.0.0 (got '${GLOW_VERSION}')" ;;
esac

case "$(uname -s)" in
    Linux)  GLOW_OS="Linux" ;;
    Darwin) GLOW_OS="Darwin" ;;
    *) die "unsupported OS $(uname -s)" ;;
esac
case "$(uname -m)" in
    x86_64)         GLOW_ARCH="x86_64" ;;
    arm64|aarch64)  GLOW_ARCH="arm64" ;;
    armv7l|armv6l)  GLOW_ARCH="arm" ;;
    *) die "unsupported arch $(uname -m)" ;;
esac

# "glow version v3.0.0 (…)" or "glow version 3.0.0" -> 3.0.0
installed_version() {
    if [ -x "${INSTALL_DIR}/glow" ]; then
        "${INSTALL_DIR}/glow" --version 2>/dev/null | awk '{ v = $3; sub(/^v/, "", v); print v; exit }'
    fi
}

HAVE="$(installed_version)"
if [ "${HAVE}" = "${GLOW_VERSION}" ] && [ "${GLOW_FORCE:-0}" != "1" ]; then
    ok "glow already installed: ${INSTALL_DIR}/glow (${HAVE})"
    exit 0
fi

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum < "$1" | awk '{ print $1 }'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 < "$1" | awk '{ print $1 }'
    elif command -v openssl >/dev/null 2>&1; then
        openssl dgst -sha256 < "$1" | awk '{ print $NF }'
    else
        die "SHA-256 verification requires sha256sum, shasum, or openssl"
    fi
}

command -v curl >/dev/null 2>&1 || die "curl is required to download glow"
STEM="glow_${GLOW_VERSION}_${GLOW_OS}_${GLOW_ARCH}"
URL="${RELEASE_BASE}/v${GLOW_VERSION}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/install_glow.XXXXXX")"

if [ -n "${HAVE}" ]; then
    info "Updating glow ${HAVE} -> ${GLOW_VERSION} (${GLOW_OS}/${GLOW_ARCH}) in ${INSTALL_DIR}..."
else
    info "Installing glow ${GLOW_VERSION} (${GLOW_OS}/${GLOW_ARCH}) to ${INSTALL_DIR}/glow..."
fi
curl -fsSL "${URL}/checksums.txt" -o "${TMP_DIR}/checksums.txt" || die "could not fetch ${URL}/checksums.txt"
SHA="$(awk -v f="${STEM}.tar.gz" '$2 == f || $2 == "*" f { print tolower($1); exit }' "${TMP_DIR}/checksums.txt")"
# Fail closed: the release must list a well-formed checksum for this asset.
if [ "${#SHA}" -ne 64 ] || [ -n "$(printf '%s' "${SHA}" | tr -d '0-9a-f')" ]; then
    die "checksums.txt lists no valid SHA-256 for ${STEM}.tar.gz; refusing to install an unverified binary"
fi
curl -fsSL "${URL}/${STEM}.tar.gz" -o "${TMP_DIR}/glow.tar.gz" || die "download failed: ${URL}/${STEM}.tar.gz"
ACTUAL="$(sha256_of "${TMP_DIR}/glow.tar.gz")"
[ "${ACTUAL}" = "${SHA}" ] || die "checksum mismatch for ${STEM}.tar.gz: expected ${SHA}, got ${ACTUAL}"

tar -xzf "${TMP_DIR}/glow.tar.gz" -C "${TMP_DIR}" || die "could not unpack ${STEM}.tar.gz"
[ -f "${TMP_DIR}/${STEM}/glow" ] || die "${STEM}.tar.gz has no ${STEM}/glow"
chmod +x "${TMP_DIR}/${STEM}/glow"
mkdir -p "${INSTALL_DIR}"
# Clear any stale entry (including a dangling symlink) before moving in.
if [ -e "${INSTALL_DIR}/glow" ] || [ -L "${INSTALL_DIR}/glow" ]; then
    rm -f "${INSTALL_DIR}/glow"
fi
mv "${TMP_DIR}/${STEM}/glow" "${INSTALL_DIR}/glow"
"${INSTALL_DIR}/glow" --version >/dev/null 2>&1 || die "installed binary does not run: ${INSTALL_DIR}/glow"
ok "Success! $("${INSTALL_DIR}/glow" --version | head -1) (sha256 verified)"
