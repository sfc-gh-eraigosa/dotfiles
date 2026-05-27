#!/usr/bin/env bash
#
# revert-keepawake.sh — Stop a macOS keep-awake session started by keep-awake.sh.
# Releases the caffeinate assertion; the system returns to its normal sleep
# behavior immediately (there is no persistent setting to restore on macOS).
#
set -euo pipefail

PIDFILE="${TMPDIR:-/tmp}/keep-awake.pid"

if [ -f "$PIDFILE" ]; then
    pid="$(cat "$PIDFILE" 2>/dev/null || true)"
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
        kill "$pid"
        echo "Keep-awake stopped (pid $pid). Normal sleep behavior restored."
    else
        echo "No active keep-awake process (stale pidfile cleared)."
    fi
    rm -f "$PIDFILE"
else
    # Fallback: stop any caffeinate we may have started without a pidfile.
    if pkill -f 'caffeinate -i -s' 2>/dev/null; then
        echo "Stopped caffeinate. Normal sleep behavior restored."
    else
        echo "Keep-awake is not active."
    fi
fi
