#!/usr/bin/env bash
# statusline-command.sh — shim Claude Code calls for the gsl status line.
#
# Claude Code pipes a JSON payload on stdin after every assistant turn and
# prints whatever this script writes to stdout as the status line.
#
# Strategy:
#   1. If ~/opt/bin/gsl exists and is executable, exec it with 'render'.
#      The exec replaces this process, so stdin flows straight through.
#   2. Otherwise (binary missing / pre-build), consume stdin and emit a
#      minimal fallback line so the status bar never goes blank or errors out.

# Use pipefail but NOT -e: we must never exit non-zero in fallback paths.
set -uo pipefail

GSL_BIN="${HOME}/opt/bin/gsl"

if [[ -x "$GSL_BIN" ]]; then
    # exec replaces the current process; stdin from Claude passes straight through.
    exec "$GSL_BIN" render
fi

# --- Fallback: binary missing ---
# Drain stdin so the calling pipe doesn't block/SIGPIPE.
cat > /dev/null 2>&1 || true

# Build a minimal status line without external deps.
_dir="$(basename "$PWD" 2>/dev/null || echo '?')"
_branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
_date="$(date '+%H:%M' 2>/dev/null || true)"

_line="$_dir"
[[ -n "$_branch" ]] && _line="$_line  $_branch"
[[ -n "$_date"   ]] && _line="$_line  $_date"

printf '%s\n' "$_line"
