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
| A `updplan` (base; blocks B) | (no worker — built directly on `worktree/fleet_config`, per task instructions) | `worktree/fleet_config` | this worktree | — | in-review |
| B `updexec` (+ runner ctx; base = A; blocks D, E) | (no worker — built directly on `worktree/fleet_config`, per task instructions) | `worktree/fleet_config` | this worktree | — | in-review |
| C `featflag` + deps (base = main; blocks D) | (no worker — built directly on `worktree/fleet_config`, per task instructions) | `worktree/fleet_config` | this worktree | — | in-review |
| D `cmd` (base = B, after C merged; blocks E, F) | (no worker — built directly on `worktree/fleet_config`, per task instructions) | `worktree/fleet_config` | this worktree | — | in-review |
| E `tui` (base = D) | | | | | todo |
| F `docs` (base = D) | | | | | todo |

## 1. Task ledger

Plan §4 task numbers. Commit = the SHA carrying the plan's exact message.

### Leaf A — `internal/updplan` (tasks 1–5; gate ≥ 90 %)

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 1 Default plan is today's behaviour | done | e36175b | `go test ./internal/updplan -run 'TestDefaultPlanIsTodaysUpdate\|TestDefaultYAMLRoundTripsToDefault' -v` → PASS (2/2); `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean; `go mod tidy` no diff | adds the `gopkg.in/yaml.v3` require (IMPLEMENTATION §2 reconcile note) |
| 2 Validation table | done | 83ee2d3 | `go test ./internal/updplan -run 'TestParseRejects\|TestParseAggregatesEveryError\|TestLocalDefaults' -v` → PASS (31 subtests + 2); `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | validate.go landed with task 1 (Parse needed it to compile the round-trip); this task added the full validation table + tests over it |
| 3 Defaults merge + backoff schedule | done | 02ec25f | `go test ./internal/updplan -run 'Inherits\|InteractiveSteps\|ExplicitTimeout\|Backoff\|Jitter\|RetryOnParses' -v` → PASS (6/6); `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | merge + backoff math landed with tasks 1-2; this task added the pinning tests |
| 4 Topological order and cascade helpers | done | 620a95f | `go test -race ./internal/updplan -run 'TestOrder\|TestDependents\|TestLastStepUsing' -v -count=20` → PASS (3 tests x 20 runs, deterministic); `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | stable Kahn scan (lowest declaration index first), transitive `Dependents`, `LastStepUsing` |
| 5 `--ref` shim + path resolution | done | 98b60fd | `go test ./internal/updplan -run 'TestWithRef\|TestRepoPathResolves' -v` → PASS (7/7); `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | added coverage-closing tests (`RepoOf`, `WithRefs`, `ValidHostname`, `ValidSHA`, backoff field overrides) to clear the 90% gate; removed dead `allowEmpty` param from `parseDuration` |
| **Leaf A gate** | done | a8a55d6 | `go test -race -cover ./internal/updplan` → **93.4%** (≥ 90% ✓) | evidence/updplan/leaf-gate.txt |

