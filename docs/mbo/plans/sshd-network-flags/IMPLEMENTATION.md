# sshd + WSL networking as gff flags — implementation playbook

- **Slug:** sshd-network-flags
- **Date:** 2026-10-18
- **Status:** Ready to execute
- **Plan (source of truth):** [`../sshd-network-flags.md`](../sshd-network-flags.md) · spec [`../../specs/sshd-network-flags.md`](../../specs/sshd-network-flags.md)
- **Objective anchors:** PR #409 (no separate issue, by the operator's choice) · `docs/mbo/index.md` row `sshd-network-flags`

> This file is the **procedure**. It does not restate the plan; it tells a fresh agent session
> how to execute it, task by task, resumably. The plan wins any conflict.

## 0. The three files

| File | Role | Written by |
| :-- | :-- | :-- |
| `IMPLEMENTATION.md` (this) | Procedure: preconditions, worker map, per-task loop, hard rules, kickoff prompt | Read-only during the run (except §7 corrections) |
| [`TRACKING.md`](./TRACKING.md) | Live **state ledger**: per-task status/commit/evidence, proof matrix, blockers, append-only session log | Updated after **every** task |
| [`TODO.md`](./TODO.md) | The **cursor**: plan tasks expanded into ordered micro-steps | Checkboxes ticked as you go |

**Resumption rule:** the first unchecked box in `TODO.md` is the next action, and `TRACKING.md`
says what has been proven. Re-run the last verification command before continuing; the ledger
is a claim, the command is the proof.

## 1. Preconditions

| Check | Verify |
| :-- | :-- |
| On the PR branch in the main checkout | `git -C ~/git/dotfiles branch --show-current` → `fix/wsl-distro-detect` |
| PR #409 open | `gh pr view 409 --json state -q .state` → `OPEN` |
| Clean tree before each task | `git status --short` → empty (or only this task's files) |
| gff builds | `bash sdk/gff/build.sh && ~/opt/bin/gff version` |
| PowerShell reachable (for LIVE tiers) | `command -v powershell.exe` |
| Go for fleet | `cd sdk/fleet && go version` |

## 2. Worker map

A single worker: the main checkout `~/git/dotfiles` on branch `fix/wsl-distro-detect` (PR #409).
No `gss feature` workers and no breakout (operator's choice). **Never run `install.sh` from a
worktree** (repo rule); the H1 gate runs it from the main checkout.

## 3. The execution loop (every task)

1. Locate: the first unchecked `TODO.md` box → its plan task; read the plan task fully.
2. RED: write the failing test first; run it; **verify it fails**; record the failure line.
3. GREEN: implement the minimum; run it to pass.
4. Gates: the task's extra checks (shellcheck, portability, gff lint, coverage, PS parser).
5. Evidence: `{ date -u; echo "\$ <cmd>"; <cmd>; } 2>&1 | tee -a evidence/task-N/<gate>.txt`
6. Ledgers: tick `TODO.md`; update the `TRACKING.md` row (status, commit, evidence).
7. Commit with the plan's exact message, staging by explicit name.
8. Checkpoint: confirm with the operator (interactive prompt), then mint the token
   (`git rev-parse HEAD > ~/.config/gss/approval.token`) in **one** call and run `gss push` in
   a **separate** call. Then refresh the PR #409 description.

## 4. Done-when gates

- Per task: the plan task's Step 4 gate passes, and its evidence is committed.
- Objective stop condition: `TRACKING.md` §3 fully ticked, including the human gates H1–H3.

## 5. Hard rules

- Tests first, every task. A test you never saw fail proves nothing.
- `opt/lib/*.sh` stays POSIX and runs under both `bash` and `sh`.
- `make lint-portability` 0 Tier 1/2; `shellcheck` clean; `sdk/fleet` coverage floor holds.
- Never run `wsl --shutdown`; never create a Hyper-V switch (spec §8).
- No real host/user names, IPs or MACs in committed files (`privacy_guard`). Redact transcripts.
- Confirm with the operator before every `git add`/`commit`/`gss push` (repo rule). Within an
  approved checkpoint, run commit → push → PR refresh without re-asking per step.
- Evidence before assertions: never mark a row `done` without a SHA and the observed output.

## 6. Command cheat-sheet

```sh
bash opt/lib/gff_test.sh && sh opt/lib/gff_test.sh
bash opt/lib/sshdflags_test.sh && sh opt/lib/sshdflags_test.sh
bash opt/lib/wslnet_test.sh && sh opt/lib/wslnet_test.sh
bash opt/bin/sshd-setup_test.sh
bash opt/scripts/system/wsl-network_test.sh
bash opt/scripts/system/wsl-distro_test.sh
(cd ~/git/dotfiles && ~/opt/bin/gff lint)
(cd sdk/fleet && go test ./... && gofmt -l .)
./scripts/test.sh            # repo-wide, enforces the fleet coverage floor
make shell-test lint-shell lint-portability lint-markdown
```

## 7. Resuming, recovery, and corrections

Fix wrong commands here freely and note it in the session log. Never edit the plan or spec as
part of the build; escalate contract defects through the TRACKING blockers table instead.

## 8. Kickoff prompt (always CURRENT — history lives in git)

> **Maintenance rule:** exactly ONE prompt here, the one that starts the NEXT session.
> Replace it at session end (past prompts are in `git log -- <this file>`).

```
Execute the sshd-network-flags plan in ~/git/dotfiles on branch fix/wsl-distro-detect (PR #409).
Read first: docs/mbo/plans/sshd-network-flags.md, docs/mbo/specs/sshd-network-flags.md, then
docs/mbo/plans/sshd-network-flags/{IMPLEMENTATION,TRACKING,TODO}.md.
Start at the first unchecked box in TODO.md (Task 1 on a fresh start) and follow
IMPLEMENTATION §3 for every task: RED → RUN-RED → GREEN → RUN-GREEN → gates → evidence → ledger → commit.
Stop and ask the operator before every commit/push checkpoint, and at the human gates H1–H3 (Task 9).
Record any blocker in TRACKING §4 with the real command output; never patch the spec silently.
Done when TRACKING §3 is fully ticked.
```
