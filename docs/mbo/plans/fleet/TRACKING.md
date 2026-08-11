# fleet — live state ledger

- **Slug:** fleet
- **Started:** 2026-08-09
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../fleet.md`](../fleet.md) · spec [`../../specs/fleet.md`](../../specs/fleet.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a
> commit SHA **and** evidence. **Never write a result you did not observe.**

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| design (this) | `fleet/<user>/mbo` | `feature/fleet/<user>/mbo` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/fleet/<user>/mbo` | [#223](https://github.com/sfc-gh-eraigosa/dotfiles/pull/223) | draft |
| `scaffold` | — | — | — | — | not created |
| `stamp-sh` | — | — | — | — | not created |
| `sshconf` | — | — | — | — | not created |
| `drift` | — | — | — | — | not created |
| `status` | — | — | — | — | not created |
| `membership` | — | — | — | — | not created |
| `keys` | — | — | — | — | not created |
| `tui` | — | — | — | — | not created |
| `migration` | — | — | — | — | not created |

> Leaf rows are placeholders until CAP-B fan-out is chosen. Building sequentially in the
> single worker above is the default — in that case leave them `not created` and record all
> task commits against the design worker.

## 1. Task ledger

| Task | Leaf | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- | :-- |
| 1 — module scaffold + wiring | scaffold | **done** | `d25235b` | `./scripts/test.sh` → `OK: coverage for fleet is 70% (minimum: 60%)`; `go vet` clean; `~/opt/bin/fleet version` → `fleet 0.1.0 (1e7eca0)`; `bash -n install.sh` clean → `evidence/task01/wiring.txt` | BLOCKING; coverage floor added. **Deviation:** version vars must be EXPORTED (`Version`/`Commit`/`Dirty`/`BuildDate`) to match build.sh's `-X` paths — lowercase would silently no-op. Test `TestLdflagsTargetsExist` pins it. |
| 2 — install.sh stamp | stamp-sh | **done** | `7610417` | `install-stamp_test.sh` → PASS=12 FAIL=0; `install_test.sh` → PASS=13 FAIL=0; `bash -n` clean → `evidence/task02/stamp.txt` | **Deviation:** implemented as `opt/scripts/system/install-stamp.sh` + its own `*_test.sh` rather than the plan's root-level `install.sh.stampblock`. Reason: `install_test.sh` deliberately never EXECUTES install.sh, and the repo already has the executable-script + sibling-test pattern. Added a guard asserting nothing runs after the stamp. |
| 3 — sshconf reader | sshconf | **done** | `9924b40` | `go test ./internal/sshconf/ -cover` → ok, 95.2% → `evidence/task03/parse.txt` | **Finding:** docs write the marker `# fleet`, the `--marker` default is `#fleet`. hasMarker strips the leading `#` and compares tokens so both spellings work while `# fleetwood` still does NOT match (the plan's `ReplaceAll(" ","")` approach would have matched it). |
| 4 — sshconf writer (add/unmark/purge) | sshconf | **done** | `9924b40` | `go test ./internal/sshconf/ -cover` → ok, 93.9%, 15 tests → `evidence/task04/writer.txt`; `go vet` clean | Idempotent upsert via purge+append; Unmark handles both trailing and own-line markers; Purge absorbs orphaned blank lines so add/purge cycles don't accumulate whitespace. |
| 5 — stamp parser | drift | **done** | `df0ec5c` | `go test ./internal/stamp/ -cover` → ok, **100.0%**, 6 tests → `evidence/task05/stamp.txt` | Strict on commit length + installed_at so a truncated write can't masquerade as a valid install; unknown keys ignored for forward compat. |
| 6 — drift classify + age | drift | **done** | `df0ec5c` | `go test ./internal/drift/ -cover` → ok, **100.0%**, 6 tests → `evidence/task06/drift.txt`; `grep time.Now()` in non-comment code → 0 | Clock injected (verified). Added two tests beyond the plan: unreachable dominates every other signal, and a zero install time renders `-` rather than a fabricated duration. |
| 7 — Runner seam + `fleet status` | status | **done** | `359d90e` | unit: `go test ./cmd/ -cover` → ok 52.6%, incl. worst-first ordering, exit codes, 4-host classify via fakes. **LIVE on 3 real hosts** → `evidence/task07/live-status.txt`: all 3 report `unknown` (correct — the stamp ships in this branch, no host has run it yet), `--json` parses under jq, an unreachable host is reported not dropped, explicit args override discovery, exit=1 throughout. | Evidence sanitized of local identity (placeholders) per repo privacy rule. |
| 8 — `fleet add` / `remove` | membership | **done** | `ec4e7fb` | unit: backup-holds-ORIGINAL, 0600 perms kept, dry-run writes nothing, new-file needs no backup. **LIVE round-trip on a config copy** → `evidence/task08/roundtrip.txt`: add→status→re-add(1 block)→unmark(ssh still resolves 10.0.0.99, out of scope)→purge, ending **BYTE-IDENTICAL to the original**, 3 backups taken. | **Defect found by the live capture:** add→purge dropped the file's trailing newline (spurious `\ No newline at end of file` in every later diff — a collateral edit). Reproduced as a unit test, fixed with `normalize()` on all three write paths. |
| 9 — keys diff core | keys | **done** | `8fc1a41` | `go test ./internal/keys/ -cover` → ok **100.0%**, 6 tests → `evidence/task09/keys-diff.txt` | Both defect-2 directions pinned: empty remote ≠ wholesale removal, and empty local reports every remote key as a REPORTED removal (never applied). Also normalizes whitespace and ignores blanks/comments so there is no phantom churn. |
| 10 — `keys list` / `sync` | keys | **done** | `8fc1a41` | `go test ./cmd/ -run TestKeysSync -v` → 3 PASS; grep shows no private-key transfer in code; **LIVE read-only** `fleet keys list` → `id_ed25519` authorized on all 3 hosts; `scripts/test.sh` → `OK: coverage for fleet is 71% (minimum: 60%)` → `evidence/task10/keys.txt` | Remote append is grep-guarded (`grep -qxF`) so re-syncing is a no-op; failures name the host and increment an exit-code counter (defect 4). |
| 11 — `keys prune` / `delete` | keys | **unit done** | `f0a2b3c`→see log | `go test ./cmd/ -run TestPrune -v` → 3 PASS (declined changes nothing / applies only when confirmed / no-op when nothing foreign) → `evidence/task11/prune.txt` | Removal is a targeted `grep -vxF` delete of that exact line, never a rewrite from local state. **LIVE declined-prune on a real host still owed — human stop.** |
| 12 — `fleet update` + dirty policy | tui | **done** | see log | `go test ./cmd/ -run 'TestUpdate\|TestForce\|TestRescue'` → 5 PASS; **rescue mechanism verified on a scratch clone, git 2.47.3** → `evidence/task12/rescue-verification.txt` | **DATA-LOSS DEFECT FOUND + FIXED.** The plan's `git stash push -u` + `git branch <n> stash@{0}` recovers tracked edits but SILENTLY LOSES untracked files (a stash commit's tree excludes them — they live in `stash^3`). Verified empirically. Replaced with a temp-commit rescue (`git add -A` onto a rescue branch) which preserves both; pinned by `TestRescuePreservesUntrackedWork`. |
| 13 — TUI | tui | **done** | `be68ff5` | `go test ./cmd/ -run 'TUI\|Interactive' -v` → 5 PASS (rows render, cursor bounded, empty fleet no panic, worst-first like the table, ssh -t + shared script) → `evidence/task13/tui.txt` | **API correction:** `tea.Exec` takes a `tea.ExecCommand`; a raw `*exec.Cmd` needs `tea.ExecProcess`. The plan's snippet would not have compiled. |
| 14 — live update of the stale host | tui | todo | | | **human-in-the-loop**; the end-to-end proof |
| 15 — parity, retirement, docs | migration | todo | | | **human-in-the-loop**: deletes `src/ssh-key-sync/` |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 stamp on install | [ ] `install_test.sh::test_stamp_written_only_for_phase_all` | [ ] stamp present after a real install (Task 14) | not retroactive |
| F2 host discovery | [ ] `sshconf::TestParseReturnsOnlyMarkedConcreteHosts` + `…SkipsPatternHosts` | — | |
| F3 status table | [ ] `cmd::TestRenderTableIsWorstFirst` | [ ] live multi-host capture (Task 7) | |
| F4 classification | [ ] `drift::TestClassifyAllFiveClasses` + `…NeverReportsBehindWhenCommitsMatch` | — | |
| F5 JSON + exit code | [ ] `cmd::TestExitCodeNonZeroWhenAnyHostStale` | [ ] `--json \| jq .` + `echo $?` | |
| F6 TUI | [ ] `cmd::TestTUIModelRendersOneLinePerHost` + `…SelectionMovesWithinBounds` | [ ] screen capture (Task 13) | |
| F7 update action | [ ] `cmd::TestUpdateProceedsOnCleanClone` | [ ] **live transcript (Task 14)** | cannot be faked |
| F8 dirty-clone safety | [ ] `cmd::TestUpdateSkipsDirtyCloneByDefault` | [ ] rescue-worktree proof (Task 12 §5) | |
| F9 add target | [ ] `sshconf::TestAddIsIdempotent…` + `cmd::TestWriteConfigTakesBackupFirst` + `TestDryRunWritesNothing` | [ ] round-trip diff (Task 8) | |
| F10 remove target | [ ] `sshconf::TestUnmarkKeepsBlock…` + `TestPurgeRemovesOnlyTheTargetBlock` + `TestUnknownAliasIsAnError` | [ ] `ssh <alias>` still works after `remove` | |
| F11 keys list | [ ] `keys::TestComputeIsNoOpWhenIdentical` | [ ] parity vs legacy `--list` (Task 15) | |
| F12 keys sync | [ ] `cmd::TestKeysSyncSendsOnlyPublicKeyMaterial` + `…ReportsPerHostFailure` | [ ] transcript shows no private key | defect 1 |
| F13 keys prune/delete | [ ] `cmd::TestPruneRequiresConfirmation…` + `TestPruneAppliesOnlyWhenConfirmed` + `keys::TestComputeAddsMissingAndFlagsForeign…` | [ ] **foreign key survives a declined prune (Task 11)** | defect 2 |

## 3. Validation done-when — the stop condition

- [ ] Tasks 1–15 all `done`, each with a commit SHA **and** observed evidence
- [ ] `cd sdk/fleet && go test ./... -cover` ≥ 60% for every package
- [ ] `./scripts/test.sh` green and listing `fleet`
- [ ] `bash -n install.sh` clean
- [ ] `go vet ./...` clean in `sdk/fleet`
- [ ] Live multi-host `status` capture exists (Task 7)
- [ ] Declined-prune capture shows the foreign key surviving (Task 11)
- [ ] Rescue-worktree mechanism verified, or a blocker filed with the fallback (Task 12)
- [ ] Live update transcript: stale host → `up-to-date` (Task 14)
- [ ] Parity capture recorded and scope differences annotated (Task 15)
- [ ] `grep -rn "ssh-key-sync" --exclude-dir=.git . | grep -v docs/mbo` is empty (Task 15)
- [ ] `docs/mbo/index.md` state → `in-review`
- [ ] Demo entry created per the repo show-and-tell convention

## 4. Blockers & escalations

Failing command + its **real** output. Contract defects go here and get escalated — never
silently patched.

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |
| | | *(none yet)* | | |

**Pre-identified risks to watch** (not yet blockers):

| Risk | Where it bites | What to do |
| :-- | :-- | :-- |
| `git branch <name> stash@{0}` may not behave across git versions | Task 12 `--force` rescue | Verify in Task 12 §5; fall back to a temp-commit rescue branch and record it |
| ARM/Jetson host is the only stale machine **and** the least-tested target | Task 14 | If `install.sh` fails there, file a blocker — do not mark the objective done |
| Local `main` has carried foreign unpushed commits | every task (P3) | `git log origin/main..HEAD` before starting; rebase `--onto origin/main` |
| `gss feature audit` has reported an open PR as "merged" | any checkpoint | Verify PR state with `gh` before acting on audit output |
| Coverage gate is repo-wide warn-only (`COVERAGE_ENFORCE=0`) | Task 15 | Treat 60% as hard for this objective; check the number, don't trust green CI |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-08-11 | review fixes | Self-review of PR #224 found 5 real gaps + 3 convention gaps; all fixed TDD. Two were correctness bugs in `keys prune` — the highest-stakes feature. Also caught a regression introduced BY the fix (SilenceErrors hid real errors) before it shipped. |
| 2026-08-10 | build s3 | Tasks 9-12. keys diff 100%, public-key-only sync (live `keys list` across 3 hosts), diff-first prune, update+dirty policy. Caught and fixed a data-loss defect in the plan's rescue mechanism. Gate: 73%. |
| 2026-08-10 | build s2 | Tasks 7-8. Live status against 3 real hosts (all `unknown` — correct, stamp not yet deployed). Membership round-trip byte-identical. Scrubbed local identity from evidence + rewrote branch history (unpushed) after finding hostnames/paths in a committed capture. |
| 2026-08-10 | build s1 | Task 1 done: sdk/fleet scaffold + Makefile/coverage-floor/gff-flag/install.sh wiring; binary builds, ldflags verified. Preflight P3 caught the foreign-commit hazard again (worktree based on stale local main) — reset to origin/main. P4: the Jetson host is currently UNREACHABLE (blocks Task 14). |
| 2026-08-09 | design | Design + spec written and committed; issue #222 and draft PR #223 opened; scope extended with membership + keys (absorbing `ssh-key-sync`, four defects documented); plan + execution trio authored. Build not started. |
