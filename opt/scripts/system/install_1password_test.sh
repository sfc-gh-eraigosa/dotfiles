#!/usr/bin/env bash
# Test driver for opt/scripts/system/install_1password.sh
#
# What must hold:
#   * the release zip's bundled op.sig is verified with gpg against the PINNED
#     signing-key fingerprint, and a missing signature, a bad signature or a
#     signature from another key is REFUSED (fail-closed, nothing installed);
#   * a host already on OP_VERSION is a no-op with no download (fleet update
#     re-runs install.sh everywhere); another version is replaced;
#   * an `op` managed outside ~/opt/bin is left alone;
#   * every fleet platform maps to a 1Password asset (amd64, arm64, 32-bit
#     arm Pi) and macOS goes through the brew cask;
#   * install.sh gates it fail-CLOSED (gff_opt_in, boolDefault false) as a
#     deps-phase key, and versions.tsv pins it.
#
# Network is never touched: OP_RELEASE_BASE / OP_KEY_URL point curl at file://
# fixtures signed by a throwaway test key, and `uname` is a stub.
#
# Run: bash opt/scripts/system/install_1password_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/install_1password.sh"
INSTALL_SH="${REPO_ROOT}/install.sh"
FEATURES="${REPO_ROOT}/.github/gff/features.yaml"
VERSIONS="${REPO_ROOT}/opt/profiles/versions.tsv"

assert_file_exists "${SCRIPT}" "install_1password.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
set +e

# --- source-level ------------------------------------------------------------
assert_grep "verifies op.sig with gpg" 'gpg[^|]*--verify[^|]*op\.sig' "${SCRIPT}"
assert_grep "pins the 1Password signing-key fingerprint" '3FEF9748469ADBE15DA7CA80AC2D62742012EA22' "${SCRIPT}"
assert_grep "fetches the key from 1Password's own host, not a keyserver" 'downloads\.1password\.com/linux/keys/1password\.asc' "${SCRIPT}"
assert_grep_negative "never asks a keyserver" '--keyserver|--recv-keys|--receive-keys' "${SCRIPT}"
assert_grep "fails closed without gpg" 'gpg is required[^"]*refusing to install' "${SCRIPT}"
assert_grep_negative "never pipes a remote script into a shell" 'curl[^|]*\|[[:space:]]*(ba)?sh' "${SCRIPT}"
assert_grep "maps aarch64 (Spark, Jetson, 64-bit Pi)" 'arm64\|aarch64\) +OP_ARCH="arm64"' "${SCRIPT}"
assert_grep "maps 32-bit ARM Pis" 'armv7l\|armv6l\) +OP_ARCH="arm"' "${SCRIPT}"
assert_grep "macOS uses the brew cask" 'brew install --cask 1password-cli' "${SCRIPT}"
assert_grep "installs to ~/opt/bin like the other fetched tools" 'OP_INSTALL_DIR:-\$\{HOME\}/opt/bin' "${SCRIPT}"

# --- fixtures: a throwaway signing key and fake releases ------------------------
TMP="$(mktemp -d "${TMPDIR:-/tmp}/install_1password_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
BIN="${TMP}/stubs"; mkdir -p "${BIN}"
# shellcheck disable=SC2016 # $1 belongs to the stub, expanded when it runs
printf '#!/bin/sh\ncase "$1" in -s) echo Linux ;; -m) echo x86_64 ;; *) echo Linux ;; esac\n' > "${BIN}/uname"
chmod +x "${BIN}/uname"

if ! command -v gpg >/dev/null 2>&1 || ! command -v zip >/dev/null 2>&1 || ! command -v unzip >/dev/null 2>&1; then
    echo "SKIP: gpg, zip and unzip are needed to build the signed fixtures (missing on this host)"
    _test_report
    exit $?
fi

