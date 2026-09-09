#!/usr/bin/env bash
# Real-terminal demo of sdk/libs/tui via the composition example (plan Task 7 Step 6).
# Runs the example in a tmux pane (a real pty, foreground — never `&`), drives the
# plan's keystrokes, and appends a capture-pane snapshot after each step to
# transcript.txt next to this script. Re-run: bash run.sh
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
libs="$(cd "$here/../../../../../../sdk/libs" && pwd)"
out="$here/transcript.txt"
sess=tuidemo
bin="${TMPDIR:-/tmp}/tui-example"

(cd "$libs" && go build -tags example -o "$bin" ./tui/example)

snap() { # $1 = step label
  sleep 0.6
  { printf '\n===== %s | %s =====\n' "$1" "$(date '+%Y-%m-%d %H:%M:%S %Z')"; tmux capture-pane -pt "$sess"; } | tee -a "$out"
}

# Type like a person: one character per key event. `tmux send-keys 'jj'` hands
# bubbletea both bytes in ONE KeyMsg (Runes "jj") — the paste shape — which no
# binding matches; a human never produces that for normal-mode keys.
type_chars() { local s="$1" i; for ((i = 0; i < ${#s}; i++)); do tmux send-keys -t "$sess" -l "${s:i:1}"; sleep 0.08; done; }
key() { tmux send-keys -t "$sess" "$1"; }

tmux kill-session -t "$sess" 2>/dev/null || true
: > "$out"
printf '# sdk-tui demo transcript — %s — %s\n' "$(date '+%Y-%m-%d %H:%M %Z')" "$(go version)" >> "$out"
tmux new-session -d -s "$sess" -x 100 -y 20 "$bin"
sleep 1.5
snap "start (30 rows, cursor on row 01)"
type_chars 'jj';            snap "jj → cursor on row 03"
key 'C-d';                  snap "ctrl+d → half page down"
type_chars 'gg';            snap "gg → back to row 01 (chord)"
type_chars '/25';           snap "/25 → incremental search, prompt open, cursor parked on the match"
key 'Enter';                snap "Enter → committed; badge /25 [1/1]"
type_chars 'n';             snap "n → wraps onto the only match"
type_chars ':mark '; key 'Tab'; snap ":mark <Tab> → completes the first row name"
key 'Enter';                snap "Enter → :mark ran (status line)"
type_chars '?';             snap "? → help overlay rendered from the keymap"
key 'Escape';               snap "Esc → help closed"
type_chars 'd';             snap "d → confirm dialog"
type_chars 'x';             snap "x → declines (row kept, status cancelled)"
type_chars ':q'; key 'Enter'
sleep 0.6
if tmux has-session -t "$sess" 2>/dev/null; then
  printf '\n===== :q → session STILL ALIVE (unexpected) =====\n' | tee -a "$out"
  tmux kill-session -t "$sess"
  exit 1
fi
printf '\n===== :q → program exited, tmux session gone | %s =====\n' "$(date '+%H:%M:%S')" | tee -a "$out"
