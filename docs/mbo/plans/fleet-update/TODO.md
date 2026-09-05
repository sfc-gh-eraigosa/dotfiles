# fleet-update — execution cursor

- **Slug:** fleet-update
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../fleet-update.md`](../fleet-update.md) — every task/§ reference points there

> **How to use:** the **first unchecked box is the next action**. Tick a box only after
> you ran the command and read the output. After finishing a `###` task: update
> `TRACKING.md`, commit with the plan's exact message, checkpoint.
>
> **Legend:** `SETUP` prep · `RED` write a failing test · `RUN-RED` run it, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run it, expect PASS · `VERIFY` extra gate ·
> `ALLOWLIST` `.gitignore` check · `DOCS` · `COMMIT` · `LEDGER` update TRACKING.md ·
> `CHECKPOINT` `gss feature checkpoint --worker <ref>`.
>
> **Paths:** `<WT>` = the leaf's worktree path captured from `gss feature worker add --json`.
> Go commands run from `<WT>/sdk/fleet/`. Evidence: `tee -a <WT>/docs/mbo/plans/fleet-update/evidence/<leaf>/task-NN.txt`
> (prefix each capture with a `date -u` header). `VERIFY` on every task means:
> `gofmt -l . && go vet ./... && go test -race ./...` → gofmt prints nothing, both others exit 0.

## Preflight (once)

