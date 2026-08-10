# fleet — execution cursor

- **Slug:** fleet
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../fleet.md`](../fleet.md) — every task/§ reference points there

> **How to use:** the **first unchecked box is the next action**. Tick a box only after you
> ran the command and read the output. After finishing a `###` task: update `TRACKING.md`,
> commit with the plan's exact message, checkpoint.
>
> **Legend:** `SETUP` prep · `RED` write a failing test · `RUN-RED` run it, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run it, expect PASS · `VERIFY` extra gate · `DOCS` ·
> `COMMIT` · `LEDGER` update TRACKING.md · `CHECKPOINT` push/PR refresh · 🛑 human stop.

## Preflight (once)

- [x] P1: `git rev-parse --abbrev-ref HEAD` → the fleet worker branch
- [x] P2: `go version` → ≥ `.go-version`
- [x] P3: `git log --oneline origin/main..HEAD` → **only this objective's commits** (a foreign commit ⇒ `git rebase --onto origin/main <foreign-sha>` first)
- [x] P4: `ssh -o BatchMode=yes -o ConnectTimeout=5 <host> true; echo $?` → `0`
- [x] P5: `git status --porcelain` → empty
- [x] P6: `mkdir -p docs/mbo/plans/fleet/evidence`

---

### Task 1 — Module scaffold + repo wiring  (plan Task 1) — **BLOCKING**

- [x] RED: write `sdk/fleet/cmd/version_test.go::TestVersionStringIncludesVersionAndCommit`
- [x] RUN-RED: `cd sdk/fleet && go test ./cmd/ -run TestVersionString -v` → **FAIL** (`undefined: versionString`)
- [x] GREEN: create `go.mod`, `main.go`, `VERSION` (`0.1.0`), `cmd/root.go`, `cmd/version.go`; copy `sdk/wol/build.sh` → `sdk/fleet/build.sh` (s/wol/fleet/)
- [x] RUN-GREEN: `cd sdk/fleet && go test ./... -v` → **PASS**
- [x] VERIFY: `Makefile:108` loop includes `fleet`
- [x] VERIFY: `scripts/test.sh` `coverage_min()` has `fleet)    echo 60 ;;`
- [x] VERIFY: `.github/gff/features.yaml` has `install.sdk.fleet`; header comment says `sdk (6)`
- [x] VERIFY: `install.sh` has the `gff_on install.sdk.fleet` block after the `wol` block
- [x] VERIFY: `bash -n install.sh` → silent
- [x] VERIFY: `./scripts/test.sh 2>&1 | grep -i fleet | tee docs/mbo/plans/fleet/evidence/task01/wiring.txt`
- [x] COMMIT: `feat(fleet): module scaffold + build/test/install wiring`
- [x] LEDGER + CHECKPOINT

**Done when:** `go test ./...` passes, `scripts/test.sh` lists `fleet`, `bash -n` clean.

---

### Task 2 — install.sh stamp  (plan Task 2) — *independent*

- [x] RED: add `test_stamp_written_only_for_phase_all` to `install_test.sh`
- [x] RUN-RED: `bash install_test.sh 2>&1 | grep stamp` → **FAIL**
- [x] GREEN: append the phase-gated stamp block (`install.sh.stampblock`, sourced from `install.sh`)
- [x] RUN-GREEN: `bash install_test.sh 2>&1 | tee docs/mbo/plans/fleet/evidence/task02/stamp.txt | grep stamp` → **PASS**
- [x] VERIFY: `bash -n install.sh` → silent
- [x] VERIFY: stamp is written **last** and skipped for `INSTALL_PHASE=deps|config`
- [x] COMMIT: `feat(install): record an install stamp for fleet (phase-gated)`
- [x] LEDGER + CHECKPOINT

**Done when:** phase-gating test passes; `bash -n` clean.

---

### Task 3 — sshconf reader  (plan Task 3)

- [ ] RED: `sshconf_test.go` — `TestParseReturnsOnlyMarkedConcreteHosts`, `TestParseSkipsPatternHostsEntirely`, `TestParseCapturesFields`
- [ ] RUN-RED: `cd sdk/fleet && go test ./internal/sshconf/ -v` → **FAIL** (`undefined: Parse`)
- [ ] GREEN: implement `Host` + `Parse(cfg, marker)`
- [ ] RUN-GREEN: `cd sdk/fleet && go test ./internal/sshconf/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task03/parse.txt` → **PASS** (3 tests)
- [ ] VERIFY: pattern hosts (`*`, `?`) provably excluded; unmarked hosts not in fleet scope
- [ ] COMMIT: `feat(fleet): parse ~/.ssh/config for marked fleet hosts`
- [ ] LEDGER + CHECKPOINT

