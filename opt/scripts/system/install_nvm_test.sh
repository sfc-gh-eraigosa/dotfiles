#!/usr/bin/env bash
# Test driver for install_nvm.sh (converging nvm install, pins #378 follow-up).
#
#   * absent             -> installs WANT (the versions.tsv pin by default);
#   * already at WANT    -> no download at all;
#   * older nvm          -> re-runs the installer for WANT (updates in place);
#   * NVM_VERSION=x      -> overrides the manifest;
#   * PROFILE=/dev/null  -> the nvm installer never edits a shell rc file (ours
#                           are symlinks into this repo).
#
# Hermetic: NVM_INSTALL_BASE points at a file:// fixture whose install.sh
# writes a fake nvm.sh; no network, no real nvm.
# Run: bash opt/scripts/system/install_nvm_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/install_nvm.sh"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/install_nvm_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT

# Fixture installers: v<ver>/install.sh writes an nvm.sh reporting <ver> and
# logs the run plus the PROFILE it was given.
REL="${TMP}/rel"
for v in 0.39.7 0.40.8; do
    mkdir -p "${REL}/v${v}"
    cat > "${REL}/v${v}/install.sh" <<FIX
#!/usr/bin/env bash
mkdir -p "\${NVM_DIR}"
printf 'nvm() { echo ${v}; }\n' > "\${NVM_DIR}/nvm.sh"
echo "ran ${v} PROFILE=\${PROFILE-unset}" >> "${TMP}/runs.log"
FIX
done
MAN="${TMP}/versions.tsv"
printf 'nvm pin 0.39.7 gh:nvm-sh/nvm opt/scripts/system/install_nvm.sh -\n' > "${MAN}"

run_nvm() {  # run_nvm <nvm dir> [VAR=value ...]
    _dir="$1"; shift
    env -u NVM_VERSION -u NVM_FORCE -u GFF_PINS_TRACK -u GFF_PINS_TRACK_MODE \
        NVM_DIR="${_dir}" NVM_INSTALL_BASE="file://${REL}" \
        PINS_MANIFEST="${MAN}" PINS_GFF="" HOME="${TMP}/home" \
        "$@" bash "${SCRIPT}" 2>&1
}
have() { bash -c '. "$1/nvm.sh" && nvm --version' _ "$1" 2>/dev/null; }
runs() { [ -f "${TMP}/runs.log" ] && wc -l < "${TMP}/runs.log" | tr -d ' ' || echo 0; }

# 1. absent -> installs the manifest pin, with PROFILE=/dev/null.
D="${TMP}/nvm"
out="$(run_nvm "${D}")"; rc=$?
assert_eq "${rc}" "0" "absent: exits 0"
assert_eq "$(have "${D}")" "0.39.7" "absent: installs the manifest pin"
assert_eq "$(grep -c 'PROFILE=/dev/null' "${TMP}/runs.log")" "1" "the nvm installer gets PROFILE=/dev/null"

# 2. already at WANT -> no download.
before=$(runs)
out="$(run_nvm "${D}")"; rc=$?
assert_eq "${rc}" "0" "already at pin: exits 0"
assert_eq "$(runs)" "${before}" "already at pin: the installer is not run"
assert_eq "$(printf '%s' "${out}" | grep -c 'already at 0.39.7')" "1" "already at pin: says so"

# 3. a bumped pin updates in place.
printf 'nvm pin 0.40.8 gh:nvm-sh/nvm opt/scripts/system/install_nvm.sh -\n' > "${MAN}"
out="$(run_nvm "${D}")"; rc=$?
assert_eq "${rc}" "0" "older nvm: exits 0"
assert_eq "$(have "${D}")" "0.40.8" "older nvm: updated to the new pin"
assert_eq "$(printf '%s' "${out}" | grep -c 'Updating nvm 0.39.7 -> 0.40.8')" "1" "older nvm: reports the update"

# 4. NVM_VERSION overrides the manifest.
D2="${TMP}/nvm2"
out="$(run_nvm "${D2}" NVM_VERSION=0.39.7)"; rc=$?
assert_eq "$(have "${D2}")" "0.39.7" "NVM_VERSION override wins over the manifest"

# 5. a malformed version never reaches a URL.
out="$(run_nvm "${TMP}/nvm3" NVM_VERSION='0.39.7/../../x')"; rc=$?
assert_eq "${rc}" "1" "malformed NVM_VERSION exits 1"

_test_report