- [x] (N/A — building on the design PR's own branch, #270) Design PR merged: `git -C "$HOME/git/dotfiles" fetch origin && git -C "$HOME/git/dotfiles" show origin/main:docs/mbo/plans/fleet-update.md | head -3`
- [x] Toolchain (go1.26.3, gh, gss present): `go version` matches `.go-version`; `git --version`; `gh auth status`; `gss feature list`
- [x] (N/A — single PR #270, no gss feature workers; leaf SETUP boxes below are skipped) Feature registered: `gss feature list | grep -A3 'fleet-update'` — else `gss feature start fleet-update --goal "config-driven multi-repo update DAG for fleet update (#265)"`
- [x] Read plan §3 (frozen contracts) and §6.1 (DAG) in full; read spec §4 F1–F11
- [x] Baseline coverage recorded (cmd 60.7%): `go test -cover ./cmd/ | tee evidence/cmd/baseline-coverage.txt`
- [x] Baseline binary size recorded: `bash sdk/fleet/build.sh && ls -l "$HOME/opt/bin/fleet" | tee evidence/featflag/binary-size-before.txt`

---

# Leaf A — `updplan` (BLOCKING — base for B and D)

## Leaf A setup
- [ ] SETUP: `gss feature worker add --feature fleet-update --purpose updplan --engine claude --json --description "internal/updplan: fleet.yaml schema, validation, topo order (plan tasks 1-5)"`
- [ ] SETUP: record `worker_ref` / `branch` / `worktree_path` / `base_branch` **verbatim** in `TRACKING.md` §0 and `IMPLEMENTATION.md` §2.1
- [ ] SETUP: `cd <WT>/sdk/fleet && go build ./... && go test ./...` green before any change

### Task 1 — built-in default plan equals today's update  (plan Task 1, leaf A)
- [x] RED: `internal/updplan/plan_test.go` — `TestDefaultPlanIsTodaysUpdate` (root `~/git`; repo `dotfiles` path `~/git/dotfiles` branches `[main]` local `skip` restore `true`; steps exactly `dotfiles.sync` → `dotfiles.install` run `./install.sh` interactive needs `[dotfiles.sync]`), `TestDefaultYAMLRoundTripsToDefault` (`Parse(DefaultYAML)` deep-equals `Default()` modulo `Source`)
- [x] RUN-RED: `go test ./internal/updplan -run 'TestDefaultPlanIsTodaysUpdate|TestDefaultYAMLRoundTripsToDefault' -v` → expect **FAIL** (package does not compile / tests fail)
- [x] GREEN: `plan.go` — types (§3.2), `Default()`, `DefaultYAML`, minimal `Parse` (yaml.v3 `KnownFields(true)`) enough for the round trip; add `gopkg.in/yaml.v3` to `go.mod` (`go get gopkg.in/yaml.v3@v3.0.1 && go mod tidy`)
- [x] RUN-GREEN: same command → expect **PASS**
- [x] VERIFY: gofmt/vet/race; `go mod tidy` leaves no diff
- [x] COMMIT: `feat(fleet/updplan): built-in default plan equals today's update`
- [x] LEDGER + CHECKPOINT

**Done when:** both tests pass; `Default()` describes exactly today's `~/git/dotfiles` → `main` → `./install.sh` flow.

### Task 2 — parse + aggregated validation  (plan Task 2, leaf A)
- [x] RED: `validate_test.go` — `TestParseRejects` table (every rule listed in plan §3.2 "Validation rules": unknown key, `version: 2`, duplicate id, unknown kind, sync w/o repo, unknown repo, gh-auth with repo, run w/o `run:`, NUL/newline in run, `interactive` on sync, `retry` on interactive, unknown/self need, cycle `a→b→a`, `default` not first, tag + extras, duplicate branch, `expect.exit: [256]`, bad `on_failure`, bad `local`, `attempts: 0`, unknown `retry.on` token, `factor: 0.5`, `timeout: soon`, negative timeout; injection vectors `main; rm -rf ~`, `$(id)`, path `../x`, `~/git; id`, `-rf`, url with `'`) — each asserts the error names the step/repo + field; `TestParseAggregatesEveryError` (two independent faults → both named); `TestLocalDefaultsToSkipAndRestoreToTrue`
- [x] RUN-RED: `go test ./internal/updplan -run 'TestParseRejects|TestParseAggregatesEveryError|TestLocalDefaults' -v` → expect **FAIL**
- [x] GREEN: `validate.go` — `ValidRef` (moved verbatim from `cmd/update.go`), `ValidPath`, `ValidRepoName`, `ValidID`, `ValidURL`, `ValidHostname`, `ValidSHA`; full `Parse` pipeline decode → defaults → validate (collect with `errors.Join`) → resolve paths
- [x] RUN-GREEN: same command → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/updplan): parse + aggregated validation`
- [x] LEDGER + CHECKPOINT

**Done when:** every table row fails for the stated reason and a two-fault file reports both.

### Task 3 — retry/timeout defaults merge + backoff schedule  (plan Task 3, leaf A)
- [x] RED: `TestStepInheritsDefaultsFieldByField` (step `retry: {attempts: 3}` keeps default `on`/`backoff`), `TestInteractiveStepsDefaultToNoTimeout`, `TestExplicitTimeoutOnInteractiveStepIsKept`, `TestBackoffScheduleIsExponentialAndCapped` (`Initial 5s Factor 2 Max 2m`, rnd = 0.5 → `5s,10s,20s,40s,80s,2m,2m`), `TestJitterStaysWithinHalfOfTheWait` (rnd 0 → 0.5×, rnd 1 → 1.5×), `TestRetryOnParsesExitCodes` (`[75, transport]` → `exit:75`, `transport`)
- [x] RUN-RED: `go test ./internal/updplan -run 'Inherits|InteractiveSteps|ExplicitTimeout|Backoff|Jitter|RetryOnParses' -v` → expect **FAIL**
- [x] GREEN: `Defaults`, `Retry`, `Backoff.Wait`, merge in `Parse`
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/updplan): retry/timeout defaults merge and backoff schedule`
- [x] LEDGER + CHECKPOINT

**Done when:** the schedule test prints the exact sequence and interactive steps carry `Timeout 0` unless set.

### Task 4 — stable topological order  (plan Task 4, leaf A)
- [x] RED: `graph_test.go` — `TestOrderIsTopologicalAndStable` (two independent chains interleave by declaration index; every need precedes its dependent), `TestDependentsIsTransitive` (`a→b→c`, `a→d`: `Dependents("a") == [b c d]` in `Order()` order; `Dependents("c") == []`), `TestLastStepUsingRepo` (`r.sync → r.build → other.sync` → `LastStepUsing("r") == "r.build"`)
- [x] RUN-RED: `go test ./internal/updplan -run 'TestOrder|TestDependents|TestLastStepUsing' -v` → expect **FAIL**
- [x] GREEN: `graph.go` — stable Kahn, transitive closure, `LastStepUsing`; cycle detection wired into `Parse`
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/updplan): stable topological order`
- [x] LEDGER + CHECKPOINT

**Done when:** order is deterministic across runs (run the test with `-count=20`).

### Task 5 — `--ref` compatibility shim + path resolution  (plan Task 5, leaf A)
- [x] RED: `TestWithRefTargetsDotfilesByName`, `TestWithRefTargetsTheSoleRepo`, `TestWithRefIsAmbiguousWithManyRepos` (error lists repo names and the `repo=branch` form), `TestWithRefRepoEqualsBranch`, `TestWithRefRejectsShellInjection` (same vectors as `cmd/update_test.go` `TestValidRefRejectsShellInjection`), `TestWithRefDropsADuplicateExtra` (`[main, staging]` + `--ref staging` → `[staging]`), `TestRepoPathResolvesUnderRoot` (`dotfiles` → `~/git/dotfiles`; `work/scripts` → `~/git/work/scripts`; `/srv/x` and `~/x` unchanged; path defaults to the repo name)
- [x] RUN-RED: `go test ./internal/updplan -run 'TestWithRef|TestRepoPathResolves' -v` → expect **FAIL**
- [x] GREEN: `WithRef`, `WithRefs`, path resolution
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/updplan): --ref compatibility shim`
- [x] LEDGER + CHECKPOINT

**Done when:** all shim tests pass and `Repo.Path` is never relative after `Parse`.

## Leaf A gate
- [x] VERIFY: `go test -race -cover ./internal/updplan | tee -a evidence/updplan/leaf-gate.txt` → coverage **≥ 90 %**
- [x] LEDGER: TRACKING §0 leaf A `in-review`; §3 gate line ticked with the observed percentage; `docs/mbo/index.md` state → `building`
- [x] CHECKPOINT; refresh IMPLEMENTATION §8 kickoff for leaf B (stacked on A) + leaf C (parallel)

---

# Leaf C — `featflag` + deps (BLOCKING for D; independent of A — may run in parallel with A)

