#!/usr/bin/env bash
# Test driver for opt/scripts/system/install_uv.sh
#
# What must hold:
#   * the release tarball is verified against the release's per-asset
#     <asset>.sha256 sidecar, and a missing/mismatched checksum is REFUSED
#     (fail-closed);
#   * both `uv` and `uvx` end up on PATH — the AWS Claude plugins
#     (deploy-on-aws/aws-serverless/aws-core, dotfiles#312) need `uvx`
#     specifically;
#   * a host already on UV_VERSION is a no-op with no download (fleet update
#     re-runs install.sh everywhere); another version replaces it;
#   * Linux x86_64/aarch64 are covered; 32-bit ARM is skipped, not failed
#     (matches install_herdr.sh's precedent — no fleet host is 32-bit);
#   * install.sh gates it ON BY DEFAULT (gff_on, boolDefault true) as a
#     deps-phase key — every host needs it, same as yq/sops, not opt-in like
#     glow.
#
# Network is never touched: UV_RELEASE_BASE points curl at a file:// fixture
# and `uname` is a stub.
#
# Run: bash opt/scripts/system/install_uv_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/install_uv.sh"
INSTALL_SH="${REPO_ROOT}/install.sh"
FEATURES="${REPO_ROOT}/.github/gff/features.yaml"
BREWFILE="${REPO_ROOT}/opt/profiles/Brewfile"

assert_file_exists "${SCRIPT}" "install_uv.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
set +e

# --- source-level ------------------------------------------------------------
assert_grep "verifies against the release's <asset>.sha256 sidecar" '\.sha256' "${SCRIPT}"
assert_grep "fails closed on a missing checksum" 'refusing to install an unverified binary' "${SCRIPT}"
assert_grep_negative "never pipes a remote script into a shell" 'curl[^|]*\|[[:space:]]*(ba)?sh' "${SCRIPT}"
assert_grep "maps x86_64 to the gnu-libc asset" 'x86_64\).*unknown-linux-gnu' "${SCRIPT}"
assert_grep "maps aarch64 (Spark, Jetson, 64-bit Pi) to the gnu-libc asset" 'arm64\|aarch64.*unknown-linux-gnu' "${SCRIPT}"
assert_grep "skips 32-bit ARM (no fleet host is 32-bit)" 'armv7l\|armv6l' "${SCRIPT}"
assert_grep "installs to ~/opt/bin like the other fetched tools" 'UV_INSTALL_DIR:-\$\{HOME\}/opt/bin' "${SCRIPT}"
assert_grep "installs the uvx binary alongside uv" 'uvx' "${SCRIPT}"

# --- fixtures: a fake release for Linux/x86_64 --------------------------------
TMP="$(mktemp -d "${TMPDIR:-/tmp}/install_uv_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
BIN="${TMP}/stubs"; mkdir -p "${BIN}"
# shellcheck disable=SC2016 # $1 belongs to the stub, expanded when it runs
printf '#!/bin/sh\ncase "$1" in -s) echo Linux ;; -m) echo x86_64 ;; *) echo Linux ;; esac\n' > "${BIN}/uname"
chmod +x "${BIN}/uname"
REL="${TMP}/releases"
make_release() {  # make_release <version>
    stem="uv-x86_64-unknown-linux-gnu"
    mkdir -p "${TMP}/build/${1}/${stem}" "${REL}/${1}"
    printf '#!/bin/sh\necho "uv %s (fixture)"\n' "$1" > "${TMP}/build/${1}/${stem}/uv"
    printf '#!/bin/sh\necho "uvx %s (fixture)"\n' "$1" > "${TMP}/build/${1}/${stem}/uvx"
    chmod +x "${TMP}/build/${1}/${stem}/uv" "${TMP}/build/${1}/${stem}/uvx"
    tar -czf "${REL}/${1}/${stem}.tar.gz" -C "${TMP}/build/${1}" "${stem}"
    if command -v sha256sum >/dev/null 2>&1; then sum="$(sha256sum < "${REL}/${1}/${stem}.tar.gz" | awk '{ print $1 }')"
    else sum="$(shasum -a 256 < "${REL}/${1}/${stem}.tar.gz" | awk '{ print $1 }')"; fi
    printf '%s  %s\n' "${sum}" "${stem}.tar.gz" > "${REL}/${1}/${stem}.tar.gz.sha256"
}
make_release 0.12.16
make_release 0.11.0

run_uv() {  # run_uv <install dir> [VAR=value ...]
    _dir="$1"; shift
    env -u UV_VERSION -u UV_FORCE PATH="${BIN}:${PATH}" UV_INSTALL_DIR="${_dir}" \
        UV_RELEASE_BASE="file://${REL}" "$@" bash "${SCRIPT}" 2>&1
}

