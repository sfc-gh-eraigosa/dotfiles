#!/usr/bin/env bash
# Test driver for opt/lib/gff.sh (mirrors ai/hooks/safety_guard_test.sh's
# assert style). POSIX-only on purpose: the F9 gate must behave identically
# under bash AND dash, so run both:
#   bash opt/lib/gff_test.sh
#   sh   opt/lib/gff_test.sh
# shellcheck disable=SC2034 # GFF_* test vars are read indirectly by gff_on via eval
set -u

# Resolve the helper from this script's own location (POSIX: no BASH_SOURCE)
# so the driver runs anywhere — developer checkout, worktree, or CI workdir.
SCRIPT_DIR=$(cd -- "$(dirname "$0")" && pwd -P)
HELPER="${SCRIPT_DIR}/gff.sh"
PASS=0
FAIL=0

if [ ! -f "$HELPER" ]; then
    echo "FAIL: helper not found at $HELPER"
    echo "---"
    echo "PASS: 0  FAIL: 1"
    exit 1
fi
# shellcheck source=opt/lib/gff.sh
. "$HELPER"

# usage: assert_gff_on <expected_rc> <key> <label>
# Caller prepares the relevant GFF_* variable (set/unset) before calling.
assert_gff_on() {
    _expected="$1"; _akey="$2"; _label="$3"
    gff_on "$_akey"
    _rc=$?
    if [ "$_rc" -eq "$_expected" ]; then
        echo "PASS: $_label (exit $_rc)"
        PASS=$((PASS+1))
    else
        echo "FAIL: $_label (expected $_expected, got $_rc)"
        FAIL=$((FAIL+1))
    fi
}

# usage: assert_gff_opt_in <expected_rc> <key> <label>
# Caller prepares the relevant GFF_* variable (set/unset) before calling.
assert_gff_opt_in() {
    _expected="$1"; _akey="$2"; _label="$3"
    gff_opt_in "$_akey"
    _rc=$?
    if [ "$_rc" -eq "$_expected" ]; then
        echo "PASS: $_label (exit $_rc)"
        PASS=$((PASS+1))
    else
        echo "FAIL: $_label (expected $_expected, got $_rc)"
        FAIL=$((FAIL+1))
    fi
}

# usage: assert_out <expected_string> <actual_string> <label>
assert_out() {
    if [ "$2" = "$1" ]; then
        echo "PASS: $3"
        PASS=$((PASS+1))
    else
        echo "FAIL: $3 (expected '$1', got '$2')"
        FAIL=$((FAIL+1))
    fi
}

# === fail-open semantics: only the literal lowercase "false" disables ===
unset GFF_INSTALL_TOOLS_SOPS 2>/dev/null || true
assert_gff_on 0 install.tools.sops "var unset => run (fail-open)"

GFF_INSTALL_TOOLS_SOPS=true
assert_gff_on 0 install.tools.sops "=true => run"

GFF_INSTALL_TOOLS_SOPS=false
assert_gff_on 1 install.tools.sops "=false => skip (the only disabling value)"

GFF_INSTALL_TOOLS_SOPS=FALSE
assert_gff_on 0 install.tools.sops "=FALSE => run (uppercase is not false)"

GFF_INSTALL_TOOLS_SOPS=0
assert_gff_on 0 install.tools.sops "=0 => run (0 is not false)"

GFF_INSTALL_TOOLS_SOPS='no thanks'
assert_gff_on 0 install.tools.sops "garbage value => run (fail-open)"
unset GFF_INSTALL_TOOLS_SOPS 2>/dev/null || true

# === key mangling: dots AND hyphens -> underscores, uppercased ===
GFF_INSTALL_WINDOWS_WISPR_FLOW=false
assert_gff_on 1 install.windows.wispr-flow \
    "key mangling: install.windows.wispr-flow reads GFF_INSTALL_WINDOWS_WISPR_FLOW"
unset GFF_INSTALL_WINDOWS_WISPR_FLOW 2>/dev/null || true
assert_gff_on 0 install.windows.wispr-flow "mangled var unset again => run"

# === fail-CLOSED semantics: only the literal lowercase "true" enables ===
# gff_opt_in is the deliberate inversion of gff_on, used for boolDefault:false
# steps (install.windows.security-audit). Anything that is not exactly "true" —
# including an unset var, an absent gff, or a garbage value — must SKIP, so an
# opt-in installer can never appear on a machine by accident.
unset GFF_INSTALL_WINDOWS_SECURITY_AUDIT 2>/dev/null || true
assert_gff_opt_in 1 install.windows.security-audit "opt-in: var unset => skip (fail-closed)"

