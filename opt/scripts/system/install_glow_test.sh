#!/usr/bin/env bash
# Test driver for opt/scripts/system/install_glow.sh
#
# What must hold:
#   * the release tarball is verified against the release's checksums.txt, and
#     a missing, malformed or mismatched checksum is REFUSED (fail-closed);
#   * a host already on GLOW_VERSION is a no-op with no download (fleet update
#     re-runs install.sh everywhere); another version is replaced;
#   * every fleet platform maps to a glow asset (x86_64, arm64, 32-bit arm Pi,
#     macOS);
#   * install.sh gates it fail-CLOSED (gff_opt_in, boolDefault false) as a
#     deps-phase key.
#
# Network is never touched: GLOW_RELEASE_BASE points curl at a file:// fixture
# and `uname` is a stub.
#
# Run: bash opt/scripts/system/install_glow_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/install_glow.sh"
INSTALL_SH="${REPO_ROOT}/install.sh"
FEATURES="${REPO_ROOT}/.github/gff/features.yaml"

assert_file_exists "${SCRIPT}" "install_glow.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
set +e

# --- source-level ------------------------------------------------------------
assert_grep "verifies against the release checksums.txt" 'checksums\.txt' "${SCRIPT}"
assert_grep "fails closed on a missing checksum" 'refusing to install an unverified binary' "${SCRIPT}"
assert_grep_negative "never pipes a remote script into a shell" 'curl[^|]*\|[[:space:]]*(ba)?sh' "${SCRIPT}"
assert_grep "maps aarch64 (Spark, Jetson, 64-bit Pi)" 'arm64\|aarch64\) +GLOW_ARCH="arm64"' "${SCRIPT}"
assert_grep "maps 32-bit ARM Pis" 'armv7l\|armv6l\) +GLOW_ARCH="arm"' "${SCRIPT}"
assert_grep "maps macOS" 'Darwin\) GLOW_OS="Darwin"' "${SCRIPT}"
assert_grep "installs to ~/opt/bin like the other fetched tools" 'GLOW_INSTALL_DIR:-\$\{HOME\}/opt/bin' "${SCRIPT}"

# --- fixtures: a fake release for Linux/x86_64 --------------------------------
TMP="$(mktemp -d "${TMPDIR:-/tmp}/install_glow_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
BIN="${TMP}/stubs"; mkdir -p "${BIN}"
# shellcheck disable=SC2016 # $1 belongs to the stub, expanded when it runs
printf '#!/bin/sh\ncase "$1" in -s) echo Linux ;; -m) echo x86_64 ;; *) echo Linux ;; esac\n' > "${BIN}/uname"
chmod +x "${BIN}/uname"
REL="${TMP}/releases"
make_release() {  # make_release <version>
    stem="glow_${1}_Linux_x86_64"
    mkdir -p "${TMP}/build/${stem}" "${REL}/v${1}"
    printf '#!/bin/sh\necho "glow version v%s (fixture)"\n' "$1" > "${TMP}/build/${stem}/glow"
    chmod +x "${TMP}/build/${stem}/glow"
    tar -czf "${REL}/v${1}/${stem}.tar.gz" -C "${TMP}/build" "${stem}"
    if command -v sha256sum >/dev/null 2>&1; then sum="$(sha256sum < "${REL}/v${1}/${stem}.tar.gz" | awk '{ print $1 }')"
    else sum="$(shasum -a 256 < "${REL}/v${1}/${stem}.tar.gz" | awk '{ print $1 }')"; fi
    printf '%s  %s\n%s  %s.sbom.json\n' "${sum}" "${stem}.tar.gz" "$(printf 'a%.0s' $(seq 1 64))" "${stem}.tar.gz" > "${REL}/v${1}/checksums.txt"
}
make_release 3.0.0
make_release 2.1.1

