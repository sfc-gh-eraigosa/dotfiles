# fleet tui v2 — interactive fleet dashboard — design

- **Slug:** fleet-tui
- **Date:** 2026-08-16
- **Status:** Proposed
- **Relates to:** builds on `fleet` (design `./fleet.md`, merged PR #224); issue [#226](https://github.com/sfc-gh-eraigosa/dotfiles/issues/226) / design PR [#227](https://github.com/sfc-gh-eraigosa/dotfiles/pull/227)
- **Author(s):** operator + assistant

## 1. Problem / context

`fleet tui` (merged in #224, `sdk/fleet/cmd/tui.go`, ~90 lines) proved the
interaction model but is a v1 skeleton. Verified against the merged code:

- **The initial collect blocks the first frame.** `collect()` SSHes every host
  *synchronously before* `tea.NewProgram(...).Run()` — on a fleet with one slow
  or unreachable host the operator stares at a frozen terminal with zero
  feedback for the full SSH timeout.
- **`r: refresh` is advertised in the header but not wired.** There is no
  `case "r"` in `Update()`; the only "refresh" is a post-update status line
  telling the operator to quit and re-run `fleet status`.
- **The table is plain `fmt.Fprintf`** fixed-width text. `lipgloss v1.1.0` and
  the bubbles ecosystem are already in the module's dependency tree — nothing
  uses them. No color, no styled cursor, no status differentiation.
- **No paging.** Rows render unconditionally; a fleet taller than the terminal
  scrolls the header away and the cursor can walk off-screen.
- **No search, no multi-select.** One cursor, `u` acts on exactly one host.
- **The TUI's update targets `main` only** (its `--ref` flag is the drift
  *baseline*, a different axis), and after an update the row goes stale.
- **No way to just SSH somewhere** — the natural "I'm looking at this host,
  drop me on it" action is missing.

The operator asked for: real keyboard navigation, paging, `/` regex search,
vim-like selection, and a colorful look.

## 2. Goals & non-goals

**Goals**

1. **Streaming async collect** — the TUI opens instantly; rows appear as each
   host answers, with a spinner on hosts still being polled. Unreachable hosts
   resolve to a red row, never a hang.
2. **Working refresh** — `r` re-polls all hosts in place, preserving cursor and
   selection.
3. **Colorful, legible table** — lipgloss theme keyed by status class
   (up-to-date green · behind yellow · divergent magenta · unknown dim ·
   unreachable red), styled header, cursor row, selection marks, search
   highlights. Degrades cleanly on dumb/NO_COLOR terminals.
4. **Vim navigation** — `j/k` + arrows, `gg`/`G`, `ctrl+d`/`ctrl+u` half-page,
   `ctrl+f`/`ctrl+b` full page, with paging driven by the real terminal height
   (`WindowSizeMsg`) and a `n/total` position indicator.
5. **`/` regex search** — incremental compile as you type, vim smartcase
   (case-insensitive unless the pattern has an uppercase letter), match
   highlighting, `n`/`N` next/prev, `esc` clears. An invalid regex is shown as
   an error, never a crash.
6. **Vim-like selection** — `space` toggles the cursor row, `v` starts a visual
   range extended by motions, `esc` clears. Selection is keyed by **host
   alias**, not row index, so it survives re-sorts and refreshes.
7. **Batch update** — `u` acts on the selection when non-empty (else the cursor
   row): shows the target list, confirms, then runs the existing interactive
   update per host sequentially (terminal handed over each time), re-polling
   each row afterward. A `--update-ref` flag (validRef-guarded, default `main`)
   brings the headless `update --ref` capability to the TUI.
8. **SSH action** — `s` (or `enter`) hands the terminal to a plain
   `ssh <host>`; the TUI resumes on exit.
9. **Help overlay** — `?` toggles a key reference.

**Non-goals**

- No mouse support, no configurable keymaps, no config file.
- No daemon/watch mode — fleet stays run-and-exit (spec F-series invariant).
- No change to `status`/`update`/`keys` semantics — the TUI is a *view* over
  the same collect/update seams.
- No bubbles/table adoption (see §3).

## 3. Options considered

**A. Adopt `bubbles/table` + `bubbles/textinput` wholesale.** Fastest to a
pretty table, but the component owns cursor/viewport state internally, which
fights alias-keyed selection, custom per-row styling (status colors + search
highlight + selection mark + spinner cell), and golden-frame testing of the
whole view. We'd wrap most of it anyway.

**B. Custom pure model/view on bubbletea + lipgloss + bubbles/spinner
(recommended).** Keep fleet's discipline — the entire decision surface is pure
data-in/string-out: `model` is a plain struct, `View()` is a pure function,
every keystroke is a value through `Update()`. lipgloss provides the color
system (with automatic terminal-capability degradation), `bubbles/spinner` the
pending glyph. Everything is unit-testable with the color profile pinned to
ASCII; no terminal needed in CI. This is exactly how the v1 TUI and the rest of
fleet are tested today.

**C. Full charm stack (bubbles/table + huh + glamour).** Wrong tools — huh is
forms, glamour is markdown; neither fits a live dashboard.

**Decision: B.**

## 4. Decision

One package (`sdk/fleet/cmd`), split by concern so no file owns two jobs:

| Unit | Responsibility |
| :-- | :-- |
| `tui.go` | cobra command, flags (`--ref` baseline, `--update-ref` target), program wiring |
| `tui_model.go` | the state machine: modes (normal · search · confirm · help), cursor/viewport, selection set, search state, pending polls |
| `tui_view.go` | pure rendering + the lipgloss theme; every style in one `theme` struct |
| `tui_keys.go` | keymap incl. the `gg` pending-key state and mode routing |
| `tui_cmds.go` | `tea.Cmd` producers: per-host `collectOne`, batch-update sequencing, ssh handoff |

Key design points:

- **Row streaming:** `Init()` returns one `collectOne(host)` cmd per host plus
  the spinner tick; each completion delivers a `hostRowMsg{Row}`; the model
  re-sorts worst-first on arrival. The cursor tracks the **alias** it was on,
  so re-sorting never teleports it.
- **Modes are explicit.** `mode` field with normal/search/confirm/help; each
  `tea.KeyMsg` is routed by mode. No hidden flag combinations.
- **Selection is a `map[alias]bool`** + an optional visual anchor. Batch
  actions snapshot it into an ordered list at confirm time.
- **The update/ssh handoff reuses `tea.ExecProcess`** exactly as v1 does (the
  sudo-prompt lesson), one host at a time; between hosts the model advances a
  queue and re-fires `collectOne` for the host just updated.
- **The theme is one struct** consumed only by `tui_view.go`; tests pin
  `lipgloss.SetColorProfile(termenv.Ascii)` so frames are byte-stable.

## 5. Risks & blast radius

- **Blast radius is `sdk/fleet/cmd` only** — no other tool imports it; the
  headless `status`/`update` paths share seams but keep their behavior
  (guarded by their existing tests).
- **bubbles/spinner becomes a direct dependency** (it's already transitively
  present). License check runs in CI as for every charm dep.
- **Interactive handoff mid-batch**: a failed `ssh -t` in a queue must not
  wedge the TUI — every ExecProcess completion (error or not) re-enters
  `Update()` and advances the queue; the error is rendered on the row.
- **Terminal capability variance**: lipgloss degrades automatically; tests pin
  ASCII so CI never depends on a TTY.

## 6. Rollback

Single revert of the feature PR restores the v1 TUI; no data, config, or remote
host state is touched by rendering changes. Batch update composes the existing
`remoteUpdateScript` — rolling back the TUI does not affect hosts.

## 7. Evidence expectations

The plan must capture, per task, under `plans/fleet-tui/evidence/`:

- **Golden frames** — `View()` output (ASCII profile) for: loading (spinners),
  populated worst-first, search active with highlights + invalid regex, visual
  selection, confirm strip, help overlay, empty fleet.
- **Model-transition test runs** — the `go test` output for every key/mode
  rule in spec §5.
- **A live capture** (human-gated): `tmux capture-pane` sequence of the real
  TUI against the real fleet — streaming arrival, search, select-two,
  batch-update handoff (sudo prompt visible = terminal genuinely released),
  ssh action, and the post-update re-poll showing a fresh stamp. This is the
  one thing CI cannot fake.
- **Coverage** — `scripts/test.sh` gate output showing fleet ≥ 60% (expect
  cmd% to *rise*; the pure view/model split exists to make that cheap).

> Produced via `superpowers:brainstorming`. Register in `../index.md`; spec at
> `../specs/fleet-tui.md`.
