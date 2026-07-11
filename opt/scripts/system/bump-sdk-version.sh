#!/usr/bin/env bash
# bump-sdk-version.sh — conventional-commit driven SDK version bumper (issue #139).
#
# For each Go module under sdk/<tool>/ (a dir with both go.mod and VERSION) it
# computes the next semantic version from the conventional-commit subjects of
# the commits that touched the module's SOURCE since its last release tag
# (sdk/<tool>/v<X.Y.Z>), then either reports the needed bump (--check) or writes
# it into the VERSION file (--write). The companion CI workflow
# (.github/workflows/sdk-auto-bump.yml) runs --write on merge to main, commits
# the VERSION change, and cuts the tag; tag-sdk-modules.yml remains the
# idempotent fallback tagger. See docs in the workflow header.
#
# Bump level (highest wins across the range):
#   - major: a subject of the form `<type>(scope)!:` OR a `BREAKING CHANGE` /
#            `BREAKING-CHANGE` trailer anywhere in a commit message.
#   - minor: a `feat:` / `feat(scope):` subject.
#   - patch: any other source-touching commit.
# The VERSION file itself is EXCLUDED from the "did source change" check, so the
# bot's own VERSION-bump commit produces no source delta and the auto-bump loop
# terminates (idempotency). A VERSION already hand-bumped ahead of the last tag
# is respected (the larger of computed vs. current wins). An untagged module is
# left untouched — its first tag is the tagger's job.
#
# Usage:
#   bump-sdk-version.sh [--check|--write] [--repo <dir>]
#     --check   (default) report modules needing a bump; exit 1 if any do,
#               0 if none, 2 on error. Writes nothing.
#     --write   apply the computed bumps to VERSION files; idempotent; exit 0,
#               2 on error.
#     --repo D  operate on repo D (default: the git toplevel of this script).
set -euo pipefail

MODE="check"
REPO=""

die() { echo "bump-sdk-version: $*" >&2; exit 2; }

while [ "$#" -gt 0 ]; do
    case "$1" in
        --check) MODE="check" ;;
        --write) MODE="write" ;;
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

# strip a leading v and surrounding whitespace from a version string
norm_ver() { tr -d ' \t\r\n' | sed 's/^v//'; }

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

# higher of two semver strings
ver_max() { printf '%s\n%s\n' "$1" "$2" | sort -V | tail -1; }

needed=0

# Iterate modules deterministically. nullglob so an empty sdk/ is a clean no-op.
shopt -s nullglob
for vf in "$REPO"/sdk/*/VERSION; do
    dir="$(dirname "$vf")"
    tool="$(basename "$dir")"
    [ -f "$dir/go.mod" ] || continue          # only real Go modules

    current="$(norm_ver < "$vf")"
    [ -n "$current" ] || { echo "bump-sdk-version: skip $tool: empty VERSION" >&2; continue; }

    base="$(last_tag_ver "$tool")"
    [ -n "$base" ] || continue                 # untagged: first tag is the tagger's job

    range="sdk/${tool}/v${base}..HEAD"
    # Commits touching module source since the last tag, EXCLUDING the VERSION
    # file (so the bot's own bump commit is not itself a reason to bump).
    count="$(git -C "$REPO" rev-list --count "$range" -- "sdk/${tool}" ":(exclude)sdk/${tool}/VERSION" 2>/dev/null || echo 0)"
    [ "$count" -gt 0 ] || continue             # no source change since last tag

    msgs="$(git -C "$REPO" log --format='%s%n%b' "$range" -- "sdk/${tool}" ":(exclude)sdk/${tool}/VERSION" 2>/dev/null)"

    level="patch"
    if printf '%s\n' "$msgs" | grep -qE '^[a-z]+(\([^)]*\))?!:' \
       || printf '%s\n' "$msgs" | grep -qE 'BREAKING[ -]CHANGE'; then
        level="major"
    elif printf '%s\n' "$msgs" | grep -qE '^feat(\([^)]*\))?:'; then
        level="minor"
    fi

    computed="$(bump_ver "$base" "$level")"
    desired="$(ver_max "$computed" "$current")"

    [ "$desired" != "$current" ] || continue   # already at/above desired -> no bump

    needed=$((needed + 1))
    if [ "$MODE" = "write" ]; then
        printf '%s\n' "$desired" > "$vf"
        echo "bumped ${tool}: ${current} -> ${desired} (${level})"
    else
        echo "${tool}: ${current} -> ${desired} (${level})"
    fi
done
shopt -u nullglob

if [ "$MODE" = "check" ] && [ "$needed" -gt 0 ]; then
    exit 1
fi
exit 0
