# fleet TUI panes + stderr error view — spec

- **Slug:** fleet-error-view
- **Date:** 2026-09-06
- **Status:** Draft
- **Relates to:** design [`../designs/fleet-error-view.md`](../designs/fleet-error-view.md) · plan [`../plans/fleet-error-view.md`](../plans/fleet-error-view.md) · issue [#308](https://github.com/sfc-gh-eraigosa/dotfiles/issues/308) · PR #TBD

## 1. Goal

`fleet tui` becomes a three-pane dashboard the operator composes: the **host** table, the
**log** stream, and a new **error** stream carrying everything the remote wrote to *stderr*.
Each pane toggles with one key (`h`, `l`, `e`); whichever panes are visible share the viewport
— one pane alone fills it. A host that wrote to stderr is flagged on its row as a **warning**
even when its update exited 0, so the failure mode this tool most needs to catch — a host that
half-installed and reported success — is visible without opening anything.

## 2. Use cases

**UC1 — read a long install without the table in the way.**
*Actor:* operator watching one host install. *Trigger:* presses `h`.
*Flow:* the host pane disappears; the log pane expands to the full viewport; `j/k` scroll the
log (it holds focus); `h` brings the table back with the cursor and selection intact.
*Acceptance:* the log body gains exactly the rows the host pane and its border released; no
frame exceeds the terminal height; the cursor host is unchanged after `h h`.

**UC2 — a wave "succeeds" but something went wrong.**
*Actor:* operator running a fleet-wide update. *Trigger:* a host exits 0 having written
`WARNING: apt-get update failed…` to stderr.
*Flow:* the row reads `ok ⚠1`; the status bar reads `⚠ 1 warning on 1 host`; pressing `e`
opens the error pane below the log showing that line, timestamped and host-coloured.
*Acceptance:* the badge appears without any keystroke; opening the error pane requires no
re-run; the same line is also present in the log pane, marked as stderr.

**UC3 — diagnose a failure across three panes.**
*Actor:* operator with a failing host. *Trigger:* row reads FAIL; presses `e`.
*Flow:* host pane keeps the top; log and error split the bottom half and half; `tab` cycles
focus host → log → error → host; `/` in the focused pane searches only that pane.
*Acceptance:* both bottom panes render at least 3 body rows at 100×40; each pane's search,
scroll and follow state is independent; `G` re-follows the tail of the focused pane only.

**UC4 — a small terminal.**
*Actor:* operator on an 80×24 (or 60×16) terminal with all three panes on.
*Flow:* the panes shrink toward their floors; if both bottom floors do not fit, the log keeps
the bottom region and the error pane yields. Floors are a preference, not a guarantee — at
60×16 the chrome plus three panel borders already claim 15 of the 16 rows.
*Acceptance:* the frame never exceeds the terminal width, and never exceeds
`max(vp.height, chromeRows)` rows; no pane height is negative; the rationing is deterministic
(pressing `tab` never resizes anything).

**UC5 — noisy but healthy.**
*Actor:* operator updating hosts whose `sync` step makes git write progress to stderr.
*Flow:* those lines appear in the error pane (they are stderr, honestly reported) but do
**not** raise the row's warning count.
*Acceptance:* a row whose only stderr is denylisted reads plain `ok`, no ⚠.

**UC6 — a headless post-mortem.**
*Actor:* operator reading `~/.local/state/fleet/logs/<host>-<ts>.log` after a `fleet update`.
*Flow:* the capture shows stderr lines prefixed `!!`, interleaved in arrival order.
*Acceptance:* `fleet update`'s own terminal output is byte-identical to before this change.

## 3. Architecture

| Component | Responsibility | Depends on |
| :-- | :-- | :-- |
| `internal/runner` — `Line{Text,Stderr}`, `SplitStreamer`, `Exec.RunSplitStreamCtx`, `Fake.ErrOut` | Stream a remote command with the two streams kept apart. `RunStreamCtx` is reimplemented on top of it (tag dropped) so one implementation serves both. | `os/exec` |
| `internal/updexec` — `Console.ErrLine`, `withLines`, `Batch`'s capability assertion | Route stderr to `ErrLine` when set (else to `Line`, today's behavior); tee **both** streams into the per-host capture, marking stderr `!!`. | `runner` |
| `internal/updexec/stderrnoise.go` — `Benign(line string) bool` | Pure, table-driven classifier: is this stderr line known-benign ssh/git/sudo chatter? | — |
| `cmd/tui_layout.go` — `layout(vpHeight, chromeRows, panes, logActive, errActive) heights` | The **only** place pane heights are computed. Pure ints in, ints out; `chromeRows` is measured by the caller because the banner wraps. | — |
| `cmd/tui_model.go` | `hostOpen`/`logOpen`/`errOpen`, `focus pane`, one tagged `[]logEntry`, per-alias `warns`, per-pane nav state. | `layout` |
| `cmd/tui_view.go` — `streamPane(...)` | One parameterized renderer for the log and error panes; row badge; status summary. | `layout`, theme |
| `cmd/tui_keys.go` — `keyHelp` + routing | `h`/`e` bindings, focus cycling over visible panes, per-pane key scoping. | model |