## Leaf C setup
- [ ] SETUP: `gss feature worker add --feature fleet-update --purpose featflag --engine claude --json --description "internal/featflag: fail-open gff adapter + go.mod deps + fleet.update.* flags (plan tasks 17-18)"`
- [ ] SETUP: record refs verbatim in TRACKING §0 / IMPLEMENTATION §2.1

### Task 17 — fail-open gff resolution  (plan Task 17, leaf C)
- [x] RED: `internal/featflag/featflag_test.go` — `TestResolveDefaultsWhenSourceErrors` (`Static{Err: errors.New("x")}` → `Enabled true`, `ConfigPath ""`, `Note` non-empty), `TestResolveHonoursDisabled`, `TestResolveMapsHomeToEmptyPath`, `TestResolveMapsRepoUnderRepoDir` (`repo` → `<repoDir>/opt/etc/fleet/fleet.yaml`), `TestResolveUnknownKeyIsFailOpen` (error wrapping `gff.ErrUnknownKey` → enabled); every test `t.Setenv("HOME", t.TempDir())`
- [x] RUN-RED: `go test ./internal/featflag -run 'TestResolve' -v` → expect **FAIL**
- [x] GREEN: `featflag.go` — `Source`, `Settings`, `Resolve`, `Static`, key constants
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/featflag): fail-open gff resolution`
- [x] LEDGER + CHECKPOINT

**Done when:** no code path can return `Enabled false` on an error.

### Task 18 — gff SDK adapter + deps + flags  (plan Task 18, leaf C)
- [x] RED: `gff_test.go` — `TestGFFFallsBackToUnscopedOnUnknownSource` (inject the two inner resolve funcs; first returns `gff.ErrUnknownSource`, second is called)
- [x] RUN-RED: `go test ./internal/featflag -run 'TestGFF' -v` → expect **FAIL**
- [x] GREEN: `gff.go` (the only import of `github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/pkg/gff`); `go.mod`: require `github.com/sfc-gh-eraigosa/dotfiles/sdk/gff v0.0.0` + `replace … => ../gff`; `go mod tidy`
- [x] GREEN: `.github/gff/features.yaml` — append `area: fleet` with `fleet.update.enabled` (bool true) and `fleet.update.config` (SINGLE choice: `home` selected, `repo`) exactly as plan §3.5
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: `cd <WT>/sdk/gff && go run . lint ../../.github/gff/features.yaml` → no findings; `go run . get fleet.update.config` → `home`; `cd <WT>/sdk/fleet && go build ./... && go mod tidy && git diff --exit-code go.mod go.sum`
- [x] VERIFY: `bash sdk/fleet/build.sh && ls -l "$HOME/opt/bin/fleet" | tee -a evidence/featflag/binary-size-after.txt`
- [x] COMMIT: `feat(fleet): gff flags fleet.update.{enabled,config} + SDK adapter`
- [x] LEDGER + CHECKPOINT

**Done when:** `gff lint` clean, `go build ./...` green, size delta recorded.

## Leaf C gate
- [x] VERIFY: `go test -race -cover ./internal/featflag | tee -a evidence/featflag/leaf-gate.txt` → **≥ 90 %** (96.4%)
- [x] LEDGER + CHECKPOINT

---

# Leaf B — `updexec` + `runner.RunStreamCtx` (BLOCKING for D/E; `--base` = leaf A's branch)

## Leaf B setup
- [ ] SETUP: `gss feature worker add --feature fleet-update --purpose updexec --base <leaf-A-branch> --engine claude --json --description "internal/updexec: remote script builders + per-host executor; runner.RunStreamCtx (plan tasks 6-16)"`
- [ ] SETUP: record refs verbatim; confirm `<WT>` contains `internal/updplan`

### Task 6 — sync script builders: byte-compatible, one fetch  (plan Task 6, leaf B)
- [x] RED: `internal/updexec/script_test.go` — `TestSyncScriptSingleBranchMatchesTodaysForm` (BODY for `[main]`, `local: skip`, `reset: false` contains exactly `cd ~/git/dotfiles && git fetch origin main && git checkout main && git merge --ff-only FETCH_HEAD`); **move** `TestUpdateMakesExactlyOneNetworkCall` from `cmd/update_test.go` with its four assertions verbatim (`strings.Count(s, "git fetch") == 1`, no `git pull`, `merge --ff-only FETCH_HEAD`, `--ff-only`); `TestEverySyncFormMakesAtMostOneUnconditionalNetworkCall` (single / multi / default / clone: `fetch`+`clone` == 1, no `pull`, `ls-remote` ≤ 1 and only after the `||` following `symbolic-ref`)
- [x] RUN-RED: `go test ./internal/updexec -run 'TestSyncScript|TestUpdateMakesExactlyOneNetworkCall|TestEverySyncForm' -v` → expect **FAIL**
- [x] GREEN: `script.go` — `SyncScript` (PROLOGUE + BODY single/multi/default + EPILOGUE per plan §3.4), `ShQuote` (moved from `tui_cmds.go`)
- [x] RUN-GREEN: same → expect **PASS**; `go test ./cmd/` still green (the moved test now lives here; `cmd` keeps a wrapper test if needed)
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/updexec): sync script builders (byte-compatible, one fetch)` — body lists the moved test
- [x] LEDGER + CHECKPOINT

**Done when:** the exact-string test passes and every form counts one unconditional network call.

