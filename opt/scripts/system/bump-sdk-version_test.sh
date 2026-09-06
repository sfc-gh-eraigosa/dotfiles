#!/usr/bin/env bash
# Test driver for bump-sdk-version.sh (issue #139) — verifies conventional-commit
# driven semver computation per sdk/<tool> module: patch/minor/major level
# selection, --check vs --plan semantics + exit codes, the untagged/first-release
# case, and loop termination (once the tag exists the planner goes quiet).
#
# TAG-DRIVEN: there is no VERSION file any more. The tag IS the version. See
# sdk/version.sh and .github/workflows/sdk-auto-bump.yml for why the file was
# removed — it had to be committed to a protected branch, which failed 14/14 CI
# runs with GH013. Fixtures are throwaway git repos.
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

# mkmodule <repo> <tool>   (no version argument — tags carry the version now)
mkmodule() {
    local r="$1" tool="$2"
    mkdir -p "$r/sdk/$tool"
    printf 'module example.com/%s\n\ngo 1.22\n' "$tool" > "$r/sdk/$tool/go.mod"
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

# Capture the script's stdout+stderr into OUT. The `|| true` is load-bearing:
# assert_exit_code() in _test_helpers.sh leaves `set -e` enabled, so an
# unguarded assignment from a non-zero-exiting --check would abort the driver.
# Exit-code assertions are made separately via assert_exit_code.
run() { OUT="$(bash "$BUMP" "$@" 2>&1)" || true; }

# --- tests -------------------------------------------------------------------

# 1. feat: commit since last tag -> minor, --check reports + exits 1.
R="$(mkrepo)"; mkmodule "$R" foo
commit "$R" "chore: seed foo" sdk/foo/main.go "package main"
tag "$R" sdk/foo/v0.1.0
commit "$R" "feat(foo): add a flag" sdk/foo/flag.go "package main // flag"
run --check --repo "$R"
assert_exit_code 1 "feat -> --check exits 1 (release needed)" bash "$BUMP" --check --repo "$R"
assert_grep "feat -> minor 0.1.0->0.2.0 reported" "foo: 0\.1\.0 -> 0\.2\.0" <(printf '%s\n' "$OUT")

# 2. --plan emits machine-readable tool/version/level and always exits 0.
run --plan --repo "$R"
assert_eq "$OUT" "$(printf 'foo\t0.2.0\tminor')" "--plan emits tab-separated tool/version/level"
assert_exit_code 0 "--plan exits 0 even when a release is pending" bash "$BUMP" --plan --repo "$R"

# 3. Once the computed tag exists, the planner goes quiet. THIS is what
#    terminates the CI loop — no VERSION commit is involved any more.
tag "$R" sdk/foo/v0.2.0
run --check --repo "$R"
assert_exit_code 0 "tag cut -> --check exits 0 (nothing left to release)" bash "$BUMP" --check --repo "$R"
assert_grep_negative "tag cut -> nothing reported for foo" "foo:" <(printf '%s\n' "$OUT")
run --plan --repo "$R"
assert_eq "$OUT" "" "tag cut -> --plan emits nothing"

# 4. fix: commit -> patch.
R="$(mkrepo)"; mkmodule "$R" bar
commit "$R" "chore: seed bar" sdk/bar/main.go "package main"
tag "$R" sdk/bar/v1.2.3
commit "$R" "fix(bar): correct off-by-one" sdk/bar/fix.go "package main // fix"
run --check --repo "$R"
assert_grep "fix -> patch 1.2.3->1.2.4" "bar: 1\.2\.3 -> 1\.2\.4" <(printf '%s\n' "$OUT")

# 5. Breaking change via '!' -> major.
R="$(mkrepo)"; mkmodule "$R" baz
commit "$R" "chore: seed baz" sdk/baz/main.go "package main"
tag "$R" sdk/baz/v2.5.1
commit "$R" "feat(baz)!: drop legacy API" sdk/baz/api.go "package main // v2"
run --check --repo "$R"
assert_grep "bang-breaking -> major 2.5.1->3.0.0" "baz: 2\.5\.1 -> 3\.0\.0" <(printf '%s\n' "$OUT")

# 6. Breaking change via 'BREAKING CHANGE:' body trailer -> major.
R="$(mkrepo)"; mkmodule "$R" qux
commit "$R" "chore: seed qux" sdk/qux/main.go "package main"
tag "$R" sdk/qux/v0.4.0
commit_body "$R" "fix(qux): rework storage" "BREAKING CHANGE: registry format changed" sdk/qux/store.go "package main // store"
run --check --repo "$R"
assert_grep "body-breaking -> major 0.4.0->1.0.0" "qux: 0\.4\.0 -> 1\.0\.0" <(printf '%s\n' "$OUT")

# 7. Highest level wins across the range (a patch and a feat -> minor, not patch).
R="$(mkrepo)"; mkmodule "$R" mix
commit "$R" "chore: seed mix" sdk/mix/main.go "package main"
tag "$R" sdk/mix/v0.3.0
commit "$R" "fix(mix): small" sdk/mix/a.go "package main // a"
commit "$R" "feat(mix): big" sdk/mix/b.go "package main // b"
run --check --repo "$R"
assert_grep "highest level wins -> minor 0.3.0->0.4.0" "mix: 0\.3\.0 -> 0\.4\.0" <(printf '%s\n' "$OUT")

# 8. A commit touching a DIFFERENT module does not trigger a release here.
R="$(mkrepo)"; mkmodule "$R" self; mkmodule "$R" other
commit "$R" "chore: seed" sdk/self/main.go "package main"
tag "$R" sdk/self/v0.1.0; tag "$R" sdk/other/v0.1.0
commit "$R" "feat(other): unrelated" sdk/other/x.go "package main // x"
run --check --repo "$R"
assert_grep_negative "unrelated module's commit does not release self" "self:" <(printf '%s\n' "$OUT")
assert_grep "unrelated module itself does release" "other: 0\.1\.0 -> 0\.2\.0" <(printf '%s\n' "$OUT")

# 9. Untagged module with history -> planned as the initial release (0.1.0).
#    Previously this was skipped and depended on a separate bootstrap workflow,
#    which is now retired, so the planner must handle it.
R="$(mkrepo)"; mkmodule "$R" new
commit "$R" "feat(new): initial" sdk/new/main.go "package main"
run --check --repo "$R"
assert_exit_code 1 "untagged module -> initial release planned (exit 1)" bash "$BUMP" --check --repo "$R"
assert_grep "untagged module -> 0.1.0 initial" "new: \(unreleased\) -> 0\.1\.0 \(initial\)" <(printf '%s\n' "$OUT")
run --plan --repo "$R"
assert_eq "$OUT" "$(printf 'new\t0.1.0\tinitial')" "untagged module --plan emits initial"

# 10. Multiple modules computed independently in one run.
R="$(mkrepo)"; mkmodule "$R" a; mkmodule "$R" b
commit "$R" "chore: seed" sdk/a/main.go "package main"
tag "$R" sdk/a/v0.1.0; tag "$R" sdk/b/v0.1.0
commit "$R" "feat(a): x" sdk/a/x.go "package main // x"
commit "$R" "fix(b): y" sdk/b/y.go "package main // y"
run --plan --repo "$R"
assert_eq "$OUT" "$(printf 'a\t0.2.0\tminor\nb\t0.1.1\tpatch')" "multi: a minor, b patch, one plan"

# 11. A directory without go.mod is not treated as a module.
R="$(mkrepo)"; mkdir -p "$R/sdk/notamod"; printf 'hello\n' > "$R/sdk/notamod/readme"
git_q "$R" add -A; git_q "$R" commit -q -m "chore: junk"
run --check --repo "$R"
assert_exit_code 0 "non-module dir ignored" bash "$BUMP" --check --repo "$R"

# 12. Bad arguments and a non-repo both exit 2 (error), never 0/1.
assert_exit_code 2 "unknown argument exits 2" bash "$BUMP" --nonsense
assert_exit_code 2 "--repo pointing outside a git repo exits 2" bash "$BUMP" --check --repo "$TMPROOT/definitely-not-a-repo"

_test_report
