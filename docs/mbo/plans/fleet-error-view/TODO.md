# fleet-error-view — the cursor

> The **first unchecked box is the next action.** Micro-steps per plan task:
> RED · RUN-RED · GREEN · RUN-GREEN · VERIFY · COMMIT · LEDGER.
> Plan: [`../fleet-error-view.md`](../fleet-error-view.md) · procedure: [`IMPLEMENTATION.md`](./IMPLEMENTATION.md)

## Task 1 — runner: `SplitStreamer` on `Exec`

- [ ] RED: write `internal/runner/split_test.go` (separation, backpressure burst, merged==split, deadline)
- [ ] RUN-RED: `go test ./internal/runner/ -run 'Split|Merged' -v` → FAIL `RunSplitStreamCtx undefined`
- [ ] GREEN: add `Line`, `SplitStreamer`, `Exec.RunSplitStreamCtx`; reimplement `Exec.RunStreamCtx` on it
- [ ] RUN-GREEN: `go test ./internal/runner/ -v`
- [ ] VERIFY: `go test ./...` (nothing else moved)
- [ ] EVIDENCE: `evidence/stderr/task01-runner-split.txt`
- [ ] COMMIT + LEDGER

## Task 2 — runner: `Fake.ErrOut`

- [ ] RED: `TestFakeReplaysErrOutAsStderr`, `TestFakeMergedStreamStillCarriesErrOut`, the `var _ SplitStreamer` assertions
- [ ] RUN-RED: `go test ./internal/runner/ -run Fake -v` → FAIL `unknown field ErrOut`
- [ ] GREEN: add `ErrOut`, `Fake.RunSplitStreamCtx`; reimplement `Fake.RunStreamCtx` on it (reuse the existing line-splitting verbatim)
- [ ] RUN-GREEN + VERIFY: `go test ./...`
- [ ] EVIDENCE: `evidence/stderr/task02-fake-errout.txt`
- [ ] COMMIT + LEDGER

## Task 3 — updexec: `Console.ErrLine`

- [ ] RED: `internal/updexec/split_test.go` — `TestErrLineReceivesStderrOnly`, `TestNilErrLineRoutesStderrToLine`, `TestBatchFallsBackWhenNotSplitCapable`
- [ ] RUN-RED: `go test ./internal/updexec/ -run 'ErrLine|FallsBack' -v` → FAIL `unknown field ErrLine`
- [ ] GREEN: add `ErrLine`, `Console.emit`, the `SplitStreamer` branch in `Batch`
- [ ] RUN-GREEN + VERIFY: `go test ./internal/updexec/ -v`
- [ ] EVIDENCE: `evidence/stderr/task03-errline.txt`
- [ ] COMMIT + LEDGER

## Task 4 — updexec: mark stderr in the capture

- [ ] RED: `TestCaptureMarksStderr` (+ `recordingOut`)
- [ ] RUN-RED: `go test ./internal/updexec/ -run Capture -v` → FAIL `undefined: stderrMark`
- [ ] GREEN: `stderrMark`, `teeable.withLines`, `RunHost`'s two-callback tee (prefix applied unconditionally)
- [ ] RUN-GREEN: re-run `TestNilErrLineRoutesStderrToLine` explicitly (the tee wraps `ErrLine`)
- [ ] VERIFY: `go test ./...`
- [ ] EVIDENCE: `evidence/stderr/task04-capture-mark.txt`
- [ ] COMMIT + LEDGER

## Task 5 — updexec: `Benign()`

- [ ] RED: `stderrnoise_test.go` table (benign list + real-error list + `remote: fatal:`)
- [ ] RUN-RED: `go test ./internal/updexec/ -run Benign -v` → FAIL `undefined: Benign`
- [ ] GREEN: `stderrnoise.go` — `benignPatterns` (anchored regexps, whole-line) + `Benign()`
- [ ] RUN-GREEN: every table row passes, none commented out
- [ ] EVIDENCE: `evidence/warn-badge/task05-benign.txt`
- [ ] COMMIT + LEDGER

## Task 6 — cmd: carry the tag into the model