### Leaf B — `internal/updexec` + `runner.RunStreamCtx` (tasks 6–16; updexec ≥ 90 %)

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 6 Byte-compat + one network call | done | 28eb946 | `go test -race ./internal/updexec/... -v` → PASS (3/3); `gofmt -l .`/`go vet ./...` clean | moves `TestUpdateMakesExactlyOneNetworkCall` (cmd copy retired in task 20) |
| 7 Remaining builders | done | 6a3b907 | `go test -race ./internal/updexec/... -v` → PASS (15/15); `gofmt -l .`/`go vet ./...` clean | moves `TestRescuePreservesUntrackedWork` |
| 8 Builders re-validate | done | f84fafc | `go test -race ./internal/updexec/... -run TestBuildersRejectUnvalidatedInput -v` → PASS | |
| 9 Runner ctx path | done | d42984f | `go test -race ./internal/runner/... -v` → PASS; stub `ssh` on a temp `PATH`, `sleep 30` killed within 1.1s via `exec.CommandContext` + `WaitDelay` (a 30s hang was reproduced and fixed — see task-09 evidence) | every `runner.Runner` test double in `cmd` grew a delegating `RunStreamCtx` |
| 10 Executor happy path | done | 49ebffd | `go test -race ./internal/updexec/... -v` → PASS (3/3 new); `gofmt -l .`/`go vet ./...` clean | exec.go lands whole (one cohesive state machine); scripted `fakeIO` + stepping clock + recorded sleep + recording Output double |
| 11 Cascade | done | 09a69e6 | `go test -race ./internal/updexec/... -run 'Cascade\|FailedStep\|OnFailure\|ExpectExit\|DependencyFailed' -v` → PASS (4/4) | tests only; cascade logic landed with exec.go in task 10 |
| 12 Sync decisions | done | af6173e | `go test -race ./internal/updexec/... -run 'TestUpdate\|MissingClone\|InProgress\|ResetMode\|UnexpectedPrecheck\|CLILocal\|ResetIsIncompatible' -v` → PASS (11/11) | migrates 4 `cmd` tests by name; tests only |
| 13 Carry & restore | done | 82ce716 | `go test -race ./internal/updexec/... -run 'Carry\|Restore\|OffBranch\|OnTarget\|Detached' -v` → PASS (14/14) | tests only, plus a small exec.go fix: failure Reason now prefers a "restore-failed" note over the bare exit-status text |
| 14 gh-auth | done | 48c3804 | `go test -race ./internal/updexec/... -run TestGhAuth -v` → PASS (8/8) | tests only |
| 15 Retry / backoff / timeout | done | ec15780 | `go test -race ./internal/updexec/... -run 'Retr\|Timeout\|Attempts\|Interactive\|NoRetry' -v` → PASS (14/14) | tests only |
| 16 Lanes + exit codes | done | 47a2667 | `go test -race ./internal/updexec/... -run 'Console\|Background\|Preamble\|ExitCode' -v` → PASS (5/5); coverage-closing tests added | `go test -race -cover ./internal/updexec ./internal/runner` → updexec **92.7%** (≥ 90% ✓) |
| **Leaf B gate** | done | 47a2667 | `go test -race -cover ./internal/updexec ./internal/runner` → updexec 92.7%, runner 57.0%; `go test -race ./...` (whole module) green; `cmd/update_test.go` unchanged (10/10 original tests present, none deleted) | evidence/updexec/leaf-gate.txt |

### Leaf C — `internal/featflag` + deps (tasks 17–18; ≥ 90 %; `gff lint` clean)

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 17 Fail-open resolution | done | 9dee309 | `go test ./internal/featflag -run 'TestResolve\|TestStatic' -v` → PASS (8/8); `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | pure package, no gff import |
| 18 gff adapter + deps + flags | done | 77008f0 | `go test ./internal/featflag -run 'TestGFF' -v` → PASS (7/7); `gofmt -l .`/`go vet ./...`/`go build ./...`/`go test -race ./...` clean; `go mod tidy` stable (snapshot→tidy→diff empty); `cd sdk/gff && go run . lint ../../.github/gff/features.yaml` → exit 0; `go run . get fleet.update.config` → `home` | binary size before 7117619 B → after 7122210 B (Δ +4591 B, +0.06%) |
| **Leaf C gate** | done | 77008f0 | `go test -race -cover ./internal/featflag` → **96.4%** (≥ 90% ✓); `gff lint` clean; `go build ./...` clean | evidence/featflag/leaf-gate.txt |

### Leaf D — `cmd` rewire (tasks 19–23; `go test -race ./...`; live `--dry-run` + one live run)

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 19 Plan loading | done | 0eb2f5d | `go test ./cmd -run 'TestLoadPlan\|TestSavedAnswers' -v` → PASS (8/8); `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | `cmd` baseline coverage before D: 60.7% (evidence/cmd/baseline-coverage.txt) |
| 20 `runUpdate` | done | ef755a5 | `go test ./cmd -run 'TestUpdateHost\|TestUpdateDefaultPlan\|TestValidRef\|TestForceIs\|TestTimeoutAndNoRetry' -v` → PASS (5/5); full `cmd` suite green; `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | migrated `TestUpdateSkipsDirtyCloneByDefault` etc. onto the executor's `state=/branch=` precheck fixture format; no test deleted (names/assertions kept) |
| 21 Report / exit / json / dry-run | done | e2db3b2 | `go test ./cmd -run 'TestReport\|TestExitCode\|TestDryRun\|TestJSONReport' -v` → PASS (5/5); `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | report/JSON/dry-run tested as pure functions (`printHostReport`, `printJSONReport`, `printDryRun`, `exitErrorForReports`) independent of cobra/runner |
| 22 `fleet update init` | done | 5f10ec7 | `go test ./cmd -run 'TestInit' -v` → PASS (3/3); `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | |
| 23 Headless run log + answers env | done | 9fe863e | `go test ./cmd -run 'TestHeadlessUpdate\|TestLocalAnswerEnv\|TestSudoSecretNever\|TestCLILaneCarriesNoStdinAtAll\|TestAnUnusableCaptureDirDoesNotBreakTheCLIRun' -v` → PASS (5/5); `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | `runUpdate` factored into `runUpdateWith(out, hosts, r)` so the whole CLI path (plan → executor → capture → report) is testable without cobra or real ssh |
| **Leaf D gate** | done | 9fe863e | `go test -race -cover ./...` → all packages ok, `cmd` **62.5%** (≥ 60.7% baseline ✓) → evidence/cmd/leaf-gate.txt; `bash sdk/fleet/build.sh && fleet version` → `fleet 0.3.1-34-g9fe863e`; live `--dry-run` (no file) → evidence/e2e/G1-dry-run-default.txt; live `--dry-run` (two-repo + gh-auth wire) → evidence/e2e/G2-dry-run-two-repos.txt; `gff set fleet.update.enabled false` → `plan: built-in default (fleet.update.enabled=false)` → evidence/e2e/G4-gff-disabled.txt; binary size after wiring 12,681,210 B (baseline 7,117,619 B; Δ +5,563,591 B / +78%) → evidence/featflag/binary-size-after-wiring.txt | G3/G6/G7/G8/G9 (real mutating runs) **not run**: every `fleet status` host reported `behind 3`, none `up-to-date`, so no host was safe for a real (non-dry-run) run under this task's constraint (no `--force`/`--reset`/`carry`, no `install.sh` unless the sync is a no-op ff) |

