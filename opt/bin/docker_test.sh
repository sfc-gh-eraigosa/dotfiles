#!/usr/bin/env bash
# Test driver for the opt/bin/docker shim's quiet probe, and for a quiet
# shell start-up on a host that does not use the optional Docker helpers.
#
# Regression (2026-11-14): a freshly installed Raspberry Pi printed three
# hints at every login: ".ruby.env is missing", ".dindcenv is missing" and
# "docker: no docker CLI found on PATH". The first two were unconditional
# hints for opt-in files. The third: ~/opt/bin/docker is our own shim, always
# executable, so .zshrc kept the oh-my-zsh docker plugin, whose
# `docker --version` probe ran the shim into its fail-loud message.
#
# Hermetic: a temp HOME and a PATH of symlinks; no real docker is touched.
# Run: bash opt/bin/docker_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

TMP=$(mktemp -d)
trap 'rm -rf "${TMP}"' EXIT

# a PATH with the few tools the shim needs and no docker
BIN="${TMP}/bin"; mkdir -p "${BIN}" "${TMP}/home/opt/bin" "${TMP}/real"
for t in bash dirname basename grep cat; do
    p=$(type -P "${t}") && ln -s "${p}" "${BIN}/${t}"
done
cp "${SELF_DIR}/docker" "${TMP}/home/opt/bin/docker"
SHIM="${TMP}/home/opt/bin/docker"
run() { env -i HOME="${TMP}/home" PATH="$1" "${SHIM}" "${@:2}"; }

# 1. No docker behind the shim: the probe says no, and says nothing.
out=$(run "${TMP}/home/opt/bin:${BIN}" --shim-has-backend 2>&1); rc=$?
assert_eq "${rc}" "1" "probe: exit 1 when there is no docker behind the shim"
assert_eq "${out}" "" "probe: prints nothing"

# 2. A normal call there still fails loud (that message is for a person who typed docker).
out=$(run "${TMP}/home/opt/bin:${BIN}" ps 2>&1); rc=$?
assert_eq "${rc}" "127" "normal call with no docker: exit 127"
assert_eq "$(grep -c 'no docker CLI found' <<<"${out}")" "1" "normal call with no docker: says so"

# 3. A real docker later on PATH: the probe says yes, silently, without running it.
printf '#!/bin/sh\necho "real docker $*" >> "%s/calls"\necho "real docker $*"\n' "${TMP}" > "${TMP}/real/docker"
chmod +x "${TMP}/real/docker"
out=$(run "${TMP}/home/opt/bin:${BIN}:${TMP}/real" --shim-has-backend 2>&1); rc=$?
assert_eq "${rc}" "0" "probe: exit 0 when a real docker is later on PATH"
assert_eq "${out}" "" "probe: prints nothing"
assert_eq "$([ -e "${TMP}/calls" ] && echo ran || echo not-run)" "not-run" "probe: does not run the real docker"

# 4. And a normal call is handed to it with its arguments.
out=$(run "${TMP}/home/opt/bin:${BIN}:${TMP}/real" ps -a 2>&1)
assert_eq "${out}" "real docker ps -a" "normal call: handed to the real docker with its arguments"

# 5. .zshrc drops the oh-my-zsh docker plugin when the shim has nothing behind
#    it, and keeps it when it has. The plugin-selection lines are run as they
#    stand in the file.
if command -v zsh >/dev/null 2>&1; then
    sed -n '/^plugins=(git docker/,/^# herdr-ohmyzsh/p' "${REPO_ROOT}/opt/profiles/.zshrc" | sed '$d' > "${TMP}/plugins.zsh"
    assert_eq "$(grep -c -- '--shim-has-backend' "${TMP}/plugins.zsh")" "1" ".zshrc asks the shim before keeping the docker plugin"
    pick() { env -i HOME="${TMP}/home" PATH="$1" "$(command -v zsh)" -fc ". '${TMP}/plugins.zsh'; print -r -- \$plugins" 2>&1; }
    assert_eq "$(pick "${TMP}/home/opt/bin:${BIN}" | grep -c -w docker)" "0" ".zshrc: no docker behind the shim -> the docker plugin is dropped"
    assert_eq "$(pick "${TMP}/home/opt/bin:${BIN}:${TMP}/real" | grep -c -w docker)" "1" ".zshrc: a real docker behind the shim -> the plugin is kept"
    assert_eq "$(pick "${BIN}" | grep -c -w docker)" "0" ".zshrc: no docker at all -> the plugin is dropped (as before)"
else
    echo "SKIP: zsh not installed"
fi

# 6. The opt-in helper files give no hint when they are absent.
assert_eq "$(grep -c 'is missing, you can install' "${REPO_ROOT}/opt/profiles/.profile" "${REPO_ROOT}/opt/profiles/.bash_aliases" | awk -F: '{n += $2} END {print n}')" "0" \
    "no 'is missing, you can install' hint left in .profile or .bash_aliases"
out=$(env -i HOME="${TMP}/home" PATH="${BIN}" bash -c "
    f=\$HOME/.ruby.env; d=\$HOME/.dindcenv
    $(sed -n '/Opt-in: only on a host where setup_ruby-docker.sh was run/,/^            fi/p' "${REPO_ROOT}/opt/profiles/.profile")
    $(sed -n '/^if \[ -f \$HOME\/.dindcenv \]/,/^fi/p' "${REPO_ROOT}/opt/profiles/.bash_aliases")" 2>&1)
assert_eq "${out}" "" "the .ruby.env and .dindcenv blocks print nothing when the files are absent"

_test_report
