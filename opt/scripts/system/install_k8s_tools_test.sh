#!/usr/bin/env bash
# Test driver for opt/scripts/system/install_k8s_tools.sh (objective `pins`, F3).
#
# kubectl, helm and kind each converge INDEPENDENTLY (plan §3.2):
#   * absent              -> installs WANT (the versions.tsv pin by default);
#   * installed == WANT   -> "already at", NO download;
#   * installed older     -> reinstalls just that tool;
#   * a tool OUTSIDE INSTALL_DIR (apt/brew/k3s-owned) is left alone;
#   * KUBECTL_VERSION / HELM_VERSION / KIND_VERSION override the manifest
#     (a leading v, the old documented form, is still accepted);
#   * WANT=latest         -> kubectl from dl.k8s.io stable.txt, helm/kind via
#                            pins_latest_gh.
#
# Network is never touched: *_RELEASE_BASE / KUBECTL_STABLE_URL / PINS_GH_API
# point curl at file:// fixtures, `uname` is a stub, PATH is a sandbox.
#
# Run: bash opt/scripts/system/install_k8s_tools_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/install_k8s_tools.sh"
assert_file_exists "${SCRIPT}" "install_k8s_tools.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
set +e

assert_grep "kubectl default comes from versions.tsv" 'KUBECTL_VERSION:-\$\(pins_want kubectl latest\)' "${SCRIPT}"
assert_grep "helm default comes from versions.tsv" 'HELM_VERSION:-\$\(pins_want helm latest\)' "${SCRIPT}"
assert_grep "kind default comes from versions.tsv" 'KIND_VERSION:-\$\(pins_want kind latest\)' "${SCRIPT}"
assert_grep "kubectl latest still comes from dl.k8s.io stable.txt" 'dl\.k8s\.io/release/stable\.txt' "${SCRIPT}"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/install_k8s_tools_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT

SYS="${TMP}/sys"; mkdir -p "${SYS}"
for t in bash sh env cat cp mv rm mkdir chmod ln ls head tail awk sed grep tr cut sort dirname basename mktemp tar gzip curl sha256sum shasum; do
    p="$(command -v "${t}" 2>/dev/null)" || continue
    case "${p}" in /*) ln -sf "${p}" "${SYS}/${t}" ;; esac
done
# shellcheck disable=SC2016 # $1 belongs to the stub, expanded when it runs
printf '#!/bin/sh\ncase "$1" in -s) echo Linux ;; -m) echo x86_64 ;; *) echo Linux ;; esac\n' > "${SYS}/uname"
chmod +x "${SYS}/uname"

fake_kubectl() { printf '#!/bin/sh\nprintf "Client Version: v%s\\nKustomize Version: v5.0.0\\n"\n' "$1" > "$2"; chmod +x "$2"; }
fake_helm()    { printf '#!/bin/sh\necho "v%s+gabc123"\n' "$1" > "$2"; chmod +x "$2"; }
fake_kind()    { printf '#!/bin/sh\necho "kind v%s go1.24.0 linux/amd64"\n' "$1" > "$2"; chmod +x "$2"; }

K="${TMP}/kubectl-rel"; H="${TMP}/helm-rel"; KI="${TMP}/kind-rel"
for v in 1.37.0 1.36.0 1.38.0; do
    mkdir -p "${K}/v${v}/bin/linux/amd64"; fake_kubectl "${v}" "${K}/v${v}/bin/linux/amd64/kubectl"
done
printf 'v1.38.0\n' > "${K}/stable.txt"
for v in 4.2.4 4.1.0 4.3.0; do
    mkdir -p "${TMP}/hb/${v}/linux-amd64" "${H}"; fake_helm "${v}" "${TMP}/hb/${v}/linux-amd64/helm"
    tar -czf "${H}/helm-v${v}-linux-amd64.tar.gz" -C "${TMP}/hb/${v}" linux-amd64
done
for v in 0.33.0 0.30.0 0.34.0; do
    mkdir -p "${KI}/v${v}"; fake_kind "${v}" "${KI}/v${v}/kind-linux-amd64"
done
mkdir -p "${TMP}/api/repos/helm/helm/releases" "${TMP}/api/repos/kubernetes-sigs/kind/releases"
printf '{"tag_name": "v4.3.0"}\n' > "${TMP}/api/repos/helm/helm/releases/latest"
printf '{"tag_name": "v0.34.0"}\n' > "${TMP}/api/repos/kubernetes-sigs/kind/releases/latest"

MAN="${TMP}/versions.tsv"
cat > "${MAN}" <<'EOF'
kubectl  pin  1.37.0  text:https://dl.k8s.io/release/stable.txt  opt/scripts/system/install_k8s_tools.sh  kubectl version --client
helm     pin  4.2.4   gh:helm/helm             opt/scripts/system/install_k8s_tools.sh  helm version --short
kind     pin  0.33.0  gh:kubernetes-sigs/kind  opt/scripts/system/install_k8s_tools.sh  kind version
EOF

run_k8s() {  # run_k8s <install dir> [VAR=value ...]
    _dir="$1"; shift
    env -u KUBECTL_VERSION -u HELM_VERSION -u KIND_VERSION -u K8S_TOOLS_FORCE \
        -u GFF_PINS_TRACK -u GFF_PINS_TRACK_MODE \
        PATH="${EXTRA_PATH:-${SYS}}:${SYS}" HOME="${TMP}/home" K8S_INSTALL_DIR="${_dir}" \
        KUBECTL_RELEASE_BASE="file://${K}" KUBECTL_STABLE_URL="file://${K}/stable.txt" \
        HELM_RELEASE_BASE="file://${H}" KIND_RELEASE_BASE="file://${KI}" \
        PINS_MANIFEST="${MAN}" PINS_GH_API="file://${TMP}/api" \
        "$@" bash "${SCRIPT}" 2>&1
}
vers() {  # vers <dir> -> "kubectl helm kind" versions
    printf '%s %s %s' \
        "$("$1/kubectl" version --client 2>/dev/null | head -1 | sed 's/.*v//')" \
        "$("$1/helm" version --short 2>/dev/null | sed 's/^v//; s/+.*//')" \
        "$("$1/kind" version 2>/dev/null | awk '{ sub(/^v/, "", $2); print $2 }')"
}
hide_fixtures() { mv "${K}" "${K}.away"; mv "${H}" "${H}.away"; mv "${KI}" "${KI}.away"; }
show_fixtures() { mv "${K}.away" "${K}"; mv "${H}.away" "${H}"; mv "${KI}.away" "${KI}"; }

