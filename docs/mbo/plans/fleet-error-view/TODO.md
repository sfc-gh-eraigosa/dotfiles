# fleet-error-view — the cursor

> The **first unchecked box is the next action.** Micro-steps per plan task:
> RED · RUN-RED · GREEN · RUN-GREEN · VERIFY · COMMIT · LEDGER.
> Plan: [`../fleet-error-view.md`](../fleet-error-view.md) · procedure: [`IMPLEMENTATION.md`](./IMPLEMENTATION.md)

## Task 1 — runner: `SplitStreamer` on `Exec`

- [x] RED: write `internal/runner/split_test.go` (separation, backpressure burst, merged==split, deadline)
- [x] RUN-RED: `go test ./internal/runner/ -run 'Split|Merged' -v` → FAIL `RunSplitStreamCtx undefined`
- [x] GREEN: add `Line`, `SplitStreamer`, `Exec.RunSplitStreamCtx`; reimplement `Exec.RunStreamCtx` on it
- [x] RUN-GREEN: `go test ./internal/runner/ -v`
- [x] VERIFY: `go test ./...` (nothing else moved)
- [x] EVIDENCE: `evidence/stderr/task01-runner-split.txt`
- [x] COMMIT + LEDGER

## Task 2 — runner: `Fake.ErrOut`

- [x] RED: `TestFakeReplaysErrOutAsStderr`, `TestFakeMergedStreamStillCarriesErrOut`, the `var _ SplitStreamer` assertions
- [x] RUN-RED: `go test ./internal/runner/ -run Fake -v` → FAIL `unknown field ErrOut`
- [x] GREEN: add `ErrOut`, `Fake.RunSplitStreamCtx`; reimplement `Fake.RunStreamCtx` on it (reuse the existing line-splitting verbatim)
- [x] RUN-GREEN + VERIFY: `go test ./...`
- [x] EVIDENCE: `evidence/stderr/task02-fake-errout.txt`
- [x] COMMIT + LEDGER

## Task 3 — updexec: `Console.ErrLine`

- [x] RED: `internal/updexec/split_test.go` — `TestErrLineReceivesStderrOnly`, `TestNilErrLineRoutesStderrToLine`, `TestBatchFallsBackWhenNotSplitCapable`
- [x] RUN-RED: `go test ./internal/updexec/ -run 'ErrLine|FallsBack' -v` → FAIL `unknown field ErrLine`
- [x] GREEN: add `ErrLine`, `Console.emit`, the `SplitStreamer` branch in `Batch`
- [x] RUN-GREEN + VERIFY: `go test ./internal/updexec/ -v`
- [x] EVIDENCE: `evidence/stderr/task03-errline.txt`
- [x] COMMIT + LEDGER

## Task 4 — updexec: mark stderr in the capture

- [x] RED: `TestCaptureMarksStderr` (+ `recordingOut`)
- [x] RUN-RED: `go test ./internal/updexec/ -run Capture -v` → FAIL `undefined: stderrMark`
- [x] GREEN: `stderrMark`, `teeable.withLines`, `RunHost`'s two-callback tee (prefix applied unconditionally)
- [x] RUN-GREEN: re-run `TestNilErrLineRoutesStderrToLine` explicitly (the tee wraps `ErrLine`)
- [x] VERIFY: `go test ./...`
- [x] EVIDENCE: `evidence/stderr/task04-capture-mark.txt`
- [x] COMMIT + LEDGER

## Task 5 — updexec: `Benign()`

- [x] RED: `stderrnoise_test.go` table (benign list + real-error list + `remote: fatal:`)
- [x] RUN-RED: `go test ./internal/updexec/ -run Benign -v` → FAIL `undefined: Benign`
- [x] GREEN: `stderrnoise.go` — `benignPatterns` (anchored regexps, whole-line) + `Benign()`
- [x] RUN-GREEN: every table row passes, none commented out
- [x] EVIDENCE: `evidence/warn-badge/task05-benign.txt`
- [x] COMMIT + LEDGER

## Task 6 — cmd: carry the tag into the model

