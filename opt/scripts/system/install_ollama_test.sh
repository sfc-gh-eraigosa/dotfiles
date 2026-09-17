#!/usr/bin/env bash
# Test driver for opt/scripts/system/install_ollama.sh (playground#379 tasks 7-8)
#
# What must hold:
#   * ollama's own install.sh already detects CUDA/ROCm/Jetson JetPack
#     capability correctly — this script does NOT reimplement that. It fetches
#     the SAME install.sh asset ollama publishes for the pinned release and
#     verifies it against that release's sha256sum.txt before running the
#     LOCAL verified copy. A missing or mismatched checksum is REFUSED
#     (fail-closed), and a remote download is never piped straight into a
#     shell;
#   * a host already on OLLAMA_VERSION is a no-op with no download (fleet
#     update re-runs install.sh everywhere); another version re-runs the
#     verified installer with OLLAMA_VERSION pinned for ollama's own script;
#   * install.sh gates it fail-closed (gff_opt_in, boolDefault false) as a
#     deps-phase key — GPU-node hosts opt in explicitly, unlike herdr/glow
#     which every host wants.
#
# Network is never touched: OLLAMA_RELEASE_BASE points curl at a file://
# fixture tree and the "installed" ollama is a stub that only answers
# --version.
#
# Run: bash opt/scripts/system/install_ollama_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/install_ollama.sh"
INSTALL_SH="${REPO_ROOT}/install.sh"
FEATURES="${REPO_ROOT}/.github/gff/features.yaml"

assert_file_exists "${SCRIPT}" "install_ollama.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
set +e

# --- source-level: the trust model -------------------------------------------
assert_grep "verifies against the release's sha256sum.txt" 'sha256sum\.txt' "${SCRIPT}"
assert_grep "fails closed on a missing/malformed checksum" \
    'refusing to run an unverified installer' "${SCRIPT}"
assert_grep "verifies the download before running it" 'checksum mismatch' "${SCRIPT}"
assert_grep_negative "never pipes a remote script straight into a shell" \
    'curl[^|]*\|[[:space:]]*(ba)?sh' "${SCRIPT}"
assert_grep "does not reimplement ollama's own CUDA/Jetson detection" \
    "does NOT reimplement" "${SCRIPT}"
assert_grep "supports a version pin" 'OLLAMA_VERSION:-' "${SCRIPT}"
assert_grep "pins ollama's own installer to the same version" \
    'OLLAMA_VERSION="\$\{OLLAMA_VERSION\}" sh' "${SCRIPT}"

# --- fixtures: a fake release tree -------------------------------------------
TMP="$(mktemp -d "${TMPDIR:-/tmp}/install_ollama_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
REL="${TMP}/releases"

make_release() { # make_release <version> <install.sh body>
    mkdir -p "${REL}/v$1"
    printf '%s\n' "$2" >"${REL}/v$1/install.sh"
    if command -v sha256sum >/dev/null 2>&1; then sum="$(sha256sum <"${REL}/v$1/install.sh" | awk '{ print $1 }')"
    else sum="$(shasum -a 256 <"${REL}/v$1/install.sh" | awk '{ print $1 }')"; fi
    printf '%s  ./install.sh\n' "${sum}" >"${REL}/v$1/sha256sum.txt"
}
# shellcheck disable=SC2016 # $OLLAMA_STUB_BINDIR/$OLLAMA_VERSION belong to
# the fixture install.sh, expanded when the script under test runs it
FIXTURE_INSTALLER='#!/bin/sh
mkdir -p "$OLLAMA_STUB_BINDIR"
printf "#!/bin/sh\necho ollama version is %s\n" "${OLLAMA_VERSION:-unknown}" > "$OLLAMA_STUB_BINDIR/ollama"
chmod +x "$OLLAMA_STUB_BINDIR/ollama"
'
make_release 0.34.1 "${FIXTURE_INSTALLER}"
make_release 0.33.0 "${FIXTURE_INSTALLER}"