# 1. Fresh host: verified install of the pinned version, both binaries land.
D="${TMP}/bin-fresh"
out="$(run_uv "${D}")"; rc=$?
assert_eq "${rc}" "0" "fresh host exits 0"
assert_eq "$("${D}/uv" --version 2>/dev/null)" "uv 0.12.16 (fixture)" "the pinned uv version is installed"
assert_eq "$("${D}/uvx" --version 2>/dev/null)" "uvx 0.12.16 (fixture)" "uvx is installed alongside uv"
assert_eq "$(printf '%s' "${out}" | grep -c 'sha256 verified')" "1" "success says it verified the checksum"

# 2. Re-run on the pinned version: no-op, no download (fixtures moved away).
mv "${REL}" "${REL}.away"
out="$(run_uv "${D}")"; rc=$?
assert_eq "${rc}" "0" "already-installed exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'uv already installed: .*0.12.16')" "1" "already-installed is reported"
mv "${REL}.away" "${REL}"

# 3. A different pinned version replaces the installed one.
out="$(run_uv "${D}" UV_VERSION=0.11.0)"; rc=$?
assert_eq "${rc}" "0" "version change exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'Updating uv 0.12.16 -> 0.11.0')" "1" "version change is reported"
assert_eq "$("${D}/uv" --version 2>/dev/null)" "uv 0.11.0 (fixture)" "the new version is in place"
assert_eq "$("${D}/uvx" --version 2>/dev/null)" "uvx 0.11.0 (fixture)" "uvx is updated alongside uv"

# 4. Checksum mismatch: refused, nothing installed.
printf '%s  uv-x86_64-unknown-linux-gnu.tar.gz\n' "$(printf '0%.0s' $(seq 1 64))" > "${REL}/0.12.16/uv-x86_64-unknown-linux-gnu.tar.gz.sha256"
D2="${TMP}/bin-bad"
out="$(run_uv "${D2}")"; rc=$?
assert_eq "${rc}" "1" "checksum mismatch exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'checksum mismatch')" "1" "checksum mismatch is the stated reason"
assert_eq "$([ -e "${D2}/uv" ] && echo present || echo absent)" "absent" "nothing is installed on a mismatch"

# 5. A sidecar with no usable content: refused before downloading the binary.
: > "${REL}/0.12.16/uv-x86_64-unknown-linux-gnu.tar.gz.sha256"
out="$(run_uv "${TMP}/bin-nosum")"; rc=$?
assert_eq "${rc}" "1" "empty sidecar exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'refusing to install an unverified binary')" "1" "empty sidecar is refused"

# 6. A malformed UV_VERSION never reaches a URL.
out="$(run_uv "${TMP}/bin-ver" UV_VERSION='0.12.16/../../x')"; rc=$?
assert_eq "${rc}" "1" "malformed UV_VERSION exits 1"

# 7. 32-bit ARM is a graceful skip (exit 0), not a failure — no fleet host is
#    32-bit, and uv's asset matrix for it is not worth the added surface.
ARMBIN="${TMP}/armstubs"; mkdir -p "${ARMBIN}"
# shellcheck disable=SC2016 # $1 belongs to the stub, expanded when it runs
printf '#!/bin/sh\ncase "$1" in -s) echo Linux ;; -m) echo armv7l ;; *) echo Linux ;; esac\n' > "${ARMBIN}/uname"
chmod +x "${ARMBIN}/uname"
out="$(env -u UV_VERSION -u UV_FORCE PATH="${ARMBIN}:${PATH}" UV_INSTALL_DIR="${TMP}/bin-arm" \
       UV_RELEASE_BASE="file://${REL}" bash "${SCRIPT}" 2>&1)"; rc=$?
assert_eq "${rc}" "0" "32-bit ARM exits 0 (skip, not fail)"
assert_eq "$([ -e "${TMP}/bin-arm/uv" ] && echo present || echo absent)" "absent" "32-bit ARM installs nothing"

# --- gff wiring -------------------------------------------------------------
assert_grep "features.yaml declares install.tools.uv" 'path: install\.tools\.uv$' "${FEATURES}"
assert_eq "$(awk '$2 == "path:" { k = $3 } k == "install.tools.uv" && $1 == "boolDefault:" { print $2; exit }' "${FEATURES}")" \
    "true" "install.tools.uv is ON by default (every host needs it, like yq/sops)"
assert_grep "install.sh gates uv with gff_on (default-on, fail-open like yq/sops)" 'gff_on install\.tools\.uv;' "${INSTALL_SH}"
assert_eq "$(grep -E '^_IP_DEPS_FLAGS=' "${INSTALL_SH}" | grep -c -w 'INSTALL_TOOLS_UV')" "1" \
    "INSTALL_TOOLS_UV is a deps-phase key"
assert_eq "$(grep -E '^_IP_CONFIG_FLAGS=' "${INSTALL_SH}" | grep -c -w 'INSTALL_TOOLS_UV')" "0" \
    "INSTALL_TOOLS_UV is NOT a config-phase key"

# macOS gets uv from the Brewfile (mirrors sops/glow's split: static Go/Rust
# binary fetched+verified on Linux, brew on macOS).
assert_grep "Brewfile installs uv on macOS" "brew 'uv'" "${BREWFILE}"

_test_report
