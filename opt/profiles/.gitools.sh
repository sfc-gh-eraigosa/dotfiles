# shellcheck shell=bash
# .gitools.sh — the slim survivors of the retired Gerrit-era ~/.gitenv
# generator (opt/scripts/git/setup_git_alias.sh). Sourced by .bash_aliases;
# interactive bash/zsh only. Keep this file small: one canonical tool per
# workflow, no generation step, no startup side effects.

# Resolve origin's default branch (e.g. "main"), caching origin/HEAD if unset.
_gitools_default_branch() {
    _b=$(git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null)
    if [ -z "$_b" ]; then
        git remote set-head origin --auto >/dev/null 2>&1
        _b=$(git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null)
    fi
    [ -n "$_b" ] && echo "${_b#origin/}"
}

# git-reset — hard-reset the repo to origin's default branch, clean untracked
# files, and pull. Prompts before operating on a repo rooted at $HOME.
git-reset() {
    _root=$(git rev-parse --show-toplevel 2>/dev/null)
    if [ -z "$_root" ]; then
        echo "git-reset: not inside a git repository" >&2
        return 1
    fi
    _real_root=$(cd -- "$_root" 2>/dev/null && pwd -P)
    _real_home=$(cd -- "$HOME" 2>/dev/null && pwd -P)
    if [ "$_real_root" = "$_real_home" ]; then
        printf 'WARNING: you are about to reset your home folder [%s]\nDo you really want to continue? Type [Yes]: ' "$_root"
        read -r _answer
        [ "$_answer" = "Yes" ] || { echo "git-reset: aborted"; return 1; }
    fi
    _branch=$(cd -- "$_root" && _gitools_default_branch)
    if [ -z "$_branch" ]; then
        echo "git-reset: cannot resolve origin's default branch" >&2
        return 1
    fi
    echo "Resetting $_root to origin/$_branch..."
    ( cd -- "$_root" && \
      git reset --hard "origin/$_branch" && \
      git clean -x -d -f && \
      git pull origin "$_branch" )
}

# git-reset-all — run git-reset in every git repo directly under the current
# directory (replaces the old hardcoded-'stable' loop). Non-repos are skipped.
git-reset-all() {
    for _d in */; do
        [ -d "$_d/.git" ] || { echo "skip: $_d (not a git repository)"; continue; }
        echo "==> $_d"
        ( cd -- "$_d" && git-reset ) || echo "WARN: git-reset failed in $_d" >&2
    done
}

# git-clean — reset the CURRENT branch to its upstream and remove untracked
# files; no pull, no branch switch. (Fixes the old alias whose missing space
# ran 'git checkoutmaster'.)
git-clean() {
    _up=$(git rev-parse --abbrev-ref --symbolic-full-name "@{u}" 2>/dev/null)
    if [ -z "$_up" ]; then
        echo "git-clean: current branch has no upstream" >&2
        return 1
    fi
    echo "Resetting current branch to $_up and cleaning untracked files..."
    git reset --hard "$_up" && git clean -x -d -f
}

# git-help — list the gitools commands.
git-help() {
    cat <<'HELP'
gitools (~/.gitools.sh) — canonical git shortcuts:

  git-reset      Hard-reset repo to origin's default branch, clean, and pull.
                 Prompts first if the repo root is your $HOME.
  git-reset-all  git-reset every repo directly under the current directory.
  git-clean      Reset the CURRENT branch to its upstream + clean untracked
                 files (no pull, no branch switch).
  git-help       This text.

Related: gss (safe commit/push/PR), make git-doctor (identity check),
git-branches-rm / git-local-master (see .bash_aliases).
HELP
}