run_glow() {  # run_glow <install dir> [VAR=value ...]
    _dir="$1"; shift
    env -u GLOW_VERSION -u GLOW_FORCE PATH="${BIN}:${PATH}" GLOW_INSTALL_DIR="${_dir}" \
        GLOW_RELEASE_BASE="file://${REL}" "$@" bash "${SCRIPT}" 2>&1
}

# 1. Fresh host: verified install of the pinned version.
D="${TMP}/bin-fresh"
out="$(run_glow "${D}")"; rc=$?
assert_eq "${rc}" "0" "fresh host exits 0"
assert_eq "$("${D}/glow" --version 2>/dev/null)" "glow version v3.0.0 (fixture)" "the pinned version is installed"
assert_eq "$(printf '%s' "${out}" | grep -c 'sha256 verified')" "1" "success says it verified the checksum"

# 2. Re-run on the pinned version: no-op, no download (fixtures moved away).
mv "${REL}" "${REL}.away"
out="$(run_glow "${D}")"; rc=$?
assert_eq "${rc}" "0" "already-installed exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'glow already installed: .*(3.0.0)')" "1" "already-installed is reported"
mv "${REL}.away" "${REL}"

# 3. A different pinned version replaces the installed one.
out="$(run_glow "${D}" GLOW_VERSION=2.1.1)"; rc=$?
assert_eq "${rc}" "0" "version change exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'Updating glow 3.0.0 -> 2.1.1')" "1" "version change is reported"
assert_eq "$("${D}/glow" --version 2>/dev/null)" "glow version v2.1.1 (fixture)" "the new version is in place"

# 4. Checksum mismatch: refused, nothing installed.
printf '%s  glow_3.0.0_Linux_x86_64.tar.gz\n' "$(printf '0%.0s' $(seq 1 64))" > "${REL}/v3.0.0/checksums.txt"
D2="${TMP}/bin-bad"
out="$(run_glow "${D2}")"; rc=$?
assert_eq "${rc}" "1" "checksum mismatch exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'checksum mismatch')" "1" "checksum mismatch is the stated reason"
assert_eq "$([ -e "${D2}/glow" ] && echo present || echo absent)" "absent" "nothing is installed on a mismatch"

# 5. No checksum listed for this asset: refused before downloading it.
printf '%s  glow_3.0.0_Darwin_arm64.tar.gz\n' "$(printf 'b%.0s' $(seq 1 64))" > "${REL}/v3.0.0/checksums.txt"
out="$(run_glow "${TMP}/bin-nosum")"; rc=$?
assert_eq "${rc}" "1" "unlisted asset exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'refusing to install an unverified binary')" "1" "unlisted asset is refused"

# 6. A malformed GLOW_VERSION never reaches a URL.
out="$(run_glow "${TMP}/bin-ver" GLOW_VERSION='3.0.0/../../x')"; rc=$?
assert_eq "${rc}" "1" "malformed GLOW_VERSION exits 1"

# --- gff wiring -------------------------------------------------------------
assert_grep "features.yaml declares install.tools.glow" 'path: install\.tools\.glow$' "${FEATURES}"
assert_eq "$(awk '$2 == "path:" { k = $3 } k == "install.tools.glow" && $1 == "boolDefault:" { print $2; exit }' "${FEATURES}")" \
    "false" "install.tools.glow is opt-in (boolDefault false)"
assert_grep "install.sh gates glow fail-closed with gff_opt_in" 'gff_opt_in install\.tools\.glow;' "${INSTALL_SH}"
assert_eq "$(grep -E '^_IP_DEPS_FLAGS=' "${INSTALL_SH}" | grep -c -w 'INSTALL_TOOLS_GLOW')" "1" \
    "INSTALL_TOOLS_GLOW is a deps-phase key"
assert_eq "$(grep -E '^_IP_CONFIG_FLAGS=' "${INSTALL_SH}" | grep -c -w 'INSTALL_TOOLS_GLOW')" "0" \
    "INSTALL_TOOLS_GLOW is NOT a config-phase key"

_test_report
