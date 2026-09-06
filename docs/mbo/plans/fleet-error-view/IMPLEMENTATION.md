# fleet-error-view — implementation playbook

- **Slug:** fleet-error-view
- **Date:** 2026-09-06
- **Status:** Ready to execute
- **Plan (source of truth):** [`../fleet-error-view.md`](../fleet-error-view.md) · spec [`../../specs/fleet-error-view.md`](../../specs/fleet-error-view.md) · design [`../../designs/fleet-error-view.md`](../../designs/fleet-error-view.md)
- **Objective anchors:** issue [#308](https://github.com/sfc-gh-eraigosa/dotfiles/issues/308) · PR [#310](https://github.com/sfc-gh-eraigosa/dotfiles/pull/310) · `docs/mbo/index.md` row `fleet-error-view`

> This file is the **procedure**. It does not restate the plan — it tells a fresh agent session
> how to execute the plan, task by task, resumably. The plan wins any conflict.

## 0. The three files

| File | Role | Written by |
| :-- | :-- | :-- |
| `IMPLEMENTATION.md` (this) | Procedure: preconditions, per-task loop, hard rules, kickoff prompt | Read-only during the run (except §7 corrections) |
| [`TRACKING.md`](./TRACKING.md) | Live **state ledger**: per-task status/commit/evidence, proof matrix, blockers, session log | Updated after **every** task |
| [`TODO.md`](./TODO.md) | The **cursor**: plan tasks expanded into ordered micro-steps | Checkboxes ticked as you go |

**Resumption rule:** the first unchecked box in `TODO.md` is the next action; `TRACKING.md` says
what has been proven. Re-run the last verification command before continuing — the ledger is a
claim, the command is the proof.

## 1. Preconditions

| Precondition | Verify with | Expected |
| :-- | :-- | :-- |
| Go toolchain resolves | `cd sdk/fleet && go version` | a version line, no GOROOT mismatch |
| The suite is green before you start | `cd sdk/fleet && go test ./...` | all `ok` |
| Baseline coverage recorded | `cd sdk/fleet && go test ./... -coverprofile=/tmp/fev.out && go tool cover -func=/tmp/fev.out \| tail -1` | total ≈ 82.3 % |
| Working on the objective's branch | `git branch --show-current` | `worktree/fleet-error-view` (or its `gss feature` worker branch) |
| Linters available | `make lint-go` | exit 0 |
| Markdown linter reachable | `npx --yes markdownlint-cli2 --version` | a version (the `make` target needs it on `PATH`) |

## 2. Worker map

Single worker — the plan §6.1 records why this objective is **not** broken out.

| Leaf | Worker ref | Branch | Worktree | PR |
| :-- | :-- | :-- | :-- | :-- |
| (all) | *(fill from `gss feature worker add --json`, verbatim)* | | | |

If no `gss feature` worker is used, the objective lives on `worktree/fleet-error-view` with a
single draft PR opened via `gss pr`.

## 3. The execution loop (every task)

1. **Locate:** first unchecked `TODO.md` box → its plan task; read the plan task fully,
   including its **Interfaces** block (that is how you learn names other tasks rely on).
2. **RED:** write the failing test *first*, exactly as the plan gives it; run it;
   **verify it fails**; record the failure line in `TRACKING.md`.
3. **GREEN:** implement the minimum; run to pass.
4. **Gates:** `cd sdk/fleet && go test ./...`, `go vet ./...`, `make lint-go`
   (+ `npx --yes markdownlint-cli2 <file>` for doc tasks, `go test -race ./...` for Task 12).
5. **Evidence:** `… 2>&1 | tee docs/mbo/plans/fleet-error-view/evidence/<folder>/<file>.txt`,
   with a dated header line naming the exact command.
6. **Ledgers:** tick `TODO.md`; update the `TRACKING.md` row (status, commit SHA, evidence).
7. **Commit** with the plan's exact message, staging by explicit path. Never `git add -A`.

## 4. Done-when gates

- [ ] Tasks 1–13 each pass their own done-when (plan §4).
- [ ] `cd sdk/fleet && go test ./...` and `go test -race ./...` green.
- [ ] Coverage: module ≥ 82 %, `cmd` ≥ 65 %, `runner` ≥ 65 %, `updexec` ≥ 90 %.
- [ ] `make lint-go` green; markdown clean apart from `MD010` inside Go snippets (plan Global Constraints).
- [ ] `TestFrameFitsTheTerminal` green, with `evidence/layout/task08-before-overflow.txt`
      committed as proof the bug existed.
- [ ] Every spec §5 rule maps to a passing named test (plan §5 table).
- [ ] Task 14's live capture is in `evidence/e2e/` **or** explicitly recorded as outstanding in
      `TRACKING.md` §4 — never silently skipped.

## 5. Hard rules

- **Frozen contracts:** the signatures in plan §3 are fixed for the whole build. If one has to
  change, stop and record a blocker in `TRACKING.md`; do not edit the plan mid-run.
- **`runner.Runner` does not gain a method.** The split is an optional, type-asserted capability.
- **`fleet update`'s terminal output stays byte-identical.** Only the capture file changes.
- **Never weaken an invariant to make a number fit** — `TestFrameFitsTheTerminal` failing means
  `layout()` or `chromeRows()` is wrong, not the test.
- **`keyHelp` is the only place a key is declared.** Implemented-but-undeclared is a defect.
- **Purity:** no `time.Now()` in a render path (`nowFn`), no I/O in `Update()`.
- **New files:** `git status --short -- <path>` before staging; the repo `.gitignore` is an
  allowlist (see `docs/gitignore-allowlist.md`). Never `git add -f`.
- **Never run `install.sh` from a worktree.**
- **Evidence before assertions:** a `TRACKING` row is `done` only with a commit SHA *and*
  observed command output.

## 6. Command cheat-sheet

```bash
cd sdk/fleet
go test ./...                                   # full suite
go test ./cmd/ -run TestFrameFitsTheTerminal -v  # the layout invariant
go test -race ./...                              # required for Task 12
go test ./... -coverprofile=/tmp/fev.out && go tool cover -func=/tmp/fev.out | tail -1
FLEET_DEMO=1 go test ./cmd/ -run TestDemoFrames  # eyeball the frames in colour
cd ../.. && make lint-go
npx --yes markdownlint-cli2 "docs/mbo/**/fleet-error-view*.md"   # MD010 in Go snippets is expected
```

## 7. Resuming, recovery, and corrections

Fix wrong commands in this file freely and note it in the `TRACKING.md` session log. Never edit
the plan or spec as part of the build — contract defects go to `TRACKING.md` §4 and get
escalated. If the suite is red on resume, `git log --oneline -5` and re-run the last task's
gate before writing any new code.

## 8. Kickoff prompt (always CURRENT — history lives in git)

> **Maintenance rule:** exactly ONE prompt here — the one that starts the NEXT session.
> Replace it at session end (past prompts are in `git log -- <this file>`).
>
> **Mission:** execute `docs/mbo/plans/fleet-error-view.md` task by task, TDD, resumably.
>
> **Read first:** this file, then `TODO.md` (the first unchecked box is your next action), then
> `TRACKING.md` §1 and §4, then the plan task you are on and the spec section it implements.
>
> **Scope, in order:** Tasks 1–2 (`internal/runner` split streams), 3–5 (`internal/updexec`
> `ErrLine`, capture marking, `Benign`), 6–7 (model tag + pure `layout()`), 8 (adopt `layout`
> and fix the frame overflow), 9–11 (`h`/`e` keys, error pane, warning badge), 12 (end-to-end
> plus the race detector), 13 (frames + docs).
> **Task 14 is human-gated — stop and hand it to the operator.**
>
> **Human-in-the-loop stops:** before any `git push` / PR action (confirm via the interactive
> prompt); at Task 14; and whenever a frozen contract in plan §3 would have to change.
>
> **Blocked?** Write the failing command and its real output into `TRACKING.md` §4 and stop.
> Do not retro-fit the spec to match a surprise.
>
> **Done when:** every box in §4 of this file is ticked.
