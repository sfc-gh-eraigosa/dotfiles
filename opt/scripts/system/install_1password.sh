#!/usr/bin/env bash
# ==============================================================================
# 1Password CLI Setup — install `op` into ~/opt/bin (Linux) or via brew (macOS)
# ==============================================================================
# Why this exists:
#   * `op` is 1Password's command-line client: `op read op://vault/item/field`,
#     `op run -- <cmd>` to inject secrets as env vars, service-account tokens
#     for headless hosts. Opt-in (gff install.tools.1password, off by default):
#     only hosts with a 1Password account want a 45 MB proprietary binary.
#   * No distro packages it. 1Password's apt repo would add a third-party
#     source to every host, so Linux fetches the release zip 1Password
#     publishes for linux × 386/amd64/arm/arm64 and VERIFIES the detached GPG
#     signature shipped inside it (op.sig) against 1Password's signing key —
#     the fingerprint is pinned here and the key itself is fetched from
#     1Password's own host, never a keyserver. Fail-closed: no gpg, no key, a
#     bad or foreign signature => nothing is installed.
#   * macOS uses the Homebrew cask: the desktop app's CLI integration
#     (biometric unlock) wants `op` at /usr/local/bin with the cask's
#     ownership, which a ~/opt/bin copy cannot provide.
#   * Pinned (OP_VERSION) rather than latest, like glow/yq: bumping the
#     versions.tsv row is a reviewed one-line change (fleet pins bump).
#
# Not done here: the Linux desktop-app integration (`groupadd onepassword-cli`,
# setgid on the binary) — that is for hosts running the 1Password GUI; the
# CLI signs in on its own (`op account add`, or OP_SERVICE_ACCOUNT_TOKEN).
#
# Safe to re-run: a host already on OP_VERSION is a no-op with no download. An
# `op` that lives OUTSIDE ~/opt/bin (brew, a hand install in /usr/local/bin) is
# left alone — another package manager owns it.
#
# Environment:
#   OP_VERSION          release to install (default: versions.tsv `1password-cli`
#                       row, else 2.41.0); `latest` asks 1Password's update feed
#   OP_INSTALL_DIR      target dir (default: ~/opt/bin)
#   OP_FORCE=1          reinstall even when the wanted version is present
#   OP_RELEASE_BASE     download base (default:
#                       https://cache.agilebits.com/dist/1P/op2/pkg)
#   OP_KEY_URL          where to fetch the signing public key (default:
#                       https://downloads.1password.com/linux/keys/1password.asc)
#   OP_KEY_FINGERPRINT  the fingerprint the signature must carry (default: the
#                       1Password release-signing key)
#   OP_UPDATE_FEED      JSON feed `latest` resolves through (default:
#                       https://app-updates.agilebits.com/check/1/0/CLI2/en/2.0.0/N)
set -e

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PINS_LIB="${SELF_DIR}/../../lib/pins.sh"
if [ -f "${PINS_LIB}" ]; then
    # shellcheck source=../../lib/pins.sh
    . "${PINS_LIB}"
fi
# Fail-open: without the lib (or the manifest) the hardcoded default applies.
command -v pins_want >/dev/null 2>&1 || pins_want() { printf '%s\n' "${2:-}"; }

OP_VERSION="${OP_VERSION:-$(pins_want 1password-cli 2.41.0)}"
OP_VERSION="${OP_VERSION#v}"
INSTALL_DIR="${OP_INSTALL_DIR:-${HOME}/opt/bin}"
RELEASE_BASE="${OP_RELEASE_BASE:-https://cache.agilebits.com/dist/1P/op2/pkg}"
KEY_URL="${OP_KEY_URL:-https://downloads.1password.com/linux/keys/1password.asc}"
KEY_FPR="${OP_KEY_FINGERPRINT:-3FEF9748469ADBE15DA7CA80AC2D62742012EA22}"
UPDATE_FEED="${OP_UPDATE_FEED:-https://app-updates.agilebits.com/check/1/0/CLI2/en/2.0.0/N}"
OWN="${INSTALL_DIR}/op"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}$*${NC}"; }
ok()   { echo -e "${GREEN}$*${NC}"; }
die()  { echo -e "${RED}install_1password: $*${NC}" >&2; exit 1; }

TMP_DIR=""
# `if`, not `[ … ] &&`: under set -e a false && in the EXIT trap would turn a
# successful run (nothing to clean) into exit status 1.
cleanup() { if [ -n "${TMP_DIR}" ]; then rm -rf "${TMP_DIR}"; fi; }
trap cleanup EXIT

# "2.41.0" (op --version prints the bare version)
version_of() { "$1" --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -n 1 || true; }

# --- macOS: the Homebrew cask owns it -----------------------------------------
if [ "$(uname -s)" = "Darwin" ]; then
    if command -v op >/dev/null 2>&1 && [ "${OP_FORCE:-0}" != "1" ]; then
        ok "1password-cli already installed: $(command -v op) ($(version_of op))"
        exit 0
    fi
    command -v brew >/dev/null 2>&1 || die "Homebrew not found — install it first: https://brew.sh/"
    info "Installing 1password-cli via brew (cask)..."
    brew install --cask 1password-cli
    command -v op >/dev/null 2>&1 || die "op not resolvable after brew install"
    ok "Success! 1password-cli $(version_of op) at $(command -v op)"
    exit 0
fi
[ "$(uname -s)" = "Linux" ] || die "unsupported OS $(uname -s)"