### Task 7 — precheck, clone, rescue, reset, restore, run, gh-auth builders  (plan Task 7, leaf B)
- [x] RED: `TestMultiBranchFetchesAllInOneCall` (`git fetch origin main staging`), `TestExtrasOnlyForceMoveAnAncestor` (`branch -q -f` only after `merge-base --is-ancestor`; `skipped(diverged)` marker; `continue` guard on `$b1`), `TestDefaultBranchPrefersLocalSymbolicRef`, `TestCloneNeverFetches`, `TestPrecheckUsesDashEForWorktrees`, `TestPrecheckReportsStateAndBranchReadOnly` (no write verbs in the script: `checkout|reset|stash|branch -f|merge|fetch|clone|add|commit|rm`), **move** `TestRescuePreservesUntrackedWork` (parameterised path; rescue dir contains the repo name), `TestResetScriptUnchanged` (today's text), `TestRunScriptIsVerbatimAfterCd` (`cd ~/git/work/scripts && make install`; no-repo form has no `cd`), `TestGhAuthNeverCarriesAToken` (no `GH_TOKEN`, `GITHUB_TOKEN`, `--with-token`, `token`), `TestGhAuthCheckReserves127`, `TestRestoreUsesApplyBySHANeverPop` (no `stash pop`, no `stash@{`), `TestRestoreRejectsUnvalidatedOrigOrSHA` (`main; id`, 39-char SHA → error, no string)
- [x] RUN-RED: `go test ./internal/updexec -run 'MultiBranch|Extras|DefaultBranch|Clone|Precheck|Rescue|Reset|RunScript|GhAuth|Restore' -v` → expect **FAIL**
- [x] GREEN: `PrecheckScript`, `CloneScript`, `RescueScript`, `ResetScript`, `RestoreScript`, `RunScript`, `GhAuthCheck`, `GhAuthLogin` per plan §3.4
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/updexec): precheck, clone, rescue, restore, run, gh-auth builders`
- [x] LEDGER + CHECKPOINT

**Done when:** every builder's string matches plan §3.4 and the moved rescue test is green here.

### Task 8 — builders refuse unvalidated input  (plan Task 8, leaf B)
- [x] RED: `TestBuildersRejectUnvalidatedInput` — hand-built `updplan.Repo{Path: "~/x; id"}`, branch `main;id`, url `'x'`, hostname `gh;id` → every builder returns an error and never a string containing `;`
- [x] RUN-RED: `go test ./internal/updexec -run 'TestBuildersReject' -v` → expect **FAIL**
- [x] GREEN: re-validation at the top of every builder
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `test(fleet/updexec): builders refuse unvalidated input`
- [x] LEDGER + CHECKPOINT

**Done when:** a caller bypassing `Parse` cannot produce a metacharacter on the wire.

### Task 9 — `runner.RunStreamCtx`  (plan Task 9, leaf B)
- [x] RED: `internal/runner/runner_test.go` — `TestRunStreamCtxKillsTheChildOnDeadline` (stub `ssh` on `PATH` in `t.TempDir()` running `sleep 30`; 100 ms deadline → `done` yields a context error within 1 s), `TestRunStreamDelegatesToCtx`, `TestFakeHonoursContextCancellation` (`Fake` with a blocking script; cancel → `done` closes), extend `TestEveryRemotePathCarriesTheMuxOptions` to `RunStreamCtx`
- [x] RUN-RED: `go test ./internal/runner -run 'RunStreamCtx|RunStreamDelegates|FakeHonours|EveryRemotePath' -v` → expect **FAIL**
- [x] GREEN: `RunStreamCtx` on `Runner`, `Exec` (`exec.CommandContext`), `Fake`; `RunStream` delegates; update every `recordingRunner`-style wrapper in `cmd` tests to compile
- [x] RUN-GREEN: same → expect **PASS**; `go test ./...` green
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/runner): RunStreamCtx for controller-enforced timeouts`
- [x] LEDGER + CHECKPOINT

**Done when:** a deadline kills the local child promptly and the mux invariant covers the new path.

### Task 10 — executor walks the plan per host  (plan Task 10, leaf B)
- [x] RED: `exec_test.go` — scripted `fakeIO` (substring-keyed, per-step response sequences, records calls, honours `ctx.Done()`, `ErrNoTerminal` toggle) + stepping clock + recorded sleep; `TestRunHostRunsStepsInOrderWithDurations` (default plan: precheck → sync → interactive install; durations from the clock; `Output` path set; capture contains `=== step dotfiles.sync (sync) ===`), `TestNotesAreParsedFromFleetLines`, `TestAttemptHeaderIsWrittenToTheCapture`
- [x] RUN-RED: `go test ./internal/updexec -run 'TestRunHost|TestNotes|TestAttemptHeader' -v` → expect **FAIL**
- [x] GREEN: `exec.go` — `Executor`, `StepIO`, `Output`/`LineWriter`/`Discard`, `Result`, `HostReport`, single-attempt loop
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/updexec): executor walks the plan per host`
- [x] LEDGER + CHECKPOINT

**Done when:** the happy path runs in `Order()` and the capture carries the step headers.

### Task 11 — failure cascade  (plan Task 11, leaf B)
- [x] RED: `TestFailedStepSkipsTransitiveDependents` (reason `blocked by scripts.make`; independent `dotfiles.*` scripts still sent), `TestOnFailureContinueLetsDependentsRunButStillFailsTheHost`, `TestExpectExitAcceptsNonZero`, `TestDependencyFailedAlsoBlocks` (`a→b→c`, a failed → c blocked by b)
- [x] RUN-RED: `go test ./internal/updexec -run 'Cascade|FailedStep|OnFailure|ExpectExit|DependencyFailed' -v` → expect **FAIL**
- [x] GREEN: `firstStopBlocker`, `expect` evaluation
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/updexec): failure cascade`
- [x] LEDGER + CHECKPOINT

