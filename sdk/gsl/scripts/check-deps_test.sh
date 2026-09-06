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

# 4) A goenv-style go-licenses SHIM that resolves on PATH but exits 127 must be
#    treated as ABSENT, not as a license violation. This is the ashyumi
#    regression: go-licenses installed under go 1.26.3 while .go-version pins
#    1.26.1, so `command -v go-licenses` succeeded, the shim exited 127, and the
#    gate reported "a disallowed license is present" — aborting install.sh.
SHIMDIR="$(mktemp -d)"
shim_cleanup() { rm -rf "$SHIMDIR"; }
trap shim_cleanup EXIT
cat >"$SHIMDIR/go-licenses" <<'SHIM'
#!/usr/bin/env bash
echo "goenv: 'go-licenses' command not found" >&2
echo "" >&2
echo "The 'go-licenses' command exists in these Go versions:" >&2
echo "  1.26.3" >&2
exit 127
SHIM
chmod +x "$SHIMDIR/go-licenses"

assert_exit 0 "unrunnable go-licenses shim skips the gate (non-strict)" \
  env -u GSL_STRICT_CHECK PATH="$SHIMDIR:$PATH" bash "$CHECK"

# 5) ...and in strict/CI mode it FAILS as "not installed", not as a bad license.
assert_exit 1 "unrunnable go-licenses shim fails strict mode" \
  env GSL_STRICT_CHECK=1 PATH="$SHIMDIR:$PATH" bash "$CHECK"

# 6) The strict failure must name the TOOL, never claim a disallowed license.
shim_out="$(env GSL_STRICT_CHECK=1 PATH="$SHIMDIR:$PATH" bash "$CHECK" 2>&1 || true)"
if printf '%s' "$shim_out" | grep -q "go-licenses not installed or not runnable" \
   && ! printf '%s' "$shim_out" | grep -q "disallowed (copyleft/forbidden) license is present"; then
  echo "ok   - shim failure is diagnosed as a missing tool, not a bad license"; pass=$((pass + 1))
else
  echo "FAIL - shim failure misdiagnosed:"; printf '%s\n' "$shim_out" | sed 's/^/       /'; fail=$((fail + 1))
fi

# ---------------------------------------------------------------------------
# go-licenses that RUNS but cannot ANALYZE.
#
# Observed on darwin/amd64 with Go 1.26.2 running toolchain 1.26.3 out of the
# module cache: the stdlib has no module info, go-licenses aborts, and the gate
# used to report "a disallowed license is present" — aborting install.sh over a
# license finding that did not exist.
# ---------------------------------------------------------------------------
LOADDIR="$(mktemp -d)"
cat > "$LOADDIR/go-licenses" <<'STUB'
#!/bin/sh
[ "$1" = "--help" ] && exit 0
echo "E0821 library.go:117] Package encoding/json does not have module info. Non go modules projects are no longer supported." >&2
echo "F0821 main.go:77] some errors occurred when loading direct and transitive dependency packages" >&2
exit 1
STUB
chmod +x "$LOADDIR/go-licenses"

# 7) An analyzer that could not run has found nothing: skip, do not fail.
assert_exit 0 "unanalyzable toolchain skips instead of failing the build" \
  env PATH="$LOADDIR:$PATH" bash "$CHECK"

# 8) ...and it must NOT be described as a license finding.
load_out="$(env PATH="$LOADDIR:$PATH" bash "$CHECK" 2>&1 || true)"
if printf '%s' "$load_out" | grep -q "not a license finding" \
   && ! printf '%s' "$load_out" | grep -q "disallowed (copyleft/forbidden) license is present"; then
  echo "ok   - unanalyzable toolchain is diagnosed as a tool limit, not a bad license"; pass=$((pass + 1))
else
  echo "FAIL - unanalyzable toolchain misdiagnosed:"; printf '%s\n' "$load_out" | sed 's/^/       /'; fail=$((fail + 1))
fi

# 9) CI must not silently skip a gate it is meant to enforce.
assert_exit 1 "unanalyzable toolchain still fails strict/CI mode" \
  env GSL_STRICT_CHECK=1 PATH="$LOADDIR:$PATH" bash "$CHECK"

# 10) A REAL finding must still fail — the whole point of the gate.
FINDDIR="$(mktemp -d)"
cat > "$FINDDIR/go-licenses" <<'STUB'
#!/bin/sh
[ "$1" = "--help" ] && exit 0
echo "forbidden license GPL-3.0 for package github.com/evil/thing" >&2
exit 1
STUB
chmod +x "$FINDDIR/go-licenses"
assert_exit 1 "a genuine disallowed license still fails the build" \
  env PATH="$FINDDIR:$PATH" bash "$CHECK"
find_out="$(env PATH="$FINDDIR:$PATH" bash "$CHECK" 2>&1 || true)"
if printf '%s' "$find_out" | grep -q "disallowed (copyleft/forbidden) license is present"; then
  echo "ok   - a real finding is still reported as a license failure"; pass=$((pass + 1))
else
  echo "FAIL - real finding was not reported:"; printf '%s\n' "$find_out" | sed 's/^/       /'; fail=$((fail + 1))
fi
rm -rf "$LOADDIR" "$FINDDIR"

shim_cleanup
trap - EXIT

echo "----"
echo "check-deps_test: $pass passed, $fail failed"
[ "$fail" = "0" ]
