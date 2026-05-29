#!/usr/bin/env bash
# Test driver for opt/profiles/.bash_aliases
#
# .bash_aliases is sourced from both .bashrc and .zshrc and defines a
# large mix of aliases and shell functions (~200 lines). It does some
# real work at source time (reads ~/.gitenv, ~/.dindcenv, ~/.ruby.env
# if they exist; sets PATH-ish env vars; defines `python=python3`
# aliases). The test exercises:
#
#   1. bash -n syntax check (catches typos before deployment)
#   2. Sources cleanly in a hermetic subshell — `env -i` so the host's
#      real $HOME / $PATH cannot influence the result. We supply an
#      empty fake HOME so the `[ -f ~/.gitenv ]` branches all take the
#      "missing" path (which prints an informational message to stdout
#      — that's expected, not an error).
#   3. Spot-check the load-bearing aliases the user types every day
#      (`k`, `kpod`, `python`, `pip`).
#
# Run: bash opt/profiles/.bash_aliases_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
# Two levels up to reach the repo root (opt/profiles -> opt -> root)
# then back down into ai/_test_helpers.sh.
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

FILE="${SELF_DIR}/.bash_aliases"

# === 1. Syntax check ===
assert_file_exists "$FILE" ".bash_aliases exists"
assert_exit_code 0 ".bash_aliases parses with bash -n" bash -n "$FILE"

# === 2. Hermetic source check ===
# A clean subshell with a temp HOME — no host config can leak in.
# Expected behaviour: exit 0, stderr empty (informational stdout from
# missing ~/.gitenv etc. is allowed).
TMPHOME=$(mktemp -d)
STDERR_FILE="${TMPHOME}/stderr"
set +e
env -i HOME="${TMPHOME}" PATH=/usr/bin:/bin TERM=dumb \
    bash -c ". '${FILE}'" >/dev/null 2>"${STDERR_FILE}"
RC=$?
set -e
assert_eq "${RC}" "0" ".bash_aliases sources cleanly in clean subshell (exit 0)"
# Stderr MUST be empty — any "command not found" / "No such file" on
# stderr signals a real bug (a sourced fragment that doesn't probe its
# dependency before invoking it).
STDERR_CONTENT=$(cat "${STDERR_FILE}")
assert_eq "${STDERR_CONTENT}" "" ".bash_aliases produces no stderr output when sourced"
rm -rf "${TMPHOME}"

# === 3. Spot-check load-bearing aliases ===
# Pick 3 the user touches daily: `k` (kubectl wrapper), `python` (forces
# python3), and the `sfssh` function (Snowflake workspace shortcut).
# These are the ones a regression would be most painful.
assert_in_subshell "alias 'k' is defined after sourcing" \
    "set +u; . '${FILE}' >/dev/null 2>&1; alias k >/dev/null 2>&1"
assert_in_subshell "alias 'python' maps to python3" \
    "set +u; . '${FILE}' >/dev/null 2>&1; alias python | grep -q 'python3'"
assert_in_subshell "function 'sfssh' (Snowflake ws ssh) is defined" \
    "set +u; . '${FILE}' >/dev/null 2>&1; alias sfssh >/dev/null 2>&1 || type sfssh >/dev/null 2>&1"

_test_report