**Done when:** dependents are blocked, siblings run, `continue` unblocks but still fails the host.

### Task 12 — local-state policies  (plan Task 12, leaf B)
- [x] RED: **migrate** with names + assertions kept: `TestUpdateSkipsDirtyCloneByDefault` (no sent command contains `install.sh`/`pull`/`fetch`), `TestUpdateProceedsOnCleanClone`, `TestForceRescuesDirtyWorkBeforePulling` (no `reset --hard`/`checkout -- `; `git add -A` + `worktree add` present; rescue sent before fetch), `TestUpdateSurfacesProbeFailure`; add `TestMissingCloneWithURLClones`, `TestMissingCloneWithoutURLFails`, `TestInProgressMergeIsSkippedUnderEveryPolicy`, `TestResetModeUsesResetScript`, `TestUnexpectedPrecheckOutputFails`, `TestCLILocalOverridesEveryRepoPolicy`, `TestResetIsIncompatibleWithCarry`
- [x] RUN-RED: `go test ./internal/updexec -run 'TestUpdate|MissingClone|InProgress|ResetMode|UnexpectedPrecheck|CLILocal|ResetIsIncompatible' -v` → expect **FAIL**
- [x] GREEN: precheck parsing (`state=… branch=…`), policy switch, `Executor.Local` override
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/updexec): local-state policies` — body lists the four migrated tests
- [x] LEDGER + CHECKPOINT

**Done when:** the spec §4 F3 table is exercised row by row.

### Task 13 — carry and branch restore  (plan Task 13, leaf B)
- [x] RED: `TestCarryStashesWithUntrackedAndCapturesTheSHA`, `TestCarryRestoreRunsAfterTheLastStepUsingTheRepo` (`r.sync → r.build → other.sync`: restore issued right after `r.build`, before `other.sync`), `TestRestoreRunsEvenWhenAnIntermediateStepFailed`, `TestRestoreRunsImmediatelyWhenSyncFailsAfterStash`, `TestRestoreConflictKeepsTheStash` (no `stash drop` on non-zero apply; reason names SHA + branch), `TestCleanOffBranchIsRestoredUnderEveryPolicy`, `TestOnTargetNeverSynthesizesARestore`, `TestRescueOffBranchRestoresTheBranchWithoutAStash`, `TestDetachedHeadRestoresToTheSHA`, `TestRestoreFalseLeavesHostOnTarget` (per-repo and `NoRestore`), `TestRestoreCheckoutFailureKeepsEverything`, `TestRestoreStepHasFixedRetryPolicy` (3× transport, 5m; apply conflict not retried), `TestCarryPrologueIsIdempotentAcrossAttempts`
- [x] RUN-RED: `go test ./internal/updexec -run 'Carry|Restore|OffBranch|OnTarget|Detached' -v` → expect **FAIL**
- [x] GREEN: note parsing (`orig=`, `carried stash=`, `switched`), `pending` map, synthesized `<repo>.restore`
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/updexec): carry and branch restore`
- [x] LEDGER + CHECKPOINT

**Done when:** every row of the spec F3/F4 restore semantics has a green test and nothing is ever dropped.

### Task 14 — gh-auth step  (plan Task 14, leaf B)
- [x] RED: `TestGhAuthSkipsLoginWhenStatusPasses` (1 Batch, 0 Interactive), `TestGhAuthLogsInInteractivelyThenReverifies`, `TestGhAuthReports127AsNotInstalled`, `TestGhAuthWithoutATerminalFailsCleanly` (reason `needs a terminal`; dependents blocked), `TestGhAuthNeverUsesStdin`, `TestGhAuthLoginIsNeverRetriedButCheckIs`
- [x] RUN-RED: `go test ./internal/updexec -run 'TestGhAuth' -v` → expect **FAIL**
- [x] GREEN: gh-auth branch of `runStep`
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/updexec): gh-auth step`
- [x] LEDGER + CHECKPOINT

**Done when:** check → (login → check) with zero prompts on an authenticated host.

### Task 15 — retries with backoff under per-attempt timeouts  (plan Task 15, leaf B)
- [x] RED: `TestTransportFailureIsRetriedWithBackoff` (255, 255, ok → `Attempts 3`, sleeps `[5s 10s]`), `TestNonMatchingFailureIsNotRetried`, `TestRetryOnAnyRetriesEveryUnexpectedExit`, `TestRetryOnExitCodeMatchesOnlyThatCode`, `TestExpectedExitIsNeverRetried`, `TestAttemptsAreExhaustedThenOnFailureApplies`, `TestTimeoutCancelsTheAttempt` (fake blocks until `ctx.Done()`; `TimedOut true`, reason `timed out after 1s`), `TestTimeoutIsRetriedOnlyWhenListed`, `TestInteractiveStepsAreNeverRetried`, `TestInteractiveHasNoDeadlineUnlessSet`, `TestExecutorTimeoutOverridesBatchSteps`, `TestNoRetryForcesOneAttempt`
- [x] RUN-RED: `go test ./internal/updexec -run 'Retr|Timeout|Attempts|Interactive|NoRetry' -v` → expect **FAIL**
- [x] GREEN: attempt loop with `context.WithTimeout`, class matching, `Backoff.Wait` via `Sleep`/`Rand`
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/updexec): retries with backoff under per-attempt timeouts`
- [x] LEDGER + CHECKPOINT

