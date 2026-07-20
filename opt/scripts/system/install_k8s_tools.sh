#!/usr/bin/env bash
# ==============================================================================
# Kubernetes Toolchain Setup — install kubectl, helm, and kind into ~/opt/bin
# ==============================================================================
# Why this exists:
#   * macOS installs these from opt/profiles/packages.tsv (brew: kubernetes-cli,
#     helm, kind).
#   * Debian/Ubuntu/WSL apt has no kind package at all, and kubectl/helm require
#     third-party apt repos that lag upstream — so Linux/WSL fetches the
#     official release binaries instead (mirrors install_yq.sh / install_sops.sh).
#   * Local k8s dev flows (e.g. playground bots `make e2e`: kind cluster ->
#     helm install -> kubectl wait) fail with a bare `command not found`
#     without them.
#
# Safe to re-run: tools already present and runnable are left alone.
# Versions resolve to the latest stable release at run time; override with
# KUBECTL_VERSION / HELM_VERSION / KIND_VERSION (e.g. HELM_VERSION=v3.17.0),
# or set K8S_TOOLS_FORCE=1 to reinstall over existing binaries.
set -eu

INSTALL_DIR="${HOME}/opt/bin"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

case "$(uname -s)" in
    Linux)  K8S_OS="linux"  ;;
    Darwin) K8S_OS="darwin" ;;
    *) echo -e "${RED}install_k8s_tools: unsupported OS $(uname -s)${NC}"; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64)        K8S_ARCH="amd64" ;;
    arm64|aarch64) K8S_ARCH="arm64" ;;
    *) echo -e "${RED}install_k8s_tools: unsupported arch $(uname -m)${NC}"; exit 1 ;;
esac

mkdir -p "${INSTALL_DIR}"

# Resolve a GitHub repo's latest release tag (e.g. v3.17.0) by following the
# /releases/latest redirect — no API token, no rate-limited JSON endpoint.
latest_github_tag() {
    curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$1/releases/latest" \
        | sed 's#.*/tag/##'
}

# have_tool NAME — true when NAME resolves and runs (a dangling symlink or a
# broken interop stub is treated as "not installed" and gets replaced).
have_tool() {
    command -v "$1" >/dev/null 2>&1 && "$1" --help >/dev/null 2>&1
}

# clear_stale PATHNAME — remove any existing entry (including dangling symlinks)
# before writing the fresh binary.
clear_stale() {
    if [ -e "$1" ] || [ -L "$1" ]; then
        rm -f "$1"
    fi
}

status=0

# --- kubectl ------------------------------------------------------------------
if [ "${K8S_TOOLS_FORCE:-0}" != "1" ] && have_tool kubectl; then
    echo -e "${GREEN}kubectl already installed: $(kubectl version --client 2>/dev/null | head -1)${NC}"
else
    KUBECTL_VERSION="${KUBECTL_VERSION:-$(curl -fsSL https://dl.k8s.io/release/stable.txt)}"
    echo -e "${BLUE}Installing kubectl ${KUBECTL_VERSION} (${K8S_OS}/${K8S_ARCH}) to ${INSTALL_DIR}/kubectl...${NC}"
    clear_stale "${INSTALL_DIR}/kubectl"
    if curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/${K8S_OS}/${K8S_ARCH}/kubectl" \
            -o "${INSTALL_DIR}/kubectl"; then
        chmod +x "${INSTALL_DIR}/kubectl"
        echo -e "${GREEN}kubectl installed: $("${INSTALL_DIR}/kubectl" version --client 2>/dev/null | head -1)${NC}"
    else
        echo -e "${RED}install_k8s_tools: kubectl download failed${NC}"; status=1
    fi
fi

# --- helm ---------------------------------------------------------------------
if [ "${K8S_TOOLS_FORCE:-0}" != "1" ] && have_tool helm; then
    echo -e "${GREEN}helm already installed: $(helm version --short 2>/dev/null)${NC}"
else
    HELM_VERSION="${HELM_VERSION:-$(latest_github_tag helm/helm)}"
    echo -e "${BLUE}Installing helm ${HELM_VERSION} (${K8S_OS}/${K8S_ARCH}) to ${INSTALL_DIR}/helm...${NC}"
    tmpdir="$(mktemp -d)"
    if curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-${K8S_OS}-${K8S_ARCH}.tar.gz" \
            | tar -xz -C "${tmpdir}"; then
        clear_stale "${INSTALL_DIR}/helm"
        mv "${tmpdir}/${K8S_OS}-${K8S_ARCH}/helm" "${INSTALL_DIR}/helm"
        chmod +x "${INSTALL_DIR}/helm"
        echo -e "${GREEN}helm installed: $("${INSTALL_DIR}/helm" version --short 2>/dev/null)${NC}"
    else
        echo -e "${RED}install_k8s_tools: helm download failed${NC}"; status=1
    fi
    rm -rf "${tmpdir}"
fi

# --- kind ---------------------------------------------------------------------
if [ "${K8S_TOOLS_FORCE:-0}" != "1" ] && have_tool kind; then
    echo -e "${GREEN}kind already installed: $(kind version 2>/dev/null)${NC}"
else
    KIND_VERSION="${KIND_VERSION:-$(latest_github_tag kubernetes-sigs/kind)}"
    echo -e "${BLUE}Installing kind ${KIND_VERSION} (${K8S_OS}/${K8S_ARCH}) to ${INSTALL_DIR}/kind...${NC}"
    clear_stale "${INSTALL_DIR}/kind"
    if curl -fsSL "https://github.com/kubernetes-sigs/kind/releases/download/${KIND_VERSION}/kind-${K8S_OS}-${K8S_ARCH}" \
            -o "${INSTALL_DIR}/kind"; then
        chmod +x "${INSTALL_DIR}/kind"
        echo -e "${GREEN}kind installed: $("${INSTALL_DIR}/kind" version 2>/dev/null)${NC}"
    else
        echo -e "${RED}install_k8s_tools: kind download failed${NC}"; status=1
    fi
fi

if [ "${status}" -ne 0 ]; then
    echo -e "${RED}install_k8s_tools: one or more tools failed to install${NC}"
fi
exit "${status}"
