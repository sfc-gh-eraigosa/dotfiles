# demo — real-terminal transcript (plan Task 4 Step 5)

- **Date:** 2026-09-05 23:22 PDT · **Binary:** `gff vdev` built from this branch · **Go:** `go1.26.1 linux/arm64`
- **Inventory:** the **live** dotfiles flags (78 rows across `fleet`, `gsl`, `install`, `keyboard`, `research-rubric`),
  not a fixture.
- **How:** [`run.sh`](./run.sh) builds the branch binary, starts `gff tui` in a tmux session (`gffdemo`, 140×40 —
  a real pty, foreground, never `&`), types the plan's keystrokes, and appends `tmux capture-pane -p` after each
  step to [`transcript.txt`](./transcript.txt). Re-run with `bash run.sh`.
- **Two deviations from the plan's Step 5 snippet, both deliberate:**
  1. The binary is built to `$TMPDIR/gff-demo` instead of `./sdk/gff/build.sh`, which installs over
     `~/opt/bin/gff`. A demo must not replace the user's installed binary with a pre-merge build.
  2. Characters are sent one per key event (`send-keys -l` + 60 ms). `tmux send-keys '/wispr'` delivers all six
     runes in a single bubbletea `KeyMsg` — the paste shape — which matches no binding. Real keyboards send one
     rune per event.
- **Flag hygiene:** the user override file read `{}` before and `{}` after; `gff get install.ai.teams` → `true`
  at the end (both recorded in the transcript header and footer). The `:set` was undone by the `:unset` step
  itself, so nothing needed restoring by hand.

| Step | Keys | Transcript proof | Feature |
| :-- | :-- | :-- | :-- |
| start | — | footer `j/k move  h/l page  / search  n/N match  : command  ? help  q quit  space toggle  enter open  u clear` | F9 footer from `gffKeys` |
| search + auto-expand | `/` `w` `i` `s` `p` `r` | prompt `/wispr▌`; cursor lands on `install.windows.wispr-flow` with its area expanded (`… 50 more above`) | F3a, F4a |
| commit | `Enter` | footer badge `/wispr [1/1]` | F3d |
| next match | `n` | stays on the single match (wrap onto itself) | F5a |
| chord | `g` `g` | cursor to the first row, badge shows `[-/1]` (cursor off a match) | F1c, F5f |
| park | `/ai.teams` `Enter` | cursor on `install.ai.teams … true default repo-live` | F4d |
| `:` typed | `:set install.ai.teams false` | prompt `:set install.ai.teams false▌` — the letters are text, no quit/unset fired | F6 routing |
| set | `Enter` | the same row now reads `install.ai.teams  false  override  user-override` | F6a |
| `:unset` typed | `:unset install.ai.teams` | prompt `:unset install.ai.teams▌` | F7 |
| unset | `Enter` | the row falls back to `true  default  repo-live` | F6c |
| help | `?` | overlay lists every binding from the same table (`/`, `n`, `N`, `esc`, `:`, `?/f1`, `q/ctrl+c`, `space`, `enter`, `u`) plus gff's `SOURCES` section | F2a, F9 |
| close | `Esc` | back to the list, badge intact | F2a |
| quit | `:q` `Enter` | `program exited, tmux session gone` | F6d |