### Leaf E — TUI (tasks 24–26; all `tui_*` green; live transcript)

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 24 Background lane | done | (this commit) | `go test ./cmd -run 'TestSudoPreambleIsPerRunStepSession' -v` → PASS (1/1); full `cmd` suite green; `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | evidence/tui/task-24.txt; `beginStream` now builds `updexec.Executor{IO: updexec.Background{...}}` from the model's injected `plan`; `--file` flag added, `--update-ref` validated via `plan.WithRef` |
| 25 Handoff delegates to the CLI verb | done | (this commit) | `go test ./cmd -run 'TestHandoff|TestNeedsTerminal' -v` → PASS (4/4); full `cmd` suite green; `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean; `grep -n "updateScript\|unattendedUpdate\|rescueWorktree\|updateHost(" cmd/*.go` → only a comment naming the deleted symbols | evidence/tui/task-25.txt; deleted `updateScript`/`remoteUpdateScript`/`resetToFetched`/`rescueWorktree`/`unattendedUpdate`/`updateHost`/`UpdateResult`; `interactiveHandoff` self-execs `<self> update <alias> [--file][--ref][--reset]` |
| 26 Flags + status text | done | (this commit) | `go test ./cmd -run 'TestResolveTUIPlanAppliesUpdateRef|TestUpdatingStatusNamesThePlan|TestDemoFrames' -v` → PASS (3/3); full `cmd` suite green; `gofmt -l .`/`go vet ./...`/`go test -race ./...` clean | evidence/tui/task-26.txt; `resolveTUIPlan` (loadPlan + plan.WithRef) extracted from `tui.go`'s RunE; status line now `updating N host(s) (plan: <Source>)` via `planLabel` |
| 27a Leaf D review cleanups | done | (this commit) | `go test -race ./...` → all ok; `go test -race -cover ./cmd` → 63.0% (>= 60.7% baseline) | evidence/tui/task-27a.txt; not part of the original plan's leaf E task list — added per the leaf E worker's extended brief (5 cmd/updexec cleanups); notes a pre-existing flaky test (TestRescueOffBranchRestoresTheBranchWithoutAStash) found but not fixed (out of scope) |
| **Leaf E gate** | done | — | `go test -race -cover ./...` → cmd 63.0%; see evidence/tui/leaf-gate.txt | G5 (live TUI transcript) left for the operator |

