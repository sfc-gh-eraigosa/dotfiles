#!/usr/bin/env bash
# Test driver for provision-claude-memory.sh (issue #134) — verifies account
# memories are seeded into the computed live slug dir, host-local memories are
# preserved, the index is the union, collisions are skipped, and runs are
# idempotent. Uses a throwaway $HOME + a synthetic BASE_DIR repo.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
# shellcheck source=/dev/null
. "$REPO_ROOT/ai/_test_helpers.sh"

PROV="$SCRIPT_DIR/provision-claude-memory.sh"

TMPS=()
cleanup() { for d in "${TMPS[@]:-}"; do [ -n "$d" ] && rm -rf "$d"; done; }
trap cleanup EXIT
mktmp() { local d; d="$(mktemp -d)"; TMPS+=("$d"); printf '%s' "$d"; }

slug_of() { printf '%s' "$(cd "$1" && pwd -P)" | sed 's#/#-#g'; }
run_prov() { HOME="$1" BASE_DIR="$2" bash "$PROV" >/dev/null 2>&1; }

# Build a synthetic repo with two account memories + an account index.
mkrepo() {
    local r="$1"
    mkdir -p "$r/ai/claude/memory"
    cat > "$r/ai/claude/memory/acct-one.md" <<'EOF'
---
name: acct-one
description: first account memory
metadata:
  type: reference
  scope: account
---
body one
EOF
    cat > "$r/ai/claude/memory/acct-two.md" <<'EOF'
---
name: acct-two
description: second account memory
metadata:
  type: reference
  scope: account
---
body two
EOF
    cat > "$r/ai/claude/memory/MEMORY.md" <<'EOF'
- [acct-one](acct-one.md) — first account memory
- [acct-two](acct-two.md) — second account memory
EOF
}

seed_hostlocal() { # $1 = live dir, $2 = basename, $3 = body marker
    mkdir -p "$1"
    cat > "$1/$2" <<EOF
---
name: ${2%.md}
description: a host-only memory
metadata:
  scope: host-local
---
$3
EOF
}

REPO="$(mktmp)"; mkrepo "$REPO"
SLUG="$(slug_of "$REPO")"

# --- F1/F2/UC-1: fresh machine pre-seed into the computed slug dir ---
H1="$(mktmp)"; LIVE1="$H1/.claude/projects/$SLUG/memory"
run_prov "$H1" "$REPO"
assert_file_exists "$LIVE1/acct-one.md" "F2: account file 1 provisioned to computed slug dir"
assert_file_exists "$LIVE1/acct-two.md" "F2: account file 2 provisioned"
assert_file_exists "$LIVE1/MEMORY.md"   "F2: index provisioned"
assert_grep "F5: index lists account entry" "acct-one" "$LIVE1/MEMORY.md"

# --- F3/F5/UC-2: seed-and-preserve + union index ---
H2="$(mktmp)"; LIVE2="$H2/.claude/projects/$SLUG/memory"
seed_hostlocal "$LIVE2" "local-note.md" "LOCAL BODY"
run_prov "$H2" "$REPO"
assert_file_exists "$LIVE2/local-note.md" "F3: host-local file preserved across provision"
assert_grep "F5: union index lists host-local entry"  "local-note" "$LIVE2/MEMORY.md"
assert_grep "F5: union index still lists account entry" "acct-two"  "$LIVE2/MEMORY.md"

# --- F4: collision guard (host-local file with an account basename) ---
H3="$(mktmp)"; LIVE3="$H3/.claude/projects/$SLUG/memory"
seed_hostlocal "$LIVE3" "acct-one.md" "HOST CONTENT KEEP"
run_prov "$H3" "$REPO"
assert_grep "F4: collision preserves the host file's content" "HOST CONTENT KEEP" "$LIVE3/acct-one.md"

# --- F6: idempotency (second run produces an identical index) ---
H4="$(mktmp)"; LIVE4="$H4/.claude/projects/$SLUG/memory"
seed_hostlocal "$LIVE4" "local-note.md" "LOCAL BODY"
run_prov "$H4" "$REPO"; cp "$LIVE4/MEMORY.md" "$H4/idx1"
run_prov "$H4" "$REPO"
assert_in_subshell "F6: second run yields an identical MEMORY.md" \
    "diff -q '$H4/idx1' '$LIVE4/MEMORY.md'"

