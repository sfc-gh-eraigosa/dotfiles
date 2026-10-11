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
#   * Local k8s dev flows (e.g. a kind-based `make e2e`: kind cluster ->
#     helm install -> kubectl wait) fail with a bare `command not found`
#     without them.
#
# Converges (objective `pins`): each tool's version comes from
# opt/profiles/versions.tsv (pins_want kubectl|helm|kind). Each tool is handled
# independently: absent or a different version in ~/opt/bin -> download WANT;
# already on it -> no-op. A tool that resolves OUTSIDE ~/opt/bin (brew, apt,
# k3s's kubectl) is left alone. WANT=latest resolves upstream first (kubectl:
# dl.k8s.io stable.txt; helm/kind: the GitHub releases API). With no manifest
# the default is `latest`, i.e. the pre-pins behaviour.
#
# Environment:
#   KUBECTL_VERSION / HELM_VERSION / KIND_VERSION   override the manifest
#                       (x.y.z; the old v-prefixed form is accepted too)
#   K8S_TOOLS_FORCE=1   reinstall even when already at the wanted version
#   K8S_INSTALL_DIR     where the binaries go (default ~/opt/bin)
#   KUBECTL_RELEASE_BASE / KUBECTL_STABLE_URL / HELM_RELEASE_BASE /
#   KIND_RELEASE_BASE   download URL bases (tests point them at file:// fixtures)
set -eu

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PINS_LIB="${SELF_DIR}/../../lib/pins.sh"
if [ -f "${PINS_LIB}" ]; then
    # shellcheck source=../../lib/pins.sh
    . "${PINS_LIB}"
fi
# Fail-open: without the lib, every tool tracks latest as it did before pins.
command -v pins_want >/dev/null 2>&1 || pins_want() { printf '%s\n' "${2:-}"; }
command -v pins_latest_gh >/dev/null 2>&1 || pins_latest_gh() { return 1; }

KUBECTL_VERSION="${KUBECTL_VERSION:-$(pins_want kubectl latest)}"
HELM_VERSION="${HELM_VERSION:-$(pins_want helm latest)}"
KIND_VERSION="${KIND_VERSION:-$(pins_want kind latest)}"

INSTALL_DIR="${K8S_INSTALL_DIR:-${HOME}/opt/bin}"
KUBECTL_RELEASE_BASE="${KUBECTL_RELEASE_BASE:-https://dl.k8s.io/release}"
KUBECTL_STABLE_URL="${KUBECTL_STABLE_URL:-https://dl.k8s.io/release/stable.txt}"
HELM_RELEASE_BASE="${HELM_RELEASE_BASE:-https://get.helm.sh}"
KIND_RELEASE_BASE="${KIND_RELEASE_BASE:-https://github.com/kubernetes-sigs/kind/releases/download}"

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
# /releases/latest redirect — the no-token fallback when the API is rate-limited.
latest_github_tag() {
    curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$1/releases/latest" \
        | sed 's#.*/tag/##'
}

# version_from TEXT -> first x.y[.z] in it ("Client Version: v1.37.0" -> 1.37.0)
version_from() { printf '%s\n' "$1" | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -n 1 || true; }

# resolve_want NAME VERSION GH_REPO -> concrete x.y.z (no v), or non-zero.
resolve_want() {
    local v="${2#v}"
    if [ "${v}" = "latest" ]; then
        if [ "$1" = "kubectl" ]; then
            v="$(curl -fsSL "${KUBECTL_STABLE_URL}")" || return 1
        else
            v="$(pins_latest_gh "$3" || latest_github_tag "$3")" || return 1
        fi
        v="${v#v}"
    fi
    case "${v}" in *[!0-9.]*|"") return 1 ;; esac
    printf '%s\n' "${v}"
}

# clear_stale PATHNAME — remove any existing entry (including dangling symlinks)
# before writing the fresh binary.
clear_stale() {
    if [ -e "$1" ] || [ -L "$1" ]; then
        rm -f "$1"
    fi
}

