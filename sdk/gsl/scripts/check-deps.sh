#!/usr/bin/env bash
# Dependency + seam gate for the gsl module.
#
# Three checks:
#   1. os/exec seam — no package under internal/ EXCEPT internal/git,
#      internal/mcp, internal/gh, and internal/exec may import "os/exec". Every
#      subprocess call must go through those three seams so the logic stays
#      fakeable. cmd/ is the composition root (wiring) and is intentionally
#      exempt.
#   2. internal/exec sub-gate — internal/exec is exempted from (1) because it
#      holds the shared subprocess-containment helper (Setpgid + process-group
#      kill + WaitDelay) that the three seams apply to their *exec.Cmd. It
#      CONFIGURES commands; it must never RUN one, or it would become a fourth,
#      unfakeable seam through the back door. Enforced by banning command
#      construction/execution there.
#   3. License gate — go-licenses check ./... must find no GPL/AGPL/LGPL
#      (restricted) or forbidden license anywhere in the dependency tree.
#      Skipped with a warning when go-licenses is absent OR not runnable under
#      the active Go toolchain (a goenv shim for another version), UNLESS
#      GSL_STRICT_CHECK=1 (set in CI), in which case that is a failure.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$MODULE_DIR"

fail=0

echo "== seam check: os/exec confined to internal/git + internal/mcp + internal/gh (+ internal/exec helper) =="
violations="$(grep -rln '"os/exec"' --include='*.go' internal \
  | grep -v '_test\.go$' \
  | grep -v '^internal/git/' \
  | grep -v '^internal/mcp/' \
  | grep -v '^internal/gh/' \
  | grep -v '^internal/exec/' || true)"
if [ -n "$violations" ]; then
  echo "FAIL: os/exec imported outside the git/mcp/gh seams:"
  echo "$violations" | sed 's/^/  - /'
  echo "  Route subprocess calls through git.Runner / mcp.Runner / gh.Runner instead."
  fail=1
else
  echo "OK: no os/exec outside internal/git + internal/mcp + internal/gh + internal/exec."
fi

echo "== sub-gate: internal/exec configures commands, never runs them =="
if [ -d internal/exec ]; then
  # It may take an *exec.Cmd and set fields on it. It may not CREATE or START
  # one — that would make it a fourth seam that no test can fake.
  #
  # Match CALLS, not prose: require the open paren, and drop comment lines (the
  # package doc necessarily *names* exec.CommandContext when documenting that
  # callers must use it).
  runners="$(grep -rnE 'exec\.Command(Context)?\(|\.Start\(\)|\.Run\(\)|\.Output\(\)|\.CombinedOutput\(\)' \
    --include='*.go' internal/exec \
    | grep -v '_test\.go:' \
    | grep -vE ':[0-9]+:[[:space:]]*//' || true)"
  if [ -n "$runners" ]; then
    echo "FAIL: internal/exec constructs or executes a command:"
    echo "$runners" | sed 's/^/  - /'
    echo "  internal/exec may only CONFIGURE a *exec.Cmd (Setpgid / Cancel / WaitDelay)."
    echo "  Subprocess execution belongs in the git/gh/mcp seams, behind their Runner interfaces."
    fail=1
  else
    echo "OK: internal/exec only configures commands."
  fi
else
  echo "SKIP: internal/exec not present."
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
    if [ "${GSL_STRICT_CHECK:-0}" = "1" ]; then
      echo "FAIL: go-licenses could not analyze the dependency tree under this Go toolchain (strict/CI mode)."
      printf '%s\n' "$lic_out" | tail -3
      fail=1
    else
      echo "SKIP: go-licenses could not analyze the dependency tree under this Go"
      echo "      toolchain (no module info for stdlib packages), so it found"
      echo "      NOTHING. This is a tool limitation, not a license finding."
      echo "      (set GSL_STRICT_CHECK=1 to make this a hard failure)"
    fi
  else
    echo "FAIL: a disallowed (copyleft/forbidden) license is present in the dep tree."
    printf '%s\n' "$lic_out" | tail -5
    fail=1
  fi
elif [ "${GSL_STRICT_CHECK:-0}" = "1" ]; then
  echo "FAIL: go-licenses not installed or not runnable under the active Go toolchain (required in strict/CI mode)."
  echo "  install: go install github.com/google/go-licenses@latest"
  echo "  (under goenv, install it for the version pinned by .go-version, then: goenv rehash)"
  fail=1
else
  echo "SKIP: go-licenses not installed or not runnable under the active Go toolchain; skipping license gate."
  echo "  (set GSL_STRICT_CHECK=1, or 'go install github.com/google/go-licenses@latest', to enforce)"
fi

exit $fail
