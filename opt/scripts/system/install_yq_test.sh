#!/usr/bin/env bash
# Test driver for opt/scripts/system/install_yq.sh (objective `pins`, F3).
#
# What must hold (the converging-installer contract, plan §3.2):
#   * absent              -> installs WANT (the versions.tsv pin by default);
#   * installed == WANT   -> "already at", exit 0, NO download;
#   * installed older     -> reinstalls WANT;
#   * a mikefarah yq OUTSIDE INSTALL_DIR (brew-owned) is left alone, but the
#     unrelated kislyuk (apt python) yq is shadowed by ours, as before;
#   * YQ_VERSION=x.y.z    -> the env override wins over the manifest;
#   * WANT=latest         -> resolved via pins_latest_gh before downloading.
#
# Network is never touched: YQ_RELEASE_BASE points curl at a file:// fixture,
# PINS_GH_API at a file:// "API", `uname` is a stub, and PATH is a sandbox.
#
# Run: bash opt/scripts/system/install_yq_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/install_yq.sh"
assert_file_exists "${SCRIPT}" "install_yq.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
set +e

assert_grep "default version comes from versions.tsv via pins_want" 'YQ_VERSION:-\$\(pins_want yq 4\.44\.6\)' "${SCRIPT}"
assert_grep "sources opt/lib/pins.sh relative to itself" 'lib/pins\.sh' "${SCRIPT}"
assert_grep_negative "never pipes a remote script into a shell" 'curl[^|]*\|[[:space:]]*(ba)?sh' "${SCRIPT}"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/install_yq_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT

SYS="${TMP}/sys"; mkdir -p "${SYS}"
for t in bash sh env cat cp mv rm mkdir chmod ln ls head tail awk sed grep tr cut sort dirname basename mktemp tar gzip curl sha256sum shasum; do
    p="$(command -v "${t}" 2>/dev/null)" || continue
    case "${p}" in /*) ln -sf "${p}" "${SYS}/${t}" ;; esac
done
# shellcheck disable=SC2016 # $1 belongs to the stub, expanded when it runs
printf '#!/bin/sh\ncase "$1" in -s) echo Linux ;; -m) echo x86_64 ;; *) echo Linux ;; esac\n' > "${SYS}/uname"
chmod +x "${SYS}/uname"

fake_yq() {  # fake_yq <version> <path>
    printf '#!/bin/sh\necho "yq (https://github.com/mikefarah/yq/) version v%s"\n' "$1" > "$2"
    chmod +x "$2"
}
REL="${TMP}/releases"
for v in 4.44.6 4.40.0 4.45.1; do
    mkdir -p "${REL}/v${v}"; fake_yq "${v}" "${REL}/v${v}/yq_linux_amd64"
done
mkdir -p "${TMP}/api/repos/mikefarah/yq/releases"
printf '{"tag_name": "v4.45.1"}\n' > "${TMP}/api/repos/mikefarah/yq/releases/latest"

MAN="${TMP}/versions.tsv"
printf 'yq pin 4.44.6 gh:mikefarah/yq opt/scripts/system/install_yq.sh yq --version\n' > "${MAN}"

run_yq() {  # run_yq <install dir> [VAR=value ...]
    _dir="$1"; shift
    env -u YQ_VERSION -u YQ_FORCE -u GFF_PINS_TRACK -u GFF_PINS_TRACK_MODE \
        PATH="${EXTRA_PATH:-${SYS}}:${SYS}" HOME="${TMP}/home" YQ_INSTALL_DIR="${_dir}" \
        YQ_RELEASE_BASE="file://${REL}" PINS_MANIFEST="${MAN}" PINS_GH_API="file://${TMP}/api" \
        "$@" bash "${SCRIPT}" 2>&1
}
ver_of() { "$1" --version 2>/dev/null | sed 's/.*version v//'; }

# 1. absent -> installs WANT.
D="${TMP}/bin"
out="$(run_yq "${D}")"; rc=$?
assert_eq "${rc}" "0" "absent: exits 0"
assert_eq "$(ver_of "${D}/yq")" "4.44.6" "absent: installs the pinned version"

# 2. installed == want -> no download.
mv "${REL}" "${REL}.away"
out="$(run_yq "${D}")"; rc=$?
mv "${REL}.away" "${REL}"
assert_eq "${rc}" "0" "equal: exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'already at 4.44.6')" "1" "equal: says already at"

# 3. installed older -> reinstalls WANT.
fake_yq 4.40.0 "${D}/yq"
out="$(run_yq "${D}")"; rc=$?
assert_eq "${rc}" "0" "older: exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c '4.40.0 -> 4.44.6')" "1" "older: reports the upgrade"
assert_eq "$(ver_of "${D}/yq")" "4.44.6" "older: converged to the pin"

# 4. env override wins.
out="$(run_yq "${D}" YQ_VERSION=4.40.0)"; rc=$?
assert_eq "${rc}" "0" "env override: exits 0"
assert_eq "$(ver_of "${D}/yq")" "4.40.0" "env override: YQ_VERSION wins"

# 5. YQ_FORCE=1 reinstalls even when equal.
out="$(run_yq "${D}" YQ_VERSION=4.40.0 YQ_FORCE=1)"; rc=$?
assert_eq "$(printf '%s' "${out}" | grep -c 'already at')" "0" "force: does not short-circuit"

# 6. WANT=latest resolves via the GitHub API first.
out="$(run_yq "${D}" GFF_PINS_TRACK_MODE=latest)"; rc=$?
assert_eq "${rc}" "0" "latest: exits 0"
assert_eq "$(ver_of "${D}/yq")" "4.45.1" "latest: installs the resolved upstream release"

# 7. a mikefarah yq outside INSTALL_DIR (brew) is left alone.
OTHER="${TMP}/other"; mkdir -p "${OTHER}"; fake_yq 4.30.0 "${OTHER}/yq"
D2="${TMP}/bin2"
mv "${REL}" "${REL}.away"
out="$(EXTRA_PATH="${OTHER}" run_yq "${D2}")"; rc=$?
mv "${REL}.away" "${REL}"
assert_eq "${rc}" "0" "foreign mikefarah: exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'leaving it alone')" "1" "foreign mikefarah: says it is leaving it alone"
assert_eq "$([ -e "${D2}/yq" ] && echo present || echo absent)" "absent" "foreign mikefarah: installs nothing"

# 8. the kislyuk python yq (apt) is NOT the tool we want: ours is installed beside it.
KIS="${TMP}/kislyuk"; mkdir -p "${KIS}"
printf '#!/bin/sh\necho "yq 3.4.3"\n' > "${KIS}/yq"; chmod +x "${KIS}/yq"
D3="${TMP}/bin3"
out="$(EXTRA_PATH="${KIS}" run_yq "${D3}")"; rc=$?
assert_eq "${rc}" "0" "kislyuk: exits 0"
assert_eq "$(ver_of "${D3}/yq")" "4.44.6" "kislyuk: mikefarah yq installed into INSTALL_DIR"

_test_report
