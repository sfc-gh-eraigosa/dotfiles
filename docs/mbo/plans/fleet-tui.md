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
Updates run **concurrently in the background** (bounded `--jobs`), with the
interactive terminal-handoff kept only as the fallback for prompt-needing
hosts — the TUI never blocks during a batch. 15 tasks; 14 automatable, 1
human-gated live capture.

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
    // update engine — background-first (design §4). A host is in exactly ONE
    // of pending / updating / resolved; refresh excludes `updating`.
    updating map[string]updState  // alias → {phase: queued|precheck|running|ok|fail; log string}
    bgQueue  []string             // background wave; feeds free job slots
    iaQueue  []string             // interactive fallback (precheck failed); serial, after the wave
    jobs     int                  // --jobs: max concurrent background updates (default 4)
    updateRef string              // from --update-ref (validRef-guarded)
    spin     spinner.Model
    now      time.Time            // injected — never time.Now() in view code
    status   string
}

// tui_cmds.go
func collectOne(h sshconf.Host, r runner.Runner, base Baseliner, now time.Time) tea.Cmd
    // → hostRowMsg{Row}
type hostRowMsg struct{ row Row }
func precheckSudo(alias string, r runner.Runner) tea.Cmd     // ssh <alias> sudo -n true
type precheckMsg struct{ alias string; interactive bool }
func bgUpdate(alias, ref string, r runner.Runner) tea.Cmd
    // runner.Run with BatchMode=yes wrapping remoteUpdateScript(ref);
    // NEVER tea.ExecProcess — must not suspend the TUI.
type bgUpdateDoneMsg struct{ alias, log string; err error }
type execDoneMsg struct{ alias string; err error }           // ia-handoff or ssh returned

