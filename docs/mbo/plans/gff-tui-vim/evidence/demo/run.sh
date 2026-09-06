#!/usr/bin/env bash
# Real-terminal demo of the gff TUI (plan Task 4 Step 5), against the LIVE
# dotfiles flag inventory. Runs `gff tui` in a tmux pane — a real pty,
# foreground, never `&` — drives the plan's keystrokes, and appends a
# capture-pane snapshot after each step to transcript.txt beside this script.
#
# The binary is built to a temp path instead of ./build.sh: build.sh installs
# over ~/opt/bin/gff and the demo must not replace the user's installed binary
# with a pre-merge build. Re-run: bash run.sh
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
gffdir="$(cd "$here/../../../../../../sdk/gff" && pwd)"
out="$here/transcript.txt"
sess=gffdemo
bin="${TMPDIR:-/tmp}/gff-demo"

(cd "$gffdir" && go build -o "$bin" .)

# The demo drives :set/:unset against the operator's REAL override file so the
# inventory stays live. Back it up and restore it on any exit — a re-run must
# not cost someone a genuine override on the demo key.
cfg="${HOME}/.config/gff/config.yaml"
backup="$(mktemp -u "${TMPDIR:-/tmp}/gff-config-XXXXXX.yaml")"
restore() {
  if [ -f "$backup" ]; then
    cp -p "$backup" "$cfg" && rm -f "$backup"
    echo "restored $cfg from the pre-demo backup"
  fi
}
trap restore EXIT
[ -f "$cfg" ] && cp -p "$cfg" "$backup"

snap() { # $1 = step label
  sleep 0.8
  { printf '\n===== %s | %s =====\n' "$1" "$(date '+%Y-%m-%d %H:%M:%S %Z')"; tmux capture-pane -pt "$sess"; } | tee -a "$out"
}

# Type like a person: one character per key event. `tmux send-keys '/wispr'`
# hands bubbletea ONE KeyMsg carrying all six runes (the paste shape), which
# matches no binding — a real keyboard never does that.
type_chars() { local s="$1" i; for ((i = 0; i < ${#s}; i++)); do tmux send-keys -t "$sess" -l "${s:i:1}"; sleep 0.06; done; }
key() { tmux send-keys -t "$sess" "$1"; }

tmux kill-session -t "$sess" 2>/dev/null || true
: > "$out"
{
  printf '# gff TUI demo transcript — %s\n' "$(date '+%Y-%m-%d %H:%M %Z')"
  printf '# %s\n' "$("$bin" version 2>&1 | head -1)"
  printf '# %s\n' "$(go version)"
  printf '# user override file before: %s\n' "$(tr -d '\n' < "${HOME}/.config/gff/config.yaml" 2>/dev/null || echo '(absent)')"
} >> "$out"

tmux new-session -d -s "$sess" -x 140 -y 40 "$bin tui"
sleep 1.5
snap "start — live inventory, footer rendered from gffKeys"
type_chars '/wispr';                     snap "/wispr → incremental search; the area holding the hit auto-expands"
key 'Enter';                             snap "Enter → pattern committed, [i/N] badge in the footer"
type_chars 'n';                          snap "n → hop to the next match (wraps)"
type_chars 'gg';                         snap "gg → first row (chord)"
# Park the cursor on install.ai.teams so the :set/:unset steps below are
# visible in the capture rather than scrolled off.
type_chars '/ai.teams'; key 'Enter';     snap "/ai.teams Enter → cursor parked on the row the : commands will change"
type_chars ':set install.ai.teams false'; snap ":set … typed (letters are text, not normal-mode keys)"
key 'Enter';                             snap "Enter → override written; the row reads false / user-override"
type_chars ':unset install.ai.teams';    snap ":unset … typed"
key 'Enter';                             snap "Enter → override cleared; the row falls back to its resolved layer"
type_chars '?';                          snap "? → help overlay, keys rendered from the same table"
key 'Escape';                            snap "Esc → back to the list"
type_chars ':q'; key 'Enter'
sleep 0.8
if tmux has-session -t "$sess" 2>/dev/null; then
  printf '\n===== :q → session STILL ALIVE (unexpected) =====\n' | tee -a "$out"
  tmux kill-session -t "$sess"
  exit 1
fi
{
  printf '\n===== :q → program exited, tmux session gone | %s =====\n' "$(date '+%H:%M:%S')"
  printf '# user override file after: %s\n' "$(tr -d '\n' < "${HOME}/.config/gff/config.yaml" 2>/dev/null || echo '(absent)')"
  printf '# gff get install.ai.teams → %s\n' "$("$bin" get install.ai.teams 2>&1 | tail -1)"
} | tee -a "$out"
