# fleet — implementation playbook

- **Slug:** fleet
- **Date:** 2026-08-09
- **Status:** Ready to execute
- **Plan (source of truth):** [`../fleet.md`](../fleet.md) · spec [`../../specs/fleet.md`](../../specs/fleet.md) · design [`../../designs/fleet.md`](../../designs/fleet.md)
- **Objective anchors:** issue [#222](https://github.com/sfc-gh-eraigosa/dotfiles/issues/222) · PR [#223](https://github.com/sfc-gh-eraigosa/dotfiles/pull/223) · `docs/mbo/index.md` row `fleet`

> This file is the **procedure**. It does not restate the plan — it tells a fresh agent
> session how to execute the plan, task by task, resumably. **The plan wins any conflict.**

## 0. The three files

| File | Role | Written by |
| :-- | :-- | :-- |
| `IMPLEMENTATION.md` (this) | Procedure: preconditions, worker map, per-task loop, hard rules, kickoff prompt | Read-only during the run (except §7 corrections) |
| [`TRACKING.md`](./TRACKING.md) | Live **state ledger**: per-task status/commit/evidence, proof matrix, blockers, session log | Updated after **every** task |
| [`TODO.md`](./TODO.md) | The **cursor**: plan tasks expanded into ordered micro-steps | Checkboxes ticked as you go |

**Resumption rule:** the first unchecked box in `TODO.md` is the next action; `TRACKING.md`
says what has been proven. **Re-run the last verification command before continuing — the
ledger is a claim, the command is the proof.**

## 1. Preconditions

| # | Precondition | Verify command | Expected |
| :-- | :-- | :-- | :-- |
| P1 | In the fleet worker worktree, not the main checkout | `git rev-parse --abbrev-ref HEAD` | `feature/fleet/<user>/mbo` (or the leaf's branch) |
| P2 | Go toolchain resolves via goenv/`.go-version` | `go version` | ≥ the version in `.go-version` |
| P3 | Branch is based on `origin/main`, not a local-only `main` | `git log --oneline origin/main..HEAD` | only *this objective's* commits — **if a foreign commit appears, rebase `--onto origin/main` before starting** |
| P4 | `~/.ssh/config` has ≥1 host you can reach | `ssh -o BatchMode=yes -o ConnectTimeout=5 <host> true; echo $?` | `0` |
| P5 | Working tree clean before each task | `git status --porcelain` | empty |
| P6 | Evidence dir exists | `mkdir -p docs/mbo/plans/fleet/evidence` | — |

**Known environment hazard:** the local `main` in this repo has, at times, carried unpushed
commits belonging to other in-flight work. P3 exists because that leaked into PR #223 once
already. Check it, don't assume it.

## 2. Worker map

Single worker for the design PR; leaves below are the **build** decomposition from plan §6.1.

| Field | Value |
| :-- | :-- |
| worker_ref | `fleet/<user>/mbo` |
| branch | `feature/fleet/<user>/mbo` |
| worktree_path | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/fleet/<user>/mbo` |
| base_branch | `main` |
| PR | #223 (draft) |

**Build order (plan §6.1):** `scaffold` → (`sshconf`, `drift`) → (`status`, `membership`,
`keys`) → `tui` → `migration`. `stamp-sh` is independent and may run at any point.

**If fanning out to parallel workers** (not the default): create blocking leaves first, each
`gss feature worker add --feature fleet --purpose <leaf> --base <in-edge-branch>`; record the
verbatim `--json` output in `TRACKING.md` §0. Otherwise execute all tasks in this one worker.

## 3. The execution loop (every task)

1. **Locate** — first unchecked `TODO.md` box → its plan task. Read the whole plan task
   before touching anything.
2. **RED** — write the failing test first. Run it. **Verify it fails**, and record the actual
   failure line in `TRACKING.md`. A test that passes before implementation is not a test.
3. **GREEN** — implement the minimum to pass. Run to pass.
4. **Gates** — the task's extra checks: `go vet ./...`, `bash -n install.sh` for shell edits,
   coverage for the touched package.
5. **Ledgers** — tick `TODO.md`; update the `TRACKING.md` row (status, commit, evidence).
6. **Commit** with the plan's exact message, staging **by explicit path** (never `git add -A`).

## 4. Done-when gates

**Per task:** the `Done when:` line at the end of each plan task.

**Per leaf:** the `done-when` column in plan §6.1.

**Objective stop condition** — all of:

- [ ] Tasks 1–15 `done` in `TRACKING.md`, each with a commit SHA **and** observed evidence.
- [ ] `cd sdk/fleet && go test ./... -cover` ≥ **60%** for every package.
- [ ] `./scripts/test.sh` green and listing `fleet`.
- [ ] `bash -n install.sh` clean.
- [ ] Live evidence exists for the four things unit tests cannot prove: multi-host `status`
      (Task 7), declined prune leaving a foreign key (Task 11), rescue-worktree mechanism
      (Task 12), and the real update of the stale host (Task 14).
- [ ] `grep -rn "ssh-key-sync" --exclude-dir=.git . | grep -v docs/mbo` is empty (Task 15).
- [ ] `docs/mbo/index.md` state advanced to `in-review`.

## 5. Hard rules

1. **No private key ever leaves the workstation.** No `scp` of a private key, no
   `~/.ssh/id_*` transfer. This is defect 1 of the absorbed script; reintroducing it is a
   contract violation, not a shortcut.
2. **Nothing destructive without a computed diff and confirmation.** `prune`/`delete` show
   what would go, then stop. Never blanket-overwrite `authorized_keys` (defect 2).
3. **Never swallow a per-host failure.** No bare `|| true`, no `2>/dev/null` that hides a
   result. Report per host; reflect it in the exit code (defect 4).
4. **`remove` unmarks; only `--purge` deletes.** Leaving the fleet ≠ losing SSH access.
5. **Back up `~/.ssh/config` before every write**, and never touch an unmarked block.
6. **Evidence before assertions.** A `TRACKING.md` row is `done` only with a commit SHA and
   output you actually observed. Never write a result you did not run.
7. **Don't edit the plan or spec during the build.** A contract defect goes in
   `TRACKING.md` §4 blockers and gets escalated. Correcting *this* file's commands is fine.
8. **Privacy.** No literal usernames/hostnames in committed content — use `<user>`,
   `<host>`, `$HOME`. A repo hook enforces this and will block the write.
9. **Confirm before publishing.** `gss` pushes/PRs need the human in the loop per repo
   convention; don't auto-publish from a task.
10. **`src/ssh-key-sync/` stays until Task 15**, and only goes after parity evidence.

## 6. Command cheat-sheet

```bash
# tests (module root: sdk/fleet)
cd sdk/fleet && go test ./... -v
cd sdk/fleet && go test ./internal/<pkg>/ -run <TestName> -v
cd sdk/fleet && go test ./... -cover
go vet ./...

# repo-wide
./scripts/test.sh                  # discovers sdk/ modules; prints the coverage line
./scripts/test.sh 2>&1 | grep -i fleet
bash -n install.sh                 # after any install.sh edit
bash install_test.sh               # shell tests

# build / run
bash sdk/fleet/build.sh && ~/opt/bin/fleet version
fleet status ; echo "exit=$?"

# evidence (always tee, never retype output)
mkdir -p docs/mbo/plans/fleet/evidence/taskNN
<command> 2>&1 | tee docs/mbo/plans/fleet/evidence/taskNN/<name>.txt

# state
git log --oneline origin/main..HEAD      # P3 — only this objective's commits
git status --porcelain                   # P5
```

## 7. Resuming, recovery, and corrections

- **Resume:** first unchecked `TODO.md` box. Before continuing, re-run the previous task's
  `done-when` command and confirm it still passes.
- **A command here is wrong?** Fix it in this file freely and note it in `TRACKING.md` §5.
- **A contract is wrong** (a plan interface can't work): do **not** patch it silently. File a
  blocker in `TRACKING.md` §4 with the failing command and its real output, and stop that
  leaf.
- **Registry/worktree drift:** `gss feature audit --feature fleet` (report) then `--repair`.
  Observable state wins over the registry. Note that `audit` has been observed reporting an
  open PR as "merged" — verify against `gh` before acting on it.
- **Rescue-worktree mechanism fails** (Task 12 Step 5): fall back to a rescue branch built
  from a temporary commit, record the change here and in the blocker table.

## 8. Kickoff prompt (always CURRENT — history lives in git)

> **Maintenance rule:** exactly ONE prompt here — the one that starts the NEXT session.
> Replace it at session end; past prompts are in `git log -- <this file>`.

```
Continue the `fleet` build.

Read first, in order:
  1. docs/mbo/plans/fleet/TODO.md      — the first unchecked box is your next action
  2. docs/mbo/plans/fleet/TRACKING.md  — what has actually been proven
  3. docs/mbo/plans/fleet.md           — the task you are about to do, in full
  4. docs/mbo/plans/fleet/IMPLEMENTATION.md §5 — the hard rules

Scope, in order: Task 1 (scaffold + wiring) is BLOCKING — nothing Go compiles
until it lands. Then Tasks 3-6 (sshconf, stamp, drift) which are pure and fully
unit-testable. Then 7-13. Task 2 (install.sh stamp) is independent and may be
done at any point. Tasks 14-15 are last and need real hosts.

Method: strict TDD per IMPLEMENTATION §3 — failing test first, observe the
failure, minimum implementation, observe the pass. Capture every done-when
command's output under docs/mbo/plans/fleet/evidence/taskNN/ and commit it with
the task.

Human-in-the-loop stops — do NOT proceed past these alone:
  - Task 11 live declined-prune (touches a real host's authorized_keys)
  - Task 12 Step 5 rescue-worktree verification
  - Task 14 the live interactive update of the stale host
  - Task 15 deleting src/ssh-key-sync/
  - any `gss` push / PR publish

Blocked? Write it in TRACKING.md §4 with the failing command and its real
output, then stop that leaf. Never edit the plan or spec to make a task pass.

Done when: IMPLEMENTATION §4's stop condition is fully ticked.

Preconditions to verify before your first edit (IMPLEMENTATION §1): you are in
the fleet worker worktree, and `git log --oneline origin/main..HEAD` shows ONLY
this objective's commits — a foreign commit means rebase --onto origin/main first.
```