# 1. absent -> all three installed at WANT.
D="${TMP}/bin"
out="$(run_k8s "${D}")"; rc=$?
assert_eq "${rc}" "0" "absent: exits 0"
assert_eq "$(vers "${D}")" "1.37.0 4.2.4 0.33.0" "absent: all three at the manifest pins"

# 2. all equal -> no downloads at all.
hide_fixtures
out="$(run_k8s "${D}")"; rc=$?
show_fixtures
assert_eq "${rc}" "0" "equal: exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'already at')" "3" "equal: all three say already at"

# 3. only kubectl is older -> only kubectl is reinstalled.
fake_kubectl 1.36.0 "${D}/kubectl"
mv "${H}" "${H}.away"; mv "${KI}" "${KI}.away"   # helm/kind must not download
out="$(run_k8s "${D}")"; rc=$?
mv "${H}.away" "${H}"; mv "${KI}.away" "${KI}"
assert_eq "${rc}" "0" "kubectl older: exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'kubectl 1.36.0 -> 1.37.0')" "1" "kubectl older: reports the upgrade"
assert_eq "$(vers "${D}")" "1.37.0 4.2.4 0.33.0" "kubectl older: converged; helm/kind untouched"

# 4. helm and kind older -> both reinstalled.
fake_helm 4.1.0 "${D}/helm"; fake_kind 0.30.0 "${D}/kind"
out="$(run_k8s "${D}")"; rc=$?
assert_eq "${rc}" "0" "helm/kind older: exits 0"
assert_eq "$(vers "${D}")" "1.37.0 4.2.4 0.33.0" "helm/kind older: both converged"

# 5. env overrides win (with and without a leading v).
out="$(run_k8s "${D}" KUBECTL_VERSION=v1.36.0 HELM_VERSION=v4.1.0 KIND_VERSION=0.30.0)"; rc=$?
assert_eq "${rc}" "0" "env override: exits 0"
assert_eq "$(vers "${D}")" "1.36.0 4.1.0 0.30.0" "env override: each *_VERSION wins"

# 6. K8S_TOOLS_FORCE=1 reinstalls even when equal.
out="$(run_k8s "${D}" K8S_TOOLS_FORCE=1)"; rc=$?
assert_eq "$(printf '%s' "${out}" | grep -c 'already at')" "0" "force: nothing short-circuits"
assert_eq "$(vers "${D}")" "1.37.0 4.2.4 0.33.0" "force: back on the pins"

# 7. latest: kubectl from stable.txt, helm/kind via the GitHub API.
out="$(run_k8s "${D}" GFF_PINS_TRACK_MODE=latest)"; rc=$?
assert_eq "${rc}" "0" "latest: exits 0"
assert_eq "$(vers "${D}")" "1.38.0 4.3.0 0.34.0" "latest: each resolved upstream first"

# 8. no manifest -> the old behaviour (latest) still works.
D4="${TMP}/bin4"
out="$(run_k8s "${D4}" PINS_MANIFEST="${TMP}/nope.tsv")"; rc=$?
assert_eq "$(vers "${D4}")" "1.38.0 4.3.0 0.34.0" "no manifest: falls back to latest, as before pins"

# 9. a kubectl outside INSTALL_DIR (e.g. k3s's /usr/local/bin/kubectl) is left alone.
OTHER="${TMP}/other"; mkdir -p "${OTHER}"; fake_kubectl 1.30.0 "${OTHER}/kubectl"
D2="${TMP}/bin2"
out="$(EXTRA_PATH="${OTHER}" run_k8s "${D2}")"; rc=$?
assert_eq "${rc}" "0" "foreign kubectl: exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'kubectl at .* leaving it alone')" "1" "foreign kubectl: says it is leaving it alone"
assert_eq "$([ -e "${D2}/kubectl" ] && echo present || echo absent)" "absent" "foreign kubectl: not installed over"
assert_eq "$([ -x "${D2}/helm" ] && [ -x "${D2}/kind" ] && echo both || echo missing)" "both" "foreign kubectl: helm/kind still installed"

# 10. a failed download is reported and fails the run, other tools still converge.
D3="${TMP}/bin3"
out="$(run_k8s "${D3}" KIND_VERSION=9.9.9)"; rc=$?
assert_eq "${rc}" "1" "download failure: exits 1"
assert_eq "$([ -x "${D3}/kubectl" ] && [ -x "${D3}/helm" ] && echo both || echo missing)" "both" "download failure: the other tools still installed"

_test_report
