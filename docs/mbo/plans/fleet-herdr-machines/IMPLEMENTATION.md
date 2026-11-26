# fleet-herdr-machines — implementation playbook

Cut 1: `herdrmach` + `fleet herdr sync`. Plan: [`../fleet-herdr-machines.md`](../fleet-herdr-machines.md).

## 0. The three files

- `IMPLEMENTATION.md` — this procedure.
- `TRACKING.md` — evidence ledger.
- `TODO.md` — the cursor; first unchecked box is the next action.

## 1. Preconditions

- Worker exists: `gss feature list --feature fleet-herdr-machines --json` shows `sync-verb`.
- Go toolchain: `cd sdk/fleet && go build ./...`.
- A local herdr ≥ 0.9 for the live check: `herdr --version`.

## 2. Worker map

| Worker | Base | Scope |
| :-- | :-- | :-- |
| `fleet-herdr-machines/<user>/sync-verb` | `main` | plan tasks 1–4 |

## 3. The execution loop (every task)

RED (write the test) → RUN-RED → GREEN → RUN-GREEN → VERIFY (`go test ./... && go vet ./...`)
→ COMMIT (after the operator confirms) → LEDGER (TRACKING row) → CHECKPOINT.

## 4. Done-when gates

`go test ./...` green in `sdk/fleet`; `herdrmach` coverage ≥ 60%; `tui_herdr_test.go` unchanged and green.

## 5. Hard rules

- No shell in any herdr invocation — argv only.
- Never run `herdr machine add` without a TTY.
- Commits and pushes go through gss after operator confirmation.

## 6. Command cheat-sheet

```
cd sdk/fleet && go test ./internal/herdrmach/ -cover && go test ./... && go vet ./...
go run . herdr sync --dry-run
gss feature checkpoint --worker fleet-herdr-machines/<user>/sync-verb
```

## 7. Resuming, recovery, and corrections

`gss feature audit --feature fleet-herdr-machines` if the worktree or PR drifts.

## 8. Kickoff prompt (always CURRENT — history lives in git)

> Continue fleet-herdr-machines cut 1 in worker `fleet-herdr-machines/<user>/sync-verb`:
> open `docs/mbo/plans/fleet-herdr-machines/TODO.md` and do the first unchecked box.
