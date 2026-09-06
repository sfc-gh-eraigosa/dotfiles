#!/usr/bin/env bash
# Test driver for opt/profiles/.bashrc and opt/profiles/.zshrc
#
# Both files do real work on source — set up PATH, source helper
# fragments, define functions. We exercise them in fully hermetic
# subshells (env -i) with a controlled PATH so the host's real config
# cannot leak in or be mutated. Each rc gets:
#
#   1. Syntax check: bash -n (.bashrc); bash -n + zsh -n (.zshrc, both
#      because zsh accepts the file in practice and bash -n is a useful
#      smoke check for portable constructs).
#   2. Clean-source check: the file sources without errors in a clean
#      subshell. The expected contract is GRACEFUL DEGRADATION — every
#      external tool reference (`fnm`, `direnv`, `pyenv`, etc.) must be
#      guarded so the rc completes cleanly even when the tool is absent.
#   3. No stderr noise: stderr must be empty after a clean source.
#      "command not found", "no such file or directory", or similar
#      stderr output indicates an unguarded fragment — a real bug.
#
# `.zshrc` runs `source $ZSH/oh-my-zsh.sh` UNCONDITIONALLY (line ~185),
# so the test stubs out an empty `oh-my-zsh.sh` inside the temp HOME
# before sourcing. The stub-and-source pattern documents the dependency
# without papering over it: if the unconditional source ever moves
# behind an `[ -f ... ]` guard, the stub becomes unnecessary and the
# test still passes.
#
# Run: bash opt/profiles/rc_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

BASHRC="${SELF_DIR}/.bashrc"
ZSHRC="${SELF_DIR}/.zshrc"

# === .bashrc ===
assert_file_exists "${BASHRC}" ".bashrc exists"
assert_exit_code 0 ".bashrc parses with bash -n" bash -n "${BASHRC}"

TMPHOME=$(mktemp -d)
STDERR_FILE="${TMPHOME}/bashrc.stderr"
# TERM_PROGRAM=vscode forces EDITOR_TERMINAL=true, which keeps the rc
# on the fast-path (skips expensive Docker socket fix, daily git pull,
# etc.). The clean-source contract is unchanged either way; this just
# makes the test deterministic and quick.
set +e
env -i HOME="${TMPHOME}" PATH=/usr/bin:/bin TERM=dumb TERM_PROGRAM=vscode \
    bash -c ". '${BASHRC}'" >/dev/null 2>"${STDERR_FILE}"
RC=$?
set -e
assert_eq "${RC}" "0" ".bashrc sources cleanly in clean subshell (exit 0)"
BASHRC_STDERR=$(cat "${STDERR_FILE}")
assert_eq "${BASHRC_STDERR}" "" ".bashrc produces no stderr output when sourced"
rm -rf "${TMPHOME}"

# === .zshrc ===
assert_file_exists "${ZSHRC}" ".zshrc exists"
# Note: .zshrc uses zsh-specific syntax (`${^fpath}`, `(N@)` glob
# qualifiers) that `bash -n` rejects — so we skip a bash syntax check
# and rely on `zsh -n` below.

if command -v zsh >/dev/null 2>&1; then
    assert_exit_code 0 ".zshrc parses with zsh -n" zsh -n "${ZSHRC}"

    # Source check: stub oh-my-zsh so the unconditional `source` at line
    # ~185 finds something. Stub is empty — we're only proving the rc
    # itself doesn't error, not exercising oh-my-zsh.
    TMPHOME=$(mktemp -d)
    mkdir -p "${TMPHOME}/.oh-my-zsh"
    : > "${TMPHOME}/.oh-my-zsh/oh-my-zsh.sh"
    STDERR_FILE="${TMPHOME}/zshrc.stderr"
    set +e
    # TERM_PROGRAM=vscode -> EDITOR_TERMINAL=true: skips the
    # zsh-completions `git clone` (line ~146) and the daily-maintenance
    # background job — both write to the host, which we MUST
    # avoid in a test. ZDOTDIR=$TMPHOME so zsh's startup files (.zshenv
    # etc.) are also sourced from the sandbox, not from $HOME.
    env -i HOME="${TMPHOME}" PATH=/usr/bin:/bin TERM=dumb \
        TERM_PROGRAM=vscode ZDOTDIR="${TMPHOME}" \
        zsh -c ". '${ZSHRC}'" >/dev/null 2>"${STDERR_FILE}"
    RC=$?
    set -e
    assert_eq "${RC}" "0" ".zshrc sources cleanly in clean subshell (exit 0)"
    ZSHRC_STDERR=$(cat "${STDERR_FILE}")
    assert_eq "${ZSHRC_STDERR}" "" ".zshrc produces no stderr output when sourced"
    rm -rf "${TMPHOME}"
else
    echo "SKIP: zsh not installed — zsh -n + zshrc source checks skipped"
fi

_test_report
