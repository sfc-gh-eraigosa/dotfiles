#!/usr/bin/env bash
# privacy_guard_timing.sh — `make hook-timing`: how much time is the privacy
# guard costing? Summarises the timing log every judged call appends to
# (ai/hooks/privacy_rules.sh: privacy_timing), overall and per hook, and lists
# every run over budget. Exit 1 when any run is over budget so the target goes
# red and we investigate — security must not cost time silently.
#
# Usage: privacy_guard_timing.sh [--log FILE] [--budget-ms N]
#   --log        default $PRIVACY_GUARD_TIMING_LOG or ~/.local/state/privacy_guard/timing.log
#   --budget-ms  default $PRIVACY_GUARD_BUDGET_MS or 1500 (the hooks' own default)
# Log line: <utc-iso>\t<hook>\t<ms>\t<bytes judged>\tgitleaks=<on|off>
set -u

LOG="${PRIVACY_GUARD_TIMING_LOG:-${XDG_STATE_HOME:-$HOME/.local/state}/privacy_guard/timing.log}"
BUDGET="${PRIVACY_GUARD_BUDGET_MS:-1500}"
while [ $# -gt 0 ]; do
    case "$1" in
        --log)       shift; LOG="${1:-}" ;;
        --budget-ms) shift; BUDGET="${1:-}" ;;
        -h|--help)   sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *) echo "unknown argument: $1" >&2; exit 2 ;;
    esac
    shift
done

if [ ! -s "$LOG" ]; then
    echo "privacy_guard timing: no timing data yet (log: $LOG)"
    exit 0
fi

echo "privacy_guard timing — $LOG (budget ${BUDGET}ms)"
# Overall + per-hook stats in one awk pass; sorted values via asort-free
# approach (portable awk has no asort): collect, then sort with sort -n.
stats() { # stdin: ms values, one per line -> "runs median p95 max"
    sort -n | awk '{ v[NR]=$1 } END {
        if (NR == 0) { print "0 0 0 0"; exit }
        m = (NR % 2) ? v[(NR+1)/2] : int((v[NR/2] + v[NR/2+1]) / 2)
        p = int(0.95 * NR + 0.5); if (p < 1) p = 1; if (p > NR) p = NR
        print NR, m, v[p], v[NR] }'
}
read -r RUNS MED P95 MAX <<EOT
$(cut -f3 "$LOG" | grep -E '^[0-9]+$' | stats)
EOT
SLOW="$(awk -F'\t' -v b="$BUDGET" '$3 ~ /^[0-9]+$/ && $3+0 > b+0 { n++ } END { print n+0 }' "$LOG")"
echo "runs=$RUNS median=${MED}ms p95=${P95}ms max=${MAX}ms slow=$SLOW budget=${BUDGET}ms"

echo "per hook:"
for h in $(cut -f2 "$LOG" | sort -u); do
    read -r r m p x <<EOT
$(awk -F'\t' -v h="$h" '$2 == h { print $3 }' "$LOG" | stats)
EOT
    s="$(awk -F'\t' -v h="$h" -v b="$BUDGET" '$2 == h && $3+0 > b+0 { n++ } END { print n+0 }' "$LOG")"
    printf '  %-16s runs=%s median=%sms p95=%sms max=%sms slow=%s\n' "$h" "$r" "$m" "$p" "$x" "$s"
done

if [ "$SLOW" -gt 0 ]; then
    echo "slow runs (over ${BUDGET}ms):"
    awk -F'\t' -v b="$BUDGET" '$3 ~ /^[0-9]+$/ && $3+0 > b+0 { printf "  %s  %s  %sms  %s bytes  %s\n", $1, $2, $3, $4, $5 }' "$LOG"
    echo "Investigate: which hook, how many bytes, gitleaks on/off. Raise the bar with PRIVACY_GUARD_BUDGET_MS only after understanding why."
    exit 1
fi
exit 0
