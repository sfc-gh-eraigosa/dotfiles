# gff-install-flow — implementation playbook

- **Slug:** gff-install-flow
- **Date:** 2026-07-26
- **Status:** Ready to execute
- **Plan (source of truth):** [`../gff-install-flow.md`](../gff-install-flow.md) · spec [`../../specs/gff-install-flow.md`](../../specs/gff-install-flow.md)
- **Objective anchors:** PR [#193](https://github.com/sfc-gh-eraigosa/dotfiles/pull/193) · `docs/mbo/index.md` row `gff-install-flow` · parent objective gff (#180, closed)

> This file is the **procedure**. It does not restate the plan — it tells a fresh agent
> session how to execute the plan, task by task, resumably. The plan wins any conflict.

## 0. The three files

| File | Role | Written by |
| :-- | :-- | :-- |
| `IMPLEMENTATION.md` (this) | Procedure: preconditions, worker map, per-task loop, hard rules, kickoff prompt | Read-only during the run (except §7 corrections) |
| [`TRACKING.md`](./TRACKING.md) | Live **state ledger**: per-task status/commit/evidence, F1–F6 proof matrix, blockers, session log | Updated after **every** task |
| [`TODO.md`](./TODO.md) | The **cursor**: plan Tasks 1–6 expanded into ordered micro-steps | Checkboxes ticked as you go |

**Resumption rule:** the first unchecked box in `TODO.md` is the next action; `TRACKING.md`
says what has been proven. Re-run the last verification command before continuing.

## 1. Preconditions

1. gff merged main incl. the `/w` fix: `git log --oneline origin/main | grep -m1 191` → `cd87074` present.
2. Toolchain: `bash`, `sh` (dash), `make`, `gss` on PATH; `gff version` works (built from main).
3. Worker exists (it does — see §2): `gss feature list | grep -A2 gff-install-flow`.
4. Read the plan fully — the task code blocks are normative. Read spec §5 (evaluation
   criteria) and the **behavior invariants** at the top of the plan.

## 2. Worker map

Single worker (no DAG — sequential tasks 1→6; task 6 is human-gated).

| Worker ref | Branch | Worktree path | PR |
| :-- | :-- | :-- | :-- |
| `gff-install-flow/edward-raigosa/impl` | `feature/gff-install-flow/edward-raigosa/impl` | `${HOME}/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff-install-flow/edward-raigosa/impl` | [#193](https://github.com/sfc-gh-eraigosa/dotfiles/pull/193) |

All code work happens **inside the worktree**. The Task 6 validation runs happen in
`${HOME}/git/dotfiles` on the branch, on the owner's WSL machine — never in the worktree.

## 3. The execution loop (every task)

1. Locate: first unchecked `TODO.md` box → its plan task; read the plan task fully.
2. RED first where the task has tests (Task 1): run and **verify it fails**; record the line.
3. GREEN: implement exactly the plan's code; run to pass (Task 1: under bash AND dash).
4. Gates: `bash -n` on any touched script; `make lint-shell && make lint-portability`
   (mandatory, every shell commit); `git status --short` + `git check-ignore -v` for new paths.
5. Ledgers: tick `TODO.md`; update the `TRACKING.md` task row + any F1–F6 matrix cells proven.
6. Commit with the plan's exact message, staging by explicit name; `gss feature checkpoint
   --worker gff-install-flow/edward-raigosa/impl` after every task.

## 4. Done-when gates

- Tasks 1–5: each task's VERIFY steps green (see plan) — Task 1 additionally: the test
  driver passes under **both** bash and dash.
- **Objective stop condition** (TRACKING §3): all six tasks done; the four-run human
  matrix captured (run 1's elevated-log `SKIP (gff: install.windows.wispr-flow=false)`
  is the capstone); evidence committed; docs updated; PR #193 ready → merged;
  `index.md` state advanced.

## 5. Hard rules

1. **Spec is approved scope — no additions.** The three unenforced `install.windows.*`
   flags, multi-source, and the TUI gif are explicitly out of scope.
2. **Behavior invariants** (plan header) are regression gates: fresh-system fail-open;
   per-run deploy independent of the y/n answer; `WIN_SETUP_MARKER` contract; `__notty__`
   guidance. Verify, don't assume.
3. **Shell portability:** `opt/lib/winsetup.sh` is POSIX/dash-safe (no `[[`, no arrays,
   no `local`); both lint gates before every shell commit.
4. **Never run `install.sh` from the worker worktree** (absolute symlinks into `$HOME`).
5. **Privacy:** no real usernames/hostnames/absolute home paths in tracked files —
   `${HOME}` / `${USER}` / `<winuser>` placeholders.
6. **Evidence before assertions:** a pwsh parse check that couldn't run is recorded as
   deferred, not ticked. Human-in-the-loop stops are never faked: every Task 6 run,
   every PR promotion, every merge.
7. **Confirmation before publishing** per the repo Git Workflow rule; checkpoint is
   not token-gated; `gh pr ready` needs owner confirmation (§7.1 correction 4 of the
   gff playbook applies — the gss token double-bind).

## 6. Command cheat-sheet

```sh
# Task 1 tests (both shells)
bash opt/lib/winsetup_test.sh && sh opt/lib/winsetup_test.sh
# gates (every shell commit)
bash -n install.sh && bash -n opt/bin/install_windows.sh
make lint-shell && make lint-portability
# checkpoint (after every task)
gss feature checkpoint --worker gff-install-flow/edward-raigosa/impl
```

## 7. Resuming, recovery, and corrections

Same rules as the gff playbook §7, including its §7.1 run corrections (positional
`gss feature merged` ref, stale-base check, checkpoint regenerates the PR body —
re-apply custom body AFTER the final checkpoint, `gh pr ready` fallback). Corrections
learned in THIS run get appended here as §7.1.

## 8. Kickoff prompt (always CURRENT — history lives in git)

> **Maintenance rule:** exactly ONE prompt here — the one that starts the NEXT session.
> Replace it at session end (past prompts are in `git log -- <this file>`).

### 8.1 CURRENT prompt — execute the build

> **Execute the gff-install-flow build, Tasks 1–5, then hand the owner the Task 6 matrix.**
>
> Read first, completely: `docs/mbo/plans/gff-install-flow/IMPLEMENTATION.md` (this
> procedure), `TRACKING.md` (what is proven), `TODO.md` (first unchecked box), then the
> plan `docs/mbo/plans/gff-install-flow.md` — its task code blocks are normative — and
> spec §5. Work only in the `gff-install-flow/edward-raigosa/impl` worktree.
>
> Strict TDD where tests exist (Task 1: RED observed before GREEN, bash AND dash).
> Both lint gates before every shell commit. One commit per task with the plan's exact
> message; checkpoint after every task; tick TODO + update TRACKING in the same breath.
>
> Human-in-the-loop stops (never fake): the four Task 6 validation runs (owner, real
> WSL terminal), PR promotion, merge. Blocked → TRACKING §4 with the real failing
> output. Done when: the TRACKING §3 stop condition is fully green.
