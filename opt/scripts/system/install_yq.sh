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

if command -v yq >/dev/null 2>&1 && yq --version >/dev/null 2>&1; then
    echo -e "${GREEN}yq installed: $(yq --version 2>&1 | head -1)${NC}"
else
    echo -e "${RED}install_yq: yq not resolvable after install — is ${INSTALL_DIR} on PATH?${NC}"
    echo -e "${RED}            (binary written to ${INSTALL_DIR}/yq)${NC}"
fi
