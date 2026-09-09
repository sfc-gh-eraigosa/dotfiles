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
#      Skipped with a warning when go-licenses is absent OR not runnable under
#      the active Go toolchain (a goenv shim for another version), UNLESS
#      GSS_STRICT_CHECK=1 (set in CI), in which case that is a failure.
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
# Presence probe, NOT `command -v`. Under a version manager (goenv/asdf) a SHIM
# exists on PATH for every binary ever installed under ANY toolchain version, so
# `command -v go-licenses` succeeds even when the tool is absent from the ACTIVE
# version. Invoking the shim then exits 127 with:
#     goenv: 'go-licenses' command not found
#     The 'go-licenses' command exists in these Go versions: 1.26.3
# The old `command -v` guard took that 127 as "check ./... returned non-zero" and
# reported "a disallowed license is present" — a false FAIL that aborted
# install.sh (exit 1) on every host whose go-licenses lived under a different go
# version than the repo-pinned .go-version. Probe that it actually RUNS.
if command -v go-licenses >/dev/null 2>&1 && go-licenses --help >/dev/null 2>&1; then
  # restricted = GPL/AGPL/LGPL family; forbidden = unlicensed / commercial.
  #
  # go-licenses exits non-zero for two unrelated reasons, and conflating them
  # is how this gate has produced a false FAIL twice now (see the shim note
  # above). Capture the output and tell them apart:
  #
  #   1. it FOUND a disallowed license   -> a real finding, fail the build
  #   2. it could not LOAD the packages  -> the tool cannot analyze this
  #      toolchain, so it has found nothing either way
  #
  # (2) is what Go 1.26 produces when the toolchain itself lives in the module
  # cache (observed on darwin/amd64: go 1.26.2 running toolchain 1.26.3 out of
  # $GOPATH/pkg/mod/golang.org/toolchain@...). The stdlib then has no module
  # info for go-licenses to read:
  #     Package encoding/json does not have module info. Non go modules
  #     projects are no longer supported.
  #     main.go:77] some errors occurred when loading direct and transitive
  #     dependency packages
  # Reporting that as "a disallowed license is present" is simply false, and
  # it aborted install.sh on a developer machine. An analyzer that could not
  # run has found nothing; treat it exactly like an absent tool.
  # The assignment MUST live in an `if` condition: this script runs under
  # `set -e`, so a bare `lic_out="$(cmd)"` whose cmd exits non-zero kills the
  # script before the exit code can be examined — which silently reintroduced
  # the very failure this block exists to prevent.
  if lic_out="$(go-licenses check ./... --disallowed_types=forbidden,restricted 2>&1)"; then
    lic_rc=0
  else
    lic_rc=$?
  fi
  if [ "$lic_rc" -eq 0 ]; then
    echo "OK: all dependency licenses are permissive."
  elif printf '%s\n' "$lic_out" | grep -qE 'does not have module info|errors occurred when loading'; then
    if [ "${GSS_STRICT_CHECK:-0}" = "1" ]; then
      echo "FAIL: go-licenses could not analyze the dependency tree under this Go toolchain (strict/CI mode)."
      printf '%s\n' "$lic_out" | tail -3
      fail=1
    else
      echo "SKIP: go-licenses could not analyze the dependency tree under this Go"
      echo "      toolchain (no module info for stdlib packages), so it found"
      echo "      NOTHING. This is a tool limitation, not a license finding."
      echo "      (set GSS_STRICT_CHECK=1 to make this a hard failure)"
    fi
  else
    echo "FAIL: a disallowed (copyleft/forbidden) license is present in the dep tree."
    printf '%s\n' "$lic_out" | tail -5
    fail=1
  fi
elif [ "${GSS_STRICT_CHECK:-0}" = "1" ]; then
  echo "FAIL: go-licenses not installed or not runnable under the active Go toolchain (required in strict/CI mode)."
  echo "  install: go install github.com/google/go-licenses@latest"
  echo "  (under goenv, install it for the version pinned by .go-version, then: goenv rehash)"
  fail=1
else
  echo "SKIP: go-licenses not installed or not runnable under the active Go toolchain; skipping license gate."
  echo "  (set GSS_STRICT_CHECK=1, or 'go install github.com/google/go-licenses@latest', to enforce)"
fi

exit $fail
