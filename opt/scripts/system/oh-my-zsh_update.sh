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
#     touches a diverged branch. A dirty tree is fine as long as upstream does
#     not touch the same paths (git refuses otherwise, and we warn, naming the
#     files — or the local commits, when that is the reason).
#   - One exception to "never touch the tree": older .zshrc copied
#     opt/themes/agnoster.zsh-theme over the TRACKED themes/agnoster.zsh-theme
#     at every shell start (it now goes to custom/themes/). When that file is
#     the clone's ONLY modification it is restored to upstream's copy before
#     the fast-forward, so clones dirtied that way heal themselves.
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

# Local commits make a fast-forward impossible whatever the working tree
# holds, so check them first and touch nothing.
AHEAD="$(git -C "${CLONE_DIR}" rev-list --count "${UPSTREAM}..HEAD" 2>/dev/null || echo 0)"
if [ "${AHEAD}" != "0" ]; then
    echo "WARNING: oh-my-zsh: ${CLONE_DIR} has ${AHEAD} local commit(s) not on ${UPSTREAM};"
    echo "         cannot fast-forward. Left untouched. To resolve by hand:"
    echo "         git -C '${CLONE_DIR}' log ${UPSTREAM}..HEAD && git -C '${CLONE_DIR}' merge ${UPSTREAM}"
    exit 0
fi

# Heal the dotfiles theme copy. Older .zshrc copied opt/themes/agnoster.zsh-theme
# over the TRACKED themes/agnoster.zsh-theme at every shell start, so every
# clone carries that one modification — and once upstream changed the same
# file (e6561f57), --ff-only refused forever. That copy is disposable (.zshrc
# now installs it into custom/themes/, which oh-my-zsh loads first), so when it
# is the clone's ONLY modification, restore upstream's file. Anything else
# dirty, or a staged change, means a human touched the clone: leave it alone.
THEME_PATH="themes/agnoster.zsh-theme"
DIRTY="$(git -C "${CLONE_DIR}" status --porcelain --untracked-files=no 2>/dev/null || true)"
if [ "${DIRTY}" = " M ${THEME_PATH}" ]; then
    if git -C "${CLONE_DIR}" checkout -q -- "${THEME_PATH}" 2>/dev/null; then
        echo "oh-my-zsh: restored ${THEME_PATH} in ${CLONE_DIR} (the dotfiles copy now lives in custom/themes/)."
    fi
fi

MERGE_ERR="$(git -C "${CLONE_DIR}" merge -q --ff-only "${UPSTREAM}" 2>&1 >/dev/null)"
MERGE_RC=$?
if [ "${MERGE_RC}" -eq 0 ]; then
    COUNT="$(git -C "${CLONE_DIR}" rev-list --count "${OLD}..${NEW}" 2>/dev/null || echo '?')"
    echo "oh-my-zsh: updated ${CLONE_DIR} ${OLD:0:8} -> ${NEW:0:8} (${COUNT} commit(s), fast-forward)."
    exit 0
fi

# Not local commits (checked above), so the working tree is in the way: name
# the locally modified tracked files that upstream also changed.
LOCAL_MODS="$(git -C "${CLONE_DIR}" diff --name-only HEAD 2>/dev/null || true)"
CONFLICTS=""
while IFS= read -r _f; do
    [ -n "${_f}" ] || continue
    case "
${LOCAL_MODS}
" in
        *"
${_f}
"*) CONFLICTS="${CONFLICTS:+${CONFLICTS} }${_f}" ;;
    esac
done <<EOF
$(git -C "${CLONE_DIR}" diff --name-only HEAD "${UPSTREAM}" 2>/dev/null)
EOF

if [ -n "${CONFLICTS}" ]; then
    echo "WARNING: oh-my-zsh: ${CLONE_DIR} cannot be fast-forwarded to ${UPSTREAM}:"
    echo "         modified tracked file(s) that upstream also changed: ${CONFLICTS}"
    echo "         Left untouched. To resolve by hand (drop or stash your edits, then re-run):"
    echo "         git -C '${CLONE_DIR}' diff -- ${CONFLICTS}"
else
    echo "WARNING: oh-my-zsh: ${CLONE_DIR} cannot be fast-forwarded to ${UPSTREAM}. Left untouched."
    if [ -n "${MERGE_ERR}" ]; then
        printf '%s\n' "${MERGE_ERR}" | head -n 5 | sed 's/^/         git: /'
    fi
    echo "         To resolve by hand: git -C '${CLONE_DIR}' status && git -C '${CLONE_DIR}' merge --ff-only ${UPSTREAM}"
fi
exit 0
