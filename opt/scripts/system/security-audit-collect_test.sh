#!/usr/bin/env bash
# =============================================================================
# security-audit-collect_test.sh - test driver for the Windows security-audit
# collector (opt/Desktop/Apps/scripts/security-audit-collect.ps1).
#
# Two tiers, so it is useful in CI AND on a real Windows/WSL host:
#
#   STATIC (always, incl. Linux CI) - grep the .ps1 source for the invariants
#     the collector's whole safety model depends on (fail-loud error handling,
#     the three-state Add-Section contract, byte-stable admin-gap markers, the
#     4104 self-exclusion, probe-first event reads, the expected section set).
#     A regression here (e.g. ErrorActionPreference flipped back to 'Continue')
#     fails CI even though CI has no PowerShell.
#
#   LIVE (only when a powershell.exe is resolvable - WSL/Windows dev) - actually
#     run the collector with -StdOut -Days 7 (read-only, writes nothing) against
#     the real host and assert the OUTPUT contract: header present, every
#     expected section emitted, NO '!! COLLECTION ERROR', no empty section,
#     exit 0. On a pure-Linux box with no PowerShell this tier SKIPs (not fails).
#
# Run standalone: bash opt/scripts/system/security-audit-collect_test.sh
# POSIX-friendly; no bashisms that trip the portability gate.
# =============================================================================
set -u

SCRIPT_DIR=$(cd -- "$(dirname "$0")" && pwd -P)
REPO_ROOT=$(cd -- "${SCRIPT_DIR}/../../.." && pwd -P)
COLLECTOR="${REPO_ROOT}/opt/Desktop/Apps/scripts/security-audit-collect.ps1"