**Done when:** the recorded sleep schedule matches the plan and no interactive step is ever retried.

### Task 16 — Console and Background lanes  (plan Task 16, leaf B)
- [x] RED: `TestConsoleStreamsBatchAndHandsOffInteractive` (Batch → `RunStreamCtx` lines reach `Line`; Interactive → `RunInteractive`), `TestBackgroundRefusesInteractive` (`ErrNoTerminal`), `TestPreambleAndStdinApplyToRunStepsOnly` (sync/gh scripts carry no `sudo -S`; stdin empty for them), `TestExitCodeMapsExitErrorAndSSH255` (255 → `ErrTransport`)
- [x] RUN-RED: `go test ./internal/updexec -run 'Console|Background|Preamble|ExitCode' -v` → expect **FAIL**
- [x] GREEN: `Console`, `Background`, `exitCode`
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/updexec): Console and Background lanes`
- [x] LEDGER + CHECKPOINT

**Done when:** both lanes exist and the secret/preamble can only reach run steps.

## Leaf B gate
- [x] VERIFY: `go test -race -cover ./internal/updexec ./internal/runner | tee -a evidence/updexec/leaf-gate.txt` → updexec **≥ 90 %**; `go test ./...` green (moved tests accounted for, none deleted: `git diff <base> -- cmd/update_test.go` shows only moves)
- [x] LEDGER + CHECKPOINT; refresh IMPLEMENTATION §8 for leaf D

---

# Leaf D — `cmd` CLI (BLOCKING for E/F; `--base` = leaf B's branch; leaf C must be merged or the worker restacked onto it)

## Leaf D setup
- [x] SETUP: leaf C confirmed landed on this branch (worktree/fleet_config); no restack needed
- [x] SETUP: N/A — built directly on worktree/fleet_config per task instructions (single PR, no gss workers)
- [x] SETUP: N/A (no worker)

### Task 19 — `loadPlan` (flag → gff → built-in)  (plan Task 19, leaf D)
- [x] RED: `cmd/update_test.go` — `TestLoadPlanPrefersFileFlag`, `TestLoadPlanUsesBuiltInWhenDisabled`, `TestLoadPlanUsesBuiltInWhenNoFile` (Source names the missing path), `TestLoadPlanReadsTheConfiguredPath` (`t.Setenv("XDG_CONFIG_HOME", dir)`), `TestLoadPlanReadsTheRepoLocation`, `TestLoadPlanRefusesAWorldWritableFile`; `TestSavedAnswersNeverContainTheCredential` still green after extracting `fleetConfigDir()`
- [x] RUN-RED: `go test ./cmd -run 'TestLoadPlan|TestSavedAnswers' -v` → expect **FAIL**
- [x] GREEN: `loadPlan`, `fleetConfigDir()`, `defaultPlanPath()`
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/cmd): loadPlan (flag → gff → built-in)`
- [x] LEDGER + CHECKPOINT

**Done when:** every resolution branch has a test and the ownership/mode check refuses a shared file.

### Task 20 — `fleet update` runs the plan through the executor  (plan Task 20, leaf D)
- [x] RED: `TestUpdateHostUsesTheRequestedRef` (`--ref feature/x` → `git fetch origin feature/x` sent), `TestUpdateDefaultPlanSendsExactlyOneFetchPerSyncStep` (recordingRunner over the whole run), `TestValidRefRejectsShellInjection` (alias to `updplan.ValidRef`), `TestForceIsAnAliasForLocalRescue`, `TestTimeoutAndNoRetryFlagsReachTheExecutor`
- [x] RUN-RED: `go test ./cmd -run 'TestUpdateHost|TestUpdateDefaultPlan|TestValidRef|TestForceIs|TestTimeoutAndNoRetry' -v` → expect **FAIL**
- [x] GREEN: `runUpdate` replaces `updateHost`; flags `--local --force --no-restore --reset --timeout --no-retry --ref[] --file --dry-run`; `--ref` default empty; `updateScript(ref, reset)` kept as a thin wrapper for the TUI
- [x] RUN-GREEN: same → expect **PASS**; `go test ./...` green; **no test deleted** (`git diff <base> --stat -- cmd/*_test.go`)
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/cmd): fleet update runs the plan through the executor`
- [x] LEDGER + CHECKPOINT

**Done when:** a default-plan run issues exactly one fetch and `--force` behaves as before.

### Task 21 — per-step report, `--json`, `--dry-run`  (plan Task 21, leaf D)
- [x] RED: `TestReportNamesEveryStepAndTheLog` (incl. `attempt 2/3`, `timeout` markers, `log:` line), `TestExitCodeReflectsAnyFailedHost` (`N host(s) not updated`), `TestDryRunSendsNothing` (recordingRunner log empty; output contains every script verbatim + effective timeout/retry), `TestJSONReportIsMachineReadable`
- [x] RUN-RED: `go test ./cmd -run 'TestReport|TestExitCode|TestDryRun|TestJSONReport' -v` → expect **FAIL**
- [x] GREEN: report rendering, JSON shape, dry-run
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/cmd): per-step report, --json, --dry-run`
- [x] LEDGER + CHECKPOINT

