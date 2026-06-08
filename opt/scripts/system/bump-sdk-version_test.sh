#!/usr/bin/env bash
# Test driver for bump-sdk-version.sh (issue #139) — verifies conventional-commit
# driven semver computation per sdk/<tool> module: patch/minor/major level
# selection, --check vs --write semantics + exit codes, idempotency (the
# property the CI auto-bump loop relies on to terminate), VERSION-file changes
# being ignored so the bot's own bump commit never re-triggers a bump, respect
# for a manual VERSION already ahead of the last tag, and the untagged-module
# (first release) case. Builds throwaway git repos as fixtures.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
# shellcheck source=/dev/null
. "$REPO_ROOT/ai/_test_helpers.sh"

BUMP="$SCRIPT_DIR/bump-sdk-version.sh"

# Anchor every fixture under one root. mkrepo()/mktmp() are called via $(...)
# command substitution, so an array-based tracker would be mutated only in the
# subshell and never cleaned. A single filesystem root sidesteps that and keeps
# the EXIT trap's exit status clean.
TMPROOT="$(mktemp -d)"
cleanup() { rm -rf "$TMPROOT"; }
trap cleanup EXIT
mktmp() { mktemp -d "$TMPROOT/repo.XXXXXX"; }

# --- fixture helpers ---------------------------------------------------------

git_q() { git -C "$1" "${@:2}" >/dev/null 2>&1; }

mkrepo() {
    local r; r="$(mktmp)"
    git_q "$r" init -q
    git_q "$r" config user.email test@example.com
    git_q "$r" config user.name "Test User"
    git_q "$r" config commit.gpgsign false
    git_q "$r" config tag.gpgsign false
    printf '%s' "$r"
}

# mkmodule <repo> <tool> <version>
mkmodule() {
    local r="$1" tool="$2" ver="$3"
    mkdir -p "$r/sdk/$tool"
    printf 'module example.com/%s\n\ngo 1.22\n' "$tool" > "$r/sdk/$tool/go.mod"
    printf '%s\n' "$ver" > "$r/sdk/$tool/VERSION"
    printf 'package main\n' > "$r/sdk/$tool/main.go"
}

# commit <repo> <subject> <file> <content>
commit() {
    local r="$1" subj="$2" file="$3" content="$4"
    mkdir -p "$(dirname "$r/$file")"
    printf '%s\n' "$content" > "$r/$file"
    git_q "$r" add -A
    git_q "$r" commit -q -m "$subj"
}

# commit_body <repo> <subject> <body> <file> <content>
commit_body() {
    local r="$1" subj="$2" body="$3" file="$4" content="$5"
    mkdir -p "$(dirname "$r/$file")"
    printf '%s\n' "$content" > "$r/$file"
    git_q "$r" add -A
    git_q "$r" commit -q -m "$subj" -m "$body"
}

tag() { git_q "$1" tag -a "$2" -m "$2"; }

ver_of() { tr -d ' \t\r\n' < "$1/sdk/$2/VERSION"; }

# Capture the script's stdout+stderr into OUT. The `|| true` is load-bearing:
# assert_exit_code() in _test_helpers.sh leaves `set -e` enabled, so an
# unguarded assignment from a non-zero-exiting --check would abort the driver.
# Exit-code assertions are made separately via assert_exit_code.
run() { OUT="$(bash "$BUMP" "$@" 2>&1)" || true; }

# --- tests -------------------------------------------------------------------

# 1. feat: commit since last tag -> minor bump, --check reports + exits 1.
R="$(mkrepo)"; mkmodule "$R" foo 0.1.0
commit "$R" "chore: seed foo" sdk/foo/main.go "package main"
tag "$R" sdk/foo/v0.1.0
commit "$R" "feat(foo): add a flag" sdk/foo/flag.go "package main // flag"
run --check --repo "$R"
assert_exit_code 1 "feat -> --check exits 1 (bump needed)" bash "$BUMP" --check --repo "$R"
assert_grep "feat -> minor 0.1.0->0.2.0 reported" "foo: 0\.1\.0 -> 0\.2\.0" <(printf '%s\n' "$OUT")

# 2. --write applies the minor bump; VERSION file updated.
run --write --repo "$R"
assert_eq "$(ver_of "$R" foo)" "0.2.0" "feat -> --write bumps VERSION to 0.2.0"
assert_exit_code 0 "--write exits 0" bash "$BUMP" --write --repo "$R"

# 3. Idempotent: after the write, --check sees VERSION already == computed -> no bump.
run --check --repo "$R"
assert_exit_code 0 "idempotent: --check exits 0 once VERSION already bumped" bash "$BUMP" --check --repo "$R"
assert_grep_negative "idempotent: nothing reported for foo" "foo:" <(printf '%s\n' "$OUT")

