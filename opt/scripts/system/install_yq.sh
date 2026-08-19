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
# Safe to re-run. Override the version with YQ_VERSION=x.y.z.
set -e

YQ_VERSION="${YQ_VERSION:-4.44.6}"
INSTALL_DIR="${HOME}/opt/bin"

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

# Already present, runnable, AND the mikefarah build? Nothing to do. Probing
# `yq --version | grep mikefarah` also means a stale kislyuk yq or a dangling
# symlink is treated as "not installed" and gets replaced below.
if command -v yq >/dev/null 2>&1 && yq --version 2>&1 | grep -qi mikefarah; then
    echo -e "${GREEN}yq already installed: $(yq --version 2>&1 | head -1)${NC}"
    exit 0
fi

# --- OS / arch detection (matches the mikefarah/yq release asset naming) ---
case "$(uname -s)" in
    Linux)  YQ_OS="linux"  ;;
    Darwin) YQ_OS="darwin" ;;
    *) echo -e "${RED}install_yq: unsupported OS $(uname -s)${NC}"; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64)         YQ_ARCH="amd64" ;;
    arm64|aarch64)  YQ_ARCH="arm64" ;;
    armv7l|armv6l)  YQ_ARCH="arm"   ;;
    *) echo -e "${RED}install_yq: unsupported arch $(uname -m)${NC}"; exit 1 ;;
esac

mkdir -p "${INSTALL_DIR}"

# Clear any stale entry (including a dangling symlink) before writing the binary.
if [ -e "${INSTALL_DIR}/yq" ] || [ -L "${INSTALL_DIR}/yq" ]; then
    rm -f "${INSTALL_DIR}/yq"
fi

URL="https://github.com/mikefarah/yq/releases/download/v${YQ_VERSION}/yq_${YQ_OS}_${YQ_ARCH}"
echo -e "${BLUE}Installing yq ${YQ_VERSION} (${YQ_OS}/${YQ_ARCH}) to ${INSTALL_DIR}/yq...${NC}"
curl -fsSL "$URL" -o "${INSTALL_DIR}/yq"
chmod +x "${INSTALL_DIR}/yq"

# Verify the binary we just wrote actually runs, THEN that PATH resolves to it.
# Separating the two makes the failure message name the real problem instead of
# blaming PATH for a corrupt download (or vice versa).
if ! "${INSTALL_DIR}/yq" --version >/dev/null 2>&1; then
    echo -e "${RED}install_yq: ${INSTALL_DIR}/yq was written but does not run (corrupt download?).${NC}"
    exit 1
fi
if command -v yq >/dev/null 2>&1 && yq --version >/dev/null 2>&1; then
    echo -e "${GREEN}yq installed: $(yq --version 2>&1 | head -1)${NC}"
else
    echo -e "${RED}install_yq: yq works at ${INSTALL_DIR}/yq but PATH does not resolve it.${NC}"
    echo -e "${RED}            Add ${INSTALL_DIR} to PATH (see opt/profiles/.profile).${NC}"
    exit 1
fi