**Done when:** an operator can read the whole wire from `--dry-run` before running anything.

### Task 22 — `fleet update init`  (plan Task 22, leaf D)
- [x] RED: `cmd/update_init_test.go` — `TestInitWritesTheDefaultPlanOnce` (file 0644, dir 0700, second run errors without `--overwrite`), `TestInitPrintToStdout`, `TestInitOutputParsesToDefault`
- [x] RUN-RED: `go test ./cmd -run 'TestInit' -v` → expect **FAIL**
- [x] GREEN: `update_init.go`
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/cmd): fleet update init`
- [x] LEDGER + CHECKPOINT

**Done when:** `fleet update init && fleet update <host> --dry-run` shows today's three commands.

### Task 23 — headless run log + answers env  (plan Task 23, leaf D)
- [x] RED: `TestHeadlessUpdateIsCaptured` (mirrors `runlog_test.go`: header `host=` `plan=` `mode=`), `TestLocalAnswerEnvIsExportedForRunStepsOnly`, extend `TestSudoSecretNeverAppearsInTheRemoteCommand` (CLI lane has no stdin at all)
- [x] RUN-RED: `go test ./cmd -run 'TestHeadlessUpdate|TestLocalAnswerEnv|TestSudoSecretNever' -v` → expect **FAIL**
- [x] GREEN: `update_output.go` (`captureOutput` over `applog.NewCapture`), answers env preamble
- [x] RUN-GREEN: same → expect **PASS**
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/cmd): headless run log`
- [x] LEDGER + CHECKPOINT

**Done when:** a headless run leaves a capture under `$XDG_STATE_HOME/fleet/logs/`.

## Leaf D gate (includes the live acceptance runs)
- [x] VERIFY: `go test -race -cover ./... | tee -a evidence/cmd/leaf-gate.txt`; `cmd` coverage not below `evidence/cmd/baseline-coverage.txt`
- [x] VERIFY: `bash sdk/fleet/build.sh && "$HOME/opt/bin/fleet" version`
- [x] VERIFY (G1): no file → `fleet update <host> --dry-run | tee -a evidence/e2e/G1-dry-run-default.txt` — today's three commands
- [x] VERIFY (G2): `fleet update init`, edit to two repos + `gh-auth`; `--dry-run` teed; live run teed to `evidence/e2e/G2-live.txt`; a deliberately failing `make` shows `dep-fail` on its dependent while `dotfiles.install` still ran
- [ ] VERIFY (G3): SKIPPED — no fleet host in `fleet status` was up-to-date (all reported `behind 3`); the task's live-acceptance gate
  restricts real (non-dry-run) runs to a no-op ff on an up-to-date host, so no live G3/G6/G7/G8/G9 run was safe to perform this pass.
- [x] VERIFY (G4): `gff set fleet.update.enabled false` → first line reads `built-in`; `gff unset fleet.update.enabled`
- [ ] VERIFY (G6): SKIPPED — same reason as G3 (see above)
- [ ] VERIFY (G7): SKIPPED — same reason as G3 (see above)
- [ ] VERIFY (G8): SKIPPED — same reason as G3 (see above)
- [ ] VERIFY (G9): SKIPPED — same reason as G3 (see above)
- [x] LEDGER: TRACKING §2/§3 human-gate boxes ticked with evidence paths; `index.md` state stays `building`
- [x] CHECKPOINT; refresh IMPLEMENTATION §8 for leaves E and F (parallel) — see TRACKING §0 note

---

# Leaf E — TUI (`--base` = leaf D's branch; parallel with F)

## Leaf E setup
- [ ] SETUP: `gss feature worker add --feature fleet-update --purpose tui --base <leaf-D-branch> --engine claude --json --description "TUI: background lane on the plan executor; interactive handoff = fleet update (plan tasks 24-26)"`
- [ ] SETUP: record refs verbatim

