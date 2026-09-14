#!/usr/bin/env bash
# Test driver for opt/scripts/docker/setup_docker_perms.sh
#
# What must hold (fleet history 2026-09-13, the WSL host without NOPASSWD):
#   * a socket that is already root:docker 666 is left alone — no sudo at all,
#     so an up-to-date host never trips "sudo: a terminal is required";
#   * a change that needs root is skipped with ONE plain line when sudo cannot
#     work in this session (not root, `sudo -n` fails, no terminal);
#   * with usable sudo the socket, group and membership are still fixed.
#
# Every system tool is a stub on PATH that logs its argv; the socket is a real
# unix socket in a temp dir (DOCKER_SOCK), so nothing on the host changes.
#
# Run: bash opt/scripts/docker/setup_docker_perms_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/setup_docker_perms.sh"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
set +e

TMP="$(mktemp -d "${TMPDIR:-/tmp}/setup_docker_perms_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
BIN="${TMP}/stubs"
mkdir -p "${BIN}"
LOG="${TMP}/calls.log"
SOCK="${TMP}/docker.sock"
python3 -c 'import socket, sys; socket.socket(socket.AF_UNIX).bind(sys.argv[1])' "${SOCK}"

stub() {
    printf '#!/bin/sh\necho "%s $*" >> "%s"\n%s\n' "$1" "${LOG}" "$2" > "${BIN}/$1"
    chmod +x "${BIN}/$1"
}

# world <socket-state> <sudo-n:ok|fail> [in-group:yes|no]
world() {
    : > "${LOG}"
    stub docker 'exit 0'
    stub uname 'echo Linux'
    if [ "${3:-yes}" = yes ]; then _g="tester docker"; else _g="tester"; fi
    stub id "case \"\$1\" in -u) echo 1000 ;; -un) echo tester ;; -Gn) echo '${_g}' ;; *) echo tester ;; esac"
    stub getent 'exit 0'
    stub groups "echo '${_g}'"
    stub stat "echo '$1'"
    if [ "$2" = ok ]; then
        # shellcheck disable=SC2016 # expanded by the stub
        stub sudo 'exit 0'
    else
        # shellcheck disable=SC2016 # expanded by the stub
        stub sudo 'case "$1" in -n) exit 1 ;; esac; echo "sudo: a terminal is required to read the password" >&2; exit 1'
    fi
}

run() {
    out="$(PATH="${BIN}:/usr/bin:/bin" USER=tester DOCKER_SOCK="${SOCK}" bash "${SCRIPT}" 2>&1 </dev/null)"
    rc=$?
}

# --- socket already root:docker 666 → no sudo at all ---------------------------
world "root:docker 666" fail
run
assert_eq "${rc}" "0" "correct socket: exit 0"
assert_grep_negative "a correct socket is never chown'd or chmod'd" '^sudo (chown|chmod)' "${LOG}"
assert_eq "$(printf '%s' "${out}" | grep -c 'terminal is required')" "0" "no sudo error on an up-to-date host"

# --- group-writable socket and this session is in the group → nothing to do ----
# The WSL host: root:docker 660, user in the docker group. 666 only exists for
# immediate use by a session that is not in the group yet; demanding it here
# made every background run report a skipped sudo change nobody needed.
world "root:docker 660" fail yes
run
assert_grep_negative "a group-writable socket for an in-group session is left alone" '^sudo (chown|chmod)' "${LOG}"
assert_eq "$(printf '%s' "${out}" | grep -c 'needs sudo')" "0" "and nothing is reported as skipped"

# ...but a session NOT in the group still needs the 666 fallback.
world "root:docker 660" ok no
run
assert_grep "a session outside the group still gets the 666 fallback" "^sudo chmod 666 ${SOCK}\$" "${LOG}"

# --- socket needs fixing, no usable sudo → one line, no sudo errors -------------
world "root:root 660" fail
run
assert_eq "${rc}" "0" "no sudo: exit 0"
assert_grep_negative "no sudo: nothing is chown'd" '^sudo chown' "${LOG}"
assert_eq "$(printf '%s' "${out}" | grep -c 'terminal is required')" "0" "no sudo: no sudo error lines"
assert_eq "$(printf '%s' "${out}" | grep -c 'needs sudo')" "1" "no sudo: one line says what was skipped"

# --- socket needs fixing, sudo works → it is fixed ------------------------------
world "root:root 660" ok
run
assert_grep "sudo: the socket group is fixed" "^sudo chown root:docker ${SOCK}\$" "${LOG}"
assert_grep "sudo: the socket mode is fixed" "^sudo chmod 666 ${SOCK}\$" "${LOG}"

# --- user not in the group, no sudo → skipped once, not an error ---------------
world "root:docker 666" fail no
run
assert_grep_negative "no sudo: usermod is not attempted" '^sudo usermod' "${LOG}"
assert_eq "$(printf '%s' "${out}" | grep -c 'terminal is required')" "0" "no sudo: no usermod sudo error"

# --- user not in the group, sudo works → added ----------------------------------
world "root:docker 666" ok no
run
assert_grep "sudo: the user is added to the docker group" '^sudo usermod -aG docker tester$' "${LOG}"

_test_report
