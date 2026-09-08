# fleet-error-view — live state ledger

- **Slug:** fleet-error-view
- **Started:** 2026-09-06
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../fleet-error-view.md`](../fleet-error-view.md) · spec [`../../specs/fleet-error-view.md`](../../specs/fleet-error-view.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a commit
> SHA **and** evidence. Never write a result you did not observe.

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| (single) | classic gss lane (not a `gss feature` worker) | `worktree/fleet-error-view` | this worktree | [#310](https://github.com/sfc-gh-eraigosa/dotfiles/pull/310) (draft) | planning |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 1 · runner: `SplitStreamer` | done | see §5 | `go test ./...` + `-race` green | |
| 2 · runner: `Fake.ErrOut` | done | see §5 | `go test ./...` + `-race` green | |
| 3 · updexec: `Console.ErrLine` | done | see §5 | `go test ./...` + `-race` green | |
| 4 · updexec: capture marks stderr | done | see §5 | `go test ./...` + `-race` green | |
| 5 · updexec: `Benign()` | done | see §5 | `go test ./...` + `-race` green | |
| 6 · cmd: tag into the model | done | see §5 | `go test ./...` + `-race` green | |
| 7 · cmd: pure `layout()` | done | see §5 | `go test ./...` + `-race` green | |
| 8 · cmd: adopt layout, fix overflow | done | see §5 | `go test ./...` + `-race` green | RED output is itself evidence |
| 9 · cmd: `h`/`e` keys + focus | done | see §5 | `go test ./...` + `-race` green | |
| 10 · cmd: error pane render | done | see §5 | `go test ./...` + `-race` green | |
| 11 · cmd: warning badge + status | done | see §5 | `go test ./...` + `-race` green | |
| 12 · cmd: end-to-end + `-race` | done | see §5 | `go test ./...` + `-race` green | |
| 13 · frames, usability, docs | done | see §5 | `go test ./...` + `-race` green | |
| 14 · live gate (human) | **todo** | | | cannot be completed by an agent — the ONE outstanding item |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 host pane toggle | [ ] `TestHostPaneTogglesAndRestores` | [ ] Task 14 | |
| F2 error pane toggle | [ ] `TestErrorPaneTogglesAndDoesNotStealConfirmEdit` | [ ] Task 14 | |
| F3 refuse last pane | [ ] `TestHidingTheLastPaneIsRefused` | — | |
| F4a/F7a layout rules | [ ] `TestLayout` (6 sub-cases) + `TestErrorPaneOpenButEmptyCollapses` | [ ] Task 14 | |
| F5a host keeps the top fifth | [ ] `TestHostPlusOneBottomPaneKeepsTheTopFifth` | — | |
| F6a even three-pane split | [ ] `TestThreePaneSplitIsEven` | — | |
| F6b tiny viewport yields | [ ] `TestTinyViewportStillFits` + `TestSmallTerminalYieldsRatherThanOverflowing` | — | |
| F8a frame fits | [ ] `TestFrameFitsTheTerminal` + `TestFrameFitsWithZeroAndOneHost` + `TestEmptyFleetStillRendersTheOtherPanes` | — | RED baseline must be captured first |
| F8b panes yield when over budget | [ ] `TestChromeOverBudgetGivesThePanesNothing` | — | |
| F8c one row = one line | [ ] `TestPanelBodyRowIsExactlyOneLine` | — | measured: 8 hosts render 20 lines at 80 cols today |
| F9 split streams | [ ] `TestRunSplitStreamCtxSeparatesTheStreams` + backpressure + deadline | [ ] Task 14 (real ssh) | |
| F9c merged == split | [ ] `TestRunStreamMatchesSplitStreamMerged` | — | |
| F10 nil `ErrLine` fallback | [ ] `TestNilErrLineRoutesStderrToLine` | — | re-run after Task 4 |
| F11 capture marking | [ ] `TestCaptureMarksStderr` | [ ] `grep -c '^!! '` on a live capture | |
| F12 one buffer, two projections | [ ] `TestStderrLineIsTaggedAndCounted` + `TestErrorPaneRendersOnlyStderr` + `TestErrCountTracksTheProjection` | — | |
| F13 benign filter | [ ] `TestBenignStderrTable` | [ ] a real git-fetch run raises no ⚠ | |
| F14 warning badge | [ ] `TestOkWithWarningsBadge` + `TestFailRowIsNeverAWarning` | [ ] Task 14 | |
| F15 status summary | [ ] `TestStatusBarWarningSummary` | — | |
| F16 per-attempt reset | [ ] `TestNewAttemptClearsTheWarningBadge` | — | |
| F17 focus + per-pane search | [ ] `TestFocusCyclesVisiblePanesOnly` + `TestPerPaneSearchIsIndependent` | — | |
| F18 keys declared | [ ] `TestPaneKeysAreDeclaredInKeyHelp` (new) + the untouched `TestKeyHelpHasNoDuplicateBindings` | — | one `e` row, shared with the confirm meaning |
| F19 `J`/`K` regression | [ ] existing `tui_logpane_test.go` cases | — | ONE case is rewritten by design (Task 8: `TestSplitKeepsBothHalvesUsableOnASmallTerminal` asserts today's overflow) |
| F20 session-only state | [ ] `TestPaneStateIsNotPersisted` | — | |

## 3. Validation done-when — the stop condition

- [ ] Tasks 1–13 `done` with commit SHAs and evidence files.
- [ ] `go test ./...` and `go test -race ./...` green in `sdk/fleet`.
- [ ] Coverage: module ≥ 82 %, `cmd` ≥ 65 %, `runner` ≥ 65 %, `updexec` ≥ 90 %.
- [ ] `make lint-go` green; markdown clean apart from `MD010` inside Go snippets.
- [ ] `evidence/layout/task08-before-overflow.txt` committed (proof the overflow was real).
- [ ] Every spec §5 rule maps to a passing named test (plan §5).
- [ ] Task 14 live capture in `evidence/e2e/`, or explicitly recorded here as outstanding.
- [ ] `docs/mbo/index.md` row advanced to `in-review`.

## 4. Blockers & escalations

- **Resolved during the build — three pre-existing tests encoded the overflow, not one.** The
  plan predicted a single rewrite (`TestSplitKeepsBothHalvesUsableOnASmallTerminal`). Two more
  failed for the same root cause: `TestWindowResizeKeepsCursorVisible` (80×10) and
  `TestHalfPageMovesAndStaysInBounds` (80×13, commented "6 list rows once framed" — it was
  never 6 without rendering past the bottom of the screen). Their viewport heights were raised
  so they still test the motion arithmetic they were written for; nothing they assert changed.
- **Resolved during the build — the demo fixture was too short.** With the corrected arithmetic
  a 16-row terminal genuinely fits three host rows, so frames 2 and 9 stopped containing the
  states they demonstrate. The fixture is 24 rows now; the frames were not weakened.
- **Two defects the golden frames caught that no unit test did:** the error pane's title leaked
  raw ANSI (a `th.warn.Render` nested inside `th.header.Render` re-escapes the inner reset), and
  the bottom split collapsed the error pane entirely at 24 rows because `splitBottom` demanded
  BOTH panes reach `minPaneRows` before splitting at all — on the very terminal size an
  operator would open it on. The floor is now an aspiration, not a precondition: any region of
  ≥2 rows splits.

## 5. Session log (append-only)

- **2026-09-06 (reconciliation — READ THIS BEFORE TRUSTING THE PLAN'S §1)** — merging
  `origin/worktree/fleet-error-view` revealed that `main` had moved under this branch and
  **two of the plan's premises were already resolved there**:
  1. **The frame overflow was fixed on main first**, as
     [#306](https://github.com/sfc-gh-eraigosa/dotfiles/pull/306) (09:19), independently
     finding the same padding cause ("lipgloss counts horizontal padding INSIDE Style.Width").
     Its `View()` **measures and corrects** — render, measure, hand rows back until it fits,
     with `fitFrame` as a last resort — which is strictly better than the predictive
     `layout()` this plan's Tasks 7–8 specified, because it cannot drift. **`cmd/tui_layout.go`
     was deleted and the panes were rebuilt on main's loop**, extended from one elastic pane to
     two (`splitStreamRows`). Plan §1 finding 2 and Tasks 7–8 are superseded; the *findings*
     were right, the design was second-best.
  2. **`ReservedKeys` is real Go code now** (`sdk/fleet/pkg/provider/provider.go`, from
     [#305](https://github.com/sfc-gh-eraigosa/dotfiles/pull/305)) and **already reserves
     `h`, `l` and `e`** — `h` deliberately "ahead of use". The design-doc edits this objective
     made were redundant and were dropped in favour of main's. `cmd/provider_keys_test.go`'s
     `TestEveryFleetKeyIsReservedAgainstProviders` mechanically enforces the agreement and
     **passes with the new `h`/`e` bindings**, which is a better guarantee than the prose was.
  Everything the objective actually adds — the split streams, `ErrLine`, the marked capture,
  `Benign`, the tagged buffer, the error pane, the warning badge — survived unchanged.

- **2026-09-06 (build)** — Tasks 1–13 implemented TDD, RED verified before every GREEN.
  Commits: `0e92030` (runner split streams), `9accdec` (updexec ErrLine + capture marking +
  Benign), `c735fd5` (the three panes, layout(), the badge, frames + the height guard).
  Gates: `go test ./...` and `go test -race ./...` green; `golangci-lint` 0 issues;
  coverage `cmd` 65.4 %, `runner` 74.0 %, `updexec` 92.4 %, module **83.4 %** (all above their
  floors, none regressed). Installed to `~/opt/bin/fleet` (`v0.4.0-10-g9accdec`) for the
  operator to try. Task 14 (the live run) is the only item left.

- **2026-09-06 (review)** — `/code-review high` against the real `sdk/fleet` source returned 12
  findings, all verified and folded in before any code was written. The load-bearing ones:
  `logView` writes a title line *inside* the panel, so every open panel costs 3 fixed rows
  (`layout()` had under-counted by 1 per stream pane, which would have left Task 8's invariant
  failing by 1–2 rows); `e` was **already** in `keyHelp` for the confirm-mode meaning (a second
  row would fail `TestKeyHelpHasNoDuplicateBindings`); and three tasks would not have compiled
  (`recordingWriter`/`recordingOutput` already exist in `updexec`'s tests, `logFocus` is
  assigned in `tui_demo_test.go` and read in `tui_reset_lognav_test.go`, and `lineQueue`/`stream`
  are used in `review_test.go` and `tui_logpane_test.go`). Two benign patterns could never match
  because `Benign()` trims first — every real `git fetch` would have raised a spurious ⚠.
- **2026-09-06** — design, spec, plan and trio written; baseline measured
  (`go test ./... -coverprofile` → total 82.3 %, `cmd` 63.5 %, `runner` 59.8 %,
  `updexec` 92.5 %); frame-overflow bug measured on today's code (+3 at 100×40 with logs,
  +6/+7 at 80×24, +12 at 60×16) via a throwaway probe test, since removed. Chrome measured the
  same way: banner 6 rows at 60 cols / 5 at ≥80; status 1 row normally but 8–12 in
  `modeConfirm`/`modeAnswers` — so at 60×16 in `modeAnswers` the chrome alone is 19 rows and
  spec F8 is scoped to "the panes never add to an over-budget frame". No production code
  written yet.