### Task 24 — background updates run the plan executor  (plan Task 24, leaf E)
- [x] RED: `TestSudoPreambleIsPerRunStepSession` (each run step's script starts with `sudo -S -p '' -v` when a secret is set; sync/gh never); existing `TestUpdateIsCapturedWithItsSubject` / `TestForcedResetIsLabelledInTheCapture` updated for the `plan=` header
- [x] RUN-RED: `go test ./cmd -run 'TestSudoPreamble|TestUpdateIsCaptured|TestForcedReset' -v` → expect **FAIL**
- [x] GREEN: `beginStream(alias, plan, answers, r, dir)` over `Executor{IO: Background{…}}` in a goroutine; `done` carries `rep.Err()`
- [x] RUN-GREEN: same → expect **PASS**; every `tui_*_test.go` green
- [x] VERIFY: gofmt/vet/race
- [x] COMMIT: `feat(fleet/tui): background updates run the plan executor`
- [x] LEDGER + CHECKPOINT

**Done when:** `u` on a passwordless-sudo host streams the plan's steps into the log pane.

### Task 25 — interactive handoff is `fleet update`  (plan Task 25, leaf E)
- [x] RED: `TestHandoffDelegatesToFleetUpdate` (argv: `<self> update <alias> --file … --ref … [--force]`), `TestHandoffEnvNeverCarriesTheSecret`, `TestNeedsTerminalRoutesToInteractiveQueue` (`ErrNoTerminal` → host moves to `iaQueue`, row not failed)
- [x] RUN-RED: `go test ./cmd -run 'TestHandoff|TestNeedsTerminal' -v` → expect **FAIL**
- [x] GREEN: `interactiveHandoff` self-execs via `os.Executable()` wrapped by `handoffWrapper`; delete the `updateScript` wrapper and `unattendedUpdate`
- [x] RUN-GREEN: same → expect **PASS**; `go test ./...` green
- [x] VERIFY: gofmt/vet/race; `grep -n "updateScript\|unattendedUpdate" cmd/*.go` → no hits
- [x] COMMIT: `feat(fleet/tui): interactive handoff is fleet update`
- [x] LEDGER + CHECKPOINT

**Done when:** exactly one definition of "update a host" remains (the executor).

### Task 26 — `--file` and plan-aware status line  (plan Task 26, leaf E)
- [ ] RED: `--update-ref` → `WithRef` test; `--file` accepted; status line `updating N host(s) (plan: <Source>)`; `TestTUIDemoWidthGuard` still passes
- [ ] RUN-RED: `go test ./cmd -run 'TestTUI' -v` → expect **FAIL**
- [ ] GREEN: flags + status text
- [ ] RUN-GREEN: same → expect **PASS**
- [ ] VERIFY: gofmt/vet/race
- [ ] COMMIT: `feat(fleet/tui): --file and plan-aware status line`
- [ ] LEDGER + CHECKPOINT

**Done when:** the demo width guard passes with the new status text.

## Leaf E gate
- [ ] VERIFY: `go test -race ./... | tee -a evidence/tui/leaf-gate.txt`
- [ ] VERIFY (G5): `fleet tui`, `u` on two hosts (one background, one needing sudo) — transcript to `evidence/tui/G5-live.txt`
- [ ] LEDGER + CHECKPOINT

---

# Leaf F — docs (`--base` = leaf D's branch; parallel with E)

## Leaf F setup
- [ ] SETUP: `gss feature worker add --feature fleet-update --purpose docs --base <leaf-D-branch> --engine claude --json --description "docs: AGENTS.md invariants, README fleet.yaml reference, sample repo plan (plan task 27)"`
- [ ] SETUP: record refs verbatim

### Task 27 — fleet.yaml update plans docs  (plan Task 27, leaf F)
- [ ] DOCS: `sdk/fleet/AGENTS.md` — `fleet update` command row; layout rows for `internal/updplan`, `internal/updexec`, `internal/featflag`; every invariant in plan §6 with its test name; amend the "never `stash@{0}`" text to say why `push -u` + `apply <sha>` is safe (cite G7 evidence)
- [ ] DOCS: `sdk/fleet/README.md` — rewrite the `fleet update` section; `fleet.yaml` reference (schema, built-in default, local-changes table, retry/timeout table, `run:` trust boundary, manual restore one-liner `git checkout <orig> && git stash apply <sha>`); console demo from **real** `--dry-run` output
- [ ] DOCS (optional): `opt/etc/fleet/fleet.yaml` sample for the `repo` location
- [ ] ALLOWLIST: `git status --short -- opt/etc/fleet/` — if absent, `git check-ignore -v opt/etc/fleet/fleet.yaml` and add a narrow `!opt/etc/fleet/**` rule (never `git add -f`)
- [ ] VERIFY: relative links resolve: `grep -o '\](\.[^)]*)' sdk/fleet/README.md sdk/fleet/AGENTS.md | sed 's/.*(\(.*\))/\1/' | sort -u | while read l; do test -e "sdk/fleet/$l" || echo "MISSING $l"; done`
- [ ] VERIFY: `make lint-shell` / `make lint-portability` if any shell was touched
- [ ] COMMIT: `docs(fleet): fleet.yaml update plans`
- [ ] LEDGER + CHECKPOINT

**Done when:** a reader can configure a two-repo plan from the README alone and every invariant names its test.

## Leaf F gate
- [ ] LEDGER: TRACKING §1 row 27 `done`; `docs/mbo/index.md` state → `in-review`
- [ ] CHECKPOINT

---

## Objective close-out
- [ ] Land leaves bottom-up (`gss feature pr --ready` per leaf — **token-gated, confirm via AskUserQuestion**; `gss feature merged <ref>` after each merge)
- [ ] TRACKING §3 stop condition fully ticked; `index.md` state → `merged` → `done`
- [ ] Close issue #265 only when every leaf has merged
- [ ] `gss feature done` for each worker; `gss feature audit --feature fleet-update`