# plan NAME VERSION GH_REPO VERSION_CMD... — decide what to do with one tool.
# Prints why it skips and returns 1, or sets WANT/HAVE and returns 0 (install).
WANT=""; HAVE=""
plan() {
    local name="$1" version="$2" repo="$3" found own out
    shift 3
    own="${INSTALL_DIR}/${name}"
    found="$(command -v "${name}" 2>/dev/null || true)"
    if [ "${K8S_TOOLS_FORCE:-0}" != "1" ] && [ ! -x "${own}" ] && [ -n "${found}" ] \
            && [ "${found}" != "${own}" ] && out="$("${found}" "$@" 2>/dev/null)"; then
        echo -e "${BLUE}${name} at ${found} ($(version_from "${out}")) is managed outside ${INSTALL_DIR}; leaving it alone${NC}"
        return 1
    fi
    WANT="$(resolve_want "${name}" "${version}" "${repo}")" || {
        echo -e "${RED}install_k8s_tools: could not resolve a ${name} version from '${version}'${NC}"
        status=1; return 1
    }
    HAVE=""
    if [ -x "${own}" ]; then HAVE="$(version_from "$("${own}" "$@" 2>/dev/null || true)")"; fi
    if [ "${HAVE}" = "${WANT}" ] && [ "${K8S_TOOLS_FORCE:-0}" != "1" ]; then
        echo -e "${GREEN}${name} already at ${HAVE}: ${own}${NC}"
        return 1
    fi
    if [ -n "${HAVE}" ]; then
        echo -e "${BLUE}Updating ${name} ${HAVE} -> ${WANT} (${K8S_OS}/${K8S_ARCH}) in ${INSTALL_DIR}...${NC}"
    else
        echo -e "${BLUE}Installing ${name} ${WANT} (${K8S_OS}/${K8S_ARCH}) to ${own}...${NC}"
    fi
    return 0
}

status=0

# --- kubectl ------------------------------------------------------------------
if plan kubectl "${KUBECTL_VERSION}" kubernetes/kubernetes version --client; then
    tmp="$(mktemp "${INSTALL_DIR}/.kubectl.XXXXXX")"
    if curl -fsSL "${KUBECTL_RELEASE_BASE}/v${WANT}/bin/${K8S_OS}/${K8S_ARCH}/kubectl" -o "${tmp}"; then
        chmod +x "${tmp}"
        clear_stale "${INSTALL_DIR}/kubectl"
        mv "${tmp}" "${INSTALL_DIR}/kubectl"
        echo -e "${GREEN}kubectl installed: $("${INSTALL_DIR}/kubectl" version --client 2>/dev/null | head -1)${NC}"
    else
        rm -f "${tmp}"
        echo -e "${RED}install_k8s_tools: kubectl download failed${NC}"; status=1
    fi
fi

# --- helm ---------------------------------------------------------------------
if plan helm "${HELM_VERSION}" helm/helm version --short; then
    tmpdir="$(mktemp -d)"
    if curl -fsSL "${HELM_RELEASE_BASE}/helm-v${WANT}-${K8S_OS}-${K8S_ARCH}.tar.gz" \
            -o "${tmpdir}/helm.tar.gz" \
            && tar -xzf "${tmpdir}/helm.tar.gz" -C "${tmpdir}" \
            && [ -f "${tmpdir}/${K8S_OS}-${K8S_ARCH}/helm" ]; then
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
if plan kind "${KIND_VERSION}" kubernetes-sigs/kind version; then
    tmp="$(mktemp "${INSTALL_DIR}/.kind.XXXXXX")"
    if curl -fsSL "${KIND_RELEASE_BASE}/v${WANT}/kind-${K8S_OS}-${K8S_ARCH}" -o "${tmp}"; then
        chmod +x "${tmp}"
        clear_stale "${INSTALL_DIR}/kind"
        mv "${tmp}" "${INSTALL_DIR}/kind"
        echo -e "${GREEN}kind installed: $("${INSTALL_DIR}/kind" version 2>/dev/null)${NC}"
    else
        rm -f "${tmp}"
        echo -e "${RED}install_k8s_tools: kind download failed${NC}"; status=1
    fi
fi

if [ "${status}" -ne 0 ]; then
    echo -e "${RED}install_k8s_tools: one or more tools failed to install${NC}"
fi
exit "${status}"
