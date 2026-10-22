#!/usr/bin/env bash
# Test driver for opt/profiles/.goenv.sh: which Go an interactive shell gets.
#
# Regression: the profile set GOENV_ROOT=$HOME/go while install.sh installs and
# pins Go in $HOME/.goenv (the clone it maintains, which the fleet pins probe
# and opt/etc/fleet/pins.yaml also read). Two goenv trees: a pin bump landed in
# ~/.goenv, every host's PINS read "ok", and every interactive shell kept
# running the old Go from ~/go. The profile also ran `goenv shell <newest
# installed>`, overriding the `goenv global <pin>` install.sh sets.
#
# Hermetic: a temp HOME with a stub goenv; no real Go is touched.
# Run: bash opt/profiles/goenv_profile_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

GOENV_SH="${SELF_DIR}/.goenv.sh"
TMP=$(mktemp -d)
trap 'rm -rf "${TMP}"' EXIT

# make_home <name> <global> <installed...> -> prints the HOME path
make_home() {
    _h="${TMP}/$1"; _g="$2"; shift 2
    mkdir -p "${_h}/.goenv/bin" "${_h}/.goenv/shims" "${_h}/.goenv/versions"
    for v in "$@"; do mkdir -p "${_h}/.goenv/versions/${v}"; done
    printf '%s\n' "${_g}" > "${_h}/.goenv/version"
    cat > "${_h}/.goenv/bin/goenv" <<'STUB'
#!/bin/sh
case "$1" in
    init) # a shell function, like the real init, so `goenv shell X` sets GOENV_VERSION
        echo 'goenv() { if [ "$1" = shell ]; then export GOENV_VERSION="$2"; else command goenv "$@"; fi; }' ;;
    global) cat "${GOENV_ROOT}/version" ;;
    versions) ls "${GOENV_ROOT}/versions" | sort -V ;;
esac
STUB
    chmod +x "${_h}/.goenv/bin/goenv"
    printf '%s' "${_h}"
}

# effective <shell> <home> [VAR=value ...] -> "GOENV_ROOT|effective version"
effective() {
    _sh="$1"; _home="$2"; shift 2
    # TERM_PROGRAM=vscode takes the editor early-return, so the daily
    # `go install` maintenance never runs in a test.
    env -i HOME="${_home}" PATH=/usr/bin:/bin TERM_PROGRAM=vscode "$@" "${_sh}" -c "
        . '${GOENV_SH}'
        printf '%s|%s' \"\${GOENV_ROOT}\" \"\${GOENV_VERSION:-\$(goenv global)}\"" 2>/dev/null
}

for sh in bash zsh; do
    command -v "${sh}" >/dev/null 2>&1 || { echo "SKIP: ${sh} not installed"; continue; }

    # 1. One root: the tree install.sh maintains.
    H=$(make_home "${sh}-pin" 1.26.9 1.26.1 1.26.9 1.27.0)
    out=$(effective "${sh}" "${H}")
    assert_eq "${out%%|*}" "${H}/.goenv" "${sh}: GOENV_ROOT is \$HOME/.goenv (install.sh's tree)"

    # 2. The pin install.sh set as global wins, even with a newer Go installed.
    assert_eq "${out#*|}" "1.26.9" "${sh}: the goenv global pin is honoured over the newest installed"

    # 3. No usable global (system, or a pin not installed yet) -> newest installed.
    H=$(make_home "${sh}-system" system 1.26.1 1.27.0)
    out=$(effective "${sh}" "${H}")
    assert_eq "${out#*|}" "1.27.0" "${sh}: global=system falls back to the newest installed"
    H=$(make_home "${sh}-missing" 1.28.0 1.26.1 1.27.0)
    out=$(effective "${sh}" "${H}")
    assert_eq "${out#*|}" "1.27.0" "${sh}: a global that is not installed falls back to the newest installed"

    # 4. An explicit GOENV_VERSION is never overridden.
    H=$(make_home "${sh}-explicit" 1.26.9 1.26.1 1.26.9)
    out=$(effective "${sh}" "${H}" GOENV_VERSION=1.26.1)
    assert_eq "${out#*|}" "1.26.1" "${sh}: an explicit GOENV_VERSION is left alone"
done

# 5. A login shell (.profile, no .goenv.sh) finds go in $HOME/.goenv/shims,
#    never the retired $HOME/go/shims, whose shims run the old tree's Go
#    (wslgiga1/spark1/jetson1 on 2026-10-22: `bash -lc 'go version'` was 1.26.1).
PROFILE="${SELF_DIR}/.profile"
for sh in bash sh; do
    command -v "${sh}" >/dev/null 2>&1 || { echo "SKIP: ${sh} not installed"; continue; }
    H="${TMP}/${sh}-login"
    for tree in go .goenv; do
        mkdir -p "${H}/${tree}/shims"
        printf '#!/bin/sh\necho %s\n' "${tree}" > "${H}/${tree}/shims/go"
        chmod +x "${H}/${tree}/shims/go"
    done
    out=$(env -i HOME="${H}" PATH=/usr/bin:/bin "${sh}" -c ". '${PROFILE}' >/dev/null 2>&1; go" 2>/dev/null)
    assert_eq "${out}" ".goenv" "${sh}: a login shell runs go from \$HOME/.goenv/shims, not the retired \$HOME/go/shims"
done

_test_report
