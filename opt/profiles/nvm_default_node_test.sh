#!/usr/bin/env bash
# Test driver for the "nvm-default-node" block: after nvm is loaded with
# --no-use, is `node` still on PATH where nvm's node is the only one?
#
# Regression (2026-11-14): #456 loads nvm with --no-use to save ~5 s per shell
# on the Intel Mac. A freshly installed Raspberry Pi then had node, npm and
# claude installed under ~/.nvm/versions/node/<v>/bin and "command not found"
# for all three. A host with an OS node as well (apt's /usr/bin/node) keeps
# `node` but loses the CLIs npm installed under nvm, claude among them.
#
# The block lives in two places (opt/profiles/.bashrc, and the profile that
# opt/scripts/system/antigravity_install.sh writes to ~/.antigravity.profile).
# This test holds the two copies identical and runs the block under bash, zsh
# and dash (the generated profile is reached from ~/.profile at a GUI login).
#
# Hermetic: a temp HOME with a stub nvm; no real node is touched.
# Run: bash opt/profiles/nvm_default_node_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

TMP=$(mktemp -d)
trap 'rm -rf "${TMP}"' EXIT
# The test PATH is one empty directory, so a node the host has in /usr/bin
# cannot stand in for nvm's. The block itself uses shell builtins only.
EMPTY="${TMP}/empty-bin"; mkdir -p "${EMPTY}"

# block <file> -> the lines between the markers, indentation removed
block() { sed -n '/# >>> nvm-default-node/,/# <<< nvm-default-node/p' "$1" | sed 's/^[[:space:]]*//'; }
block "${SELF_DIR}/.bashrc" > "${TMP}/bashrc.block"
block "${REPO_ROOT}/opt/scripts/system/antigravity_install.sh" > "${TMP}/profile.block"

assert_eq "$(wc -l < "${TMP}/profile.block" | tr -d ' ')" "$(wc -l < "${TMP}/bashrc.block" | tr -d ' ')" "both copies of the block have the same number of lines"
assert_eq "$(cmp -s "${TMP}/bashrc.block" "${TMP}/profile.block" && echo same || echo differ)" "same" "the .bashrc block and the generated-profile block are identical"
assert_eq "$([ -s "${TMP}/profile.block" ] && echo found || echo empty)" "found" "the block is present"
BLOCK="${TMP}/profile.block"

# make_home <name> <default alias> <what `nvm version default` answers> <installed...> -> HOME
make_home() {
    _h="${TMP}/$1"; _alias="$2"; _answer="$3"; shift 3
    mkdir -p "${_h}/.nvm/alias" "${_h}/.nvm/versions/node"
    for v in "$@"; do
        mkdir -p "${_h}/.nvm/versions/node/${v}/bin"
        printf '#!/bin/sh\necho %s\n' "${v}" > "${_h}/.nvm/versions/node/${v}/bin/node"
        printf '#!/bin/sh\necho claude-of-%s\n' "${v}" > "${_h}/.nvm/versions/node/${v}/bin/claude"
        chmod +x "${_h}/.nvm/versions/node/${v}/bin/node" "${_h}/.nvm/versions/node/${v}/bin/claude"
    done
    [ -z "${_alias}" ] || printf '%s\n' "${_alias}" > "${_h}/.nvm/alias/default"
    # the stub nvm records that it was asked, so a test can tell it was not
    cat > "${_h}/.nvm/nvm.sh" <<STUB
nvm() { echo asked >> "${_h}/nvm.calls"; [ "\$1 \$2" = "version default" ] && echo "${_answer}"; }
STUB
    printf '%s' "${_h}"
}

# node_seen <shell> <home> [extra PATH dir] -> "<node> <an npm-installed CLI> leftover=..."
# after the block; "none" where the command is not found
node_seen() {
    _sh="$1"; _home="$2"; _extra="${3:-}"
    env -i HOME="${_home}" PATH="${_extra:+${_extra}:}${EMPTY}" "$(command -v "${_sh}")" -c "
        NVM_DIR=\"\$HOME/.nvm\"
        . \"\$NVM_DIR/nvm.sh\"
        . '${BLOCK}'
        if command -v node >/dev/null 2>&1; then node; else echo none; fi
        if command -v claude >/dev/null 2>&1; then claude; else echo none; fi
        echo \"leftover=\${_nvm_default-unset}\"" 2>&1 | tr '\n' ' ' | sed 's/ $//'
}

