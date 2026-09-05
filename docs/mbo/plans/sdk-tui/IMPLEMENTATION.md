# sdk-tui — implementation playbook

- **Slug:** `sdk-tui`
- **Date:** 2026-09-05
- **Status:** Ready to execute
- **Plan (source of truth):** [`../sdk-tui.md`](../sdk-tui.md) · spec [`../../specs/sdk-tui.md`](../../specs/sdk-tui.md) · design [`../../designs/sdk-tui.md`](../../designs/sdk-tui.md) · guide `sdk/libs/tui/GUIDE.md`
- **Objective anchors:** see `docs/mbo/index.md` row `sdk-tui` (design issue + design PR) · feature `gss feature sdk-tui` · consumer `gff-tui-vim` (#281)

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
| Go toolchain 1.26.x | `go version` |
| libs module green on the base | `cd sdk/libs && go test ./... 2>&1 \| tail -2` → `ok` |
| golangci-lint present (`make lint-go` needs it) | `golangci-lint version` |
| Baseline gss binary size recorded (deps blast-radius proof) | from a clean `main` checkout: `cd sdk/gss && go build -o /tmp/gss-before . && ls -l /tmp/gss-before > <worktree>/docs/mbo/plans/sdk-tui/evidence/deps/gss-size-before.txt` |
| You are inside the **lib** worker worktree | `git rev-parse --abbrev-ref HEAD` → `feature/sdk-tui/<user>/lib` |

## 2. Worker map

Single leaf, **blocking** for `gff-tui-vim/build` (plan §6.1).

| Worker | worker_ref | branch | worktree_path | base |
| :-- | :-- | :-- | :-- | :-- |
| design (docs) | `sdk-tui/edward-raigosa/design` | `feature/sdk-tui/edward-raigosa/design` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/sdk-tui/edward-raigosa/design` | `main` |
| lib | *(fill from `--json`)* | | | design branch (or `main` after the design PR merges) |

Create it with:
```
gss feature worker add --feature sdk-tui --purpose lib \
  --description "sdk/libs/tui: keymap, nav, prompt, search, cmdline, overlay + example (plan tasks 1-7)" \
  --base feature/sdk-tui/edward-raigosa/design --engine claude --json
```
Then the consumer: `gss feature worker add --feature gff-tui-vim --purpose build --base feature/sdk-tui/<user>/lib …` (see `plans/gff-tui-vim/IMPLEMENTATION.md` §2).

## 3. The execution loop (every task)
1. Locate: first unchecked `TODO.md` box → its plan task; read the plan task fully.
2. RED: write the failing test first; run it; **verify it fails**; record the failure line.
3. GREEN: implement the minimum; run to pass.
4. Gates: `go test ./tui/<pkg>/ -cover` ≥ 90%; `go vet ./...`.
5. Ledgers: tick `TODO.md`; update the `TRACKING.md` task row (status, commit, evidence).
6. Commit with the plan's exact message, staging by explicit name. `gss feature checkpoint` (confirm first).

## 4. Done-when gates

- Per task: the plan's Step 4 command passes; output under `evidence/task<N>/`.
- Objective: Tasks 1–7 `done`; every `tui/*` package ≥ 90%; `COVERAGE_ENFORCE=1 ./scripts/test.sh unit` green for `libs`; `make lint-go` clean; example test green; demo transcript committed; gss size before/after recorded; `docs/mbo/index.md` row `sdk-tui` at `in-review`; draft PR promoted. **§3 interfaces unchanged since Task 2** (or a TRACKING blocker explains why and `gff-tui-vim`'s plan was updated in the same PR).

## 5. Hard rules

- Plan §3 interfaces are **frozen after Task 2**. A needed change is a TRACKING §4 blocker + a coordinated edit to `plans/gff-tui-vim.md`, never a silent signature drift.
- `sdk/libs/go.mod` gains only bubbletea (runtime) and testify (tests). No lipgloss, no bubbles.
- No package-level mutable state. No I/O. No tool imports. No colors chosen in the lib.
- Real terminal key shapes in tests. NO_COLOR-independent output (Plain palette in tests).
- Stage by explicit name; one commit per task; plan messages verbatim + session trailers.
- Evidence before assertions: a TRACKING row is `done` only with a commit SHA **and** observed output.
- Never run `install.sh` from a worker worktree. Confirm before every push/checkpoint.

## 6. Command cheat-sheet

```
cd sdk/libs
EV=../../docs/mbo/plans/sdk-tui/evidence
go test ./tui/<pkg>/ -cover -v                     # one package
go test ./... -cover && go vet ./...               # module
go test -tags example ./tui/example/ -v            # composition proof (Task 7)
(cd ../.. && COVERAGE_ENFORCE=1 ./scripts/test.sh unit)   # module gate (libs floor 80%)
(cd ../.. && make lint-go)
go run -tags example ./tui/example                 # the demo binary (real terminal only)
gss feature checkpoint --worker sdk-tui/<user>/lib # push + draft PR (confirm first)
```

## 7. Resuming, recovery, and corrections
Fix wrong commands here freely and note it in the session log. Never edit the plan/spec
as part of the build — escalate contract defects via TRACKING blockers instead.

## 8. Kickoff prompt (always CURRENT — history lives in git)
> **Maintenance rule:** exactly ONE prompt here — the one that starts the NEXT session.
> Replace it at session end (past prompts are in `git log -- <this file>`).

> **Build `sdk-tui` — the shared TUI behaviors lib (`sdk/libs/tui`).**
>
> Read first, completely: `sdk/libs/tui/GUIDE.md`, `docs/mbo/plans/sdk-tui/IMPLEMENTATION.md`
> (§1 preconditions, §5 hard rules), `docs/mbo/plans/sdk-tui/TODO.md` (the cursor), then the
> plan `docs/mbo/plans/sdk-tui.md` (§3 frozen contracts, §4 tasks with the exact test and
> implementation code) and the spec `docs/mbo/specs/sdk-tui.md` (§5 rules).
>
> State on entry: design docs + GUIDE live on the `design` worker; no `lib` worker yet.
> Record the gss baseline binary size (IMPLEMENTATION §1), create the `lib` worker per §2
> (`--base` the design branch), paste its `--json` into TRACKING §0, and work inside it.
>
> Scope, in order: plan Tasks 1→7, TDD (RED → RUN-RED → GREEN → RUN-GREEN → VERIFY → COMMIT
> → LEDGER → CHECKPOINT), one commit per task, evidence `tee`'d into
> `docs/mbo/plans/sdk-tui/evidence/task<N>/`. Task 7 Step 6 is the human-in-the-loop stop:
> the tmux demo runs in a real terminal, never backgrounded.
>
> Confirm via the interactive prompt before every `git commit` / `gss feature checkpoint`.
> Blocked → TRACKING §4 with the failing command and its real output; never patch the plan.
> If a §3 signature must change, stop and escalate — `gff-tui-vim` compiles against it.
>
> Done when: IMPLEMENTATION §4 objective gate is green, `docs/mbo/index.md` says
> `in-review` with the lib PR number, and the `gff-tui-vim/build` worker can be created
> `--base` this branch.
