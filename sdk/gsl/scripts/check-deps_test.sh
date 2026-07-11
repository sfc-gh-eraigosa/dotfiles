#!/usr/bin/env bash
# Self-test for check-deps.sh. Proves the seam gate passes on the
# clean tree and fails when an internal/ package (outside git/mcp/gh)
# imports os/exec. The license-fail path is exercised in CI (needs
# go-licenses + a banned dep) and is not simulated here.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CHECK="$SCRIPT_DIR/check-deps.sh"
pass=0
fail=0

assert_exit() {
  local want="$1"; shift
  local desc="$1"; shift
  "$@" >/dev/null 2>&1
  local got=$?
  if [ "$got" = "$want" ]; then
    echo "ok   - $desc (exit $got)"; pass=$((pass + 1))
  else
    echo "FAIL - $desc (want exit $want, got $got)"; fail=$((fail + 1))
  fi
}

# 1) Clean tree passes (seam OK; license skipped without go-licenses, non-strict).
assert_exit 0 "clean tree passes (non-strict)" env -u GSL_STRICT_CHECK bash "$CHECK"

# 2) A seam violation under internal/ (outside git/mcp/gh) fails.
PROBE="$MODULE_DIR/internal/config/seamcheck_probe.go"
cleanup() { rm -f "$PROBE"; }
trap cleanup EXIT
cat >"$PROBE" <<'EOF'
package config

import _ "os/exec"
EOF
assert_exit 1 "seam violation fails" env -u GSL_STRICT_CHECK bash "$CHECK"
cleanup
trap - EXIT

# 3) Clean again after cleanup.
assert_exit 0 "clean again after probe removed" env -u GSL_STRICT_CHECK bash "$CHECK"

echo "----"
echo "check-deps_test: $pass passed, $fail failed"
[ "$fail" = "0" ]