# --- Pre-migration host copies (a Raspberry Pi fleet host, 2026-09-11): the host
# holds the ORIGINAL of a memory the repo now ships, differing only by the repo's
# `scope: account` line (+ trailing whitespace). It is the same memory, so the
# repo copy takes over; genuinely edited host copies are kept and reported ONCE
# on stdout (not 5 stderr WARNs per run, which fleet counts as warnings). ---
run_prov_capture() { # $1 HOME, $2 BASE_DIR, $3 out file, $4 err file
    HOME="$1" BASE_DIR="$2" bash "$PROV" >"$3" 2>"$4"
}
# pre_migration <repo file> <dest> → the repo file minus its scope: line, with
# trailing whitespace added/removed the way hand-edited copies drift.
pre_migration() {
    grep -v '^[[:space:]]*scope:' "$1" | sed 's/^metadata:.*$/metadata:/; s/^body \(.*\)$/body \1   /' > "$2"
}

# (a) identical except the scope: line (+ trailing whitespace) → replaced by the repo copy
H5="$(mktmp)"; LIVE5="$H5/.claude/projects/$SLUG/memory"; mkdir -p "$LIVE5"
pre_migration "$REPO/ai/claude/memory/acct-one.md" "$LIVE5/acct-one.md"
assert_grep_negative "fixture: pre-migration copy has no scope line" 'scope:' "$LIVE5/acct-one.md"
run_prov_capture "$H5" "$REPO" "$H5/out" "$H5/err"
assert_in_subshell "scope-only diff: host copy replaced by the repo version" \
    "cmp -s '$REPO/ai/claude/memory/acct-one.md' '$LIVE5/acct-one.md'"
assert_grep "scope-only diff: stdout says the host copy was replaced" 'acct-one\.md' "$H5/out"
assert_eq "$(wc -c < "$H5/err" | tr -d ' ')" "0" "scope-only diff: nothing on stderr"
run_prov_capture "$H5" "$REPO" "$H5/out2" "$H5/err2"
assert_grep_negative "scope-only diff: second run is silent about it (now repo-managed)" \
    'acct-one' "$H5/out2"

# (b) genuinely different host copies → kept; ONE stdout summary; stderr empty
H6="$(mktmp)"; LIVE6="$H6/.claude/projects/$SLUG/memory"
seed_hostlocal "$LIVE6" "acct-one.md" "HOST EDIT ONE"
seed_hostlocal "$LIVE6" "acct-two.md" "HOST EDIT TWO"
run_prov_capture "$H6" "$REPO" "$H6/out" "$H6/err"
assert_grep "differs: host acct-one.md kept"  "HOST EDIT ONE" "$LIVE6/acct-one.md"
assert_grep "differs: host acct-two.md kept"  "HOST EDIT TWO" "$LIVE6/acct-two.md"
assert_eq "$(grep -c 'acct-one\.md' "$H6/out" | tr -d ' ')" "1" \
    "differs: acct-one.md named on exactly one stdout line"
assert_eq "$(grep -c 'acct-two\.md' "$H6/out" | tr -d ' ')" "1" \
    "differs: acct-two.md named on exactly one stdout line"
assert_eq "$(grep -c 'acct-one\.md.*acct-two\.md' "$H6/out" | tr -d ' ')" "1" \
    "differs: both names on ONE summary line"
assert_grep "differs: summary says how to reconcile (diff)" 'diff ' "$H6/out"
assert_eq "$(wc -c < "$H6/err" | tr -d ' ')" "0" "differs: nothing on stderr"

# (c) absent → provisioned as before, no summary, stderr empty
H7="$(mktmp)"; LIVE7="$H7/.claude/projects/$SLUG/memory"; mkdir -p "$LIVE7"
run_prov_capture "$H7" "$REPO" "$H7/out" "$H7/err"
assert_in_subshell "absent: account file provisioned verbatim" \
    "cmp -s '$REPO/ai/claude/memory/acct-two.md' '$LIVE7/acct-two.md'"
assert_eq "$(wc -c < "$H7/out" | tr -d ' ')" "0" "absent: nothing to report on stdout"
assert_eq "$(wc -c < "$H7/err" | tr -d ' ')" "0" "absent: nothing on stderr"

_test_report