- [x] RED: `cmd/tui_stderr_test.go` — `TestStderrLineIsTaggedAndCounted`, `TestAppendLogStaysStdout`
- [x] RUN-RED: `go test ./cmd/ -run 'Stderr|AppendLog' -v` → FAIL `appendLogLine undefined`
- [x] GREEN: `logEntry.stderr/.warn`, `appendLogLine`, `errEntries`, `m.warns`, `m.errCount`, `outLine` through `lineQueue`/`stream`/`readLine`, `beginStream` wires `Line`+`ErrLine`
- [x] RETYPE the existing call sites: `review_test.go:194,205` and `tui_logpane_test.go:129` (compile errors otherwise; assertions unchanged)
- [x] RUN-GREEN + VERIFY: `go test ./...` (existing log-pane suite must pass untouched)
- [x] EVIDENCE: `evidence/stderr/task06-model-tag.txt`
- [x] COMMIT + LEDGER

## Task 7 — cmd: pure `layout()`

- [x] RED: `cmd/tui_layout_test.go` — `TestLayout`, `TestThreePaneSplitIsEven`, `TestHostPlusOneBottomPaneKeepsTheTopFifth`, `TestTinyViewportStillFits`, `TestSmallTerminalYieldsRatherThanOverflowing`
- [x] RUN-RED: `go test ./cmd/ -run TestLayout -v` → FAIL `undefined: layout`
- [x] GREEN: `cmd/tui_layout.go` (`pane`, `panes`+`count()`, `heights`, `minFrameRows`, `layout`, `splitBottom`, constants — every open panel costs `panelFixedRows` = 3)
- [x] RUN-GREEN: fill the two `want`-less cases with real numbers, invariants unweakened
- [x] EVIDENCE: `evidence/layout/task07-layout-unit.txt`
- [x] COMMIT + LEDGER

## Task 8 — cmd: adopt layout, fix the overflow

