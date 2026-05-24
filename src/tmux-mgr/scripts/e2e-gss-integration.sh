#!/bin/bash
# End-to-end evaluation of the tmux-mgr <-> gss integration seam (PR-54..57).
#
# tmux-mgr shells out to `gss feature worker add --json` and parses the result
# into cmd/agent_gss.go's workerAddResult{worker_ref, branch, worktree_path,
# base_branch}. Unit tests on both sides mock the gss runner, so they CANNOT
# catch a runtime mismatch — e.g. an installed `gss` binary that is older than
# the source and lacks `--json`. This script closes that gap: it drives the
# REAL gss binary in an isolated sandbox (its own registry/worktree/state dirs
# and a throwaway git repo) and asserts the JSON contract + that the worktree
# is actually created.
#
# Usage:
#   scripts/e2e-gss-integration.sh           # tests the gss on PATH (default)
#   scripts/e2e-gss-integration.sh /tmp/gss  # tests a specific binary
#
# Exit non-zero on any contract failure. Requires: git, python3, gss.
set -uo pipefail

GSS="${1:-gss}"   # default: the INSTALLED gss on PATH (so staleness is caught)
SBX="$(mktemp -d)"
trap 'rm -rf "$SBX"' EXIT

export GSS_REGISTRY_DIR="$SBX/registry"
export GSS_WORKTREE_ROOT="$SBX/worktrees"
export GSS_STATE_DIR="$SBX/state"
export GSS_DEFAULTS_USER="erai"
export GSS_DEFAULTS_BASE_BRANCH="main"

REPO="$SBX/repo"
mkdir -p "$REPO"
git -C "$REPO" init -q -b main
git -C "$REPO" config user.email t@t.io
git -C "$REPO" config user.name  t
git -C "$REPO" commit -q --allow-empty -m init
git -C "$REPO" remote add origin git@github.com:test/sandbox.git

echo "### gss = $GSS"
echo "### sandbox = $SBX"
echo

echo "=== 1) gss feature start demo ==="
"$GSS" feature start demo --repo "$REPO"; echo "  exit=$?"
echo

echo "=== 2) gss feature worker add --json (the tmux-mgr shell-out) ==="
OUT="$("$GSS" feature worker add \
  --feature demo --purpose api --description "e2e smoke" \
  --user erai --engine claude --session-id S1 --tmux-mgr-session T1 \
  --json --repo "$REPO" 2>&1)"
RC=$?
echo "$OUT"
echo "  exit=$RC"
echo

echo "=== 3) validate the JSON contract workerAddResult expects ==="
echo "$OUT" | python3 -c '
import sys, json
d = json.load(sys.stdin)
need = ["worker_ref", "branch", "worktree_path", "base_branch"]
miss = [k for k in need if not d.get(k)]
if miss:
    print("FAIL missing/empty:", miss); sys.exit(1)
print("OK  worker_ref   =", d["worker_ref"])
print("OK  branch       =", d["branch"])
print("OK  worktree_path=", d["worktree_path"])
print("OK  base_branch  =", d["base_branch"])
' || { echo "JSON CONTRACT FAILED"; exit 1; }
echo

echo "=== 4) the worktree gss reported actually exists ==="
WT="$(echo "$OUT" | python3 -c 'import sys,json;print(json.load(sys.stdin)["worktree_path"])')"
if [ -d "$WT" ]; then echo "OK  worktree dir present: $WT"; else echo "FAIL worktree dir missing: $WT"; exit 1; fi
echo
echo "### E2E PASS"
