# fleet vault-backed workstation keys — implementation playbook

- **Slug:** fleet-vault-keys
- **Date:** 2026-11-13
- **Status:** Ready to execute
- **Plan (source of truth):** [`../fleet-vault-keys.md`](../fleet-vault-keys.md) · spec [`../../specs/fleet-vault-keys.md`](../../specs/fleet-vault-keys.md) · design [`../../designs/fleet-vault-keys.md`](../../designs/fleet-vault-keys.md)
- **Objective anchors:** issue #451 · PR #452 · `docs/mbo/index.md` row `fleet-vault-keys`

> This file is the **procedure**. It does not restate the plan — it tells a fresh agent
> session how to execute the plan, task by task, resumably. The plan wins any conflict.

## 0. The three files

| File | Role | Written by |
| :-- | :-- | :-- |
| `IMPLEMENTATION.md` (this) | Procedure: preconditions, worker map, per-task loop, hard rules, kickoff prompt | Read-only during the run (except §7 corrections) |
| [`TRACKING.md`](./TRACKING.md) | Live **state ledger**: per-task status/commit/evidence, proof matrix, blockers, append-only session log | Updated after **every** task |
| [`TODO.md`](./TODO.md) | The **cursor**: plan tasks expanded into ordered micro-steps | Checkboxes ticked as you go |

**Resumption rule:** the first unchecked box in `TODO.md` is the next action; `TRACKING.md`
says what has been proven. Re-run the last verification command before continuing —
the ledger is a claim, the command is the proof.

## 1. Preconditions

| Check | Verify command | Expect |
| :-- | :-- | :-- |
| Go toolchain | `go version` | matches `sdk/fleet/go.mod` `go` line |
| OpenSSH | `ssh -V` | ≥ 8.4 (askpass `force`); older → note in TRACKING |
| Vendor CLIs | `command -v op lpass` | both present (Task 0 needs them signed in by the operator) |
| Worker exists | `gss feature list --feature fleet-vault-keys --json` | worker `design` present, PR #452 |
| Base is current | `git -C <worktree> fetch origin && git -C <worktree> log --oneline -1 origin/main` | rebase if the worker is behind |
| Fleet suite green on base | `./scripts/test.sh` (repo root) | PASS except the pre-existing failures listed in §5 |

## 2. Worker map

**Operator rulings 2026-11-13: four stacked PRs** (1Password moved last — needs an account) (supersedes the plan §6.1 single-PR default):