**Done when:** all three tests pass.

---

### Task 4 — sshconf writer (add / unmark / purge)  (plan Task 4)

- [ ] RED: `TestAddIsIdempotentAndPreservesOtherBlocks`, `TestUnmarkKeepsBlockButLeavesFleet`, `TestPurgeRemovesOnlyTheTargetBlock`, `TestUnknownAliasIsAnError`
- [ ] RUN-RED: `cd sdk/fleet && go test ./internal/sshconf/ -run 'TestAdd|TestUnmark|TestPurge|TestUnknown' -v` → **FAIL** (`undefined: Add`)
- [ ] GREEN: implement `blockRange`, `Add`, `Unmark`, `Purge`
- [ ] RUN-GREEN: `cd sdk/fleet && go test ./internal/sshconf/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task04/writer.txt` → **PASS**
- [ ] VERIFY: `Add` twice is byte-identical; `Unmark` keeps the block; `Purge` touches only the target
- [ ] COMMIT: `feat(fleet): idempotent ssh-config add/unmark/purge`
- [ ] LEDGER + CHECKPOINT

**Done when:** idempotency + unmark-keeps-block + purge-only-target + unknown-alias-errors pass.

---

### Task 5 — stamp parser  (plan Task 5)

- [ ] RED: `TestParseWellFormed`, `TestParseEmptyIsError`, `TestParseTruncatedIsError` (helper is `repeat`, **not** `strings`)
- [ ] RUN-RED: `cd sdk/fleet && go test ./internal/stamp/ -v` → **FAIL** (`undefined: Parse`)
- [ ] GREEN: implement `Stamp` + `Parse` (40-char commit required; epoch parsed)
- [ ] RUN-GREEN: `cd sdk/fleet && go test ./internal/stamp/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task05/stamp.txt` → **PASS**
- [ ] COMMIT: `feat(fleet): parse the install stamp`
- [ ] LEDGER + CHECKPOINT

**Done when:** well-formed parses; empty and truncated both error.

---

### Task 6 — drift classify + age  (plan Task 6)

- [ ] RED: `TestClassifyAllFiveClasses`, `TestClassifyNeverReportsBehindWhenCommitsMatch`, `TestFormatAge`
- [ ] RUN-RED: `cd sdk/fleet && go test ./internal/drift/ -v` → **FAIL** (`undefined: Classify`)
- [ ] GREEN: implement `Class`, `Input`, `Result`, `Classify`, `FormatAge(now, then)`
- [ ] RUN-GREEN: `cd sdk/fleet && go test ./internal/drift/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task06/drift.txt` → **PASS**
- [ ] VERIFY: `now` is injected — no `time.Now()` inside the package
- [ ] COMMIT: `feat(fleet): classify install drift and format age`
- [ ] LEDGER + CHECKPOINT

**Done when:** all five classes + the age table pass.

---

### Task 7 — Runner seam + `fleet status`  (plan Task 7)

- [ ] RED: `cmd/status_test.go::TestRenderTableIsWorstFirst`, `TestExitCodeNonZeroWhenAnyHostStale`
- [ ] RUN-RED: `cd sdk/fleet && go test ./cmd/ -run 'TestRenderTable|TestExitCode' -v` → **FAIL** (`undefined: Row`)
- [ ] GREEN: `internal/runner/runner.go` (`Runner`, `Exec`, `Fake`) + `cmd/status.go` (`Row`, `renderTable`, `exitCode`, fan-out)
- [ ] RUN-GREEN: `cd sdk/fleet && go test ./cmd/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task07/status.txt` → **PASS**
- [ ] VERIFY: `--json | jq .` parses; `echo $?` is 1 while a host is stale
- [ ] VERIFY (live): `fleet status | tee docs/mbo/plans/fleet/evidence/task07/live-status.txt` → one row per marked host, stale host shows `behind N`
- [ ] COMMIT: `feat(fleet): status command with worst-first table and stale exit code`
- [ ] LEDGER + CHECKPOINT

**Done when:** unit tests pass **and** a real multi-host capture exists.

---

### Task 8 — `fleet add` / `fleet remove`  (plan Task 8)