run_ollama() { # run_ollama <install dir/bindir> [VAR=value ...]
    _dir="$1"
    shift
    env -u OLLAMA_VERSION -u OLLAMA_FORCE PATH="${_dir}:${PATH}" \
        OLLAMA_RELEASE_BASE="file://${REL}" OLLAMA_STUB_BINDIR="${_dir}" \
        "$@" bash "${SCRIPT}" 2>&1
}

# 1. Fresh host: verified install of the pinned version.
D="${TMP}/bin-fresh"
mkdir -p "${D}"
out="$(run_ollama "${D}")"
rc=$?
assert_eq "${rc}" "0" "fresh host exits 0"
assert_eq "$("${D}/ollama" --version 2>/dev/null)" "ollama version is 0.34.1" "the pinned version is installed"
assert_eq "$(printf '%s' "${out}" | grep -c 'sha256 verified')" "1" "success says it verified the checksum"

# 2. Re-run on the pinned version: no-op, no download (fixtures moved away).
mv "${REL}" "${REL}.away"
out="$(run_ollama "${D}")"
rc=$?
assert_eq "${rc}" "0" "already-installed exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'ollama already installed: .*(0.34.1)')" "1" "already-installed is reported"
mv "${REL}.away" "${REL}"

# 3. A different pinned version replaces the installed one.
out="$(run_ollama "${D}" OLLAMA_VERSION=0.33.0)"
rc=$?
assert_eq "${rc}" "0" "version change exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'Updating ollama 0.34.1 -> 0.33.0')" "1" "version change is reported"
assert_eq "$("${D}/ollama" --version 2>/dev/null)" "ollama version is 0.33.0" "the new version is in place"

# 4. Checksum mismatch: refused, nothing installed.
printf '%s  ./install.sh\n' "$(printf '0%.0s' $(seq 1 64))" >"${REL}/v0.34.1/sha256sum.txt"
D2="${TMP}/bin-bad"
mkdir -p "${D2}"
out="$(run_ollama "${D2}")"
rc=$?
assert_eq "${rc}" "1" "checksum mismatch exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'checksum mismatch')" "1" "checksum mismatch is the stated reason"
assert_eq "$([ -e "${D2}/ollama" ] && echo present || echo absent)" "absent" "nothing is installed on a mismatch"

# 5. No checksum listed for install.sh: refused before running it.
printf '%s  ./some-other-asset.tar.zst\n' "$(printf 'b%.0s' $(seq 1 64))" >"${REL}/v0.34.1/sha256sum.txt"
D3="${TMP}/bin-nosum"
mkdir -p "${D3}"
out="$(run_ollama "${D3}")"
rc=$?
assert_eq "${rc}" "1" "unlisted install.sh exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'refusing to run an unverified installer')" "1" "unlisted install.sh is refused"

# 6. A pinned release absent from the fixture tree fails loudly (no fallback
#    to an unpinned/unverified fetch).
D4="${TMP}/bin-noversion"
mkdir -p "${D4}"
out="$(run_ollama "${D4}" OLLAMA_VERSION=9.9.9)"
rc=$?
assert_eq "${rc}" "1" "unknown pinned version exits 1"

# --- gff wiring -------------------------------------------------------------
assert_grep "features.yaml declares install.tools.ollama" 'path: install\.tools\.ollama$' "${FEATURES}"
assert_eq "$(awk '$2 == "path:" { k = $3 } k == "install.tools.ollama" && $1 == "boolDefault:" { print $2; exit }' "${FEATURES}")" \
    "false" "install.tools.ollama is opt-in (boolDefault false, GPU-node hosts only)"
assert_grep "install.sh gates ollama fail-closed with gff_opt_in" 'gff_opt_in install\.tools\.ollama;' "${INSTALL_SH}"
assert_eq "$(grep -E '^_IP_DEPS_FLAGS=' "${INSTALL_SH}" | grep -c -w 'INSTALL_TOOLS_OLLAMA')" "1" \
    "INSTALL_TOOLS_OLLAMA is a deps-phase key"
assert_eq "$(grep -E '^_IP_CONFIG_FLAGS=' "${INSTALL_SH}" | grep -c -w 'INSTALL_TOOLS_OLLAMA')" "0" \
    "INSTALL_TOOLS_OLLAMA is NOT a config-phase key"

_test_report