for sh in bash zsh dash; do
    command -v "${sh}" >/dev/null 2>&1 || { echo "SKIP: ${sh} not installed"; continue; }

    # 1. The fresh-Pi case: default -> lts/*, nvm resolves it, no other node on PATH.
    H=$(make_home "${sh}-lts" 'lts/*' v26.13.1 v24.18.0 v26.13.1)
    assert_eq "$(node_seen "${sh}" "${H}")" "v26.13.1 claude-of-v26.13.1 leftover=unset" "${sh}: default=lts/* puts nvm's default node and its CLIs on PATH"

    # 2. An exact version in the alias file needs no call to nvm at all.
    H=$(make_home "${sh}-exact" v24.18.0 WRONG v24.18.0 v26.13.1)
    assert_eq "$(node_seen "${sh}" "${H}")" "v24.18.0 claude-of-v24.18.0 leftover=unset" "${sh}: an exact default version is used as written"
    assert_eq "$([ -e "${H}/nvm.calls" ] && echo asked || echo not-asked)" "not-asked" "${sh}: ... without asking nvm (no start-up cost)"

    # 3. Another node already on PATH (Homebrew, nodenv, apt) keeps winning, and
    #    the CLIs npm installed under nvm are still found.
    H=$(make_home "${sh}-other" 'lts/*' v26.13.1 v26.13.1)
    mkdir -p "${H}/other"; printf '#!/bin/sh\necho other-node\n' > "${H}/other/node"; chmod +x "${H}/other/node"
    assert_eq "$(node_seen "${sh}" "${H}" "${H}/other")" "other-node claude-of-v26.13.1 leftover=unset" "${sh}: a node already on PATH wins; nvm's CLIs are still found"

    # 4. nvm's bin already on PATH (a nested shell, or the profile read twice):
    #    nothing is added again and nvm is not asked.
    H=$(make_home "${sh}-nested" 'lts/*' v26.13.1 v26.13.1)
    out=$(env -i HOME="${H}" PATH="${H}/.nvm/versions/node/v26.13.1/bin:${EMPTY}" "$(command -v "${sh}")" -c "
        NVM_DIR=\"\$HOME/.nvm\"; . \"\$NVM_DIR/nvm.sh\"; . '${BLOCK}'; printf '%s' \"\$PATH\"" 2>&1)
    assert_eq "${out}" "${H}/.nvm/versions/node/v26.13.1/bin:${EMPTY}" "${sh}: nvm's bin already on PATH is not added twice"
    assert_eq "$([ -e "${H}/nvm.calls" ] && echo asked || echo not-asked)" "not-asked" "${sh}: ... and nvm is not asked"

    # 5. The default is not installed (nvm answers N/A): PATH is left alone, no error.
    H=$(make_home "${sh}-na" 'lts/*' N/A v24.18.0)
    assert_eq "$(node_seen "${sh}" "${H}")" "none none leftover=unset" "${sh}: a default that is not installed adds nothing"

    # 6. No default alias and no answer: nothing added, no error.
    H=$(make_home "${sh}-noalias" '' '' v24.18.0)
    assert_eq "$(node_seen "${sh}" "${H}")" "none none leftover=unset" "${sh}: no default alias adds nothing"

    # 7. nvm was never installed: the block does nothing.
    mkdir -p "${TMP}/${sh}-nonvm"
    out=$(env -i HOME="${TMP}/${sh}-nonvm" PATH="${EMPTY}" "$(command -v "${sh}")" -c "
        NVM_DIR=\"\$HOME/.nvm\"; . '${BLOCK}'; echo \"rc=\$? path=\$PATH\"" 2>&1)
    assert_eq "${out}" "rc=0 path=${EMPTY}" "${sh}: without nvm the block does nothing"
done

_test_report