# Two keys: the "1Password" key the fixtures are signed with, and a stranger.
make_key() {  # make_key <home> <name>  -> prints the fingerprint
    mkdir -p "$1" && chmod 700 "$1"
    GNUPGHOME="$1" gpg --batch --quiet --passphrase '' --pinentry-mode loopback \
        --quick-gen-key "$2 <$2@test.invalid>" ed25519 sign never >/dev/null 2>&1 \
        || GNUPGHOME="$1" gpg --batch --quiet --passphrase '' --pinentry-mode loopback \
        --quick-gen-key "$2 <$2@test.invalid>" default sign never >/dev/null 2>&1
    GNUPGHOME="$1" gpg --batch --with-colons --fingerprint 2>/dev/null | awk -F: '$1 == "fpr" { print $10; exit }'
}
SIGNER_HOME="${TMP}/signer"; STRANGER_HOME="${TMP}/stranger"
SIGNER_FPR="$(make_key "${SIGNER_HOME}" op-release)"
STRANGER_FPR="$(make_key "${STRANGER_HOME}" stranger)"
assert_eq "${#SIGNER_FPR}" "40" "generated a test signing key"
assert_eq "${#STRANGER_FPR}" "40" "generated a stranger key"
KEY_FILE="${TMP}/1password.asc"
# The key file carries BOTH keys: a valid signature from the wrong key must
# still be refused, which only the fingerprint pin can do.
{ GNUPGHOME="${SIGNER_HOME}" gpg --batch --armor --export "${SIGNER_FPR}"; GNUPGHOME="${STRANGER_HOME}" gpg --batch --armor --export "${STRANGER_FPR}"; } > "${KEY_FILE}" 2>/dev/null

REL="${TMP}/releases"
make_release() {  # make_release <version> [signer-home|none]
    _v="$1"; _home="${2:-${SIGNER_HOME}}"
    _dir="${TMP}/build/${_v}"; rm -rf "${_dir}"; mkdir -p "${_dir}" "${REL}/v${_v}"
    printf '#!/bin/sh\necho "%s"\n' "${_v}" > "${_dir}/op"
    chmod +x "${_dir}/op"
    if [ "${_home}" != "none" ]; then
        GNUPGHOME="${_home}" gpg --batch --quiet --passphrase '' --pinentry-mode loopback \
            --detach-sign --output "${_dir}/op.sig" "${_dir}/op" 2>/dev/null
    fi
    rm -f "${REL}/v${_v}/op_linux_amd64_v${_v}.zip"
    if [ -f "${_dir}/op.sig" ]; then (cd "${_dir}" && zip -q "${REL}/v${_v}/op_linux_amd64_v${_v}.zip" op op.sig)
    else (cd "${_dir}" && zip -q "${REL}/v${_v}/op_linux_amd64_v${_v}.zip" op); fi
}
make_release 2.41.0
make_release 2.40.0

run_op() {  # run_op <install dir> [VAR=value ...]
    _dir="$1"; shift
    env -u OP_VERSION -u OP_FORCE -u GNUPGHOME PATH="${BIN}:${PATH}" OP_INSTALL_DIR="${_dir}" \
        OP_RELEASE_BASE="file://${REL}" OP_KEY_URL="file://${KEY_FILE}" OP_KEY_FINGERPRINT="${SIGNER_FPR}" \
        "$@" bash "${SCRIPT}" 2>&1
}

# 1. Fresh host: verified install of the pinned version.
D="${TMP}/bin-fresh"
out="$(run_op "${D}" OP_VERSION=2.41.0)"; rc=$?
assert_eq "${rc}" "0" "fresh host exits 0"
assert_eq "$("${D}/op" --version 2>/dev/null)" "2.41.0" "the pinned version is installed"
assert_eq "$(printf '%s' "${out}" | grep -c 'gpg signature verified')" "1" "success says it verified the signature"

# 2. Re-run on the pinned version: no-op, no download (fixtures moved away).
mv "${REL}" "${REL}.away"
out="$(run_op "${D}" OP_VERSION=2.41.0)"; rc=$?
assert_eq "${rc}" "0" "already-installed exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c '1password-cli already installed: .*(2.41.0)')" "1" "already-installed is reported"
mv "${REL}.away" "${REL}"