### Leaf F — docs (task 27; links resolve)

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 27 AGENTS.md / README / sample plan / index | done | 91ce688 | AGENTS.md invariants (34 cited tests verified by grep) · README `fleet update` rewrite + real redacted `--dry-run` demo · `opt/etc/fleet/fleet.yaml` tracked (`git ls-files`, already under `!opt/**`) · index → in-review | umask-002 gotcha: a fresh clone makes the repo plan 664 and `fleet` refuses it until `chmod g-w` (documented in README/AGENTS) |
| **Leaf F gate** | todo | — | link check → ; README demo is real output → | |

## 2. Feature → proof matrix (from spec §5)

Automated = the named tests green (tick with the task that proved them). Human/live = the
spec §6 gate transcript under `evidence/e2e/` (G5: `evidence/tui/`). `—` = no human proof required.

| Feature | Automated proof (spec §5 tests) | Automated | Human/live proof | Human | Notes |
| :-- | :-- | :-- | :-- | :-- | :-- |
| F1 defaults | `TestDefaultPlanIsTodaysUpdate`, `TestDefaultYAMLRoundTripsToDefault` (task 1) | [ ] | G1 no-file `--dry-run` equals today's commands | [ ] | |
| F1 validation | `TestParseRejects`, `TestParseAggregatesEveryError` (task 2) | [ ] | `--dry-run` on a plan with branch `main;id` → rejected (plan §7 adversarial) | [ ] | |
| F1 retry/timeout merge | `TestStepInheritsDefaultsFieldByField`, `TestInteractiveStepsDefaultToNoTimeout` (task 3) | [ ] | — | [ ] | |
| F2 plan resolution | `TestLoadPlan*` (task 19), `TestResolveDefaultsWhenSourceErrors` (task 17) | [x] | G4 `gff set fleet.update.enabled false` ⇒ built-in source named on line 1 | [x] | |
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
| F8 CLI | `TestReportNamesEveryStepAndTheLog`, `TestDryRunSendsNothing` (task 21), `TestForceIsAnAliasForLocalRescue` (task 20) | [x] | G1, G2 report transcripts | [x] G1 only; G2 is wire-only (dry-run), no live cascade run performed | |
| F9 gff flags | `TestResolveHonoursDisabled`, `TestResolveUnknownKeyIsFailOpen` (task 17), `gff lint` clean (task 18) | [x] | G4 | [x] | |
| F10 headless run log | `TestHeadlessUpdateIsCaptured` (task 23), `TestAttemptHeaderIsWrittenToTheCapture` (task 10) | [x] | G2 / G9 `log: <capture path>` line + attempt markers in the capture | [ ] | no live run performed this pass (see leaf D gate note) |
| F11 TUI | `TestNeedsTerminalRoutesToInteractiveQueue`, `TestHandoffEnvNeverCarriesTheSecret` (task 25) | [ ] | G5 TUI with one background + one interactive host | [ ] | |

## 3. Validation done-when — the stop condition

Leaf gates (plan §4 / §6.1):

- [x] Leaf A: tasks 1–5 done; `go test -race -cover ./internal/updplan` ≥ 90 % (observed: 93.4 %, 93.9 % after the review fixes) → `evidence/updplan/leaf-gate.txt`
- [ ] Leaf B: tasks 6–16 done; `go test -race -cover ./internal/updexec ./internal/runner` — updexec ≥ 90 % (observed: __ %); moved invariant tests green; mux invariant green → `evidence/updexec/leaf-gate.txt`
- [x] Leaf C: tasks 17–18 done; featflag ≥ 90 % (observed: 96.4 %); `cd sdk/gff && go run . lint ../../.github/gff/features.yaml` clean; `go build ./...` → `evidence/featflag/leaf-gate.txt`
- [x] Leaf D: tasks 19–23 done; `go test -race ./...` green; `cmd` coverage ≥ baseline (before: 60.7 % / after: 62.5 %); `bash sdk/fleet/build.sh` installs; live `--dry-run` ×2 done (no host was `up-to-date`, so the one live host run was skipped — see leaf D gate note) → `evidence/cmd/`, `evidence/e2e/`
- [ ] Leaf E: tasks 24–26 done; every `tui_*` test green; live TUI transcript → `evidence/tui/`
- [ ] Leaf F: task 27 done; every relative link resolves; README console demo is real output → `evidence/docs/`, `evidence/demo/`
- [ ] Repo floor `fleet=60` in `scripts/test.sh` unchanged; `./scripts/test.sh` green
- [ ] No test opens a socket / reads `$HOME` / touches `~/.config/gff` (grep for `os.Getenv("HOME")`, `net.Dial`, `.config/gff` in `*_test.go` → none)
- [ ] Every §2 matrix row: automated ticked; human ticked where a gate is named

