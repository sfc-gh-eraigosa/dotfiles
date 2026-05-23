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
# Safe to re-run. Override the version with SOPS_VERSION=x.y.z.
set -e

SOPS_VERSION="${SOPS_VERSION:-3.13.1}"
INSTALL_DIR="${HOME}/opt/bin"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

# Already present and runnable? Then there's nothing to do. Probing with
# `sops --version` (not just `command -v`) also means a dangling symlink — e.g.
# an old `go install` link into a now-removed goenv Go version — is treated as
# "not installed" and gets repaired below.
if command -v sops >/dev/null 2>&1 && sops --version >/dev/null 2>&1; then
    echo -e "${GREEN}sops already installed: $(sops --version | head -1)${NC}"
    exit 0
fi

# --- OS / arch detection (matches the getsops release asset naming) ---
case "$(uname -s)" in
    Linux)  SOPS_OS="linux"  ;;
    Darwin) SOPS_OS="darwin" ;;
    *) echo -e "${RED}install_sops: unsupported OS $(uname -s)${NC}"; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64)        SOPS_ARCH="amd64" ;;
    arm64|aarch64) SOPS_ARCH="arm64" ;;
    *) echo -e "${RED}install_sops: unsupported arch $(uname -m)${NC}"; exit 1 ;;
esac

mkdir -p "${INSTALL_DIR}"

# Clear any stale entry (including a dangling symlink) before writing the real
# binary in its place. `-e` is false for a dangling symlink, so test `-L` too.
if [ -e "${INSTALL_DIR}/sops" ] || [ -L "${INSTALL_DIR}/sops" ]; then
    rm -f "${INSTALL_DIR}/sops"
fi

URL="https://github.com/getsops/sops/releases/download/v${SOPS_VERSION}/sops-v${SOPS_VERSION}.${SOPS_OS}.${SOPS_ARCH}"
echo -e "${BLUE}Installing sops ${SOPS_VERSION} (${SOPS_OS}/${SOPS_ARCH}) to ${INSTALL_DIR}/sops...${NC}"
curl -fsSL "$URL" -o "${INSTALL_DIR}/sops"
chmod +x "${INSTALL_DIR}/sops"

echo -e "${GREEN}Success! $("${INSTALL_DIR}/sops" --version | head -1)${NC}"
