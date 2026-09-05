#!/usr/bin/env bash
# _privacy_common.sh — shared plumbing for the privacy git hooks.
#
# Sourced by pre-commit, commit-msg and pre-push. Each hook does three things:
# find the rule library, judge its slice of what git is about to record or
# publish, and then hand off to the repo-local hook of the same name so a
# global core.hooksPath does not silently switch off husky/lefthook/hand-made
# hooks in any repo.
#
# PRIVACY_GUARD_SKIP=1 skips the judgement (not the hand-off). It is the
# reviewed-exception lever; it is loud on purpose.

# privacy_hooks_load_rules <hook-path>: source privacy_rules.sh from, in
# order, $PRIVACY_RULES, beside the hook (the installed layout), or
# ../hooks/ (the repo layout, used by the tests).
privacy_hooks_load_rules() {
    local here cand
    here="$(cd "$(dirname "$1")" && pwd)"
    for cand in "${PRIVACY_RULES:-}" "$here/privacy_rules.sh" "$here/../hooks/privacy_rules.sh"; do
        if [ -n "$cand" ] && [ -r "$cand" ]; then
            # shellcheck source=../hooks/privacy_rules.sh
            . "$cand"
            return 0
        fi
    done
    echo "privacy git hook: privacy_rules.sh not found (looked beside $1 and in ../hooks); refusing to guess" >&2
    return 1
}

# privacy_hooks_judge <hook-name> <text>: run the rules; on a finding print
# the deny and exit 1. Silent and returns 0 when clean or skipped.
privacy_hooks_judge() {
    local name="$1" text="$2" finding
    [ -n "${PRIVACY_GUARD_SKIP:-}" ] && return 0
    [ -n "$text" ] || return 0
    if finding="$(privacy_scan "$text" "$PWD")"; then
        return 0
    fi
    printf 'BLOCKED by privacy_guard (%s): %s. Offending text: %s\n' \
        "$name" "${finding%%	*}" "${finding#*	}" >&2
    printf 'Do not record local identity or secrets. Use $HOME / ~ / ${USER} / <user> / <host> / <email> / <REDACTED>.\n' >&2
    printf 'For a reviewed exception: PRIVACY_GUARD_SKIP=1 git %s ...\n' "$name" >&2
    exit 1
}

# privacy_hooks_chain <hook-name> [args...]: exec the repo-local hook of the
# same name if there is one and it is not this very file.
privacy_hooks_chain() {
    local name="$1" gitdir local_hook self; shift
    # NOT `--git-path hooks/<name>`: that honours core.hooksPath and would
    # resolve to this very hook. The repo-local hooks live in the common dir
    # (shared by worktrees).
    gitdir="$(git rev-parse --git-common-dir 2>/dev/null || git rev-parse --git-dir 2>/dev/null || true)"
    [ -n "$gitdir" ] || return 0
    local_hook="$gitdir/hooks/$name"
    self="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"
    if [ -x "$local_hook" ] && [ "$(cd "$(dirname "$local_hook")" && pwd)/$(basename "$local_hook")" != "$self" ]; then
        exec "$local_hook" "$@"
    fi
    return 0
}

# privacy_hooks_added_lines: the ADDED lines of a unified diff on stdin, '+'
# stripped, file headers dropped. Only additions are judged: a leak already
# in history is not this change's doing, and judging it would block every
# later commit to the file.
privacy_hooks_added_lines() {
    grep -E '^\+' | grep -Ev '^\+\+\+ ' | cut -c2-
}
