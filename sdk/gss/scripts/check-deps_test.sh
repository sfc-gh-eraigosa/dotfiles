#!/usr/bin/env bash
# Self-test for check-deps.sh (PR-50). Proves the seam gate passes on the
# clean tree and fails when an internal/ package (outside git/gh) imports
# os/exec. The license-fail path is exercised in CI (needs go-licenses + a
# banned dep) and is not simulated here.
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
assert_exit 0 "clean tree passes (non-strict)" env -u GSS_STRICT_CHECK bash "$CHECK"

# 2) A seam violation under internal/ (outside git/gh) fails.
PROBE="$MODULE_DIR/internal/seamcheck_probe.go"
cleanup() { rm -f "$PROBE"; }
trap cleanup EXIT
cat >"$PROBE" <<'EOF'
package internal

import _ "os/exec"
EOF
assert_exit 1 "seam violation fails" env -u GSS_STRICT_CHECK bash "$CHECK"
cleanup
trap - EXIT

# 3) Clean again after cleanup.
assert_exit 0 "clean again after probe removed" env -u GSS_STRICT_CHECK bash "$CHECK"

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
  env -u GSS_STRICT_CHECK PATH="$SHIMDIR:$PATH" bash "$CHECK"

# 5) ...and in strict/CI mode it FAILS as "not installed", not as a bad license.
assert_exit 1 "unrunnable go-licenses shim fails strict mode" \
  env GSS_STRICT_CHECK=1 PATH="$SHIMDIR:$PATH" bash "$CHECK"

# 6) The strict failure must name the TOOL, never claim a disallowed license.
shim_out="$(env GSS_STRICT_CHECK=1 PATH="$SHIMDIR:$PATH" bash "$CHECK" 2>&1 || true)"
if printf '%s' "$shim_out" | grep -q "go-licenses not installed or not runnable" \
   && ! printf '%s' "$shim_out" | grep -q "disallowed (copyleft/forbidden) license is present"; then
  echo "ok   - shim failure is diagnosed as a missing tool, not a bad license"; pass=$((pass + 1))
else
  echo "FAIL - shim failure misdiagnosed:"; printf '%s\n' "$shim_out" | sed 's/^/       /'; fail=$((fail + 1))
fi

shim_cleanup
trap - EXIT

echo "----"
echo "check-deps_test: $pass passed, $fail failed"
[ "$fail" = "0" ]
