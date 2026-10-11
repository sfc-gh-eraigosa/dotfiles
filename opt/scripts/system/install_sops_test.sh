#!/usr/bin/env bash
# Test driver for opt/scripts/system/install_sops.sh (objective `pins`, F3).
#
# What must hold (the converging-installer contract, plan §3.2):
#   * absent              -> installs WANT (the versions.tsv pin by default);
#   * installed == WANT   -> "already at", exit 0, NO download;
#   * installed older     -> reinstalls WANT;
#   * SOPS_FORCE=1        -> reinstalls even when equal;
#   * a sops that lives OUTSIDE INSTALL_DIR (apt/brew-owned) is left alone;
#   * SOPS_VERSION=x.y.z  -> the env override wins over the manifest;
#   * WANT=latest         -> resolved via pins_latest_gh before downloading.
#
# Network is never touched: SOPS_RELEASE_BASE points curl at a file:// fixture,
# PINS_GH_API at a file:// "API", `uname` is a stub, and PATH is a sandbox of
# symlinked coreutils so a sops on the build host can't leak in.
#
# Run: bash opt/scripts/system/install_sops_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/install_sops.sh"
assert_file_exists "${SCRIPT}" "install_sops.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
set +e

assert_grep "default version comes from versions.tsv via pins_want" 'SOPS_VERSION:-\$\(pins_want sops 3\.13\.1\)' "${SCRIPT}"
assert_grep "sources opt/lib/pins.sh relative to itself" 'lib/pins\.sh' "${SCRIPT}"
assert_grep_negative "never pipes a remote script into a shell" 'curl[^|]*\|[[:space:]]*(ba)?sh' "${SCRIPT}"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/install_sops_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT

# Sandbox PATH: only the tools the installer needs, plus a uname stub.
SYS="${TMP}/sys"; mkdir -p "${SYS}"
for t in bash sh env cat cp mv rm mkdir chmod ln ls head tail awk sed grep tr cut sort dirname basename mktemp tar gzip curl sha256sum shasum; do
    p="$(command -v "${t}" 2>/dev/null)" || continue
    case "${p}" in /*) ln -sf "${p}" "${SYS}/${t}" ;; esac
done
# shellcheck disable=SC2016 # $1 belongs to the stub, expanded when it runs
printf '#!/bin/sh\ncase "$1" in -s) echo Linux ;; -m) echo x86_64 ;; *) echo Linux ;; esac\n' > "${SYS}/uname"
chmod +x "${SYS}/uname"

REL="${TMP}/releases"
make_release() {  # make_release <version>
    mkdir -p "${REL}/v${1}"
    printf '#!/bin/sh\necho "sops %s (fixture)"\n' "$1" > "${REL}/v${1}/sops-v${1}.linux.amd64"
}
make_release 3.13.1
make_release 3.12.0
make_release 3.14.0
mkdir -p "${TMP}/api/repos/getsops/sops/releases"
printf '{"tag_name": "v3.14.0"}\n' > "${TMP}/api/repos/getsops/sops/releases/latest"

# A manifest so the default is deterministic (and provably manifest-sourced).
MAN="${TMP}/versions.tsv"
printf 'sops pin 3.13.1 gh:getsops/sops opt/scripts/system/install_sops.sh sops --version\n' > "${MAN}"

run_sops() {  # run_sops <install dir> [VAR=value ...]
    _dir="$1"; shift
    env -u SOPS_VERSION -u SOPS_FORCE -u GFF_PINS_TRACK -u GFF_PINS_TRACK_MODE \
        PATH="${EXTRA_PATH:-${SYS}}:${SYS}" HOME="${TMP}/home" SOPS_INSTALL_DIR="${_dir}" \
        SOPS_RELEASE_BASE="file://${REL}" PINS_MANIFEST="${MAN}" PINS_GH_API="file://${TMP}/api" \
        "$@" bash "${SCRIPT}" 2>&1
}

# 1. absent -> installs the manifest's WANT.
D="${TMP}/bin"
out="$(run_sops "${D}")"; rc=$?
assert_eq "${rc}" "0" "absent: exits 0"
assert_eq "$("${D}/sops" --version 2>/dev/null)" "sops 3.13.1 (fixture)" "absent: installs the pinned version"

# 2. installed == want -> no download (fixtures moved away prove it).
mv "${REL}" "${REL}.away"
out="$(run_sops "${D}")"; rc=$?
assert_eq "${rc}" "0" "equal: exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'already at 3.13.1')" "1" "equal: says already at"
mv "${REL}.away" "${REL}"

# 3. installed older -> reinstalls WANT.
printf '#!/bin/sh\necho "sops 3.12.0 (fixture)"\n' > "${D}/sops"
out="$(run_sops "${D}")"; rc=$?
assert_eq "${rc}" "0" "older: exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c '3.12.0 -> 3.13.1')" "1" "older: reports the upgrade"
assert_eq "$("${D}/sops" --version 2>/dev/null)" "sops 3.13.1 (fixture)" "older: converged to the pin"

# 4. SOPS_FORCE=1 reinstalls even when equal.
out="$(run_sops "${D}" SOPS_FORCE=1)"; rc=$?
assert_eq "${rc}" "0" "force: exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'already at')" "0" "force: does not short-circuit"

# 5. env override wins over the manifest.
out="$(run_sops "${D}" SOPS_VERSION=3.12.0)"; rc=$?
assert_eq "${rc}" "0" "env override: exits 0"
assert_eq "$("${D}/sops" --version 2>/dev/null)" "sops 3.12.0 (fixture)" "env override: SOPS_VERSION wins"
out="$(run_sops "${D}" SOPS_VERSION=v3.13.1)"; rc=$?
assert_eq "$("${D}/sops" --version 2>/dev/null)" "sops 3.13.1 (fixture)" "env override: a leading v is tolerated"

# 6. the manifest drives the default (a bumped pin converges the host).
printf 'sops pin 3.12.0 gh:getsops/sops - -\n' > "${MAN}.bumped"
out="$(run_sops "${D}" PINS_MANIFEST="${MAN}.bumped")"; rc=$?
assert_eq "$("${D}/sops" --version 2>/dev/null)" "sops 3.12.0 (fixture)" "manifest: a changed pin is what gets installed"

# 7. WANT=latest resolves via the GitHub API first.
out="$(run_sops "${D}" GFF_PINS_TRACK_MODE=latest)"; rc=$?
assert_eq "${rc}" "0" "latest: exits 0"
assert_eq "$("${D}/sops" --version 2>/dev/null)" "sops 3.14.0 (fixture)" "latest: installs the resolved upstream release"
out="$(run_sops "${D}" GFF_PINS_TRACK_MODE=latest PINS_GH_API="file://${TMP}/no-api")"; rc=$?
assert_eq "${rc}" "1" "latest: an unresolvable upstream fails instead of guessing"

# 8. a sops outside INSTALL_DIR (apt/brew-owned) is left alone.
OTHER="${TMP}/other"; mkdir -p "${OTHER}"
printf '#!/bin/sh\necho "sops 3.0.0 (distro)"\n' > "${OTHER}/sops"; chmod +x "${OTHER}/sops"
D2="${TMP}/bin2"
mv "${REL}" "${REL}.away"
out="$(EXTRA_PATH="${OTHER}" run_sops "${D2}")"; rc=$?
mv "${REL}.away" "${REL}"
assert_eq "${rc}" "0" "foreign: exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'leaving it alone')" "1" "foreign: says it is leaving it alone"
assert_eq "$([ -e "${D2}/sops" ] && echo present || echo absent)" "absent" "foreign: installs nothing"

# 9. a malformed version never reaches a URL.
out="$(run_sops "${TMP}/bin3" SOPS_VERSION='3.13.1/../../x')"; rc=$?
assert_eq "${rc}" "1" "malformed SOPS_VERSION exits 1"

# 10. no pins lib/manifest at all -> the hardcoded fallback (fail-open).
out="$(run_sops "${TMP}/bin4" PINS_MANIFEST="${TMP}/nope.tsv")"; rc=$?
assert_eq "$("${TMP}/bin4/sops" --version 2>/dev/null)" "sops 3.13.1 (fixture)" "no manifest: falls back to 3.13.1"

_test_report
