# agy-parity — implementation playbook

- **Slug:** agy-parity
- **Date:** 2026-09-03
- **Status:** Ready to execute
- **Plan (source of truth):** [`../agy-parity.md`](../agy-parity.md) · spec [`../../specs/agy-parity.md`](../../specs/agy-parity.md) · design [`../../designs/agy-parity.md`](../../designs/agy-parity.md)
- **Objective anchors:** issue #268 · PR #269 · `docs/mbo/index.md` row `agy-parity`

> This file is the **procedure**. It does not restate the plan — it tells a fresh agent
> session how to execute the plan, task by task, resumably. The plan wins any conflict.

## 0. The three files

| File | Role | Written by |
| :-- | :-- | :-- |
| `IMPLEMENTATION.md` (this) | Procedure: preconditions, branch map, per-task loop, hard rules, kickoff prompt | Read-only during the run (except §7 corrections) |
| [`TRACKING.md`](./TRACKING.md) | Live **state ledger**: per-task status/commit/evidence, proof matrix, blockers, append-only session log | Updated after **every** task |
| [`TODO.md`](./TODO.md) | The **cursor**: plan tasks expanded into ordered micro-steps | Checkboxes ticked as you go |

**Resumption rule:** the first unchecked box in `TODO.md` is the next action; `TRACKING.md`
says what has been proven. Re-run the last verification command before continuing —
the ledger is a claim, the command is the proof.

## 1. Preconditions

1. **Branch:** you are in the checkout of `worktree/agy_defaults` (a herdr worktree, NOT a gss
   feature worker). Verify: `git status -sb | head -1` → `## worktree/agy_defaults...origin/worktree/agy_defaults`.
2. **Toolchain:** `jq --version`, `bash --version | head -1`, `shellcheck --version | head -2`,
   `python3 -c 'import tomllib'` (optional; the renderer test degrades to grep without it).
3. **PR #269 open and draft:** `gh pr view 269 --json isDraft,state --jq '{isDraft,state}'`.
4. **Read plan §3 (interface contracts) and §4 (task steps) in full** before touching code.
5. **Baseline green:** `bash ai/antigravity/aliases_test.sh | tail -1` and
   `bash opt/scripts/system/install_antigravity_skills_test.sh | tail -1` both print `FAIL=0`
   before Task 1 starts (record in TRACKING §5).

## 2. Worker map

Single branch, no gss feature workers (the objective is one PR).

| Leaf | Branch | Worktree path | PR | Order |
| :-- | :-- | :-- | :-- | :-- |
| all tasks | `worktree/agy_defaults` | `${HOME}/.herdr/worktrees/dotfiles/worktree-agy-defaults` | [#269](https://github.com/sfc-gh-eraigosa/dotfiles/pull/269) | T1 → T2 → T3 → T4 → T5 → T6 → T7 |

## 3. The execution loop (every task)

1. Locate: first unchecked `TODO.md` box → its plan task; read the plan task fully.
2. RED: write the failing test first; run it; **verify it fails**; record the failure line.
3. GREEN: implement the minimum; run to pass.
4. Gates: `bash -n` + `shellcheck` on every touched `.sh`; `make lint-portability` when a
   `.sh` changed; `git status --short -- <new path>` for every new file (allowlist check).
5. Evidence: `tee` the gate output into `evidence/<unit>/<file>.txt` with a dated header
   (`date -u +%FT%TZ`), append-only.
6. Ledgers: tick `TODO.md`; update the `TRACKING.md` task row (status, commit, evidence).
7. Commit with the plan's exact message, staging by explicit name (include the evidence file
   and the ledgers). Checkpoint per §6.

## 4. Done-when gates

- Per task: the task's `FAIL=0` driver run + lint gates, captured as evidence.
- Objective (TRACKING §3): all seven task rows `done` with SHAs; `make shell-test`,
  `make lint-shell`, `make lint-portability` clean; live-host transcript captured;
  `docs/mbo/index.md` row `in-review`; PR #269 body refreshed to the full scope.

## 5. Hard rules

- **Never** run `install.sh` from this worktree. `install_antigravity_skills.sh` alone is
  allowed for live evidence (copy-forward; no symlinks).
- No underscore-prefixed functions in `ai/antigravity/aliases.sh`.
- Do not edit `apply-forced-settings.sh`; the merge semantics are a frozen contract.
- Do not weaken a test to pass; a failing gate is a blocker row in TRACKING §4.
- Privacy: no literal home path or username in any file, commit message, or PR body
  (`privacy_guard` blocks it; use `$HOME`, `~`, `<user>`).
- Publishing (`gss push`) needs the two-call token recipe; the user authorized checkpoints to
  PR #269 for this run (memory: gss land flow, no per-step re-confirmation). Refresh the PR body
  after every push (`gh pr edit 269 --body-file -`).
- Evidence before assertions: a row is `done` only with a SHA and observed output.

## 6. Command cheat-sheet

```sh
# tests
bash ai/antigravity/aliases_test.sh | tail -3
bash opt/scripts/system/install_antigravity_skills_test.sh | tail -3
bash ai/hooks/antigravity_adapter_test.sh | tail -3
bash opt/scripts/system/render-agy-plugin_test.sh | tail -3
make shell-test 2>&1 | tail -5
# lint
shellcheck ai/antigravity/aliases.sh ai/hooks/antigravity_adapter.sh opt/scripts/system/install_antigravity_skills.sh opt/scripts/system/render-agy-plugin.sh
make lint-shell 2>&1 | tail -3 ; make lint-portability 2>&1 | tail -3
# evidence header
{ echo "# $(date -u +%FT%TZ) $(git rev-parse --short HEAD) <command>"; <command>; } | tee -a docs/mbo/plans/agy-parity/evidence/<unit>/<file>.txt
# checkpoint (two separate shell calls — the safety hook blocks chaining)
mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token
gss push
gh pr edit 269 --body-file -   # refreshed body on stdin
```

## 7. Resuming, recovery, and corrections

Fix wrong commands here freely and note it in the session log. Never edit the plan/spec
as part of the build — escalate contract defects via TRACKING blockers instead.
Corrections log: (none yet).

## 8. Kickoff prompt (always CURRENT — history lives in git)

> **Maintenance rule:** exactly ONE prompt here — the one that starts the NEXT session.

Mission: finish the agy-parity build on branch `worktree/agy_defaults` (PR #269) to
dev-complete with evidence. Read first: `docs/mbo/plans/agy-parity/TODO.md` (first unchecked
box is your next action), `TRACKING.md` (what is proven), plan `docs/mbo/plans/agy-parity.md`
§3–§4. Scope in order: T1 aliases → T2 template → T3 forced policy → T4 hooks merge → T5
adapter ask → T6 plugin renderer → T7 docs + gates + live evidence. Human-in-the-loop stops:
none inside the loop (checkpoints to #269 are pre-authorized); stop and write a TRACKING §4
blocker if a gate cannot pass without changing a frozen contract. Done-when: TRACKING §3 all
ticked, index row `in-review`, PR body current.
