#!/bin/bash
# CI guard: pkg/workspace/ was deleted in PR-59 — gss now owns worktree
# creation/teardown (the tmux-mgr refactor's whole point). Fail if it reappears
# or anything imports it, so the dependency cannot creep back in.
#
# Usage: scripts/no-workspace-guard.sh   (run from anywhere)
set -uo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)" # src/tmux-mgr
fail=0

if [ -d "$DIR/pkg/workspace" ]; then
	echo "GUARD FAIL: pkg/workspace/ exists again — it was deleted in PR-59 (gss owns worktrees)."
	fail=1
fi

# The import path is the only way to consume the package; grep for it in source.
hits="$(grep -rn "pkg/workspace" --include='*.go' "$DIR" 2>/dev/null || true)"
if [ -n "$hits" ]; then
	echo "GUARD FAIL: source still references the deleted pkg/workspace:"
	echo "$hits"
	fail=1
fi

if [ "$fail" -eq 0 ]; then
	echo "OK: no pkg/workspace directory or imports (gss owns worktrees)."
fi
exit "$fail"