# 4. fix: commit -> patch bump.
R="$(mkrepo)"; mkmodule "$R" bar 1.2.3
commit "$R" "chore: seed bar" sdk/bar/main.go "package main"
tag "$R" sdk/bar/v1.2.3
commit "$R" "fix(bar): correct off-by-one" sdk/bar/fix.go "package main // fix"
run --check --repo "$R"
assert_grep "fix -> patch 1.2.3->1.2.4" "bar: 1\.2\.3 -> 1\.2\.4" <(printf '%s\n' "$OUT")

# 5. Breaking change via '!' -> major bump.
R="$(mkrepo)"; mkmodule "$R" baz 2.5.1
commit "$R" "chore: seed baz" sdk/baz/main.go "package main"
tag "$R" sdk/baz/v2.5.1
commit "$R" "feat(baz)!: drop legacy API" sdk/baz/api.go "package main // v2"
run --check --repo "$R"
assert_grep "bang-breaking -> major 2.5.1->3.0.0" "baz: 2\.5\.1 -> 3\.0\.0" <(printf '%s\n' "$OUT")

# 6. Breaking change via 'BREAKING CHANGE:' body trailer -> major bump.
R="$(mkrepo)"; mkmodule "$R" qux 0.4.0
commit "$R" "chore: seed qux" sdk/qux/main.go "package main"
tag "$R" sdk/qux/v0.4.0
commit_body "$R" "fix(qux): rework storage" "BREAKING CHANGE: registry format changed" sdk/qux/store.go "package main // store"
run --check --repo "$R"
assert_grep "body-breaking -> major 0.4.0->1.0.0" "qux: 0\.4\.0 -> 1\.0\.0" <(printf '%s\n' "$OUT")

# 7. Commits that touch ONLY the VERSION file are excluded from the source-delta
#    check -> no bump. This is the guard that keeps the CI auto-bump loop from
#    re-triggering on the bot's own VERSION commit. Round-trip 0.1.0->0.2.0->0.1.0
#    so `current` ends back AT the tag: if the exclude were removed, the two
#    VERSION-only commits would be counted and force a spurious patch bump.
R="$(mkrepo)"; mkmodule "$R" loop 0.1.0
commit "$R" "chore: seed loop" sdk/loop/main.go "package main"
tag "$R" sdk/loop/v0.1.0
commit "$R" "chore(loop): bump VERSION" sdk/loop/VERSION "0.2.0"
commit "$R" "chore(loop): revert VERSION" sdk/loop/VERSION "0.1.0"
run --check --repo "$R"
assert_exit_code 0 "VERSION-only commits -> no bump (loop guard)" bash "$BUMP" --check --repo "$R"
assert_grep_negative "VERSION-only commits report nothing" "loop:" <(printf '%s\n' "$OUT")

# 8. Manual VERSION already ahead of last tag wins over computed bump.
R="$(mkrepo)"; mkmodule "$R" man 0.1.0
commit "$R" "chore: seed man" sdk/man/main.go "package main"
tag "$R" sdk/man/v0.1.0
# author manually set VERSION to 0.5.0 AND added a feat (computed would be 0.2.0)
printf '0.5.0\n' > "$R/sdk/man/VERSION"
commit "$R" "feat(man): big feature, hand-set VERSION" sdk/man/feat.go "package main // f"
run --check --repo "$R"
assert_exit_code 0 "manual VERSION ahead -> no further bump" bash "$BUMP" --check --repo "$R"
assert_grep_negative "manual VERSION ahead reports nothing" "man: " <(printf '%s\n' "$OUT")

# 9. Untagged module (first release) -> VERSION is the release as-is, no bump.
R="$(mkrepo)"; mkmodule "$R" new 0.1.0
commit "$R" "feat(new): initial" sdk/new/main.go "package main"
run --check --repo "$R"
assert_exit_code 0 "untagged module -> no bump (first tag handled by tagger)" bash "$BUMP" --check --repo "$R"

# 10. Multiple modules computed independently in one run.
R="$(mkrepo)"; mkmodule "$R" a 0.1.0; mkmodule "$R" b 0.1.0
commit "$R" "chore: seed" sdk/a/main.go "package main"
tag "$R" sdk/a/v0.1.0; tag "$R" sdk/b/v0.1.0
commit "$R" "feat(a): x" sdk/a/x.go "package main // x"
commit "$R" "fix(b): y" sdk/b/y.go "package main // y"
run --write --repo "$R"
assert_eq "$(ver_of "$R" a)" "0.2.0" "multi: module a minor -> 0.2.0"
assert_eq "$(ver_of "$R" b)" "0.1.1" "multi: module b patch -> 0.1.1"

# 11. A directory without go.mod is not treated as a module.
R="$(mkrepo)"; mkdir -p "$R/sdk/notamod"; printf '0.1.0\n' > "$R/sdk/notamod/VERSION"
git_q "$R" add -A; git_q "$R" commit -q -m "chore: junk"
run --check --repo "$R"
assert_exit_code 0 "non-module dir ignored" bash "$BUMP" --check --repo "$R"

_test_report
