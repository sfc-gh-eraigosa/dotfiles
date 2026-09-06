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

_test_report
