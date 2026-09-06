# <objective> — implementation playbook

- **Slug:** <slug>
- **Date:** <YYYY-MM-DD>
- **Status:** Ready to execute
- **Plan (source of truth):** [`../<slug>.md`](../<slug>.md) · spec [`../../specs/<slug>.md`](../../specs/<slug>.md)
- **Objective anchors:** issue #<n> · PR #<n> · `docs/mbo/index.md` row `<slug>`

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
<toolchain, merged bases, registry rows — each with the verify command>

## 2. Worker map
<worker_ref / branch / worktree_path captured VERBATIM from `gss feature worker add --json`;
for multi-leaf objectives: the DAG, blocking-first order>

## 3. The execution loop (every task)
1. Locate: first unchecked `TODO.md` box → its plan task; read the plan task fully.
2. RED: write the failing test first; run it; **verify it fails**; record the failure line.
3. GREEN: implement the minimum; run to pass.
4. Gates: the task's extra checks (lint, coverage, `bash -n`, allowlist for new paths).
5. Ledgers: tick `TODO.md`; update the `TRACKING.md` task row (status, commit, evidence).
6. Commit with the plan's exact message, staging by explicit name. Checkpoint (push/PR refresh).

## 4. Done-when gates
<per-leaf / per-objective gates copied from the plan; the overall stop condition>

## 5. Hard rules
<the non-negotiables for this objective: frozen contracts, coverage bars, lint gates,
privacy, confirmation-before-publish, "evidence before assertions">

## 6. Command cheat-sheet
<the exact test/lint/build/checkpoint commands>

## 7. Resuming, recovery, and corrections
Fix wrong commands here freely and note it in the session log. Never edit the plan/spec
as part of the build — escalate contract defects via TRACKING blockers instead.

## 8. Kickoff prompt (always CURRENT — history lives in git)
> **Maintenance rule:** exactly ONE prompt here — the one that starts the NEXT session.
> Replace it at session end (past prompts are in `git log -- <this file>`).

<the prompt: mission · read-first list · scope in order · human-in-the-loop stops ·
blocked→TRACKING · done-when>