# --- Linux: verified release zip into ~/opt/bin --------------------------------
# Ownership: a runnable op on PATH that is NOT ours belongs to another package
# manager (or a hand install) — report it and leave it alone (OP_FORCE=1
# installs ours anyway).
FOUND="$(command -v op 2>/dev/null || true)"
if [ "${OP_FORCE:-0}" != "1" ] && [ ! -x "${OWN}" ] && [ -n "${FOUND}" ] \
        && [ "${FOUND}" != "${OWN}" ] && "${FOUND}" --version >/dev/null 2>&1; then
    info "op at ${FOUND} ($(version_of "${FOUND}")) is managed outside ${INSTALL_DIR}; leaving it alone"
    exit 0
fi

case "$(uname -m)" in
    x86_64)         OP_ARCH="amd64" ;;
    arm64|aarch64)  OP_ARCH="arm64" ;;
    armv7l|armv6l)  OP_ARCH="arm" ;;
    i386|i686)      OP_ARCH="386" ;;
    *) die "unsupported arch $(uname -m)" ;;
esac

command -v curl >/dev/null 2>&1 || die "curl is required to download the 1Password CLI"
if [ "${OP_VERSION}" = "latest" ]; then
    # {"available":"1","version":"2.41.0",...}
    OP_VERSION="$(curl -fsSL "${UPDATE_FEED}" 2>/dev/null \
        | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
    [ -n "${OP_VERSION}" ] || die "could not resolve the latest 1Password CLI release from ${UPDATE_FEED}"
fi
case "${OP_VERSION}" in
    *[!0-9.]*|"") die "OP_VERSION must look like 2.41.0 (got '${OP_VERSION}')" ;;
esac
case "${KEY_FPR}" in
    *[!0-9A-Fa-f]*|"") die "OP_KEY_FINGERPRINT must be a hex fingerprint (got '${KEY_FPR}')" ;;
esac
KEY_FPR="$(printf '%s' "${KEY_FPR}" | tr '[:lower:]' '[:upper:]')"

HAVE=""
if [ -x "${OWN}" ]; then HAVE="$(version_of "${OWN}")"; fi
if [ "${HAVE}" = "${OP_VERSION}" ] && [ "${OP_FORCE:-0}" != "1" ]; then
    ok "1password-cli already installed: ${OWN} (${HAVE})"
    exit 0
fi

command -v unzip >/dev/null 2>&1 || die "unzip is required to unpack the 1Password CLI release"
command -v gpg >/dev/null 2>&1 || die "gpg is required to verify the 1Password CLI signature; refusing to install an unverified binary"

ASSET="op_linux_${OP_ARCH}_v${OP_VERSION}.zip"
URL="${RELEASE_BASE}/v${OP_VERSION}/${ASSET}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/install_1password.XXXXXX")"

if [ -n "${HAVE}" ]; then
    info "Updating 1password-cli ${HAVE} -> ${OP_VERSION} (linux/${OP_ARCH}) in ${INSTALL_DIR}..."
else
    info "Installing 1password-cli ${OP_VERSION} (linux/${OP_ARCH}) to ${OWN}..."
fi
curl -fsSL "${URL}" -o "${TMP_DIR}/${ASSET}" || die "download failed: ${URL}"
unzip -q "${TMP_DIR}/${ASSET}" -d "${TMP_DIR}/unpacked" || die "could not unpack ${ASSET}"
[ -f "${TMP_DIR}/unpacked/op" ]     || die "${ASSET} has no op binary"
[ -f "${TMP_DIR}/unpacked/op.sig" ] || die "${ASSET} has no op.sig; refusing to install an unverified binary"

# Signature check in a throwaway keyring: the host's gpg trust store is never
# read or written. The key comes from 1Password's own host; the signature must
# then be VALID and made by exactly the pinned fingerprint.
export GNUPGHOME="${TMP_DIR}/gnupg"
mkdir -p "${GNUPGHOME}" && chmod 700 "${GNUPGHOME}"
curl -fsSL "${KEY_URL}" -o "${TMP_DIR}/signing-key.asc" || die "could not fetch the 1Password signing key from ${KEY_URL}"
gpg --batch --quiet --import "${TMP_DIR}/signing-key.asc" 2>/dev/null || die "could not import the signing key from ${KEY_URL}"
STATUS="$(gpg --batch --status-fd 1 --verify "${TMP_DIR}/unpacked/op.sig" "${TMP_DIR}/unpacked/op" 2>/dev/null || true)"
# [GNUPG:] VALIDSIG <fingerprint> <date> <ts> ... <primary fingerprint>
SIGNER="$(printf '%s\n' "${STATUS}" | awk '$1 == "[GNUPG:]" && $2 == "VALIDSIG" { print toupper($3); exit }')"
if [ -z "${SIGNER}" ]; then
    die "op.sig is not a valid signature for the downloaded op (${ASSET}); refusing to install"
fi
if [ "${SIGNER}" != "${KEY_FPR}" ]; then
    die "op.sig was made by ${SIGNER}, not the pinned 1Password key ${KEY_FPR}; refusing to install"
fi

chmod +x "${TMP_DIR}/unpacked/op"
"${TMP_DIR}/unpacked/op" --version >/dev/null 2>&1 || die "downloaded binary does not run: ${URL}"
mkdir -p "${INSTALL_DIR}"
# Clear any stale entry (including a dangling symlink) before moving in.
if [ -e "${OWN}" ] || [ -L "${OWN}" ]; then
    rm -f "${OWN}"
fi
mv "${TMP_DIR}/unpacked/op" "${OWN}"
ok "Success! 1password-cli $(version_of "${OWN}") at ${OWN} (gpg signature verified, key ${KEY_FPR})"
