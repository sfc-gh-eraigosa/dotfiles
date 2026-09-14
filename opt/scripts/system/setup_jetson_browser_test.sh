#!/usr/bin/env bash
# Test driver for opt/scripts/system/setup_jetson_browser.sh
#
# What must hold (fleet history 2026-09-11, a Jetson Nano):
#   * it installs the `chromium` .deb — never the `chromium-browse` typo that
#     made every Jetson run fail with "Unable to locate package", and never
#     Ubuntu's `chromium-browser`, a snap transitional package that does not
#     run on Jetson;
#   * with no chromium candidate it skips with a message saying why, and calls
#     nothing that needs sudo;
#   * an already-installed chromium and an already-set default are no-ops, so a
#     host that is up to date never calls sudo;
#   * install.sh delegates to this script inside the Jetson block.
#
# Every system tool is a stub on PATH that logs its argv; nothing is installed.
#
# Run: bash opt/scripts/system/setup_jetson_browser_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/setup_jetson_browser.sh"
INSTALL_SH="${REPO_ROOT}/install.sh"

assert_file_exists "${SCRIPT}" "setup_jetson_browser.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
set +e

TMP="$(mktemp -d "${TMPDIR:-/tmp}/setup_jetson_browser_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
BIN="${TMP}/stubs"
mkdir -p "${BIN}"
LOG="${TMP}/calls.log"

# stub <name> <body>: a /bin/sh stub that logs "<name> <argv>" then runs <body>.
stub() {
    printf '#!/bin/sh\necho "%s $*" >> "%s"\n%s\n' "$1" "${LOG}" "$2" > "${BIN}/$1"
    chmod +x "${BIN}/$1"
}

# world <candidate> <installed:yes|no> <current-default>
world() {
    : > "${LOG}"
    stub apt-get 'exit 0'
    stub apt-cache "printf 'chromium:\n  Installed: (none)\n  Candidate: %s\n' '$1'"
    if [ "$2" = yes ]; then
        stub dpkg-query "printf 'install ok installed'"
    else
        stub dpkg-query "exit 1"
    fi
    # sudo runs the command it was given, so the wrapped call is logged too.
    # shellcheck disable=SC2016 # expanded by the stub, not here
    stub sudo 'while [ "${1#*=}" != "$1" ]; do shift; done; exec "$@"'
    # shellcheck disable=SC2016 # expanded by the stub, not here
    stub update-alternatives 'case "$1" in --query) printf "Name: x\nValue: %s\n" "'"$3"'" ;; esac; exit 0'
    stub xdg-settings 'case "$1" in get) echo other.desktop ;; esac; exit 0'
    CHROMIUM="${TMP}/usr-bin-chromium"
    : > "${CHROMIUM}"
    chmod +x "${CHROMIUM}"
}

run() {
    out="$(PATH="${BIN}:/usr/bin:/bin" CHROMIUM_BIN="${CHROMIUM}" bash "${SCRIPT}" 2>&1)"
    rc=$?
}

# --- candidate available, not installed → installs `chromium` ------------------
world "152.0.7977.82-1xtradeb1" no "/usr/bin/chromium-browser"
run
assert_eq "${rc}" "0" "exits 0 after installing"
assert_grep "installs the chromium .deb" '^apt-get install -y -qq chromium$' "${LOG}"
assert_grep_negative "never installs the chromium-browse typo" 'chromium-browse([^r]|$)' "${LOG}"
assert_grep_negative "never installs Ubuntu's snap-transitional chromium-browser" 'apt-get install.*chromium-browser' "${LOG}"
assert_grep "points x-www-browser at chromium" "^update-alternatives --set x-www-browser ${CHROMIUM}\$" "${LOG}"
assert_grep "points gnome-www-browser at chromium" "^update-alternatives --set gnome-www-browser ${CHROMIUM}\$" "${LOG}"
assert_grep "sets the desktop default" '^xdg-settings set default-web-browser chromium.desktop$' "${LOG}"

# --- already installed and already the default → no sudo at all --------------
world "152.0.7977.82-1xtradeb1" yes "${TMP}/usr-bin-chromium"
run
assert_eq "${rc}" "0" "up-to-date host exits 0"
assert_grep_negative "an installed chromium is not reinstalled" '^apt-get install' "${LOG}"
assert_grep_negative "an up-to-date host never calls sudo" '^sudo ' "${LOG}"
assert_eq "$(printf '%s' "${out}" | grep -c 'already installed')" "1" "says chromium is already installed"

# --- no candidate → skip with the reason, nothing that needs sudo --------------
world "(none)" no "/usr/bin/chromium-browser"
rm -f "${CHROMIUM}"
run
assert_eq "${rc}" "0" "no candidate is not fatal"
assert_grep_negative "no candidate: no apt-get install" '^apt-get install' "${LOG}"
assert_grep_negative "no candidate: no sudo" '^sudo ' "${LOG}"
assert_eq "$(printf '%s' "${out}" | grep -c 'snap')" "1" "the skip message explains the snap-only chromium-browser"

# --- install fails → warning, still exit 0 (install.sh keeps going) -----------
world "152.0.7977.82-1xtradeb1" no "/usr/bin/chromium-browser"
stub apt-get 'exit 100'
rm -f "${CHROMIUM}"
run
assert_eq "${rc}" "0" "a failed install does not abort install.sh"
assert_eq "$(printf '%s' "${out}" | grep -c '^WARNING: could not install chromium')" "1" "a failed install is reported once"
assert_grep_negative "no default is set to a browser that did not install" '^update-alternatives --set' "${LOG}"

# --- install.sh wiring ---------------------------------------------------------
assert_grep "install.sh delegates the Jetson browser to this script" 'setup_jetson_browser\.sh' "${INSTALL_SH}"
assert_grep_negative "install.sh no longer carries the chromium-browse typo" 'apt-get install[^#]*chromium-browse([^r]|$)' "${INSTALL_SH}"

_test_report