| PR | Tasks | Worker | Base |
| :-- | :-- | :-- | :-- |
| 1 save/push (#452, **merged** 90d1ed1) | docs, T1, T2, T3, T4, T5, T6, T7, T9, T10 | `fleet-vault-keys/${USER}/design` (removed) | `main` |
| 2 pull (#457, **merged** into #452) | T11 | `fleet-vault-keys/${USER}/pull` (removed) | PR 1's branch |
| 3 rest | T8, T12, T13, T14, T15, T16 | `fleet-vault-keys/${USER}/lifecycle` (exists; rebased 2026-11-15) | `main` |
| 4 1Password | T3 (built: local branch `fleet-vault-keys-op-adapter-saved` @ 953dca5, reviewed) + its live test G1 | `fleet-vault-keys/${USER}/op` (create after PR 3, once the operator has a 1Password account) | PR 3's branch |

Record each new worker's `gss feature worker add --json` output verbatim below when created.

`pull` (2026-11-13, user and home path shown as `${USER}` / `$HOME`):

```json
{
  "worker_ref": "fleet-vault-keys/${USER}/pull",
  "branch": "feature/fleet-vault-keys/${USER}/pull",
  "worktree_path": "$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/fleet-vault-keys/${USER}/pull",
  "base_branch": "feature/fleet-vault-keys/${USER}/design"
}
```

Original single-PR row (PR 1):

| Leaf | Worker ref | Branch | Worktree path | PR |
| :-- | :-- | :-- | :-- | :-- |
| all (T0–T16) | `fleet-vault-keys/${USER}/design` | `feature/fleet-vault-keys/${USER}/design` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/fleet-vault-keys/${USER}/design` | #452 (draft) |

If the operator opts into the parallel breakout, cut at plan §6.1 (iface → {adapters, local} → verbs),
create workers blocking-first with `--base`, and re-record the `--json` output here verbatim.

## 3. The execution loop (every task)

1. Locate: first unchecked `TODO.md` box → its plan task; read the plan task fully.
2. RED: write the failing test first; run it; **verify it fails**; record the failure line.
3. GREEN: implement the minimum; run to pass.
4. Gates: `go vet ./...`, the task's package coverage ≥ 60 %, `git status --short -- <new paths>` (allowlist).
5. Evidence: `tee` the RUN-RED/RUN-GREEN/gate output into `evidence/T<n>/` with a dated header.
6. Ledgers: tick `TODO.md`; update the `TRACKING.md` task row (status, commit, evidence).
7. Commit with the plan's exact message, staging by explicit name, unsigned
   (`git -c commit.gpgsign=false commit`). Checkpoint after each task group (T2, T4, T7, T11, T15, T16)
   with `gss feature checkpoint --worker fleet-vault-keys/${USER}/design` — after operator confirmation.

## 4. Done-when gates

- Per task: the plan task's tests pass, package ≥ 60 %, evidence committed.
- Objective: TRACKING §3 fully ticked — every spec §5 feature has an automated proof **and** its
  live gate (G1–G6) transcript; `./scripts/test.sh` + `make lint-go` green; README demos are real output.

## 5. Hard rules

- §3 interface contracts are **frozen** after Task 1; a needed change is a TRACKING blocker first.
- No secret (private key, passphrase, vault payload) in argv, env, logs, test names, evidence files,
  or committed fixtures. Test fixtures use markers like `PRIVATE-MATERIAL-123`, never real key text.
- Default behaviour without `vault.yaml` / with the flag off must stay byte-identical (golden argv tests).
- Confirm with the operator (AskUserQuestion) before every `git add`/commit/checkpoint/`pr --ready`.
- Never run `install.sh` from the worker worktree. Use `$HOME`/`${USER}` in commands, never literals.
- Evidence before assertions: a row is `done` only with a SHA and observed output.
- Pre-existing failures (record, do not fix here): `TestLoadPlanUsesBuiltInWhenNoFile`,
  `TestLoadPlanReadsTheConfiguredPath`, `TestGitRevParseRefusesADirectoryInsideAnotherRepo` (macOS),
  CI "Shell Test Drivers" `symlink idempotency guard present`.

## 6. Command cheat-sheet

```bash
W="$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/fleet-vault-keys/${USER}/design"
cd "$W/sdk/fleet"
go test ./internal/vault/... -run TestName -v          # one test
go test ./... -cover                                     # package coverage
cd "$W" && ./scripts/test.sh                            # full suite + 60 % floor (repo root)
cd "$W" && make lint-go                                  # lint
git -C "$W" -c commit.gpgsign=false commit -m "<plan message>"
gss feature checkpoint --worker fleet-vault-keys/${USER}/design   # after confirmation
```

## 7. Resuming, recovery, and corrections

Fix wrong commands here freely and note it in the session log. Never edit the plan/spec
as part of the build — escalate contract defects via TRACKING blockers instead.
If a checkpoint rebase fails on signing, rebase manually with
`git -c commit.gpgsign=false rebase origin/main`, then checkpoint again.

## 8. Kickoff prompt (always CURRENT — history lives in git)

> **Maintenance rule:** exactly ONE prompt here — the one that starts the NEXT session.
> Replace it at session end (past prompts are in `git log -- <this file>`).

```text
Continue the fleet-vault-keys build (issue #451) with PR 3 on gss worker
fleet-vault-keys/${USER}/lifecycle (worktree
$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/fleet-vault-keys/${USER}/lifecycle,
branch based on main). PR 1 #452 and PR 2 #457 are MERGED (main 90d1ed1+): vault setup/status/push/pull ship.
Read first, in the worktree: docs/mbo/plans/fleet-vault-keys/TRACKING.md (§0 stack, §1 ledger, §6 rulings +
deferred review findings), TODO.md (first unchecked box = next action), this file, then plan Tasks 8 and 12–16.
State: the branch is main + T8 ("stop before dialling when the key is locked") + this handoff docs commit; T8 is
built but UNREVIEWED; nothing is pushed and there is no PR 3 yet. Before anything:
`git fetch && git -c commit.gpgsign=false rebase origin/main`, then `cd sdk/fleet && go test ./...`
(expect only the 3 known failures in §5).
Next, in order:
 1. T8 fix: existing status/update/keys-sync/tui tests now reach the real ~/.ssh + ssh-agent via keyLocked — stub it
    (test-isolation rule). Then task review of T8; fix loop until clean.
 2. T12 keys unlock, T13 runner probe + keys rotate, T14 roster-aware sync/prune (wire vaultActive), T15 TUI V,
    T16 docs + live gates — subagent-driven, TDD, one task review each (opus for security-heavy work).
 3. After T12/T13 land, re-point the push/pull hints and the features.yaml description at `keys unlock`/`rotate` (§6).
 4. Final whole-branch review on the most capable model, seeded with TRACKING §6's deferred list.
Rules: local unsigned commits (`git -c commit.gpgsign=false commit`, stage by name) are pre-authorized; ask
(AskUserQuestion) before every checkpoint/push/PR/`pr --ready`. The SDD ledger is git-ignored scratch and is lost
when a worktree is removed — keep TRACKING.md current after every task. Use $HOME/${USER}, never literal
paths/usernames; no secret literals. Live vault writes and live gates G1–G6 only with the operator.
PR 4 (1Password live gate) waits for a 1Password account.
Operator side-quest pending: the first live capture — `gff set fleet.vault.enabled true`, `fleet vault setup`,
`fleet vault push` on the trusted node (`--encrypt-local` only after `keys unlock` exists).
```