Human-evidenced gates (spec §6; transcripts under `evidence/e2e/G<n>-*.txt`, hostnames scrubbed to `<host>`):

- [x] G1 no-file `fleet update <host> --dry-run` equals today's commands (diff against the pre-change `updateScript` string) → `evidence/e2e/G1-dry-run-default.txt`
- [x] G2 (wire only) two-repo + gh-auth `--dry-run` shows the plan/steps/scripts correctly → `evidence/e2e/G2-dry-run-two-repos.txt`; the LIVE failing-`make` cascade run was **not** performed (no `up-to-date` host available and this pass's constraints forbid a mutating run on a `behind` host)
- [ ] G3 gh-auth on an already-authenticated host makes zero interactive calls — SKIPPED (no live run performed)
- [x] G4 `gff set fleet.update.enabled false` ⇒ first output line names the built-in source (then `gff unset`) → `evidence/e2e/G4-gff-disabled.txt`
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
| 2026-09-04 | build-2 | Leaf A code review (`/code-review 71f2f97..HEAD medium`): 10 findings confirmed, all fixed TDD-first in `internal/updplan/review_test.go` (ValidRef option-lookalikes, WithRef re-validation, root validation, strict `exit:<n>` tokens, float-space backoff cap + NaN/Inf, tag heuristic removed, Default() deep copy, hostname gh-auth-only, no kind→run error cascade, empty/multi-document files). Coverage 93.9 %. Ledger placeholders backfilled. Leaf C reviewed by gate only (small). Leaf B dispatched. |
| 2026-09-04 | build-4 | Leaf D (`cmd` rewire, tasks 19–23) built directly on `worktree/fleet_config`, one commit per task (0eb2f5d, ef755a5, e2db3b2, 5f10ec7, 9fe863e), each with a genuine captured RED (compile-failure or assertion-failure) before GREEN. `runUpdate`/`updateHost` now drive `updexec.Executor`; the 5 migrated `TestUpdate*` tests were rewritten onto the executor's `state=/branch=` precheck fixture format (names/assertions kept, none deleted); `runUpdate` factored into `runUpdateWith(out, hosts, r)` for full-pipeline testability. Leaf D gate: `go test -race -cover ./...` all green, `cmd` 62.5% (≥ 60.7% baseline); build + `fleet version` OK; G1/G4 live-evidenced against a real host (`--dry-run`, hostnames redacted); G2 wire-only (two-repo + gh-auth `--dry-run`); binary size after wiring 12,681,210 B (baseline 7,117,619 B, Δ +78%). G3/G6/G7/G8/G9 (real mutating runs) skipped: every fleet host was `behind 3`, none `up-to-date`, and the task's constraints forbid a mutating run on a behind host. `status.go`'s `remoteRepo` was left as the `~/git/dotfiles` constant (a TODO comment points at plan §2) — deriving it from the loaded plan is deferred, noted as a deviation rather than silently skipped. |
| 2026-09-04 | build-3 | Leaf C code review (`/code-review 3191aa0..77008f0 medium`): 10 findings. Fixed TDD-first in `internal/featflag/review_test.go`: adapter now scopes to `gff.WithSource(<--repo path>)` (live file, cwd-independent) instead of the namespace snapshot; `Strings` returns option IDs (`gff.Selected`); typed-nil `*GFF` fail-open; >1 selection and non-absolute repoDir → Note + ""; the TMPDIR-dependent real-SDK test replaced by a `git init` fixture repo with pinned non-default flags. Plan §3.5 / spec §3 updated (contract change recorded here, not silently). Ledger caveats acknowledged: leaf C's RUN-RED boxes were ticked without a captured FAIL line (the tests were written first but the agent recorded only the GREEN run) and the +4.6 KB binary delta predates wiring — re-measured at leaf D. |
| 2026-09-04 | build-4 | Leaf B code review (`/code-review 77008f0..HEAD medium`): 10 findings, all fixed TDD-first in `6ba4d53` (`updexec/review_test.go`, `runner/review_test.go`): reset in multi/default forms targets `origin/$b1`; default-form fetch failure is fatal; carry notes survive a retry; rescue handles detached HEAD / nothing-to-commit; disabled restore emits no result (host no longer reported failed); `--no-restore` honoured on the immediate path; extras loop failure flag; interactive deadline via optional `RunInteractiveCtx` (no goroutine race); nil `Rand` → real jitter; a successful attempt is never `timed out`. updexec 93.6 %. Leaf D landed (`0eb2f5d..29b1e75`): cmd 62.5 % (baseline 60.7 %); binary after wiring 12,681,210 B (+5.56 MB, +78 % — the real gff SDK link cost; **follow-up candidate:** swap the adapter for a `gff get` shell-out behind the same `Source` interface). Live gates: G1/G2(wire)/G4 done; G2-live/G3/G6–G9 pending — every host is `behind 3`, so the mutating runs wait for the operator. Whole-module `go test -race ./...` green at `6ba4d53`. |
| 2026-09-05 | build-5 | Rate limit killed E/F/review-D on 09-04 with a clean tree; re-dispatched. Leaf F landed (`91ce688`). Leaf E landed (`1fc6f74`, `417f97a`, `d37a98b`): TUI background lane runs the executor (sudo preamble on run steps only), interactive handoff self-execs `fleet update` (answers via env, never the secret), `--file`/plan-aware status; deleted from cmd: `updateScript`, `remoteUpdateScript`, `resetToFetched`, `rescueWorktree`, `unattendedUpdate`, `updateHost`, `UpdateResult`. Leaf D review cleanups `232806e` (`Executor.Scripts` for dry-run, `Result.MaxAttempts`, single capture, `fleetConfigFile`, simpler `Console.Interactive`); item (a) partial — the executor's run paths still call the builders directly rather than through `Scripts`. cmd 63.0 %, updexec 91.9 %. G5 pending on the operator. Final D+E correctness review running. |
| 2026-09-05 | build-6 | Final D+E code-review findings (10, all CONFIRMED) closed TDD-first, RED captured per finding then reverted/restored (`docs/mbo/plans/fleet-update/evidence/cmd/review-fixes-final.txt`): (1) `Console.runScript`'s `" && "` join broke every preamble already terminated with `"; "` — now prepends verbatim, producers terminate their own text; (2) `Background.Interactive` unconditionally refused every interactive step, so the shipped `interactive:true` install step never ran unattended — now a `KindRun` step routes through `Console.Batch`, only gh-auth still needs a terminal; (3) `extrasScript`'s bare `fail=0; for …` let the loop run after a failed merge and masked its exit code — wrapped in `{ …; }` as one `&&`-chained unit; (4) `fleet update --json` exited 0 on a failed host — `exitErrorForReports` now runs regardless of `--json`; (5) a gh-auth login needing a terminal no longer matched `HostReport.NeedsTerminal()`'s string check — added a typed `Result.NeedsTerminal`; (6) bare `fleet`/`fleet tui` silently forced every plan onto `main` via an unconditional `WithRef` — `--update-ref` now defaults to `""` and is applied only when set; (7) a headless run's capture held only step banners, never the remote's own output — `Executor.RunHost` now tees every lane's Batch output into the capture itself (removed the now-redundant manual tee in `beginStream`); (8) the TUI's `Line` callback could block the executor on a full channel behind a suspended event loop — replaced with an unbounded `lineQueue` (mutex/cond, non-blocking push, a forwarder goroutine feeds the existing `<-chan string` API so no consumer-side test changed); (9) the interactive handoff never forwarded `--repo`, resolving gff/the plan against the wrong checkout — now always forwarded, `tuiModel` gained a `repo` field; (10) `planLabel` truncated a long Source from the front (dropping the `built-in default` marker) and could split a rune — now elides the middle, rune-safe. Also folded three copies of `WINSETUP_ANSWER`/`GEMINI_TEARDOWN_ANSWER` into one const pair and aliased cmd's `shQuote` to `updexec.ShQuote`. Gate: `gofmt -l .` clean, `go vet ./...` clean, `go test -race -cover ./...` all green — cmd 63.3 % (floor 63.0 %), updexec 92.5 % (floor 90.0 %). Rebuilt (`bash sdk/fleet/build.sh`) and re-verified `fleet update <host> --dry-run` against a live `#fleet` alias (hostname redacted) — unchanged, correct output. Landed in this session's single commit (see `git log` for the SHA). |
