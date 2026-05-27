#!/usr/bin/env bash
#
# keep-awake.sh — Keep macOS awake (system stays on) during long/overnight runs
# while letting the DISPLAY sleep normally. Uses `caffeinate`, which is
# process-based: the assertion is held by a background process and released when
# that process exits, the timeout elapses, or you run revert-keepawake.sh.
#
# Unlike the Windows version there is no persistent power setting to restore —
# "reverting" simply means stopping the caffeinate process.
#
set -euo pipefail

PIDFILE="${TMPDIR:-/tmp}/keep-awake.pid"
SLEEP_DISPLAY=0
DURATION_SEC=0      # 0 = stay awake until stopped
WAIT_PID=0

usage() {
    cat <<'EOF'
Usage: keep-awake.sh [options]

  --sleep-display     Put the display(s) to sleep immediately (pmset displaysleepnow).
  --until HH:MM       Auto-release at this local time (today if still ahead, else tomorrow).
  --minutes N         Auto-release after N minutes.
  --wait-pid PID      Stay awake only while process PID is alive (e.g. your loop).
  -h, --help          Show this help.

With no timing option, keep-awake stays active until you run revert-keepawake.sh.
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --sleep-display) SLEEP_DISPLAY=1 ;;
        --until)
            shift; [ $# -gt 0 ] || { echo "--until needs HH:MM" >&2; exit 1; }
            target=$(date -j -f "%H:%M" "$1" +%s 2>/dev/null) || { echo "bad time: $1" >&2; exit 1; }
            now=$(date +%s)
            [ "$target" -le "$now" ] && target=$((target + 86400))
            DURATION_SEC=$((target - now))
            ;;
        --minutes)
            shift; [ $# -gt 0 ] || { echo "--minutes needs N" >&2; exit 1; }
            DURATION_SEC=$(( $1 * 60 ))
            ;;
        --wait-pid)
            shift; [ $# -gt 0 ] || { echo "--wait-pid needs PID" >&2; exit 1; }
            WAIT_PID="$1"
            ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown option: $1" >&2; usage; exit 1 ;;
    esac
    shift
done

# Already running?
if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE" 2>/dev/null)" 2>/dev/null; then
    echo "Keep-awake already active (pid $(cat "$PIDFILE")). Run revert-keepawake.sh to stop."
    exit 0
fi

# -i prevents idle system sleep; -s prevents system sleep on AC. We deliberately
# omit -d so the display can still sleep on its own schedule.
args=(-i -s)
[ "$WAIT_PID" -gt 0 ]     && args+=(-w "$WAIT_PID")
[ "$DURATION_SEC" -gt 0 ] && args+=(-t "$DURATION_SEC")

caffeinate "${args[@]}" &
caf_pid=$!
echo "$caf_pid" > "$PIDFILE"

# Report the active state, mirroring the Windows script.
disp_min=$(pmset -g custom 2>/dev/null | awk '/[^a-z]displaysleep/{print $2; exit}')
[ -z "${disp_min:-}" ] && disp_min=$(pmset -g 2>/dev/null | awk '/displaysleep/{print $2; exit}')
if [ -n "${disp_min:-}" ] && [ "${disp_min}" != "0" ]; then
    disp_text="display still sleeps after ${disp_min} min"
else
    disp_text="display sleep unchanged"
fi
if [ "$DURATION_SEC" -gt 0 ]; then
    release="auto-releases in $((DURATION_SEC / 60)) min"
elif [ "$WAIT_PID" -gt 0 ]; then
    release="releases when pid $WAIT_PID exits"
else
    release="stays until revert-keepawake.sh"
fi
echo "Keep-awake ON: system sleep prevented; ${disp_text} (caffeinate pid ${caf_pid}; ${release})."

if [ "$SLEEP_DISPLAY" -eq 1 ]; then
    pmset displaysleepnow
    echo "Display put to sleep."
fi
