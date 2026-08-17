# fleet tui v2 — interactive fleet dashboard — implementation plan

- **Slug:** fleet-tui
- **Date:** 2026-08-16
- **Status:** Draft
- **Relates to:** spec `../specs/fleet-tui.md` · design `../designs/fleet-tui.md` · issue [#226](https://github.com/sfc-gh-eraigosa/dotfiles/issues/226) · design PR [#227](https://github.com/sfc-gh-eraigosa/dotfiles/pull/227)

## 1. Summary & verdict

Upgrade `fleet tui` from the v1 skeleton to a streaming, vim-navigable,
searchable, multi-select, colorful dashboard — spec F1–F12 — without touching
any headless command's semantics. Pure-model/pure-view discipline throughout so
every visual state is a golden frame and every keystroke a table-driven test.
14 tasks; 13 automatable, 1 human-gated live capture.

**Standing rules (from the fleet build — apply to every task):**

- TDD: RED (write the failing test) → RUN-RED (observe it fail) → GREEN →
  RUN-GREEN → VERIFY (vet + full suite). Never write a result you didn't see.
- Evidence: every done-when gate's real output `tee`'d to
  `docs/mbo/plans/fleet-tui/evidence/taskNN/`, committed with the task.
- Privacy: placeholders only (`<user>`, `<host>`, `host-pi`…) in anything
  committed; sanitize captures before `git add`.
- Preflight each session: `git log --oneline origin/main..HEAD` — foreign
  commits mean the worktree is on a stale local main (bit us 3×; reset to
  `origin/main`).
- Update `TRACKING.md` after every task; `TODO.md` is the resumable cursor.

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `sdk/fleet/cmd/tui.go` | cobra cmd, flags (`--ref` baseline, `--update-ref` target), program wiring only | F9, UC5 |
| `sdk/fleet/cmd/tui_model.go` | model struct, modes, cursor/viewport/selection/search/queue state + `Update()` routing | F1,F2,F4–F8 |
| `sdk/fleet/cmd/tui_view.go` | `View()` + `theme` struct (all lipgloss styles live here) | F3, F11, F12 |
| `sdk/fleet/cmd/tui_keys.go` | keymap incl. `gg` pending-key state, mode routing table | F4–F7, F10, F11 |
| `sdk/fleet/cmd/tui_cmds.go` | `collectOne` tea.Cmd, batch-update queue sequencing, ssh handoff builders | F1, F8, F10 |
| `sdk/fleet/cmd/tui_model_test.go` · `tui_view_test.go` · `tui_keys_test.go` · `tui_cmds_test.go` | layers 1–3 of spec §6 | all |
| `sdk/fleet/cmd/status.go` | extract `collectOne(host)` from `collect()` (collect keeps behavior; both share it) | F1 |
| `sdk/fleet/README.md` + `AGENTS.md` | TUI section rewrite: keys table, search, selection, batch update | docs |
| `docs/mbo/plans/fleet-tui/evidence/taskNN/` | per-task captured gates | §7 |
| `go.mod` | `bubbles` (spinner) → direct dependency | F1 |

Touch-points outside `sdk/fleet`: **none** (no install.sh/gff/CI changes — the
binary and build path are unchanged).

## 3. Interface contracts

```go
// tui_model.go — the whole dashboard is one value.
type tuiModel struct {
    rows     []Row               // resolved hosts (worst-first)
    pending  map[string]bool     // alias → still polling
    cursor   string              // ALIAS, not index (survives re-sort)
    vp       viewport            // top index, height, width (from WindowSizeMsg)
    mode     tuiMode             // modeNormal | modeSearch | modeConfirm | modeHelp
    search   searchState         // input, compiled *regexp.Regexp, err, committed
    selected map[string]bool     // alias set
    vAnchor  *string             // visual-mode anchor alias (nil = not in visual)
    queue    []string            // batch-update aliases, head = in flight
    results  map[string]string   // alias → "ok"/"FAIL: …" from the last batch
    updateRef string             // from --update-ref (validRef-guarded)
    spin     spinner.Model
    now      time.Time           // injected — never time.Now() in view code
    status   string
}

// tui_cmds.go
func collectOne(h sshconf.Host, r runner.Runner, base Baseliner, now time.Time) tea.Cmd
    // → hostRowMsg{Row}
type hostRowMsg struct{ row Row }
type execDoneMsg struct{ alias string; err error }   // update or ssh returned

// Key routing: exactly one function owns it.
func route(m tuiModel, k tea.KeyMsg) (tuiModel, tea.Cmd)
```

Search semantics (F5): pattern compiled per keystroke; smartcase = wrap in
`(?i)` iff pattern has no uppercase; match target is the **rendered row text**
(alias + commit + age + status) so `/behind` works. Invalid regex: keep last
good matches, render `err` inline, never crash.

Viewport invariant (F4): `vp.top ≤ indexOf(cursor) < vp.top+vp.height` after
every transition — one `clampViewport()` called at the end of `route`.

Batch contract (F8): confirm builds `queue` from `selected` (table order,
fallback cursor); each `execDoneMsg` records ok/fail, fires `collectOne` for
that alias, pops the queue, fires the next handoff. Errors advance, never wedge.

## 4. TDD build order

Each task: **tests → observe RED → implement → observe GREEN → vet + full
suite → evidence → commit → TRACKING/TODO.**

**Task 1 — mechanical split, zero behavior change.**
Move model/view/keys code from `tui.go` into the four new files; convert
`cursor int` → `cursor string` (alias) with a shim keeping v1 semantics.
RED: existing TUI tests must still pass unchanged *plus* new
`TestCursorIsAliasKeyed` (re-sort keeps cursor on its host).
Done-when: `go test ./cmd/` green, `go vet` clean, `tui.go` < 60 lines.
Evidence: `task01/split.txt`.

**Task 2 — theme + styled frames (F3).**
`theme` struct; status→style map; header/cursor/status-bar styles; tests pin
`lipgloss.SetColorProfile(termenv.Ascii)` in `TestMain`.
RED: golden frames for populated + empty states.
Done-when: goldens match; a `termenv.TrueColor` smoke shows ANSI present.
Evidence: `task02/frames.txt` (both profiles).

**Task 3 — viewport + resize (F4c,d).**
`viewport` struct, `clampViewport`, `WindowSizeMsg` handling, `n/total`
position indicator, header pinned.
RED: paging math table (tiny/huge terminals, cursor at both extremes).
Done-when: cursor always visible in every table case; goldens updated.
Evidence: `task03/paging.txt`.

**Task 4 — vim motions (F4a,b).**
`j/k`/arrows clamp; `gg` via pending-key state (any other key cancels); `G`;
`ctrl+d/u` half-page; `ctrl+f/b` full page.
RED: sequence tests incl. `g` then `j` (cancel) and `gg` at top (no-op).
Done-when: all motion rules green.
Evidence: `task04/motion.txt`.

**Task 5 — streaming collect + spinner (F1).**
Extract `collectOne` in `status.go` (collect() now maps over it — headless
behavior identical, existing status tests must stay green untouched);
`Init()` fires n cmds + spinner tick; `hostRowMsg` handling re-sorts and
clears pending; spinner cell on pending rows.
RED: `TestInitFiresOneCmdPerHost`, `TestRowArrivalResortsButKeepsCursor`,
`TestUnreachableResolvesToRow`, headless `TestCollectUnchanged`.
Done-when: all green; frame with mixed pending/resolved matches golden.
Evidence: `task05/streaming.txt`.

**Task 6 — refresh (F2).**
`r` → all rows pending, refire `collectOne` per host; selection + cursor kept.
RED: `TestRefreshRepollsAllKeepingSelection`.
Done-when: green; the v1 "advertised but dead key" is a wired feature.
Evidence: `task06/refresh.txt`.

**Task 7 — search mode (F5).**
`/` enters modeSearch; incremental compile; smartcase; highlight in view;
`enter` commits, `esc` cancels; invalid-regex inline error.
RED: mode-routing (a `j` typed in search is text, not motion), smartcase
table, invalid `[` keeps editing, esc-clears test, highlight golden.
Done-when: all green.
Evidence: `task07/search.txt`.

**Task 8 — match navigation (F6).**
`n`/`N` wrap both directions; "no matches" status; interacts with viewport
(jump scrolls).
RED: wraparound table both directions + no-match no-op.
Done-when: green.
Evidence: `task08/nav.txt`.

**Task 9 — selection + visual mode (F7).**
`space` toggle (alias-keyed); `v` anchor; motions extend live; `space`
commits range / `esc` cancels; selection count in status bar; survives
refresh + re-sort.
RED: toggle/re-sort, visual range extend+commit+cancel, refresh-survival.
Done-when: green; selection golden frame.
Evidence: `task09/select.txt`.

**Task 10 — batch update + --update-ref (F8, F9).**
`u`: targets = selection (table order) else cursor; modeConfirm strip lists
targets + ref; `y` builds queue and fires first `ExecProcess` handoff
(reusing `remoteUpdateScript(updateRef)`); `execDoneMsg` records result,
re-polls that host, advances; `n`/`esc` = nothing. `--update-ref` on tuiCmd,
`validRef` at RunE start.
RED: target-list rules, declined-changes-nothing, queue-advances-past-failure
(erroring fake), injection ref rejected before program start, ref reaches the
script (recordingRunner).
Done-when: all green; confirm-strip golden.
Evidence: `task10/batch.txt`.

**Task 11 — ssh action (F10).**
`s` → `ssh <cursor alias>` ExecProcess; `execDoneMsg` restores UI, re-polls
that host (an ssh visit often changes nothing, but the re-poll is free and
keeps the invariant "handoff returns ⇒ row refreshed").
RED: cmd-construction (argv exactly `ssh <alias>`), empty-fleet no-op.
Done-when: green.
Evidence: `task11/ssh.txt`.

**Task 12 — help overlay + polish (F11, F12).**
`?` overlay from the single keymap table (no second hand-written list to
drift); any key closes; empty-fleet guidance kept; quit guard while a batch
queue is non-empty ("update in progress — q again to force").
RED: overlay golden, overlay-swallows-keys, double-q guard, empty-fleet keys.
Done-when: green.
Evidence: `task12/help.txt`.

**Task 13 — docs + gate.**
README TUI section (keys table, search, selection, batch, `--update-ref`),
AGENTS.md command row + invariants; `scripts/test.sh` full run.
Done-when: gate green ≥ 60% (fleet), cmd ≥ 55%; `go vet` clean; no identity
leak in diff (`grep` sweep).
Evidence: `task13/gate.txt`.

**Task 14 — HUMAN STOP: live capture.**
Real fleet: streaming arrival, `/` search, select 2, batch update with a
visible sudo prompt (proves terminal handoff), a FAIL row advancing the
queue, `s` ssh in/out, post-update fresh stamp. `tmux capture-pane`
sequence, sanitized, committed to `evidence/task14/`.
Done-when: capture shows every F-feature live; TRACKING row cites it.

## 5. Verification mapping

Every spec §5 rule maps to the named test written in its task: F1a–c → Task 5,
F2a → 6, F3a → 2, F4a–d → 3–4, F5a–c → 7, F6a → 8, F7a–b → 9, F8a–c + F9a
→ 10, F10a → 11, F11a + F12a → 12. The mapping is restated per-row in
TRACKING.md as tasks complete.

## 6. Integration & rollout

No wiring changes: same binary, same build.sh, same coverage floor entry.
Rollout = merge; rollback = revert (spec §9). Manual acceptance = Task 14.

## 7. Validation & evidence (show the work)

Evidence tree `docs/mbo/plans/fleet-tui/evidence/task01..14/`, append-only,
dated headers, committed with each task (privacy-sanitized). Coverage bars in
§4 Task 13. Adversarial scenarios covered: invalid regex (F5b), erroring host
mid-batch (F8c), injection ref (F9a), resize-under-cursor (F4d), empty fleet
(F12a), quit-during-batch (Task 12). A feature without captured evidence is
not done.

### 6.1 Build leaves / DAG

Default: single worker (`fleet-tui/<user>/build`), sequential Tasks 1–14 —
the tasks chain through one model struct, so parallel leaves would fight over
the same files. No CAP-B breakout.