- [ ] RED: `cmd/tui_stderr_test.go` — `TestStderrLineIsTaggedAndCounted`, `TestAppendLogStaysStdout`
- [ ] RUN-RED: `go test ./cmd/ -run 'Stderr|AppendLog' -v` → FAIL `appendLogLine undefined`
- [ ] GREEN: `logEntry.stderr/.warn`, `appendLogLine`, `errEntries`, `m.warns`, `m.errCount`, `outLine` through `lineQueue`/`stream`/`readLine`, `beginStream` wires `Line`+`ErrLine`
- [ ] RETYPE the existing call sites: `review_test.go:194,205` and `tui_logpane_test.go:129` (compile errors otherwise; assertions unchanged)
- [ ] RUN-GREEN + VERIFY: `go test ./...` (existing log-pane suite must pass untouched)
- [ ] EVIDENCE: `evidence/stderr/task06-model-tag.txt`
- [ ] COMMIT + LEDGER

## Task 7 — cmd: pure `layout()`

- [ ] RED: `cmd/tui_layout_test.go` — `TestLayout`, `TestThreePaneSplitIsEven`, `TestHostPlusOneBottomPaneKeepsTheTopFifth`, `TestTinyViewportStillFits`, `TestSmallTerminalYieldsRatherThanOverflowing`
- [ ] RUN-RED: `go test ./cmd/ -run TestLayout -v` → FAIL `undefined: layout`
- [ ] GREEN: `cmd/tui_layout.go` (`pane`, `panes`+`count()`, `heights`, `minFrameRows`, `layout`, `splitBottom`, constants — every open panel costs `panelFixedRows` = 3)
- [ ] RUN-GREEN: fill the two `want`-less cases with real numbers, invariants unweakened
- [ ] EVIDENCE: `evidence/layout/task07-layout-unit.txt`
- [ ] COMMIT + LEDGER

## Task 8 — cmd: adopt layout, fix the overflow

