#!/usr/bin/env bash
# bump-sdk-version.sh — conventional-commit driven SDK release planner (issue #139).
#
# TAGS ARE THE SINGLE SOURCE OF TRUTH. For each Go module under sdk/<tool>/ this
# computes the next semantic version from the conventional-commit subjects of the
# commits that touched the module's source since its last release tag
# (sdk/<tool>/vX.Y.Z). It REPORTS that plan; it never writes a file and never
# commits. Cutting the tag is the caller's job (.github/workflows/sdk-auto-bump.yml).
#
# Why no VERSION file any more: the file duplicated the tag, and had to be
# COMMITTED to `main` to take effect. The `main` ruleset (target: branch)
# requires a pull request plus 8 status checks, so every bot push was rejected
# with GH013 — that is why `SDK auto-bump` failed 14/14 runs between 2026-06-08
# and 2026-08-18 without ever succeeding once. Tag pushes were never blocked
# (the ruleset does not target tags), so publishing the tag alone both fixes the
# failure and removes the second source of truth. See sdk/version.sh.
#
# Bump level (highest wins across the range):
#   - major: a subject of the form `<type>(scope)!:` OR a `BREAKING CHANGE` /
#            `BREAKING-CHANGE` trailer anywhere in a commit message.
#   - minor: a `feat:` / `feat(scope):` subject.
#   - patch: any other source-touching commit.
#
# A module with no release tag yet is planned at FIRST_RELEASE (0.1.0) so a new
# module enters the release stream on its own; previously an untagged module was
# skipped and depended on a separate bootstrap workflow.
#
# Idempotency — what terminates the CI loop: the plan is computed from the last
# TAG. Once the tag is cut, `rev-list <tag>..HEAD` over the module's source is
# empty, so a re-run plans nothing. No commit is made, so nothing can re-trigger
# the workflow either.
#
# Usage:
#   bump-sdk-version.sh [--check|--plan] [--repo <dir>]
#     --check  (default) human-readable report; exit 1 if any module needs a
#              release, 0 if none, 2 on error.
#     --plan   machine-readable `<tool>\t<version>\t<level>` lines for CI to
#              consume; always exit 0 (empty output = nothing to release).
#     --repo D operate on repo D (default: the git toplevel of this script).
set -euo pipefail

MODE="check"
REPO=""
FIRST_RELEASE="0.1.0"

die() { echo "bump-sdk-version: $*" >&2; exit 2; }

while [ "$#" -gt 0 ]; do
    case "$1" in
        --check) MODE="check" ;;
        --plan)  MODE="plan" ;;
        --repo)  shift; REPO="${1:-}"; [ -n "$REPO" ] || die "--repo needs a directory" ;;
        -h|--help) sed -n '2,40p' "$0"; exit 0 ;;
        *) die "unknown argument: $1" ;;
    esac
    shift
done

if [ -z "$REPO" ]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    REPO="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null)" || die "not in a git repo"
fi
[ -d "$REPO/.git" ] || git -C "$REPO" rev-parse --git-dir >/dev/null 2>&1 || die "not a git repo: $REPO"

# highest existing release tag version for a tool, or empty if untagged
last_tag_ver() {
    local tool="$1"
    git -C "$REPO" tag -l "sdk/${tool}/v*" \
        | sed "s#^sdk/${tool}/v##" \
        | sort -V | tail -1
}

# semver bump: <base X.Y.Z> <level major|minor|patch>
bump_ver() {
    local base="$1" level="$2" X Y Z
    IFS=. read -r X Y Z <<EOF
$base
EOF
    X="${X:-0}"; Y="${Y:-0}"; Z="${Z:-0}"
    case "$level" in
        major) X=$((X + 1)); Y=0; Z=0 ;;
        minor) Y=$((Y + 1)); Z=0 ;;
        patch) Z=$((Z + 1)) ;;
    esac
    printf '%s.%s.%s' "$X" "$Y" "$Z"
}

needed=0

# Iterate modules deterministically, keyed on go.mod (the definition of "is a Go
# module") rather than on a VERSION file that no longer exists. nullglob so an
# empty sdk/ is a clean no-op.
shopt -s nullglob
for gomod in "$REPO"/sdk/*/go.mod; do
    dir="$(dirname "$gomod")"
    tool="$(basename "$dir")"

    base="$(last_tag_ver "$tool")"

    if [ -z "$base" ]; then
        # Never released. Plan the first release only if the module has any
        # history at all, so an empty scaffold directory stays quiet.
        count="$(git -C "$REPO" rev-list --count HEAD -- "sdk/${tool}" 2>/dev/null || echo 0)"
        [ "$count" -gt 0 ] || continue
        needed=$((needed + 1))
        if [ "$MODE" = "plan" ]; then
            printf '%s\t%s\t%s\n' "$tool" "$FIRST_RELEASE" "initial"
        else
            echo "${tool}: (unreleased) -> ${FIRST_RELEASE} (initial)"
        fi
        continue
    fi

    range="sdk/${tool}/v${base}..HEAD"
    count="$(git -C "$REPO" rev-list --count "$range" -- "sdk/${tool}" 2>/dev/null || echo 0)"
    [ "$count" -gt 0 ] || continue             # no source change since last tag

    msgs="$(git -C "$REPO" log --format='%s%n%b' "$range" -- "sdk/${tool}" 2>/dev/null)"

    level="patch"
    if printf '%s\n' "$msgs" | grep -qE '^[a-z]+(\([^)]*\))?!:' \
       || printf '%s\n' "$msgs" | grep -qE 'BREAKING[ -]CHANGE'; then
        level="major"
    elif printf '%s\n' "$msgs" | grep -qE '^feat(\([^)]*\))?:'; then
        level="minor"
    fi

    computed="$(bump_ver "$base" "$level")"

    needed=$((needed + 1))
    if [ "$MODE" = "plan" ]; then
        printf '%s\t%s\t%s\n' "$tool" "$computed" "$level"
    else
        echo "${tool}: ${base} -> ${computed} (${level})"
    fi
done
shopt -u nullglob

if [ "$MODE" = "check" ] && [ "$needed" -gt 0 ]; then
    exit 1
fi
exit 0
