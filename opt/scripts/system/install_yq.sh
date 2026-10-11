#!/usr/bin/env bash
# ==============================================================================
# yq Binary Setup — install mikefarah `yq` (YAML processor) into ~/opt/bin
# ==============================================================================
# Why this exists:
#   * macOS installs yq from opt/profiles/packages.tsv (brew = mikefarah yq).
#   * Debian/Ubuntu/WSL apt ships `yq` as the unrelated *kislyuk Python* wrapper
#     (different, jq-style syntax), so the packages.tsv apt path can't provide
#     the tool sync-plugins.sh expects.
#   * mikefarah yq is a single static Go binary, so fetching the official
#     release is the most portable option (mirrors install_sops.sh).
#
# Converges (objective `pins`): the version comes from opt/profiles/versions.tsv
# (pins_want yq); a host whose ~/opt/bin/yq differs is re-downloaded, a host
# already on it is a no-op. A mikefarah yq OUTSIDE ~/opt/bin (brew) is left
# alone; the unrelated kislyuk yq (apt) is shadowed by ours, never modified.
#
# Environment:
#   YQ_VERSION       version to install (default: versions.tsv, else 4.44.6)
#   YQ_FORCE=1       reinstall even when already at YQ_VERSION
#   YQ_INSTALL_DIR   where the binary goes (default ~/opt/bin)
#   YQ_RELEASE_BASE  release URL base (tests point it at a file:// fixture)
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

YQ_VERSION="${YQ_VERSION:-$(pins_want yq 4.44.6)}"
YQ_VERSION="${YQ_VERSION#v}"
INSTALL_DIR="${YQ_INSTALL_DIR:-${HOME}/opt/bin}"
RELEASE_BASE="${YQ_RELEASE_BASE:-https://github.com/mikefarah/yq/releases/download}"
OWN="${INSTALL_DIR}/yq"

# Put the install dir on PATH for the rest of this script. Without it, both the
# "already installed" probe and the post-install verification below ask a PATH
# that may not contain ~/opt/bin at all — which is exactly what happens when
# install.sh runs from a NON-login shell (`fleet update` over ssh): yq gets
# written correctly, then reports "not resolvable after install", and the
# downstream yq consumers (sync-plugins, install_ai_teams) fail outright.
case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *) PATH="${INSTALL_DIR}:${PATH}"; export PATH ;;
esac

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}$*${NC}"; }
ok()   { echo -e "${GREEN}$*${NC}"; }
die()  { echo -e "${RED}install_yq: $*${NC}" >&2; exit 1; }

is_mikefarah() { "$1" --version 2>&1 | grep -qi mikefarah; }
# "yq (https://github.com/mikefarah/yq/) version v4.44.6" -> 4.44.6
version_of() { "$1" --version 2>&1 | sed -n 's/.*version v\{0,1\}\([0-9][0-9.]*\).*/\1/p' | head -n 1 || true; }

# Ownership: a mikefarah yq on PATH that is NOT ours belongs to brew — report it
# and leave it alone (YQ_FORCE=1 installs ours anyway). A stale kislyuk yq or a
# dangling symlink is not the tool we want, so we install ours in front of it.
FOUND="$(command -v yq 2>/dev/null || true)"
if [ "${YQ_FORCE:-0}" != "1" ] && [ ! -x "${OWN}" ] && [ -n "${FOUND}" ] \
        && [ "${FOUND}" != "${OWN}" ] && is_mikefarah "${FOUND}"; then
    info "yq at ${FOUND} ($(version_of "${FOUND}")) is managed outside ${INSTALL_DIR}; leaving it alone"
    exit 0
fi

# --- OS / arch detection (matches the mikefarah/yq release asset naming) ---
case "$(uname -s)" in
    Linux)  YQ_OS="linux"  ;;
    Darwin) YQ_OS="darwin" ;;
    *) die "unsupported OS $(uname -s)" ;;
esac
case "$(uname -m)" in
    x86_64)         YQ_ARCH="amd64" ;;
    arm64|aarch64)  YQ_ARCH="arm64" ;;
    armv7l|armv6l)  YQ_ARCH="arm"   ;;
    *) die "unsupported arch $(uname -m)" ;;
esac

if [ "${YQ_VERSION}" = "latest" ]; then
    YQ_VERSION="$(pins_latest_gh mikefarah/yq)" || die "could not resolve the latest yq release"
fi
case "${YQ_VERSION}" in
    *[!0-9.]*|"") die "YQ_VERSION must look like 4.44.6 (got '${YQ_VERSION}')" ;;
esac

HAVE=""
if [ -x "${OWN}" ] && is_mikefarah "${OWN}"; then HAVE="$(version_of "${OWN}")"; fi
if [ "${HAVE}" = "${YQ_VERSION}" ] && [ "${YQ_FORCE:-0}" != "1" ]; then
    ok "yq already at ${HAVE}: ${OWN}"
    exit 0
fi

command -v curl >/dev/null 2>&1 || die "curl is required to download yq"
mkdir -p "${INSTALL_DIR}"
URL="${RELEASE_BASE}/v${YQ_VERSION}/yq_${YQ_OS}_${YQ_ARCH}"
if [ -n "${HAVE}" ]; then
    info "Updating yq ${HAVE} -> ${YQ_VERSION} (${YQ_OS}/${YQ_ARCH}) in ${INSTALL_DIR}..."
else
    info "Installing yq ${YQ_VERSION} (${YQ_OS}/${YQ_ARCH}) to ${OWN}..."
fi
TMP_BIN="$(mktemp "${INSTALL_DIR}/.yq.XXXXXX")"
curl -fsSL "${URL}" -o "${TMP_BIN}" || { rm -f "${TMP_BIN}"; die "download failed: ${URL}"; }
chmod +x "${TMP_BIN}"

# Verify the binary we just wrote actually runs, THEN that PATH resolves to it.
# Separating the two makes the failure message name the real problem instead of
# blaming PATH for a corrupt download (or vice versa).
if ! "${TMP_BIN}" --version >/dev/null 2>&1; then
    rm -f "${TMP_BIN}"
    die "${URL} was downloaded but does not run (corrupt download?)."
fi
# Clear any stale entry (including a dangling symlink) before moving it in.
if [ -e "${OWN}" ] || [ -L "${OWN}" ]; then
    rm -f "${OWN}"
fi
mv "${TMP_BIN}" "${OWN}"

if command -v yq >/dev/null 2>&1 && yq --version >/dev/null 2>&1; then
    ok "yq installed: $(yq --version 2>&1 | head -1)"
else
    echo -e "${RED}install_yq: yq works at ${OWN} but PATH does not resolve it.${NC}"
    echo -e "${RED}            Add ${INSTALL_DIR} to PATH (see opt/profiles/.profile).${NC}"
    exit 1
fi
