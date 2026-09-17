#!/usr/bin/env bash
# ==============================================================================
# ollama Setup — install ollama via its own, checksum-verified installer
# ==============================================================================
# Why this exists:
#   * ollama publishes an official installer (https://ollama.com/install.sh)
#     that already detects CUDA / ROCm / Jetson JetPack capability correctly.
#     This script does NOT reimplement that detection — doing so would just be
#     a worse, unverified copy of logic ollama already gets right. What this
#     adds is what the upstream curl-into-shell one-liner does not: pin an
#     exact release, verify the SHA-256 ollama publishes for that release's
#     install.sh (github.com/ollama/ollama/releases, sha256sum.txt), and skip
#     the network entirely once the host is already at the pin. Same trust
#     model as install_herdr.sh / install_glow.sh, applied to a script asset
#     instead of a binary one: fetch the release asset ourselves, verify it,
#     THEN run the LOCAL verified copy — never pipe a remote download
#     straight into a shell.
#   * Pinned (OLLAMA_VERSION), not "latest", like install_glow.sh/install_yq.sh:
#     playground#379's tune.sh memory budgets (ollama/scripts/tune.sh) are
#     measured against one release's behavior, so bumping the pin is a
#     reviewed one-line change, not silent drift.
#   * ollama's own install.sh reads OLLAMA_VERSION itself to pick which
#     release payload it fetches — we pass our pinned version through to it,
#     so the verified installer script and the binary it installs are always
#     the same release.
#
# Safe to re-run: a host already on OLLAMA_VERSION is a no-op with no
# download.
#
# Env overrides:
#   OLLAMA_VERSION       release to install, e.g. 0.34.1 (default: 0.34.1)
#   OLLAMA_FORCE=1        reinstall even when the wanted version is present
#   OLLAMA_RELEASE_BASE   release download base (default:
#                         https://github.com/ollama/ollama/releases/download)
set -e

OLLAMA_VERSION="${OLLAMA_VERSION:-0.34.1}"
RELEASE_BASE="${OLLAMA_RELEASE_BASE:-https://github.com/ollama/ollama/releases/download}"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}$*${NC}"; }
ok()   { echo -e "${GREEN}$*${NC}"; }
die()  { echo -e "${RED}install_ollama: $*${NC}" >&2; exit 1; }

case "${OLLAMA_VERSION}" in
    *[!0-9.]*|"") die "OLLAMA_VERSION must look like 0.34.1 (got '${OLLAMA_VERSION}')" ;;
esac

case "$(uname -s)" in
    Linux) ;;
    *) die "install_ollama.sh manages Linux GPU-node hosts only; got $(uname -s)" ;;
esac

# "ollama version is 0.34.1" (client-only) or with a leading warning line when
# the client can't reach a running server — either way the version is the
# first dotted-number token.
installed_version() {
    command -v ollama >/dev/null 2>&1 || return 0
    ollama --version 2>&1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1
}

HAVE="$(installed_version)"
if [ "${HAVE}" = "${OLLAMA_VERSION}" ] && [ "${OLLAMA_FORCE:-0}" != "1" ]; then
    ok "ollama already installed: $(command -v ollama) (${HAVE})"
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

command -v curl >/dev/null 2>&1 || die "curl is required to download ollama's installer"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/install_ollama.XXXXXX")"
# `if`, not `[ … ] &&`: under set -e a false && in the EXIT trap would turn a
# successful run (nothing to clean) into exit status 1.
cleanup() { if [ -n "${TMP_DIR}" ]; then rm -rf "${TMP_DIR}"; fi; }
trap cleanup EXIT

URL="${RELEASE_BASE}/v${OLLAMA_VERSION}"

if [ -n "${HAVE}" ]; then
    info "Updating ollama ${HAVE} -> ${OLLAMA_VERSION} via the pinned, verified installer..."
else
    info "Installing ollama ${OLLAMA_VERSION} via the pinned, verified installer..."
fi

curl -fsSL "${URL}/sha256sum.txt" -o "${TMP_DIR}/sha256sum.txt" || die "could not fetch ${URL}/sha256sum.txt"
# ollama's sha256sum.txt lines are `<hash>  ./<name>` — strip a leading ./ so
# the match is exact rather than a substring that could hit a longer name.
SHA="$(awk '{ n = $2; sub(/^\.\//, "", n); if (n == "install.sh") { print tolower($1); exit } }' "${TMP_DIR}/sha256sum.txt")"
# Fail closed: the release must list a well-formed checksum for install.sh.
if [ "${#SHA}" -ne 64 ] || [ -n "$(printf '%s' "${SHA}" | tr -d '0-9a-f')" ]; then
    die "release v${OLLAMA_VERSION} lists no valid SHA-256 for install.sh in sha256sum.txt; refusing to run an unverified installer"
fi

curl -fsSL "${URL}/install.sh" -o "${TMP_DIR}/install.sh" || die "download failed: ${URL}/install.sh"
ACTUAL="$(sha256_of "${TMP_DIR}/install.sh")"
[ "${ACTUAL}" = "${SHA}" ] || die "checksum mismatch for v${OLLAMA_VERSION}'s install.sh: expected ${SHA}, got ${ACTUAL}"

chmod +x "${TMP_DIR}/install.sh"
OLLAMA_VERSION="${OLLAMA_VERSION}" sh "${TMP_DIR}/install.sh" || die "the verified ollama installer exited non-zero"

command -v ollama >/dev/null 2>&1 || die "install completed but ollama is not on PATH"
ok "Success! $(ollama --version 2>&1 | head -1) (sha256 verified)"
