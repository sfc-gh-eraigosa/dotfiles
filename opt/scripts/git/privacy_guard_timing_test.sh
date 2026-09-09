#!/usr/bin/env bash
# Test driver for privacy_guard_timing.sh — the `make hook-timing` report over
# the privacy guard's timing log. exit 0 = all pass, 1 = a failure.
set -u
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/privacy_guard_timing.sh"
PASS=0; FAIL=0
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
LOG="$TMP/timing.log"

OUT=""
run() { local want="$1" label="$2" rc; shift 2; OUT="$(bash "$SCRIPT" "$@" 2>&1)"; rc=$?
    if [ "$rc" -eq "$want" ]; then echo "PASS: $label"; PASS=$((PASS+1)); else echo "FAIL: $label (want $want, got $rc) :: $OUT"; FAIL=$((FAIL+1)); fi; }
has() { if printf '%s' "$OUT" | grep -Eq -- "$2"; then echo "PASS: $1"; PASS=$((PASS+1)); else echo "FAIL: $1 :: $OUT"; FAIL=$((FAIL+1)); fi; }

run 0 "missing log => exit 0" --log "$LOG"
has "missing log: says no data" 'no timing data'

line() { printf '2026-09-05T15:00:0%sZ\t%s\t%s\t%s\tgitleaks=on\n' "$1" "$2" "$3" "$4" >> "$LOG"; }
line 1 pre-commit 120 800
line 2 commit-msg 40 60
line 3 agent:Bash 300 2000
line 4 pre-push 900 50000
line 5 pre-commit 110 700
run 0 "all within budget => exit 0" --log "$LOG"
has "summary: run count" 'runs=5'
has "summary: max" 'max=900ms'
has "summary: median" 'median=120ms'
has "summary: slow count zero" 'slow=0'
has "per-hook breakdown names pre-commit" 'pre-commit'

line 6 pre-push 2600 90000
run 1 "one run over the default budget => exit 1 (make hook-timing goes red)" --log "$LOG"
has "slow count reported" 'slow=1'
has "slow run identified by hook and time" 'pre-push.*2600ms'

run 0 "--budget-ms raises the bar => exit 0" --log "$LOG" --budget-ms 5000
run 1 "--budget-ms lowers the bar => more slow" --log "$LOG" --budget-ms 100
has "lower budget counts every run over 100ms" 'slow=5'

echo "----"; echo "privacy_guard_timing_test: $PASS passed, $FAIL failed"; [ "$FAIL" -eq 0 ]
