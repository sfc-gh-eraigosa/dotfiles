#!/usr/bin/env bash
# oh-my-zsh_update.sh — fast-forward the oh-my-zsh clone to its upstream tip.
#
# Why this exists: ~/.gitrepos clones oh-my-zsh but registers it with
# ";false" in .repos.env, i.e. "clone once, never pull". The clone therefore
# went stale for years and missed upstream fixes (e.g. the docker plugin's
# `cp ./completions/_docker` startup error, fixed upstream two days after it
# shipped). This script is the update half, run by install.sh under the gff
# flag install.shell.oh-my-zsh-update (fail-open, default on).
#
# Contract:
#   - Fast-forward ONLY. Never creates merge commits, never rebases, never
#     touches a diverged branch. .zshrc copies opt/themes/agnoster.zsh-theme
#     over the clone's copy at every shell start, so the tree is expected to
#     be dirty in that one file; a dirty tree is fine as long as upstream does
#     not touch the same paths (git refuses otherwise, and we warn).
#   - Never fails the installer: every problem (offline, diverged, detached
#     HEAD, missing clone) prints a note or WARNING and exits 0.
#   - Never prompts: GIT_TERMINAL_PROMPT=0 and a bounded fetch timeout.
#
# Usage: oh-my-zsh_update.sh [CLONE_DIR]
#   CLONE_DIR defaults to the same resolution .zshrc uses for $ZSH:
#   ~/.oh-my-zsh if it exists, else ${GIT_WORKSPACE:-~/git}/oh-my-zsh.
set -u

if [ -n "${1:-}" ]; then
    CLONE_DIR="$1"
elif [ -d "${HOME}/.oh-my-zsh" ]; then
    CLONE_DIR="${HOME}/.oh-my-zsh"
else
    CLONE_DIR="${GIT_WORKSPACE:-${HOME}/git}/oh-my-zsh"
fi
FETCH_TIMEOUT="${OMZ_UPDATE_TIMEOUT:-60}"

export GIT_TERMINAL_PROMPT=0

# Not cloned (yet), or not a git checkout: nothing to update, not an error.
# ~/.gitrepos owns the clone; this script only ever advances an existing one.
if [ ! -d "${CLONE_DIR}" ] || ! git -C "${CLONE_DIR}" rev-parse --git-dir >/dev/null 2>&1; then
    echo "oh-my-zsh: no clone at ${CLONE_DIR}; nothing to update."
    exit 0
fi

BRANCH="$(git -C "${CLONE_DIR}" symbolic-ref --short -q HEAD 2>/dev/null || true)"
if [ -z "${BRANCH}" ]; then
    echo "WARNING: oh-my-zsh clone at ${CLONE_DIR} has a detached HEAD; not updating."
    exit 0
fi

# Bounded, non-interactive fetch. `timeout` is coreutils (Linux/WSL); macOS
# lacks it unless coreutils is installed, so fall back to a plain fetch.
_fetch() {
    if command -v timeout >/dev/null 2>&1; then
        timeout "${FETCH_TIMEOUT}" git -C "${CLONE_DIR}" fetch -q origin
    else
        git -C "${CLONE_DIR}" fetch -q origin
    fi
}
if ! _fetch 2>/dev/null; then
    echo "WARNING: oh-my-zsh: could not fetch origin for ${CLONE_DIR} (offline?); leaving it as is."
    exit 0
fi

# Prefer the configured upstream; fall back to origin/<branch> for clones
# whose branch has no tracking info.
UPSTREAM="$(git -C "${CLONE_DIR}" rev-parse --abbrev-ref -q "${BRANCH}@{upstream}" 2>/dev/null || true)"
[ -n "${UPSTREAM}" ] || UPSTREAM="origin/${BRANCH}"
if ! git -C "${CLONE_DIR}" rev-parse -q --verify "${UPSTREAM}^{commit}" >/dev/null 2>&1; then
    echo "WARNING: oh-my-zsh: no upstream ref ${UPSTREAM} in ${CLONE_DIR}; not updating."
    exit 0
fi

OLD="$(git -C "${CLONE_DIR}" rev-parse HEAD)"
NEW="$(git -C "${CLONE_DIR}" rev-parse "${UPSTREAM}")"
if [ "${OLD}" = "${NEW}" ]; then
    echo "oh-my-zsh: ${CLONE_DIR} is up to date (${OLD:0:8})."
    exit 0
fi

if git -C "${CLONE_DIR}" merge -q --ff-only "${UPSTREAM}" >/dev/null 2>&1; then
    COUNT="$(git -C "${CLONE_DIR}" rev-list --count "${OLD}..${NEW}" 2>/dev/null || echo '?')"
    echo "oh-my-zsh: updated ${CLONE_DIR} ${OLD:0:8} -> ${NEW:0:8} (${COUNT} commit(s), fast-forward)."
    exit 0
fi

echo "WARNING: oh-my-zsh: ${CLONE_DIR} cannot be fast-forwarded to ${UPSTREAM} (local commits or"
echo "         local edits to files upstream changed). Left untouched. To resolve by hand:"
echo "         git -C '${CLONE_DIR}' status && git -C '${CLONE_DIR}' merge ${UPSTREAM}"
exit 0
