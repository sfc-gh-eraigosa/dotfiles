# fleet-update — live state ledger

- **Slug:** fleet-update
- **Started:** 2026-09-02
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../fleet-update.md`](../fleet-update.md) · spec [`../../specs/fleet-update.md`](../../specs/fleet-update.md)
- **Objective anchors:** issue [#265](https://github.com/sfc-gh-eraigosa/dotfiles/issues/265) · PR [#270](https://github.com/sfc-gh-eraigosa/dotfiles/pull/270) · `docs/mbo/index.md` row `fleet-update`

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result (e.g.
> `go test -race -cover ./internal/updplan -> ok, 93.1%`). A row is `done` only with a
> commit SHA **and** evidence. Never write a result you did not observe.
> Evidence files live under `./evidence/<leaf>/task-NN.txt` (dated header, append-only).

## 0. Worker registry

Fill in from the `gss feature worker add --json` output — **verbatim**, never reconstructed.
Leaf state vocabulary (mirrors `docs/mbo/index.md`): `todo → building → in-review → merged`.

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| A `updplan` (base; blocks B) | | | | | todo |
| B `updexec` (+ runner ctx; base = A; blocks D, E) | | | | | todo |
| C `featflag` + deps (base = main; blocks D) | | | | | todo |
| D `cmd` (base = B, after C merged; blocks E, F) | | | | | todo |
| E `tui` (base = D) | | | | | todo |
| F `docs` (base = D) | | | | | todo |

## 1. Task ledger

Plan §4 task numbers. Commit = the SHA carrying the plan's exact message.

### Leaf A — `internal/updplan` (tasks 1–5; gate ≥ 90 %)

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 1 Default plan is today's behaviour | done | e36175b | `go test ./internal/updplan -run 'TestDefaultPlanIsTodaysUpdate\|TestDefaultYAMLRoundTripsToDefault' -v` → PASS (2/2); `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean; `go mod tidy` no diff | adds the `gopkg.in/yaml.v3` require (IMPLEMENTATION §2 reconcile note) |
| 2 Validation table | done | (this commit) | `go test ./internal/updplan -run 'TestParseRejects\|TestParseAggregatesEveryError\|TestLocalDefaults' -v` → PASS (31 subtests + 2); `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | validate.go landed with task 1 (Parse needed it to compile the round-trip); this task added the full validation table + tests over it |
| 3 Defaults merge + backoff schedule | done | (this commit) | `go test ./internal/updplan -run 'Inherits\|InteractiveSteps\|ExplicitTimeout\|Backoff\|Jitter\|RetryOnParses' -v` → PASS (6/6); `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | merge + backoff math landed with tasks 1-2; this task added the pinning tests |
| 4 Topological order and cascade helpers | done | (this commit) | `go test -race ./internal/updplan -run 'TestOrder\|TestDependents\|TestLastStepUsing' -v -count=20` → PASS (3 tests x 20 runs, deterministic); `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | stable Kahn scan (lowest declaration index first), transitive `Dependents`, `LastStepUsing` |
| 5 `--ref` shim + path resolution | todo | | | |
| **Leaf A gate** | todo | — | `go test -race -cover ./internal/updplan` → | |

### Leaf B — `internal/updexec` + `runner.RunStreamCtx` (tasks 6–16; updexec ≥ 90 %)

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 6 Byte-compat + one network call | todo | | | moves `TestUpdateMakesExactlyOneNetworkCall` (cmd copy retired in task 20) |
| 7 Remaining builders | todo | | | moves `TestRescuePreservesUntrackedWork` |
| 8 Builders re-validate | todo | | | |
| 9 Runner ctx path | todo | | | stub `ssh` on a temp `PATH` |
| 10 Executor happy path | todo | | | scripted `fakeIO` + stepping clock + recorded sleep |
| 11 Cascade | todo | | | |
| 12 Sync decisions | todo | | | migrates 4 `cmd` tests by name |
| 13 Carry & restore | todo | | | |
| 14 gh-auth | todo | | | |
| 15 Retry / backoff / timeout | todo | | | |
| 16 Lanes + exit codes | todo | | | |
| **Leaf B gate** | todo | — | `go test -race -cover ./internal/updexec ./internal/runner` → | |

### Leaf C — `internal/featflag` + deps (tasks 17–18; ≥ 90 %; `gff lint` clean)

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 17 Fail-open resolution | todo | | | |
| 18 gff adapter + deps + flags | todo | | | record binary size before/after; `go mod tidy` no diff |
| **Leaf C gate** | todo | — | `go test -race -cover ./internal/featflag` → ; `go run . lint` → ; `go build ./...` → | |

### Leaf D — `cmd` rewire (tasks 19–23; `go test -race ./...`; live `--dry-run` + one live run)

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 19 Plan loading | todo | | | `cmd` baseline coverage before D: (fill from IMPLEMENTATION §1.6; 60.7 % at planning) |
| 20 `runUpdate` | todo | | | retires the `cmd` copies of the moved/migrated tests; names kept |
| 21 Report / exit / json / dry-run | todo | | | |
| 22 `fleet update init` | todo | | | |
| 23 Headless run log + answers env | todo | | | |
| **Leaf D gate** | todo | — | `go test -race ./...` → ; `bash sdk/fleet/build.sh` → ; live `--dry-run` (no file) → ; live `--dry-run` (two-repo) → ; live run → | `cmd` coverage after D: |

### Leaf E — TUI (tasks 24–26; all `tui_*` green; live transcript)

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 24 Background lane | todo | | | |
| 25 Handoff delegates to the CLI verb | todo | | | deletes the `updateScript` wrapper |
| 26 Flags + status text | todo | | | |
| **Leaf E gate** | todo | — | `go test -race ./cmd` → ; live TUI transcript → | |

### Leaf F — docs (task 27; links resolve)

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 27 AGENTS.md / README / sample plan / index | todo | | | `!opt/etc/fleet/**` allowlist rule if the sample ships |
| **Leaf F gate** | todo | — | link check → ; README demo is real output → | |

## 2. Feature → proof matrix (from spec §5)

Automated = the named tests green (tick with the task that proved them). Human/live = the
spec §6 gate transcript under `evidence/e2e/` (G5: `evidence/tui/`). `—` = no human proof required.

| Feature | Automated proof (spec §5 tests) | Automated | Human/live proof | Human | Notes |
| :-- | :-- | :-- | :-- | :-- | :-- |
| F1 defaults | `TestDefaultPlanIsTodaysUpdate`, `TestDefaultYAMLRoundTripsToDefault` (task 1) | [ ] | G1 no-file `--dry-run` equals today's commands | [ ] | |
| F1 validation | `TestParseRejects`, `TestParseAggregatesEveryError` (task 2) | [ ] | `--dry-run` on a plan with branch `main;id` → rejected (plan §7 adversarial) | [ ] | |
| F1 retry/timeout merge | `TestStepInheritsDefaultsFieldByField`, `TestInteractiveStepsDefaultToNoTimeout` (task 3) | [ ] | — | [ ] | |
| F2 plan resolution | `TestLoadPlan*` (task 19), `TestResolveDefaultsWhenSourceErrors` (task 17) | [ ] | G4 `gff set fleet.update.enabled false` ⇒ built-in source named on line 1 | [ ] | |
| F3 one fetch | `TestUpdateMakesExactlyOneNetworkCall` (moved), `TestEverySyncFormMakesAtMostOneUnconditionalNetworkCall` (task 6) | [ ] | G1 (the dry-run wire shows one `git fetch`) | [ ] | |
| F3 byte-compat | `TestSyncScriptSingleBranchMatchesTodaysForm` (task 6) | [ ] | G1 | [ ] | |
| F3 multi-branch | `TestMultiBranchFetchesAllInOneCall`, `TestExtrasOnlyForceMoveAnAncestor` (task 7) | [ ] | G2 two-repo live run | [ ] | |
| F3 local policy | `TestUpdateSkipsDirtyCloneByDefault`, `TestForceRescuesDirtyWorkBeforePulling`, `TestCarryStashesWithUntrackedAndCapturesTheSHA` (tasks 12, 13) | [ ] | G7 carry round-trip | [ ] | G7 licenses amending the "never stash" invariant (task 27) |
| F4 restore | `TestCleanOffBranchIsRestoredUnderEveryPolicy`, `TestRestoreRunsEvenWhenAnIntermediateStepFailed`, `TestRestoreConflictKeepsTheStash`, `TestRestoreRejectsUnvalidatedOrigOrSHA` (tasks 7, 13) | [ ] | G6 clean feature-branch round-trip; G8 carry conflict keeps the stash | [ ] | |
| F5 run step | `TestRunScriptIsVerbatimAfterCd` (task 7), `TestPreambleAndStdinApplyToRunStepsOnly` (task 16) | [ ] | G2 (`scripts.make` exits 2 → cascade) | [ ] | |
| F6 gh-auth | `TestGhAuthSkipsLoginWhenStatusPasses`, `TestGhAuthNeverCarriesAToken`, `TestGhAuthWithoutATerminalFailsCleanly` (tasks 7, 14) | [ ] | G3 authenticated host ⇒ zero interactive calls | [ ] | |
| F7 cascade | `TestFailedStepSkipsTransitiveDependents`, `TestOnFailureContinueLetsDependentsRunButStillFailsTheHost` (task 11) | [ ] | G2 | [ ] | |
| F7 retry | `TestTransportFailureIsRetriedWithBackoff`, `TestExpectedExitIsNeverRetried`, `TestInteractiveStepsAreNeverRetried` (task 15), `TestBackoffScheduleIsExponentialAndCapped` (task 3) | [ ] | G9 forced transport failure retried with logged backoff | [ ] | |
| F7 timeout | `TestTimeoutCancelsTheAttempt`, `TestInteractiveHasNoDeadlineUnlessSet` (task 15), `TestRunStreamCtxKillsTheChildOnDeadline` (task 9) | [ ] | — (stub-driven; plan §7 "a stub that never exits") | [ ] | |
| F8 CLI | `TestReportNamesEveryStepAndTheLog`, `TestDryRunSendsNothing` (task 21), `TestForceIsAnAliasForLocalRescue` (task 20) | [ ] | G1, G2 report transcripts | [ ] | |
| F9 gff flags | `TestResolveHonoursDisabled`, `TestResolveUnknownKeyIsFailOpen` (task 17), `gff lint` clean (task 18) | [ ] | G4 | [ ] | |
| F10 headless run log | `TestHeadlessUpdateIsCaptured` (task 23), `TestAttemptHeaderIsWrittenToTheCapture` (task 10) | [ ] | G2 / G9 `log: <capture path>` line + attempt markers in the capture | [ ] | |
| F11 TUI | `TestNeedsTerminalRoutesToInteractiveQueue`, `TestHandoffEnvNeverCarriesTheSecret` (task 25) | [ ] | G5 TUI with one background + one interactive host | [ ] | |

## 3. Validation done-when — the stop condition

Leaf gates (plan §4 / §6.1):

- [ ] Leaf A: tasks 1–5 done; `go test -race -cover ./internal/updplan` ≥ 90 % (observed: __ %) → `evidence/updplan/leaf-gate.txt`
- [ ] Leaf B: tasks 6–16 done; `go test -race -cover ./internal/updexec ./internal/runner` — updexec ≥ 90 % (observed: __ %); moved invariant tests green; mux invariant green → `evidence/updexec/leaf-gate.txt`
- [ ] Leaf C: tasks 17–18 done; featflag ≥ 90 % (observed: __ %); `cd sdk/gff && go run . lint ../../.github/gff/features.yaml` clean; `go build ./...` → `evidence/featflag/leaf-gate.txt`
- [ ] Leaf D: tasks 19–23 done; `go test -race ./...` green; `cmd` coverage ≥ baseline (before: __ % / after: __ %); `bash sdk/fleet/build.sh` installs; live `--dry-run` ×2 + one live host run → `evidence/cmd/`, `evidence/e2e/`
- [ ] Leaf E: tasks 24–26 done; every `tui_*` test green; live TUI transcript → `evidence/tui/`
- [ ] Leaf F: task 27 done; every relative link resolves; README console demo is real output → `evidence/docs/`, `evidence/demo/`
- [ ] Repo floor `fleet=60` in `scripts/test.sh` unchanged; `./scripts/test.sh` green
- [ ] No test opens a socket / reads `$HOME` / touches `~/.config/gff` (grep for `os.Getenv("HOME")`, `net.Dial`, `.config/gff` in `*_test.go` → none)
- [ ] Every §2 matrix row: automated ticked; human ticked where a gate is named

Human-evidenced gates (spec §6; transcripts under `evidence/e2e/G<n>-*.txt`, hostnames scrubbed to `<host>`):

- [ ] G1 no-file `fleet update <host> --dry-run` equals today's commands (diff against the pre-change `updateScript` string)
- [ ] G2 two-repo + gh-auth live run with a failing `make` showing the cascade (`failed` → `dependency-failed "blocked by …"`, sibling chain completes)
- [ ] G3 gh-auth on an already-authenticated host makes zero interactive calls
- [ ] G4 `gff set fleet.update.enabled false` ⇒ first output line names the built-in source (then `gff unset`)
- [ ] G5 TUI `u` with one background host + one interactive host (routed to the interactive queue) — `evidence/tui/`
- [ ] G6 clean feature-branch round-trip: host on `feature/x` clean → updated → back on `feature/x`
- [ ] G7 carry round-trip: tracked + untracked changes restored, `git stash list` empty
- [ ] G8 carry conflict keeps the stash and the report names `stash=<sha> branch=<orig>`
- [ ] G9 forced transport failure (ssh exits 255 twice) retried with logged backoff, `attempt 3/3` in the report

Closeout:

- [ ] `docs/mbo/index.md` row `fleet-update` state `merged`; issue [#265](https://github.com/sfc-gh-eraigosa/dotfiles/issues/265) closed after all six leaves land
- [ ] IMPLEMENTATION §8 prompt replaced by the next session's (or a closeout note) at the end of every session

## 4. Blockers & escalations

Failing command + its **real** output. Contract defects (plan §3) go here and get escalated —
never silently patched.

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-09-02 | planning | planning: design/spec/plan/trio written (`designs/`, `specs/`, `plans/fleet-update.md`, this trio, empty evidence tree); no worker created yet; issue/PR numbers still #TBD |
| 2026-09-04 | build-1 | Build kicked off on the design PR's own branch (single PR #270 — no gss feature workers; §0 registry rows are N/A). Baselines captured: cmd coverage 60.7%, binary size in evidence/featflag/. Leaf A dispatched. |