**Data flow:** `ssh` → two pipes → `runner.Line{Text,Stderr}` → `Console.Batch` →
`Line`/`ErrLine` → (TUI) `appendLog(alias, line, stderr)` → one tagged ring → log pane renders
all, error pane renders the `stderr` subset, `warns[alias]` counts the non-benign ones.
In parallel, `Executor.RunHost`'s tee → capture file (stderr prefixed `!!`).

## 4. Behavior / features

| ID | Feature |
| :-- | :-- |
| F1 | `h` toggles the host pane. Default **on**. |
| F2 | `e` toggles the error pane. Default **off**. |
| F3 | Hiding the last visible pane is refused, with a status message; there is no zero-pane state. |
| F4 | Exactly one active pane ⇒ it occupies the whole viewport below the banner. |
| F5 | Host + exactly one active bottom pane ⇒ today's split (host `max(3, vp/5 − border)`, bottom the rest). |
| F6 | Host + both bottom panes active ⇒ host on top; the bottom region split in half, the log keeping the odd row, each *aiming* for 3 rows. When the region cannot fit both, the log keeps it and the error pane yields — deterministically, never depending on focus. |
| F7 | A pane that is open but has nothing to show collapses to a one-line hint and reserves no body rows (today's log behavior, extended to the error pane). |
| F8c | Every panel truncates to its **inner** width (`panelWidth() - 2`, the lipgloss padding) — host rows, the column header (which is not truncated at all today), and each stream line — so one body row is exactly one rendered line at every terminal width. |
| F8 | The rendered frame never exceeds `vp.width` columns, and never exceeds `max(vp.height, chromeRows)` rows, for every pane combination, mode and terminal size — i.e. **the panes never add to an over-budget frame**. `chromeRows` is the measured banner + status; in `modeAnswers` at 60×16 it is 19 rows on its own, which no pane arithmetic can fix (design §1.3). **This is a bug fix, not only a guard:** today's frame overflows by +3 (100×40 with logs), +6/+7 (80×24) and +12 (60×16). |
| F9 | `runner` exposes stderr separately via the optional `SplitStreamer`; a runner without the capability behaves exactly as today (all lines stdout). |
| F10 | `Console.ErrLine == nil` ⇒ stderr is delivered to `Line` — every existing caller and test is unaffected. |
| F11 | The per-host capture marks stderr lines with a `!!` prefix; `fleet update`'s terminal output is unchanged. |
| F12 | The model keeps ONE tagged buffer; the log pane shows every entry (stderr marked with a `!` gutter), the error pane shows only `stderr` entries. |
| F13 | `Benign()` excludes known ssh/git/sudo chatter from the warning count (never from the error pane). |
| F14 | A host that finished OK with `warns > 0` renders `ok ⚠N`; a failed host still renders FAIL. |
| F15 | The status bar summarises `⚠ N warning(s) on M host(s)` when any exist. |
| F16 | Starting a new update for a host clears that host's warning count. |
| F17 | `tab` cycles focus over **visible** panes only; vim motions, `/`, `n`/`N`, `gg`/`G` act on the focused pane; each stream pane has independent follow/top/search state. |
| F18 | `h` and `e` are declared in `keyHelp` (header strip + `?` overlay), and mirrored in `sdk/fleet/AGENTS.md` / `README.md`. |
| F19 | `J`/`K` (host-focused log scroll) keep working on the log pane; the error pane is scrolled by focusing it. |
| F20 | Pane visibility is session-only — never persisted. Every `fleet tui` starts host+log on, error off. |

## 5. Evaluation criteria (per feature)

Format: **trigger · fires · must-not-fire · edge · pass**.

- **F1a** `h` in normal mode · `hostOpen` flips and `visibleRows()` changes · must not fire in
  `modeSearch`/`modeAnswers`/`modeConfirm` (there `h` is text) · `h h` restores the exact prior
  height and cursor · `TestHostPaneTogglesAndRestores`.
- **F2a** `e` in normal mode · `errOpen` flips · must not fire while a prompt mode owns the key ·
  `e` in `modeConfirm` still means "edit answers" (existing binding, unchanged) ·
  `TestErrorPaneTogglesAndDoesNotStealConfirmEdit`.
- **F2b** a fresh model · `errOpen == false`, `hostOpen == true`, `logOpen == true` · — · — ·
  `TestPaneDefaults`.
- **F3a** last visible pane, its toggle key · nothing changes; `m.status` explains ·
  must not leave all three false · pressing it twice is idempotent ·
  `TestHidingTheLastPaneIsRefused`.
- **F4a** one active pane, `vp{100,40}` · that pane's body height == the whole region below the
  chrome, less its own `panelFixedRows` · the other panes contribute 0 rows · works for each of
  the three panes alone · `TestLayout` sub-cases *host only / log only / err only takes
  everything*.
- **F5a** host + log active · host body == `max(minPaneRows, vp/5)` clamped so the log keeps its
  floor, log == the remainder · error contributes 0 · **not** "identical to today": today's
  numbers overflow the terminal (design §1.3), so the proportion is preserved (host ≈ the top
  fifth) while the total is corrected · `TestHostPlusOneBottomPaneKeepsTheTopFifth`.
- **F6a** all three active at 100×40 · `|log − err| ≤ 1`, both ≥ 3 · host unchanged from F5a ·
  the odd remainder goes to the log · `TestThreePaneSplitIsEven`.
- **F6b** a viewport too small for both floors · the log keeps the bottom region, `err == 0` ·
  the result must not depend on `m.focus` · no negative heights · `TestTinyViewportStillFits`.
- **F7a** pane open, buffer empty · body rows 0, its one hint line rendered in place of the
  title · must not claim a share · log empty + error non-empty ⇒ error gets the whole bottom
  region · `TestLayout` sub-case *open but empty stream panes reserve no body rows*, plus
  `TestErrorPaneOpenButEmptyCollapses` (Task 10) for the rendered form.
- **F8a** matrix of {60×16, 80×24, 100×40, 200×60} × {7 legal pane combinations} × {empty, full
  buffers} × {normal, search, answers, confirm, help} · `lipgloss.Height(View()) ≤
  max(vp.height, m.chromeRows())` and `lipgloss.Width(View()) ≤ vp.width` · must not pass by
  rendering an empty frame (each case also asserts the frame is non-blank) · includes a 0-row
  and 1-row fleet · `TestFrameFitsTheTerminal`. **Baseline: this test FAILS on today's code**
  (design §1.3) — the RED that opens the layout task.
- **F8b** fixed rows ≥ viewport (60×16 in `modeAnswers`, or three panels at 60×16) · every pane
  height is 0 · the panes must not add a single row to an already-over-budget frame · the dialog
  and banner still render · `TestChromeOverBudgetGivesThePanesNothing`.
- **F8c** a host panel holding N rows, at 60/80/100/200 columns · renders exactly `N + 3` lines
  (border 2 + column header 1) · must not wrap — today an 8-host panel is **20** lines at 80
  columns · the same holds for a stream panel with N lines and for the longest possible row (a
  30-character alias, a long branch, a long FAIL cause) · `TestPanelBodyRowIsExactlyOneLine`.
- **F9a** `Exec` against a `stubSSH` writing to both streams · stdout lines arrive
  `Stderr:false`, stderr lines `Stderr:true`, both channels drained, `done` fires ·
  no deadlock, no dropped line · a 5000-line stderr burst with a slow reader ·
  `TestRunSplitStreamCtxSeparatesTheStreams`, `TestSplitStreamDoesNotDeadlockUnderBackpressure`.
- **F9b** a runner without the capability · `Console.Batch` still streams, every line
  `Stderr:false` · must not panic on the type assertion · `TestBatchFallsBackWhenNotSplitCapable`.
- **F9c** `Exec.RunStreamCtx` · returns the same merged sequence as before this change ·
  must not change signature · the deadline test still passes · existing
  `TestRunStreamCtxKillsTheChildOnDeadline` + `TestRunStreamMatchesSplitStreamMerged`.
- **F10a** `Console` with `ErrLine == nil` · stderr reaches `Line` · must not be dropped ·
  the existing console suite passes untouched · `TestNilErrLineRoutesStderrToLine`.
- **F11a** `Executor.RunHost` with a scripted stderr line · the capture contains `!! <line>` ·
  stdout lines unprefixed · a line already starting with `!!` is not double-prefixed ·
  `TestCaptureMarksStderr`.
- **F12a** `appendLogLine` with mixed streams · `errEntries()` == the stderr subset, in order ·
  the log pane must still contain them · the shared `logCap` evicts both views consistently, and
  `errCount` never drifts from the projection across eviction ·
  `TestStderrLineIsTaggedAndCounted`, `TestErrorPaneRendersOnlyStderr`,
  `TestErrCountTracksTheProjection`.
- **F13a** each denylist line · `Benign` true · a real `WARNING: apt-get update failed…` must
  be false · case and leading-whitespace variants · `TestBenignStderrTable`.
- **F14a** host finishes `updOK` with 2 non-benign stderr lines · `updateCell` contains `ok` and
  `⚠2` · a host with only benign stderr shows no ⚠ · a failed host shows FAIL, never `ok ⚠` ·
  `TestOkWithWarningsBadge`, `TestFailRowIsNeverAWarning`.
- **F15a** any warnings present · status bar contains `⚠` with the host count · absent when
  zero · plural/singular wording · `TestStatusBarWarningSummary`.
- **F16a** `startUpdate` on a host with a prior badge · `warns[alias] == 0` afterwards ·
  must not clear other hosts' counts · re-running the same host mid-session ·
  `TestNewAttemptClearsTheWarningBadge`.
- **F17a** `tab` with the error pane hidden · focus goes host → log → host · must never focus a
  hidden pane · hiding the focused pane moves focus to the next visible one ·
  `TestFocusCyclesVisiblePanesOnly`.
- **F17b** `/` while the error pane is focused · compiles into the error pane's own search
  state · must not disturb the host filter or the log's pattern · `n`/`N` walk error matches ·
  `TestPerPaneSearchIsIndependent`.
- **F18a** `keyHelp` · contains EXACTLY ONE row each for `h`, `e` and `l`, all `hdr:true` ·
  must not add a *second* `e` row — one already exists for the confirm-mode meaning
  (`tui_keys.go:46`), and a duplicate fails the pre-existing
  `TestKeyHelpHasNoDuplicateBindings` (`tui_config_test.go`) · the header strip still fits at
  60 columns · `TestPaneKeysAreDeclaredInKeyHelp` (new, with the pane tests) plus the existing
  `TestKeyHelpHasNoDuplicateBindings` and `TestHelpListsTheNewKeys` (`tui_sticky_test.go`),
  both left unmodified.
- **F19a** `J`/`K` in normal mode with the log open · log scrolls, follow stops · must not
  scroll the error pane · unchanged from today · existing `tui_logpane_test.go` cases pass.
- **F20a** two `newTUIModel` calls · both start `host+log` on, `err` off · nothing written to
  disk · `TestPaneStateIsNotPersisted`.

## 6. Verification harness

1. **Pure unit tests** — `layout()` (table-driven over the F8a matrix), `Benign()`,
   `appendLog` projections, warning counting. No I/O.
2. **Model/key tests** — the existing `send(m, "key")` harness in `cmd/`; every new key and
   focus rule table-driven.
3. **Golden frames** — `TestDemoFrames` gains one frame per pane combination (ASCII profile,
   byte-stable via the injected clock and spinner) plus the new **height** assertion, which
   today's suite lacks entirely (it asserts width only).
4. **Real-process runner tests** — `stubSSH` (already used by `runner_ctx_test.go`) writes to
   both streams from a `/bin/sh` stub, proving the two pipes drain without deadlock, under a
   burst and under a deadline. No socket, CI-safe.
5. **Executor/capture tests** — scripted stderr through `Executor.RunHost` into a temp-dir
   capture.
6. **Human-gated live gate (G-live)** — one real `fleet tui` run against a live host: `h`, `e`,
   the three-pane split, and a genuine warning badge, captured as an asciinema/screenshot under
   `plans/fleet-error-view/evidence/e2e/`. Splitting the pipes is the one change a stub cannot
   fully prove against a real `ssh` under a real install.

**Coverage bars** (baseline measured 2026-09-06 on this branch: module total 82.3 %,
`cmd` 63.5 %, `runner` 59.8 %, `updexec` 92.5 %): module total ≥ 82 %, `cmd` ≥ 65 %,
`runner` ≥ 65 %, `updexec` ≥ 90 %. `make lint-go` and `scripts/test.sh` must stay green.

## 7. Prerequisites / dependencies

- None blocking. Builds on `fleet-update` (the `updexec` executor and lanes) which is merged
  into this branch's base.
- **Coordination, not dependency:** `sdk-tui` phase 3 will port fleet onto `sdk/libs/tui`;
  this design's `h`/`e` rebinding is recorded so that port inherits it. `fleet-connect`'s
  `ReservedKeys` (design §4.2) must gain `h` and `e` before its provider-action keys ship, or
  a plugin could shadow a pane toggle. Both are follow-ups filed against those objectives.

## 8. Out of scope (and why)

- **Persisting pane layout** — the defaults are the contract (§F20); a preference file is a
  second source of truth for "what does fleet look like on startup".
- **A configurable noise denylist** — one in-tree table with one test is cheaper to widen than
  a schema, validation, docs, and a migration.
- **Exact stdout/stderr interleaving** — design §3.1: the only portable way to keep it is a
  bash-only remote wrapper. Arrival order plus per-line timestamps is the accepted trade.
- **Warning reporting in headless `fleet update` / `--json`** — the capture gains the `!!`
  marks (F11), but the report schema is untouched; changing it is a separate objective with
  its own consumers.
- **Pane resizing / reordering / mouse** — YAGNI for v1.
- **Making the answer/confirm dialogs scroll on a very short terminal** — measured: the answer
  form alone is 12 rows at 60 columns, so at 60×16 it cannot fit beside anything. That is a
  pre-existing limitation of those dialogs, not of the pane split; F8 is scoped so the panes
  never make it worse, and fixing the dialogs is a separate objective.

## 9. Rollback

Three independent reverts, in reverse dependency order (design §6): panes+view, then
`ErrLine`, then `SplitStreamer`. `RunStreamCtx`'s signature never changes, so no consumer
outside these three files can be left broken. No config, no on-disk state, no migration.
