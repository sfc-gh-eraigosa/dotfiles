#!/usr/bin/env bash
# sdk/version.sh — derive an sdk module's version from its git release tag.
#
# TAGS ARE THE SINGLE SOURCE OF TRUTH for sdk module versions. Go already
# requires the path-prefixed form `sdk/<tool>/vX.Y.Z` to resolve a subdirectory
# module (`go install github.com/.../sdk/<tool>@sdk/<tool>/v0.2.0`), so the tag
# is the authoritative release artifact. A VERSION file would only duplicate it.
#
# It used to. That duplicate is why `SDK auto-bump` failed 14/14 runs from
# 2026-06-08 to 2026-08-18: the file had to be COMMITTED to `main`, and the
# `main` ruleset (target: branch) requires a pull request plus 8 status checks,
# so every bot push was rejected with GH013. Tag pushes were never blocked —
# the ruleset does not target tags — which is why tag-sdk-modules.yml pushed
# tags with the very same GITHUB_TOKEN and went 5/5 green over the same period.
# Dropping the file removes the protected-branch write, and with it the failure.
#
# Usage:  sdk_version <tool> <dir-inside-the-repo>
#   echoes e.g. `0.2.0`            (HEAD is exactly the release tag)
#               `0.2.0-73-gd267d7f` (73 commits past v0.2.0 — a dev build)
#               `0.0.0-untagged`   (module has no release tag yet)
#               `0.0.0-nogit`      (no git metadata, e.g. an extracted tarball)
#
# Callers stamp the result through -ldflags into the module's internal/version.

sdk_version() {
    local tool="$1" dir="$2" described

    # Not a git checkout at all (tarball, vendored copy): say so rather than
    # emitting a version that claims to be a release.
    if ! git -C "$dir" rev-parse --git-dir >/dev/null 2>&1; then
        printf '0.0.0-nogit'
        return 0
    fi

    # --tags is redundant for the annotated tags CI cuts, but makes a
    # lightweight tag (a human's `git tag sdk/foo/v1.0.0`) resolve too.
    described="$(git -C "$dir" describe --tags --match "sdk/${tool}/v*" --abbrev=7 2>/dev/null || true)"

    if [ -z "$described" ]; then
        printf '0.0.0-untagged'
        return 0
    fi

    # `sdk/gss/v0.2.0-73-gd267d7f` -> `0.2.0-73-gd267d7f`
    printf '%s' "${described#sdk/"${tool}"/v}"
}
