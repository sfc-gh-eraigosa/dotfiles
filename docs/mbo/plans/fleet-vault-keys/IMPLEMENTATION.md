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

Sequential, single PR (plan §6.1 default).

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
Execute the fleet-vault-keys plan on gss worker fleet-vault-keys/${USER}/design (PR #452, issue #451).
Read first: docs/mbo/plans/fleet-vault-keys.md, its spec + design, then this folder's
IMPLEMENTATION.md, TRACKING.md, TODO.md. Start at the first unchecked TODO box.
Scope, in order: Task 0 (vendor spike — STOP and ask the operator to sign in to op and lpass,
then record transcripts), Tasks 1–2 (vault contract, payload, roster), 3–4 (adapters, using the
spike-confirmed argv), 5–7 (config/gate, keystore, agent), 8–15 (pre-flight, verbs, TUI), 16 (docs + live gates).
TDD every task; tee evidence; update TRACKING after each task.
Stop and ask the operator: before every commit/checkpoint, for Task 0 sign-in, and for each live gate G1–G6.
Blocked → TRACKING §4 with the real output, then continue with the next independent task.
Done when TRACKING §3 is fully ticked.
```
