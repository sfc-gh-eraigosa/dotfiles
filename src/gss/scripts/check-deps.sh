#!/usr/bin/env bash
# Dependency + seam gate for the gss module (PR-50; design.md → "Pinned
# external dependencies", resolution #15; STATE.md carry-forward #8).
#
# Two checks:
#   1. os/exec seam — no package under internal/ EXCEPT internal/git and
#      internal/gh may import "os/exec". Every subprocess call must go through
#      the git.Runner / gh.Client seams so the logic stays fakeable. cmd/ is
#      the composition root (wiring) and is intentionally exempt.
#   2. License gate — go-licenses check ./... must find no GPL/AGPL/LGPL
#      (restricted) or forbidden license anywhere in the dependency tree.
#      Skipped with a warning when go-licenses is absent, UNLESS
#      GSS_STRICT_CHECK=1 (set in CI), in which case its absence is a failure.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$MODULE_DIR"

fail=0

echo "== seam check: os/exec confined to internal/git + internal/gh =="
violations="$(grep -rln '"os/exec"' --include='*.go' internal \
  | grep -v '_test\.go$' \
  | grep -v '^internal/git/' \
  | grep -v '^internal/gh/' || true)"
if [ -n "$violations" ]; then
  echo "FAIL: os/exec imported outside the git/gh seams:"
  echo "$violations" | sed 's/^/  - /'
  echo "  Route subprocess calls through git.Runner / gh.Client instead."
  fail=1
else
  echo "OK: no os/exec outside internal/git + internal/gh."
fi

echo "== license check: no GPL/AGPL/LGPL/forbidden in the dependency tree =="
if command -v go-licenses >/dev/null 2>&1; then
  # restricted = GPL/AGPL/LGPL family; forbidden = unlicensed / commercial.
  if go-licenses check ./... --disallowed_types=forbidden,restricted; then
    echo "OK: all dependency licenses are permissive."
  else
    echo "FAIL: a disallowed (copyleft/forbidden) license is present in the dep tree."
    fail=1
  fi
elif [ "${GSS_STRICT_CHECK:-0}" = "1" ]; then
  echo "FAIL: go-licenses not installed (required in strict/CI mode)."
  echo "  install: go install github.com/google/go-licenses@latest"
  fail=1
else
  echo "SKIP: go-licenses not installed; skipping license gate."
  echo "  (set GSS_STRICT_CHECK=1, or 'go install github.com/google/go-licenses@latest', to enforce)"
fi

exit $fail