PASS=0
FAIL=0
ok()   { echo "PASS: $1"; PASS=$((PASS + 1)); }
bad()  { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

# has_lit <label> <literal-substring>  - fixed-string (grep -F) presence check.
has_lit() {
    if grep -Fq -- "$2" "$COLLECTOR"; then ok "$1"; else bad "$1 (missing: $2)"; fi
}
# lacks_lit <label> <literal-substring> - fixed-string absence check.
lacks_lit() {
    if grep -Fq -- "$2" "$COLLECTOR"; then bad "$1 (present but must not be: $2)"; else ok "$1"; fi
}

if [ ! -f "$COLLECTOR" ]; then
    echo "FAIL: collector not found at $COLLECTOR"
    echo "----"
    echo "PASS: 0  FAIL: 1"
    exit 1
fi

echo "==== STATIC contract checks (source of $COLLECTOR) ===="

# Fail-loud error handling: Stop present, Continue absent (the v1 silent-empty bug).
has_lit   "ErrorActionPreference = Stop"            "\$ErrorActionPreference = 'Stop'"
lacks_lit "no ErrorActionPreference = Continue"     "\$ErrorActionPreference = 'Continue'"

# Three-state Add-Section contract. Anchor to CODE tokens, not the docstring:
# the bare strings "!! COLLECTION ERROR" / "## ADMIN-REQUIRED" also appear in the
# .SYNOPSIS comment, so a grep for those would stay green even if the code were
# deleted. Match the interpolation/definition that appears ONLY in code.
has_lit   "Add-Section function defined"            "function Add-Section"
has_lit   "empty section -> (none), never blank"    "IsNullOrWhiteSpace"
has_lit   "lens failure emits COLLECTION ERROR"     "\"!! COLLECTION ERROR: \$("

# Byte-stable admin-gap marker: exception TYPE only, never the localized .Message.
has_lit   "Deny-Marker uses exception type name"    "\"## ADMIN-REQUIRED (\$(\$e.GetType().Name))"
# Regression guard: the Deny-Marker definition must not interpolate .Message
# (locale-variable text would break the byte-stable diff contract).
if grep -n 'function Deny-Marker' "$COLLECTOR" >/dev/null 2>&1; then
    deny_body=$(sed -n '/function Deny-Marker/,/^}/p' "$COLLECTOR")
    if printf '%s' "$deny_body" | grep -Fq '.Message'; then bad "Deny-Marker must not use .Message (breaks byte-stability)"; else ok "Deny-Marker has no .Message (byte-stable)"; fi
fi

# 4104 self-exclusion: regex built from fragments AND the own-path filter APPLIED
# (not just declared), so the collector never flags its own script.
has_lit   "4104 self-exclusion filter applied"      "-notmatch \$selfRx"
has_lit   "4104 regex built from fragments"         "'Download'+'String'"

# Probe-first event reads: the DENIED catch must be the typed catch, not a comment.
has_lit   "probe-first Read-Events helper"          "function Read-Events"
has_lit   "denied-log caught by type"               "catch [System.UnauthorizedAccessException]"
# Empty-vs-denied uses the locale-independent error id, not an English message.
has_lit   "empty-log detected locale-independently" "NoMatchingEventsFound"

# Path anchoring (no unanchored \Windows\ substring bypass) + IPv4 preservation.
has_lit   "masquerade path anchored to SystemRoot"  "env:SystemRoot"
has_lit   "Norm-Ver preserves IPv4 literals"        "ipv4"

# Testability: params that let this driver run read-only into a temp dir.
has_lit   "-StdOut param (no-write mode)"           "[switch]\$StdOut"
has_lit   "-BaseDir param"                          "\$BaseDir"
has_lit   "-Days param"                             "\$Days"

# Expected section set (a representative sample across every category group).
for marker in \
    "A1. RUN / RUNONCE" "A3. WINLOGON" "A4. IFEO" "A5. COM hijack" \
    "A6. LOGON-SCRIPT" "A7. UAC-BYPASS" \
    "B1. NON-MICROSOFT AUTO-START SERVICES" "B4. WMI EVENT-SUBSCRIPTION" \
    "C1. DEFENDER CORE STATUS" "C2. DEFENDER EXCLUSIONS" \
    "E2. TCP LISTENERS" "E4. PORTPROXY" \
    "F1. OUTBOUND TO PUBLIC IPs" "F2. HOSTS FILE" \
    "G2. SYSTEM-PROCESS MASQUERADE" "H2. PRIVILEGED GROUP MEMBERSHIP" \
    "I2. UAC" "J1. SERVICE INSTALLED" "K. SECURITY-LOG VISIBILITY GAP"
do
    has_lit "section present: $marker" "$marker"
done

# ---- LIVE tier: resolve a PowerShell, run read-only, assert the output -----
echo "==== LIVE run tier ===="
PS_EXE=""
if command -v powershell.exe >/dev/null 2>&1; then
    PS_EXE=$(command -v powershell.exe)
elif [ -x "/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe" ]; then
    PS_EXE="/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe"
fi

if [ -z "$PS_EXE" ]; then
    echo "SKIP: no powershell.exe resolvable - live collector run not executed (expected in Linux CI)."
else
    COLLECTOR_WIN=$(wslpath -w "$COLLECTOR" 2>/dev/null || echo "$COLLECTOR")
    # Write to a temp file (not a pipe) so $? is PowerShell's real exit, not tr's.
    TMP_OUT=$(mktemp)
    "$PS_EXE" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$COLLECTOR_WIN" -StdOut -Days 7 </dev/null >"$TMP_OUT" 2>/dev/null
    RC=$?
    OUT=$(tr -d '\r' <"$TMP_OUT")
    rm -f "$TMP_OUT"
    if [ "$RC" -eq 0 ]; then ok "collector exited 0"; else bad "collector exit code $RC"; fi

    if printf '%s\n' "$OUT" | grep -q '^AUDIT TIMESTAMP:'; then
        ok "output has AUDIT TIMESTAMP header"
    else
        bad "no AUDIT TIMESTAMP header"
    fi

    # No lens may have thrown.
    if printf '%s\n' "$OUT" | grep -q '!! COLLECTION ERROR'; then
        bad "output contains a COLLECTION ERROR:"
        printf '%s\n' "$OUT" | grep '!! COLLECTION ERROR' | sed 's/^/      /'
    else
        ok "output has no COLLECTION ERROR"
    fi

    # Every declared section must be present in the output and non-empty (the
    # header line is immediately followed by at least one non-blank content line).
    SECTIONS=$(printf '%s\n' "$OUT" | grep -c '^=== ')
    if [ "$SECTIONS" -ge 25 ]; then ok "output has >=25 sections ($SECTIONS)"; else bad "only $SECTIONS sections in output"; fi

    EMPTY=$(printf '%s\n' "$OUT" | awk '
        /^=== /   { if (prev_hdr) { empties++ }; prev_hdr=1; next }
        /^[^[:space:]]/ { prev_hdr=0 }
        /^[[:space:]]*[^[:space:]]/ { prev_hdr=0 }
        END { print empties+0 }')
    if [ "$EMPTY" -eq 0 ]; then ok "no section header is followed immediately by another header"; else bad "$EMPTY section(s) rendered empty"; fi

    # The collector must NOT self-flag its own script in the 4104 lens (the
    # self-detection loop). A clean run reports suspicious = 0.
    if printf '%s\n' "$OUT" | grep -q 'keyword-suspicious (self-excluded) = 0'; then
        ok "4104 lens does not self-flag (suspicious = 0)"
    else
        bad "4104 keyword-suspicious is non-zero on a clean self-run (self-exclusion may be broken)"
    fi

    # Fault-injection: prove the throw -> marker contract behaviorally, not just by
    # static grep. With -FaultInject the collector adds one throwing lens followed
    # by a sentinel lens; assert the marker appears, the sentinel STILL renders
    # after it (script did not abort), and the exit code is still 0.
    TMP_F=$(mktemp)
    "$PS_EXE" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$COLLECTOR_WIN" -StdOut -Days 7 -FaultInject </dev/null >"$TMP_F" 2>/dev/null
    FRC=$?
    FOUT=$(tr -d '\r' <"$TMP_F")
    rm -f "$TMP_F"
    if [ "$FRC" -eq 0 ]; then ok "fault-inject run still exits 0"; else bad "fault-inject run exit $FRC (a thrown lens aborted the script)"; fi
    if printf '%s\n' "$FOUT" | grep -Fq '!! COLLECTION ERROR: injected fault'; then ok "thrown lens renders a COLLECTION ERROR marker"; else bad "thrown lens did not produce a marker"; fi
    if printf '%s\n' "$FOUT" | grep -Fq 'sentinel-ok'; then ok "section AFTER the fault still renders (no abort)"; else bad "post-fault sentinel missing (script aborted at the throw)"; fi
fi

echo "----"
echo "PASS: $PASS  FAIL: $FAIL"
[ "$FAIL" -eq 0 ]