- [ ] RED: `TestWriteConfigTakesBackupFirst`, `TestDryRunWritesNothing`
- [ ] RUN-RED: `cd sdk/fleet && go test ./cmd/ -run 'TestWriteConfig|TestDryRun' -v` → **FAIL** (`undefined: writeConfig`)
- [ ] GREEN: `cmd/add.go` (`writeConfig`, `applyConfig`, `addCmd`), `cmd/remove.go` (`removeCmd`, `--purge`)
- [ ] RUN-GREEN: `cd sdk/fleet && go test ./cmd/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task08/membership.txt` → **PASS**
- [ ] VERIFY (live, on a COPY of the config): add → status shows it → remove → still `ssh`-able → `--purge` → block gone; diff into `evidence/task08/roundtrip.txt`
- [ ] COMMIT: `feat(fleet): add/remove fleet targets with backup and dry-run`
- [ ] LEDGER + CHECKPOINT

**Done when:** backup-per-write, dry-run-writes-nothing, live round-trip all pass.

---

### Task 9 — keys diff core  (plan Task 9)

- [ ] RED: `TestComputeAddsMissingAndFlagsForeignForRemoval`, `TestComputeNeverReturnsAnEmptyRemoteAsWholesaleRemoval`, `TestComputeIsNoOpWhenIdentical`
- [ ] RUN-RED: `cd sdk/fleet && go test ./internal/keys/ -v` → **FAIL** (`undefined: Compute`)
- [ ] GREEN: implement `Diff` + `Compute(local, remote)` — reports removals, never applies
- [ ] RUN-GREEN: `cd sdk/fleet && go test ./internal/keys/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task09/keys-diff.txt` → **PASS** (3 tests)
- [ ] COMMIT: `feat(fleet): authorized_keys diff (report removals, never apply)`
- [ ] LEDGER + CHECKPOINT

**Done when:** the three diff tests pass, including the defect-2 regression.

---

### Task 10 — `keys list` / `keys sync`  (plan Task 10)

- [ ] RED: `TestKeysSyncSendsOnlyPublicKeyMaterial`, `TestKeysSyncReportsPerHostFailure` (+ `recordingRunner`, `errFake`)
- [ ] RUN-RED: `cd sdk/fleet && go test ./cmd/ -run TestKeysSync -v` → **FAIL** (`undefined: syncKeyToHost`)
- [ ] GREEN: `cmd/keys.go` — `syncKeyToHost` (public key only), `keysCmd`, `list`, `sync`
- [ ] RUN-GREEN: `cd sdk/fleet && go test ./cmd/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task10/keys.txt` → **PASS**
- [ ] VERIFY: no `scp`, no `id_*` private path anywhere in the sync path (`grep -rn "scp\|id_ed25519\b" sdk/fleet/cmd/keys.go`)
- [ ] COMMIT: `feat(fleet): keys list/sync (public-key-only, per-host results)`
- [ ] LEDGER + CHECKPOINT

**Done when:** no-private-key and per-host-failure tests pass.

---

### Task 11 — `keys prune` / `delete` (diff-first)  (plan Task 11)

- [ ] RED: `TestPruneRequiresConfirmationAndChangesNothingWhenDeclined`, `TestPruneAppliesOnlyWhenConfirmed`
- [ ] RUN-RED: `cd sdk/fleet && go test ./cmd/ -run TestPrune -v` → **FAIL** (`undefined: pruneHost`)
- [ ] GREEN: implement `pruneHost` (compute diff → print → apply only when confirmed) + `--yes`
- [ ] RUN-GREEN: `cd sdk/fleet && go test ./cmd/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task11/prune.txt` → **PASS**
- [ ] 🛑 **HUMAN STOP** — the next step writes to a real host's `authorized_keys`. Get confirmation.
- [ ] VERIFY (live): add a throwaway `ssh-ed25519 ZZZ test@foreign` entry on one host; run `fleet keys prune`; answer **no**; confirm the entry survives → `evidence/task11/declined.txt`
- [ ] COMMIT: `feat(fleet): diff-first keys prune/delete with confirmation gate`
- [ ] LEDGER + CHECKPOINT

**Done when:** a declined prune provably changes nothing on a real host.

---

### Task 12 — `fleet update` + dirty-clone policy  (plan Task 12)

- [ ] RED: `TestUpdateSkipsDirtyCloneByDefault`, `TestUpdateProceedsOnCleanClone`
- [ ] RUN-RED: `cd sdk/fleet && go test ./cmd/ -run TestUpdate -v` → **FAIL** (`undefined: updateHost`)
- [ ] GREEN: `cmd/update.go` — `UpdateResult`, `updateHost`, `remoteUpdate`, `rescueWorktree`
- [ ] RUN-GREEN: `cd sdk/fleet && go test ./cmd/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task12/update.txt` → **PASS**
- [ ] 🛑 **HUMAN STOP** — verify the rescue mechanism before relying on it.
- [ ] VERIFY: run the scratch-clone stash→branch→worktree check (plan Task 12 Step 5) → prints `b`. **If it fails: file a blocker and switch to the temp-commit fallback.**
- [ ] COMMIT: `feat(fleet): headless update with skip-on-dirty and --force rescue`
- [ ] LEDGER + CHECKPOINT