GFF_INSTALL_WINDOWS_SECURITY_AUDIT=true
assert_gff_opt_in 0 install.windows.security-audit "opt-in: =true => run (the only enabling value)"

GFF_INSTALL_WINDOWS_SECURITY_AUDIT=false
assert_gff_opt_in 1 install.windows.security-audit "opt-in: =false => skip"

GFF_INSTALL_WINDOWS_SECURITY_AUDIT=TRUE
assert_gff_opt_in 1 install.windows.security-audit "opt-in: =TRUE => skip (uppercase is not true)"

GFF_INSTALL_WINDOWS_SECURITY_AUDIT=1
assert_gff_opt_in 1 install.windows.security-audit "opt-in: =1 => skip (1 is not true)"

GFF_INSTALL_WINDOWS_SECURITY_AUDIT='true '
assert_gff_opt_in 1 install.windows.security-audit "opt-in: ='true ' => skip (no trimming)"

GFF_INSTALL_WINDOWS_SECURITY_AUDIT=''
assert_gff_opt_in 1 install.windows.security-audit "opt-in: empty => skip"
unset GFF_INSTALL_WINDOWS_SECURITY_AUDIT 2>/dev/null || true

# gff_on and gff_opt_in must disagree on an unset var — that inversion IS the
# feature. A refactor that accidentally unified them would pass every test
# above; this case is what catches it.
unset GFF_INSTALL_WINDOWS_SECURITY_AUDIT 2>/dev/null || true
gff_on install.windows.security-audit; _on_rc=$?
gff_opt_in install.windows.security-audit; _in_rc=$?
if [ "$_on_rc" -eq 0 ] && [ "$_in_rc" -eq 1 ]; then
    echo "PASS: unset var: gff_on runs (0) but gff_opt_in skips (1) — directions are inverted"
    PASS=$((PASS+1))
else
    echo "FAIL: unset var inversion (gff_on=$_on_rc expected 0, gff_opt_in=$_in_rc expected 1)"
    FAIL=$((FAIL+1))
fi

# === opt-in key mangling: dots AND hyphens -> underscores, uppercased ===
GFF_INSTALL_WINDOWS_SECURITY_AUDIT=true
assert_gff_opt_in 0 install.windows.security-audit \
    "opt-in key mangling: install.windows.security-audit reads GFF_INSTALL_WINDOWS_SECURITY_AUDIT"
unset GFF_INSTALL_WINDOWS_SECURITY_AUDIT 2>/dev/null || true

# === gff_skip_msg output contract ===
_out=$(gff_skip_msg install.tools.sops)
assert_out "SKIP (gff: install.tools.sops=false)" "$_out" \
    "gff_skip_msg echoes exactly 'SKIP (gff: <key>=false)'"

# === missing binary: gff off PATH => gates still fail open ===
# gff_on is env-only by contract (it never invokes a binary), so a PATH with
# no gff must not change the answer. Keep system dirs so tr/printf still work.
(
    PATH="/usr/bin:/bin"
    if command -v gff >/dev/null 2>&1; then
        echo "NOTE: a gff binary is still on the reduced PATH; case remains valid (gff_on is env-only)"
    fi
    unset GFF_INSTALL_TOOLS_SOPS 2>/dev/null || true
    gff_on install.tools.sops
)
_rc=$?
if [ "$_rc" -eq 0 ]; then
    echo "PASS: gff binary absent from PATH => run (exit $_rc)"
    PASS=$((PASS+1))
else
    echo "FAIL: gff binary absent from PATH => run (expected 0, got $_rc)"
    FAIL=$((FAIL+1))
fi

# === missing binary: gff off PATH => opt-in gate still fails CLOSED ===
# The mirror of the case above. gff_opt_in is env-only by the same contract, so
# a machine with no gff binary must still refuse to run the opt-in step.
(
    PATH="/usr/bin:/bin"
    unset GFF_INSTALL_WINDOWS_SECURITY_AUDIT 2>/dev/null || true
    gff_opt_in install.windows.security-audit
)
_rc=$?
if [ "$_rc" -eq 1 ]; then
    echo "PASS: gff binary absent from PATH => opt-in skips (exit $_rc)"
    PASS=$((PASS+1))
else
    echo "FAIL: gff binary absent from PATH => opt-in skips (expected 1, got $_rc)"
    FAIL=$((FAIL+1))
fi

echo "---"
echo "PASS: $PASS  FAIL: $FAIL"
[ "$FAIL" -eq 0 ]