// Key routing: exactly one function owns it.
func route(m tuiModel, k tea.KeyMsg) (tuiModel, tea.Cmd)
```

Search semantics (F5): pattern compiled per keystroke; smartcase = wrap in
`(?i)` iff pattern has no uppercase; match target is the **rendered row text**
(alias + commit + age + status) so `/behind` works. Invalid regex: keep last
good matches, render `err` inline, never crash.

Viewport invariant (F4): `vp.top ≤ indexOf(cursor) < vp.top+vp.height` after
every transition — one `clampViewport()` called at the end of `route`.

Batch contract (F8/F13): confirm snapshots targets (table order, fallback
cursor) and fires `precheckSudo` per host; each `precheckMsg` routes the host
to `bgQueue` or `iaQueue`. The background wave fills up to `jobs` slots with
`bgUpdate` cmds; each `bgUpdateDoneMsg` records ok/fail + log, re-fires
`collectOne`, and hands the slot to the next queued host. When `bgQueue` and
all slots drain, the `iaQueue` runs serially via `tea.ExecProcess` handoffs
(`ssh -t`), each `execDoneMsg` advancing it. Errors advance, never wedge —
in either path. Refresh (`r`) at any moment re-polls only hosts not in
`updating` (F2b).

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

**Task 10 — concurrent background update engine (F8, F9, F2b).**
`u`: targets = selection (table order) else cursor; modeConfirm strip lists
targets + ref; `y` fires `precheckSudo` per target; passing hosts fill
`bgQueue`; wave runs ≤ `jobs` concurrent `bgUpdate` cmds (BatchMode ssh over
the runner seam wrapping `remoteUpdateScript(updateRef)` — **never
ExecProcess**); each `bgUpdateDoneMsg` records ok/FAIL + log tail, re-polls
the host, refills the slot; `n`/`esc` = nothing. `r` during the wave re-polls
only non-updating hosts. Flags: `--update-ref` (validRef) + `--jobs`
(default 4, ≥1) on tuiCmd.
RED: target-list rules (F8a), declined-changes-nothing (F8b), slot-advance
past a failing fake (F8c), saturation ≤ jobs at every step (F8d), BatchMode
argv + no-ExecProcess (F8e), keystrokes interleave with completions (F8f),
refresh exclusion (F2b), injection ref + bad --jobs rejected (F9a, F9b).
Done-when: all green; confirm-strip + updating-rows goldens.
Evidence: `task10/batch-bg.txt`.

**Task 11 — interactive fallback queue (F13).**
Hosts whose precheck fails route to `iaQueue`; when the background wave
drains, serial `ssh -t` ExecProcess handoffs run (the v1 path, unchanged
lesson: sudo prompts need a real terminal); each `execDoneMsg` records,
re-polls, advances. A host is never in both queues.
RED: mixed-fleet routing (F13a), fallback-advance incl. declined/error
(F13b), never-both-queues invariant.
Done-when: green.
Evidence: `task11/fallback.txt`.

**Task 12 — ssh action (F10).**
`s` → `ssh <cursor alias>` ExecProcess; `execDoneMsg` restores UI, re-polls
that host (an ssh visit often changes nothing, but the re-poll is free and
keeps the invariant "handoff returns ⇒ row refreshed"). Disabled while that
host is updating.
RED: cmd-construction (argv exactly `ssh <alias>`), empty-fleet no-op,
blocked-while-updating.
Done-when: green.
Evidence: `task12/ssh.txt`.

**Task 13 — help overlay + polish (F11, F12).**
`?` overlay from the single keymap table (no second hand-written list to
drift); any key closes; empty-fleet guidance kept; quit guard while updates
are in flight ("updates in progress — q again to force").
RED: overlay golden, overlay-swallows-keys, double-q guard, empty-fleet keys.
Done-when: green.
Evidence: `task13/help.txt`.

**Task 14 — docs + gate.**
README TUI section (keys table, search, selection, concurrent batch +
fallback, `--update-ref`, `--jobs`), AGENTS.md command row + invariants;
`scripts/test.sh` full run.
Done-when: gate green ≥ 60% (fleet), cmd ≥ 55%; `go vet` clean; no identity
leak in diff (`grep` sweep).
Evidence: `task14/gate.txt`.

**Task 15 — HUMAN STOP: live capture.**
Real fleet: streaming arrival, `/` search, select 2+, **two hosts updating
concurrently while navigating and pressing `r`**, a FAIL row refilling its
slot, an interactive-fallback handoff with a visible sudo prompt (proves the
terminal release), `s` ssh in/out, post-update fresh stamps. `tmux
capture-pane` sequence, sanitized, committed to `evidence/task15/`.
Done-when: capture shows every F-feature live; TRACKING row cites it.

## 5. Verification mapping

Every spec §5 rule maps to the named test written in its task: F1a–c → Task 5,
F2a → 6, F2b → 10, F3a → 2, F4a–d → 3–4, F5a–c → 7, F6a → 8, F7a–b → 9,
F8a–f + F9a–b → 10, F13a–b → 11, F10a → 12, F11a + F12a → 13. The mapping is
restated per-row in TRACKING.md as tasks complete.

## 6. Integration & rollout

No wiring changes: same binary, same build.sh, same coverage floor entry.
Rollout = merge; rollback = revert (spec §9). Manual acceptance = Task 15.

## 7. Validation & evidence (show the work)

Evidence tree `docs/mbo/plans/fleet-tui/evidence/task01..15/`, append-only,
dated headers, committed with each task (privacy-sanitized). Coverage bars in
§4 Task 14. Adversarial scenarios covered: invalid regex (F5b), erroring host
mid-wave (F8c), slot saturation (F8d), a background prompt hanging → BatchMode
fast-FAIL (design §5), precheck misrouting (F13a), injection ref (F9a),
resize-under-cursor (F4d), empty fleet (F12a), quit-during-updates (Task 13).
A feature without captured evidence is not done.

### 6.1 Build leaves / DAG

Default: single worker (`fleet-tui/<user>/build`), sequential Tasks 1–15 —
the tasks chain through one model struct, so parallel leaves would fight over
the same files. No CAP-B breakout.
