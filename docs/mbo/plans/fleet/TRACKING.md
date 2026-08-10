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
| 1 — module scaffold + wiring | scaffold | **done** | (this commit) | `./scripts/test.sh` → `OK: coverage for fleet is 70% (minimum: 60%)`; `go vet` clean; `~/opt/bin/fleet version` → `fleet 0.1.0 (1e7eca0)`; `bash -n install.sh` clean → `evidence/task01/wiring.txt` | BLOCKING; coverage floor added. **Deviation:** version vars must be EXPORTED (`Version`/`Commit`/`Dirty`/`BuildDate`) to match build.sh's `-X` paths — lowercase would silently no-op. Test `TestLdflagsTargetsExist` pins it. |
| 2 — install.sh stamp | stamp-sh | todo | | | independent; phase-gated |
| 3 — sshconf reader | sshconf | todo | | | |
| 4 — sshconf writer (add/unmark/purge) | sshconf | todo | | | idempotency is the key property |
| 5 — stamp parser | drift | todo | | | |
| 6 — drift classify + age | drift | todo | | | |
| 7 — Runner seam + `fleet status` | status | todo | | | needs a live multi-host capture |
| 8 — `fleet add` / `remove` | membership | todo | | | backup + dry-run |
| 9 — keys diff core | keys | todo | | | defect-2 regression lives here |
| 10 — `keys list` / `sync` | keys | todo | | | public-key-only |
| 11 — `keys prune` / `delete` | keys | todo | | | **human-in-the-loop**: live declined prune |
| 12 — `fleet update` + dirty policy | tui | todo | | | **human-in-the-loop**: rescue verification |
| 13 — TUI | tui | todo | | | |
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
| 2026-08-10 | build s1 | Task 1 done: sdk/fleet scaffold + Makefile/coverage-floor/gff-flag/install.sh wiring; binary builds, ldflags verified. Preflight P3 caught the foreign-commit hazard again (worktree based on stale local main) — reset to origin/main. P4: the Jetson host is currently UNREACHABLE (blocks Task 14). |
| 2026-08-09 | design | Design + spec written and committed; issue #222 and draft PR #223 opened; scope extended with membership + keys (absorbing `ssh-key-sync`, four defects documented); plan + execution trio authored. Build not started. |
