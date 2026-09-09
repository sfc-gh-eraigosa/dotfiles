# demo — real-terminal transcript (plan Task 7 Step 6)

- **Date:** 2026-09-05 20:54 PDT · **Go:** `go version go1.26.1 linux/arm64` · **bubbletea:** v1.3.10
- **How:** [`run.sh`](./run.sh) builds `sdk/libs/tui/example` (`-tags example`), starts it in a detached tmux
  session (`tuidemo`, 100×20 — a real pty, foreground, never `&`), types the plan's keystrokes, and appends
  `tmux capture-pane -p` after each step to [`transcript.txt`](./transcript.txt). Re-run with `bash run.sh`.
- **Typing shape:** characters are sent one per key event (`send-keys -l <char>` + 80 ms). The plan's
  `send-keys 'jj'` form delivers both bytes in a single bubbletea `KeyMsg` (`Runes: "jj"`) — the *paste*
  shape — which matches no binding, so the first run (its transcript was overwritten by the re-run; the
  observation is quoted in TRACKING §4) showed `jj`, `/25`, `:mark`, `:q` doing nothing while single keys
  worked — and `gg` "working" only because the two-rune batch literally matched the `"gg"` chord name. That is a property of the input
  shape, not of the lib; it is recorded as a follow-up for tools (TRACKING §4).

| Step | Keys | Transcript proof (`===== … =====` block) | Package(s) |
| :-- | :-- | :-- | :-- |
| start | — | `> row 01`, footer `j/k move  / search  n/N match  : command  ? help  q quit` | keymap.HeaderHint |
| motion | `j` `j` | `> row 03` | nav.Cursor |
| half page | `ctrl+d` | `> row 11` (body = 16 rows → stride 8) | nav.Half |
| chord | `g` `g` | `> row 01` — pending `g` state on the cursor, not a global | nav.Key |
| search | `/` `2` `5` | prompt line `/25▌`, cursor parked on `> row 25` while typing | search.State, prompt.Line |
| commit | `Enter` | badge `/25 [1/1]` in the footer | search.Commit/Badge |
| next | `n` | stays on `row 25` (the only match wraps onto itself) | search.Next |
| command + completion | `:` `mark` `space` `Tab` | prompt `:mark row 01▌` (first candidate) | cmdline.State.Complete |
| command run | `Enter` | status `marked row 01` | cmdline.Registry.Run |
| help | `?` | overlay lists every binding incl. the tool's `d  delete the row (asks first)`, closes with `any key closes` | overlay.Help |
| close help | `Esc` | list view back, status kept | mode routing |
| confirm | `d` | `delete row 25?` / `enter/y delete · esc/n cancel` | overlay.Confirm.Render |
| decline | `x` | list intact (`> row 25`), status `cancelled` | overlay.Confirm.Key |
| quit | `:` `q` `Enter` | `program exited, tmux session gone` | cmdline.Standard |

Observed, example-only: after a status line appears the title `example list` scrolls off the 20-row pane —
`model.go` budgets `Height-4` for the list but renders 5 chrome lines (title, blank, blank, footer, status).
Cosmetic, confined to the demo model (the plan's test pins the `-4` arithmetic); noted in TRACKING §4.
