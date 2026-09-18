#!/usr/bin/env bash
# ==============================================================================
# uv Setup — install Astral's `uv` (and its `uvx` tool-runner) into ~/opt/bin
# ==============================================================================
# Why this exists:
#   * The AWS Claude plugins enabled by default in ai/plugins.yaml
#     (deploy-on-aws, aws-serverless, aws-core) launch their MCP servers via
#     `uvx` — the npx equivalent for a PyPI CLI. Nothing installed it, so on
#     a host without it those servers fail to connect at every session start
#     ("Executable not found in $PATH: uvx", dotfiles#312). sync-plugins.sh
#     warns about the gap; this closes it.
#   * macOS installs uv from opt/profiles/Brewfile (brew 'uv').
#   * uv is a single static Rust binary with official release tarballs and a
#     per-asset SHA-256 sidecar (<asset>.sha256), so fetching + verifying it
#     ourselves is the most portable option on Linux/WSL (mirrors
#     install_sops.sh/install_yq.sh's pattern for other static-binary tools
#     apt does not package usefully — Debian/Ubuntu's `uv` lags upstream and
#     WSL has no guaranteed apt source at all).
#
# Safe to re-run. Override the version with UV_VERSION=x.y.z (default:
# latest known-good at the time this script was written).
set -e

UV_VERSION="${UV_VERSION:-0.12.16}"
INSTALL_DIR="${UV_INSTALL_DIR:-${HOME}/opt/bin}"
RELEASE_BASE="${UV_RELEASE_BASE:-https://github.com/astral-sh/uv/releases/download}"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}$*${NC}"; }
ok()   { echo -e "${GREEN}$*${NC}"; }
die()  { echo -e "${RED}install_uv: $*${NC}" >&2; exit 1; }

TMP_DIR=""
# `if`, not `[ … ] &&`: under set -e a false && in the EXIT trap would turn a
# successful run (nothing to clean) into exit status 1.
cleanup() { if [ -n "${TMP_DIR}" ]; then rm -rf "${TMP_DIR}"; fi; }
trap cleanup EXIT

case "${UV_VERSION}" in
    *[!0-9.]*|"") die "UV_VERSION must look like 0.12.16 (got '${UV_VERSION}')" ;;
esac

case "$(uname -s)" in
    Linux) ;;
    *) die "install_uv.sh is Linux-only; macOS installs uv from opt/profiles/Brewfile" ;;
esac
case "$(uname -m)" in
    x86_64)        UV_TARGET="x86_64-unknown-linux-gnu" ;;
    arm64|aarch64) UV_TARGET="aarch64-unknown-linux-gnu" ;;
    armv7l|armv6l)
        # uv publishes 32-bit ARM builds, but no fleet host is 32-bit — skip
        # rather than add asset-matrix surface nothing here exercises.
        info "no fleet host is 32-bit ARM; skipping uv on $(uname -m)"
        exit 0 ;;
    *) die "unsupported arch $(uname -m)" ;;
esac

# "uv 0.12.16 (…)" -> 0.12.16
installed_version() {
    if [ -x "${INSTALL_DIR}/uv" ]; then
        "${INSTALL_DIR}/uv" --version 2>/dev/null | awk '{ print $2 }'
    fi
}

HAVE="$(installed_version)"
if [ "${HAVE}" = "${UV_VERSION}" ] && [ "${UV_FORCE:-0}" != "1" ]; then
    ok "uv already installed: ${INSTALL_DIR}/uv (${HAVE})"
    exit 0
fi

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum < "$1" | awk '{ print $1 }'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 < "$1" | awk '{ print $1 }'
    else
        die "SHA-256 verification requires sha256sum or shasum"
    fi
}

command -v curl >/dev/null 2>&1 || die "curl is required to download uv"
STEM="uv-${UV_TARGET}"
URL="${RELEASE_BASE}/${UV_VERSION}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/install_uv.XXXXXX")"

if [ -n "${HAVE}" ]; then
    info "Updating uv ${HAVE} -> ${UV_VERSION} (${UV_TARGET}) in ${INSTALL_DIR}..."
else
    info "Installing uv ${UV_VERSION} (${UV_TARGET}) to ${INSTALL_DIR}..."
fi

curl -fsSL "${URL}/${STEM}.tar.gz.sha256" -o "${TMP_DIR}/sha256" || die "could not fetch ${URL}/${STEM}.tar.gz.sha256"
SHA="$(awk '{ print tolower($1) }' "${TMP_DIR}/sha256")"
# Fail closed: the release must list a well-formed checksum for this asset.
if [ "${#SHA}" -ne 64 ] || [ -n "$(printf '%s' "${SHA}" | tr -d '0-9a-f')" ]; then
    die "release ${UV_VERSION} lists no valid SHA-256 for ${STEM}.tar.gz; refusing to install an unverified binary"
fi
curl -fsSL "${URL}/${STEM}.tar.gz" -o "${TMP_DIR}/uv.tar.gz" || die "download failed: ${URL}/${STEM}.tar.gz"
ACTUAL="$(sha256_of "${TMP_DIR}/uv.tar.gz")"
[ "${ACTUAL}" = "${SHA}" ] || die "checksum mismatch for ${STEM}.tar.gz: expected ${SHA}, got ${ACTUAL}"

tar -xzf "${TMP_DIR}/uv.tar.gz" -C "${TMP_DIR}" || die "could not unpack ${STEM}.tar.gz"
[ -f "${TMP_DIR}/${STEM}/uv" ] || die "${STEM}.tar.gz has no ${STEM}/uv"
[ -f "${TMP_DIR}/${STEM}/uvx" ] || die "${STEM}.tar.gz has no ${STEM}/uvx"
chmod +x "${TMP_DIR}/${STEM}/uv" "${TMP_DIR}/${STEM}/uvx"

mkdir -p "${INSTALL_DIR}"
for bin in uv uvx; do
    # Clear any stale entry (including a dangling symlink) before moving in.
    if [ -e "${INSTALL_DIR}/${bin}" ] || [ -L "${INSTALL_DIR}/${bin}" ]; then
        rm -f "${INSTALL_DIR}/${bin}"
    fi
    mv "${TMP_DIR}/${STEM}/${bin}" "${INSTALL_DIR}/${bin}"
done

"${INSTALL_DIR}/uv" --version >/dev/null 2>&1 || die "installed binary does not run: ${INSTALL_DIR}/uv"
"${INSTALL_DIR}/uvx" --version >/dev/null 2>&1 || die "installed binary does not run: ${INSTALL_DIR}/uvx"
ok "Success! $("${INSTALL_DIR}/uv" --version | head -1) + uvx (sha256 verified)"