# 3. A different pinned version replaces the installed one.
out="$(run_op "${D}" OP_VERSION=2.40.0)"; rc=$?
assert_eq "${rc}" "0" "version change exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'Updating 1password-cli 2.41.0 -> 2.40.0')" "1" "version change is reported"
assert_eq "$("${D}/op" --version 2>/dev/null)" "2.40.0" "the new version is in place"

# 4. Signed by a stranger: refused, nothing installed.
make_release 2.39.0 "${STRANGER_HOME}"
D2="${TMP}/bin-stranger"
out="$(run_op "${D2}" OP_VERSION=2.39.0)"; rc=$?
assert_eq "${rc}" "1" "foreign signature exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'not the pinned 1Password key')" "1" "foreign signature names the mismatch"
assert_eq "$([ -e "${D2}/op" ] && echo present || echo absent)" "absent" "nothing is installed on a foreign signature"

# 5. Tampered binary (signature no longer matches): refused.
make_release 2.38.0
printf '#!/bin/sh\necho "2.38.0"; echo tampered\n' > "${TMP}/build/2.38.0/op"
(cd "${TMP}/build/2.38.0" && zip -q "${REL}/v2.38.0/op_linux_amd64_v2.38.0.zip" op op.sig)
out="$(run_op "${TMP}/bin-tampered" OP_VERSION=2.38.0)"; rc=$?
assert_eq "${rc}" "1" "tampered binary exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'not a valid signature')" "1" "tampered binary is refused as an invalid signature"

# 6. No op.sig in the zip: refused before anything runs.
make_release 2.37.0 none
out="$(run_op "${TMP}/bin-nosig" OP_VERSION=2.37.0)"; rc=$?
assert_eq "${rc}" "1" "unsigned release exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'has no op.sig')" "1" "unsigned release is refused"

# 7. An op managed outside the install dir is left alone.
OTHER="${TMP}/other-bin"; mkdir -p "${OTHER}"
printf '#!/bin/sh\necho "2.30.0"\n' > "${OTHER}/op"; chmod +x "${OTHER}/op"
out="$(run_op "${TMP}/bin-owned" OP_VERSION=2.41.0 PATH="${OTHER}:${BIN}:${PATH}")"; rc=$?
assert_eq "${rc}" "0" "foreign-managed op exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'is managed outside')" "1" "foreign-managed op is reported and left alone"
assert_eq "$([ -e "${TMP}/bin-owned/op" ] && echo present || echo absent)" "absent" "nothing is installed next to a foreign-managed op"

# 8. A malformed OP_VERSION never reaches a URL.
out="$(run_op "${TMP}/bin-ver" OP_VERSION='2.41.0/../../x')"; rc=$?
assert_eq "${rc}" "1" "malformed OP_VERSION exits 1"

# --- gff + pins wiring --------------------------------------------------------
assert_grep "features.yaml declares install.tools.1password" 'path: install\.tools\.1password$' "${FEATURES}"
assert_eq "$(awk '$2 == "path:" { k = $3 } k == "install.tools.1password" && $1 == "boolDefault:" { print $2; exit }' "${FEATURES}")" \
    "false" "install.tools.1password is opt-in (boolDefault false)"
assert_grep "install.sh gates 1password fail-closed with gff_opt_in" 'gff_opt_in install\.tools\.1password;' "${INSTALL_SH}"
assert_eq "$(grep -E '^_IP_DEPS_FLAGS=' "${INSTALL_SH}" | grep -c -w 'INSTALL_TOOLS_1PASSWORD')" "1" \
    "INSTALL_TOOLS_1PASSWORD is a deps-phase key"
assert_eq "$(grep -E '^_IP_CONFIG_FLAGS=' "${INSTALL_SH}" | grep -c -w 'INSTALL_TOOLS_1PASSWORD')" "0" \
    "INSTALL_TOOLS_1PASSWORD is NOT a config-phase key"
assert_eq "$(awk '$1 == "1password-cli" { print $2 }' "${VERSIONS}")" "pin" "versions.tsv pins 1password-cli"
assert_eq "$(awk '$1 == "1password-cli" { print $5 }' "${VERSIONS}")" "opt/scripts/system/install_1password.sh" "versions.tsv names this installer"

_test_report