- [x] RED: `TestFrameFitsTheTerminal`, `TestChromeOverBudgetGivesThePanesNothing`, `TestPanelBodyRowIsExactlyOneLine`, `TestFrameFitsWithZeroAndOneHost`, `TestEmptyFleetStillRendersTheOtherPanes`, `settledTestModel` helper
- [x] RUN-RED: `go test ./cmd/ -run TestFrameFits -v` → FAIL with concrete overflow lines
- [x] **EVIDENCE (RED):** `evidence/layout/task08-before-overflow.txt` — this proves the bug existed
- [x] GREEN: `panelInnerWidth()` + move EVERY truncation onto it (rows, the column header — untruncated today, `failWidth`, `logWidth`); `chromeRows()` (measures banner AND status — the status is a panel in modeAnswers/modeConfirm), `statusSeparatorRows`, `paneState()`, `heights()` (called ONCE per frame in `View()`), `errCount`, `listHeight`/`logHeight`/`errHeight` delegate; `View()` renders per pane and the empty-fleet early return folds into the host pane
- [x] REWRITE: `tui_logpane_test.go:97` `TestSplitKeepsBothHalvesUsableOnASmallTerminal` → the two replacements in plan Task 8 Step 3 (it asserts today's overflow; every OTHER case must pass untouched)
- [x] RUN-GREEN: `go test ./cmd/ -v`
- [x] VERIFY: `go test ./...`
- [x] EVIDENCE (GREEN): `evidence/layout/task08-after-fits.txt`
- [x] COMMIT + LEDGER

## Task 9 — cmd: `h` / `e` keys and focus

- [x] PRE: add a `"tab"` case to `key()` in `tui_model_test.go` (it currently sends the letters t,a,b)
- [x] RED: `cmd/tui_panes_test.go` — defaults, host toggle+restore, error toggle vs `modeConfirm` `e`, refuse-last, focus cycling, per-pane search, no persistence
- [x] RUN-RED: `go test ./cmd/ -run 'Pane|Focus' -v` → FAIL `hostOpen undefined`
- [x] GREEN: model fields + `focus`/`logFocused()` shim, `visiblePanes`, `togglePane`, `cycleFocus`, `routeNormal` cases, `routeSearch` per-pane selection, error-pane motion block; `keyHelp` gains ONE `h` row and EDITS the existing `e` row (a second `e` fails `TestKeyHelpHasNoDuplicateBindings`)
- [x] FIX the removed field's users: `tui_demo_test.go:124` (assigns `logFocus`) and `tui_reset_lognav_test.go:125,145`
- [x] RUN-GREEN + VERIFY: `go test ./...`
- [x] EVIDENCE: `evidence/layout/task09-keys.txt`
- [x] COMMIT + LEDGER

## Task 10 — cmd: render the error pane

- [x] RED: `TestErrorPaneRendersOnlyStderr`, `TestErrorPaneOpenButEmptyCollapses`
- [x] RUN-RED: `go test ./cmd/ -run ErrorPane -v` → FAIL (no error panel in the frame)
- [x] GREEN: `streamPane` shared renderer, `errView`, `!` gutter on stderr in the log pane, titles + empty hints
- [x] RUN-GREEN + VERIFY: `go test ./...` incl. `TestFrameFitsTheTerminal`
- [x] EVIDENCE: `evidence/layout/task10-errview.txt`
- [x] COMMIT + LEDGER

## Task 11 — cmd: warning badge, status, reset

- [x] RED: `TestOkWithWarningsBadge`, `TestFailRowIsNeverAWarning`, `TestStatusBarWarningSummary`, `TestNewAttemptClearsTheWarningBadge`
- [x] RUN-RED: `go test ./cmd/ -run 'Warning|FailRow' -v` → FAIL (bare `ok`)
- [x] GREEN: `th.warn`, `updateCell` OK branch, `statusView` bit, `delete(m.warns, a)` in `startUpdate`
- [x] RUN-GREEN + VERIFY: `go test ./...`
- [x] EVIDENCE: `evidence/warn-badge/task11-badge.txt`
- [x] COMMIT + LEDGER

## Task 12 — cmd: end to end + race

- [x] RED: `TestStderrReachesBothPanesAndTheBadge`
- [x] RUN-RED: `go test ./cmd/ -run TestStderrReaches -v`
- [x] GREEN: fix whatever mis-wiring it reveals (note the layer in TRACKING)
- [x] RUN-GREEN: `go test ./cmd/ -run TestStderrReaches -v`
- [x] VERIFY: `go test -race ./...`
- [x] EVIDENCE: `evidence/stderr/task12-e2e-model.txt` (incl. the `-race` run)
- [x] COMMIT + LEDGER

## Task 13 — frames, usability, docs

- [x] GREEN: seven new demo frames + the demo height guard; add `TestPaneKeysAreDeclaredInKeyHelp` (the existing keyHelp guards stay untouched)
- [x] RUN: `go test ./cmd/ -run 'TestDemoFrames|KeyHelp|PaneKeys' -v`; `FLEET_DEMO=1 go test ./cmd/ -run TestDemoFrames`
- [x] DOCS: `sdk/fleet/AGENTS.md` (keys + two invariants), `sdk/fleet/README.md`, cross-notes in `designs/fleet-connect.md` and `designs/sdk-tui.md`, `docs/mbo/index.md` state
- [x] VERIFY: `npx --yes markdownlint-cli2 "docs/mbo/**/fleet-error-view*.md"` (MD010 in Go snippets expected) + `make lint-go` + `go test ./...`
- [x] EVIDENCE: `evidence/demo/task13-frames.txt`
- [x] COMMIT + LEDGER

## Task 14 — live gate (HUMAN — an agent must stop here)

- [ ] Build + install: `sdk/fleet/build.sh` (from the main checkout, never a worktree)
- [ ] Run `fleet tui` against a live host and update it
- [ ] Capture: three-pane split · `h` hides hosts · `e` opens errors · real stderr in both panes · `ok ⚠N` or FAIL · `grep -c '^!! '` on the run's capture file
- [ ] File any contradiction as a TRACKING blocker (do NOT retro-fit the spec)
- [ ] EVIDENCE: `evidence/e2e/` · COMMIT + LEDGER · tick the stop condition

## Close-out

- [x] `docs/mbo/index.md` row → `in-review`
- [x] `sdk/fleet/AGENTS.md` + `README.md` document the panes and the stderr invariants
- [x] `fleet-connect` reserves `h`/`l`/`e`; its k8s actions remap to `L`/`E`
- [x] `sdk-tui` records the `h` rebinding for the phase-3 port
- [ ] All of `IMPLEMENTATION.md` §4 ticked
- [ ] `docs/mbo/index.md` row → `in-review`
- [ ] `IMPLEMENTATION.md` §8 kickoff prompt replaced with the next session's