- [ ] RED: `TestFrameFitsTheTerminal`, `TestChromeOverBudgetGivesThePanesNothing`, `TestPanelBodyRowIsExactlyOneLine`, `TestFrameFitsWithZeroAndOneHost`, `TestEmptyFleetStillRendersTheOtherPanes`, `settledTestModel` helper
- [ ] RUN-RED: `go test ./cmd/ -run TestFrameFits -v` → FAIL with concrete overflow lines
- [ ] **EVIDENCE (RED):** `evidence/layout/task08-before-overflow.txt` — this proves the bug existed
- [ ] GREEN: `panelInnerWidth()` + move EVERY truncation onto it (rows, the column header — untruncated today, `failWidth`, `logWidth`); `chromeRows()` (measures banner AND status — the status is a panel in modeAnswers/modeConfirm), `statusSeparatorRows`, `paneState()`, `heights()` (called ONCE per frame in `View()`), `errCount`, `listHeight`/`logHeight`/`errHeight` delegate; `View()` renders per pane and the empty-fleet early return folds into the host pane
- [ ] REWRITE: `tui_logpane_test.go:97` `TestSplitKeepsBothHalvesUsableOnASmallTerminal` → the two replacements in plan Task 8 Step 3 (it asserts today's overflow; every OTHER case must pass untouched)
- [ ] RUN-GREEN: `go test ./cmd/ -v`
- [ ] VERIFY: `go test ./...`
- [ ] EVIDENCE (GREEN): `evidence/layout/task08-after-fits.txt`
- [ ] COMMIT + LEDGER

## Task 9 — cmd: `h` / `e` keys and focus

- [ ] PRE: add a `"tab"` case to `key()` in `tui_model_test.go` (it currently sends the letters t,a,b)
- [ ] RED: `cmd/tui_panes_test.go` — defaults, host toggle+restore, error toggle vs `modeConfirm` `e`, refuse-last, focus cycling, per-pane search, no persistence
- [ ] RUN-RED: `go test ./cmd/ -run 'Pane|Focus' -v` → FAIL `hostOpen undefined`
- [ ] GREEN: model fields + `focus`/`logFocused()` shim, `visiblePanes`, `togglePane`, `cycleFocus`, `routeNormal` cases, `routeSearch` per-pane selection, error-pane motion block; `keyHelp` gains ONE `h` row and EDITS the existing `e` row (a second `e` fails `TestKeyHelpHasNoDuplicateBindings`)
- [ ] FIX the removed field's users: `tui_demo_test.go:124` (assigns `logFocus`) and `tui_reset_lognav_test.go:125,145`
- [ ] RUN-GREEN + VERIFY: `go test ./...`
- [ ] EVIDENCE: `evidence/layout/task09-keys.txt`
- [ ] COMMIT + LEDGER

## Task 10 — cmd: render the error pane

- [ ] RED: `TestErrorPaneRendersOnlyStderr`, `TestErrorPaneOpenButEmptyCollapses`
- [ ] RUN-RED: `go test ./cmd/ -run ErrorPane -v` → FAIL (no error panel in the frame)
- [ ] GREEN: `streamPane` shared renderer, `errView`, `!` gutter on stderr in the log pane, titles + empty hints
- [ ] RUN-GREEN + VERIFY: `go test ./...` incl. `TestFrameFitsTheTerminal`
- [ ] EVIDENCE: `evidence/layout/task10-errview.txt`
- [ ] COMMIT + LEDGER

## Task 11 — cmd: warning badge, status, reset

- [ ] RED: `TestOkWithWarningsBadge`, `TestFailRowIsNeverAWarning`, `TestStatusBarWarningSummary`, `TestNewAttemptClearsTheWarningBadge`
- [ ] RUN-RED: `go test ./cmd/ -run 'Warning|FailRow' -v` → FAIL (bare `ok`)
- [ ] GREEN: `th.warn`, `updateCell` OK branch, `statusView` bit, `delete(m.warns, a)` in `startUpdate`
- [ ] RUN-GREEN + VERIFY: `go test ./...`
- [ ] EVIDENCE: `evidence/warn-badge/task11-badge.txt`
- [ ] COMMIT + LEDGER

## Task 12 — cmd: end to end + race

- [ ] RED: `TestStderrReachesBothPanesAndTheBadge`
- [ ] RUN-RED: `go test ./cmd/ -run TestStderrReaches -v`
- [ ] GREEN: fix whatever mis-wiring it reveals (note the layer in TRACKING)
- [ ] RUN-GREEN: `go test ./cmd/ -run TestStderrReaches -v`
- [ ] VERIFY: `go test -race ./...`
- [ ] EVIDENCE: `evidence/stderr/task12-e2e-model.txt` (incl. the `-race` run)
- [ ] COMMIT + LEDGER

## Task 13 — frames, usability, docs

- [ ] GREEN: seven new demo frames + the demo height guard; add `TestPaneKeysAreDeclaredInKeyHelp` (the existing keyHelp guards stay untouched)
- [ ] RUN: `go test ./cmd/ -run 'TestDemoFrames|KeyHelp|PaneKeys' -v`; `FLEET_DEMO=1 go test ./cmd/ -run TestDemoFrames`
- [ ] DOCS: `sdk/fleet/AGENTS.md` (keys + two invariants), `sdk/fleet/README.md`, cross-notes in `designs/fleet-connect.md` and `designs/sdk-tui.md`, `docs/mbo/index.md` state
- [ ] VERIFY: `npx --yes markdownlint-cli2 "docs/mbo/**/fleet-error-view*.md"` (MD010 in Go snippets expected) + `make lint-go` + `go test ./...`
- [ ] EVIDENCE: `evidence/demo/task13-frames.txt`
- [ ] COMMIT + LEDGER

## Task 14 — live gate (HUMAN — an agent must stop here)

- [ ] Build + install: `sdk/fleet/build.sh` (from the main checkout, never a worktree)
- [ ] Run `fleet tui` against a live host and update it
- [ ] Capture: three-pane split · `h` hides hosts · `e` opens errors · real stderr in both panes · `ok ⚠N` or FAIL · `grep -c '^!! '` on the run's capture file
- [ ] File any contradiction as a TRACKING blocker (do NOT retro-fit the spec)
- [ ] EVIDENCE: `evidence/e2e/` · COMMIT + LEDGER · tick the stop condition

## Close-out

- [ ] All of `IMPLEMENTATION.md` §4 ticked
- [ ] `docs/mbo/index.md` row → `in-review`
- [ ] `IMPLEMENTATION.md` §8 kickoff prompt replaced with the next session's