**Done when:** both unit tests pass and the rescue mechanism is proven (or a blocker filed).

---

### Task 13 — TUI  (plan Task 13)

- [ ] SETUP: confirm Task 12 has landed — `cmd/tui.go` reuses `remoteUpdate` from `cmd/update.go`
- [ ] RED: `TestTUIModelRendersOneLinePerHost`, `TestTUISelectionMovesWithinBounds`, `TestInteractiveUpdateUsesTTYAndTheSharedRemoteScript`
- [ ] RUN-RED: `cd sdk/fleet && go test ./cmd/ -run TestTUI -v` → **FAIL** (`undefined: newModel`)
- [ ] GREEN: `cmd/tui.go` — `model`, `newModel`, `moveCursor`, `View`, `Update` (`u` → `tea.Exec`)
- [ ] RUN-GREEN: `cd sdk/fleet && go test ./cmd/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task13/tui.txt` → **PASS**
- [ ] VERIFY (live): run `fleet tui`; capture a still/asciinema → `evidence/task13/`
- [ ] VERIFY: `u` releases the terminal (sudo prompt is answerable), does not capture stdin
- [ ] COMMIT: `feat(fleet): TUI host list with update key`
- [ ] LEDGER + CHECKPOINT

**Done when:** model tests pass and a screen capture exists.

---

### Task 14 — Live update of the stale host  (plan Task 14) — **the end-to-end proof**

- [ ] 🛑 **HUMAN STOP** — this modifies a real machine. Get confirmation before starting.
- [ ] VERIFY: `fleet status | tee docs/mbo/plans/fleet/evidence/task14/before.txt` → stale host shows `behind N` / `unknown`
- [ ] RUN: `fleet update <stale-host> 2>&1 | tee docs/mbo/plans/fleet/evidence/task14/transcript.txt` → interactive; `install.sh` prompts answerable; completes
- [ ] VERIFY: `fleet status | tee docs/mbo/plans/fleet/evidence/task14/after.txt` → that host is `up-to-date` with a fresh stamp
- [ ] COMMIT: `test(fleet): live update evidence for the stale host`
- [ ] LEDGER + CHECKPOINT

**Done when:** before=stale, after=up-to-date, transcript in between. **If the ARM/Jetson
host fails, file a blocker — do not mark the objective done.**

---

### Task 15 — Parity, retirement, docs  (plan Task 15)

- [ ] VERIFY: `bash src/ssh-key-sync/ssh-key-sync.sh --list | tee docs/mbo/plans/fleet/evidence/task15/legacy-list.txt`
- [ ] VERIFY: `fleet keys list | tee docs/mbo/plans/fleet/evidence/task15/fleet-list.txt`
- [ ] VERIFY: annotate scope-driven differences (all-config-hosts → `#fleet`) **in the evidence file**
- [ ] 🛑 **HUMAN STOP** — the next step deletes `src/ssh-key-sync/`. Get confirmation.
- [ ] GREEN: `git rm -r src/ssh-key-sync`
- [ ] VERIFY: `grep -rn "ssh-key-sync" --exclude-dir=.git . | grep -v docs/mbo | tee docs/mbo/plans/fleet/evidence/task15/refs.txt` → **empty**
- [ ] DOCS: write `sdk/fleet/AGENTS.md` + `CLAUDE.md` symlink; add `fleet` to root `AGENTS.md`
- [ ] DOCS: `docs/mbo/index.md` state → `in-review`
- [ ] VERIFY: `cd sdk/fleet && go test ./... -cover` → every package ≥ 60%
- [ ] VERIFY: `./scripts/test.sh` → green, lists `fleet`
- [ ] COMMIT: `refactor(fleet): retire ssh-key-sync after parity; document fleet`
- [ ] LEDGER + CHECKPOINT

**Done when:** parity captured, no dangling references, index advanced, coverage ≥ 60%.

---

### Wrap-up

- [ ] Tick every box in `TRACKING.md` §3 (the stop condition)
- [ ] Create the demo entry per the repo show-and-tell convention
- [ ] Replace `IMPLEMENTATION.md` §8 with the next session's kickoff prompt (or note the run is complete)
- [ ] 🛑 Promote PR #223 to ready-for-review (human-confirmed publish)
