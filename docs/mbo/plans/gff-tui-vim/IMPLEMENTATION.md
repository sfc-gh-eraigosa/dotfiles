# gff-tui-vim — implementation playbook

- **Slug:** `gff-tui-vim`
- **Date:** 2026-09-05
- **Status:** Ready to execute
- **Plan (source of truth):** [`../gff-tui-vim.md`](../gff-tui-vim.md) · spec [`../../specs/gff-tui-vim.md`](../../specs/gff-tui-vim.md)
- **Objective anchors:** issue — see `docs/mbo/index.md` row `gff-tui-vim` · design PR via `gss feature gff-tui-vim` (worker `gff-tui-vim/<user>/design`) · build worker `gff-tui-vim/<user>/build`

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

| Precondition | Verify |
| :-- | :-- |
| Go toolchain per repo `.go-version` (1.26.x) | `cd sdk/gff && go version` → `go1.26` |
| gff module builds and its suite is green on the base | `cd sdk/gff && go test ./... 2>&1 \| tail -3` → every line `ok` |
| `internal/tui` baseline coverage recorded | `cd sdk/gff && go test -cover ./internal/tui/` → `coverage: 91.3%` (or higher) |
| Design docs merged or stacked below the build worker | `gss feature list --feature gff-tui-vim --tree` shows `design` (docs) and `build` (`--base` = design branch, or `main` once the design PR merged) |
| You are inside the **build** worker worktree | `git rev-parse --abbrev-ref HEAD` → `feature/gff-tui-vim/<user>/build` |

## 2. Worker map

Single leaf (plan §6.1). Fill the build row verbatim from `gss feature worker add --json`:

| Worker | worker_ref | branch | worktree_path | base |
| :-- | :-- | :-- | :-- | :-- |
| design (docs) | `gff-tui-vim/edward-raigosa/design` | `feature/gff-tui-vim/edward-raigosa/design` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff-tui-vim/edward-raigosa/design` | `main` |
| build | *(fill from `--json`)* | | | design branch (or `main` after merge) |

Create it with:
```
gss feature worker add --feature gff-tui-vim --purpose build \
  --description "gff TUI vim nav + / search + : command line (plan tasks 1-7)" \
  --base feature/gff-tui-vim/edward-raigosa/design --engine claude --json
```

## 3. The execution loop (every task)
1. Locate: first unchecked `TODO.md` box → its plan task; read the plan task fully.
2. RED: write the failing test first; run it; **verify it fails**; record the failure line.
3. GREEN: implement the minimum; run to pass.
4. Gates: `go test ./internal/tui/ -cover` (≥ 91.3%), `go vet ./...`; for Task 7 the module gate.
5. Ledgers: tick `TODO.md`; update the `TRACKING.md` task row (status, commit, evidence).
6. Commit with the plan's exact message, staging by explicit name. `gss feature checkpoint` (confirm first).

## 4. Done-when gates

- Per task: the Step 4 command in the plan passes and its output is under `evidence/task<N>/`.
- Objective: all 7 tasks `done` in TRACKING §1; `go test ./... -cover` ≥ 90% module, `internal/tui` ≥ 91.3%, `internal/resolve` ≥ 95%; `go vet` clean; the real-terminal demo transcript committed under `evidence/demo/`; `docs/mbo/index.md` row at `in-review`; draft PR promoted.

## 5. Hard rules

- Frozen contracts in `docs/mbo/plans/gff.md` §3 are untouched. Writes go only to the user override file; tests write only under `t.TempDir()`.
- `go.mod` unchanged — no `bubbles`, no new deps.
- Real terminal key shapes in tests (`KeyRunes` for letters, `KeySpace`, `KeyCtrlD`, `KeyEscape`, `KeyTab`, `KeyShiftTab`).
- `h`/`H` never open help after Task 2. The footer hint string in `view.go` is the key table's single source; README, `--help`, and the overlay mirror it.
- Evidence before assertions: a TRACKING row is `done` only with a commit SHA **and** observed output.
- Stage by explicit name. Never `install.sh` from the worker worktree. Confirm before every push/checkpoint.
- Do not edit the plan or spec during the build — escalate via TRACKING §4.

## 6. Command cheat-sheet

```
cd sdk/gff
EV=../../docs/mbo/plans/gff-tui-vim/evidence
go test ./internal/tui/ -run 'TestName' -v          # one test
go test ./internal/tui/ -cover                       # package gate (≥ 91.3%)
go test ./... -cover && go vet ./...                 # module
grep -n 'coverpkg' ../../.github/workflows/gff-ci.yml   # the exact CI coverage recipe for Task 7
./build.sh                                           # install the binary for the demo
gss feature checkpoint --worker gff-tui-vim/<user>/build   # push + draft PR (confirm first)
```

## 7. Resuming, recovery, and corrections
Fix wrong commands here freely and note it in the session log. Never edit the plan/spec
as part of the build — escalate contract defects via TRACKING blockers instead.

## 8. Kickoff prompt (always CURRENT — history lives in git)
> **Maintenance rule:** exactly ONE prompt here — the one that starts the NEXT session.
> Replace it at session end (past prompts are in `git log -- <this file>`).

> **Build `gff-tui-vim` — vim navigation, `/` regex search, `:` command line for `gff tui`.**
>
> Read first, completely: `docs/mbo/plans/gff-tui-vim/IMPLEMENTATION.md` (§1 preconditions,
> §5 hard rules), `docs/mbo/plans/gff-tui-vim/TODO.md` (the cursor), then the plan
> `docs/mbo/plans/gff-tui-vim.md` (§3 contracts, §4 tasks with the exact test and
> implementation code) and the spec `docs/mbo/specs/gff-tui-vim.md` (§5 rules).
>
> State on entry: design docs live on the `design` worker; no build worker yet. Create it per
> IMPLEMENTATION §2 (`--base` the design branch), record its `--json` in TRACKING §0, and
> work inside that worktree.
>
> Scope, in order: plan Tasks 1→7, TDD (RED → RUN-RED → GREEN → RUN-GREEN → VERIFY → COMMIT
> → LEDGER → CHECKPOINT), one commit per task with the plan's message, evidence `tee`'d into
> `docs/mbo/plans/gff-tui-vim/evidence/task<N>/`. Task 7 Step 5 is the human-in-the-loop
> stop: the real-terminal tmux demo must run in a real terminal, never backgrounded.
>
> Confirm via the interactive prompt before every `git commit` / `gss feature checkpoint`.
> Blocked → TRACKING §4 with the failing command and its real output; never patch the plan.
>
> Done when: IMPLEMENTATION §4 objective gate is green and `docs/mbo/index.md` says
> `in-review` with the build PR number.
