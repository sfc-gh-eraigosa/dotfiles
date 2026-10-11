# pins — implementation playbook

- **Slug:** pins
- **Date:** 2026-10-10
- **Status:** Ready to execute
- **Plan (source of truth):** [`../pins.md`](../pins.md) · spec [`../../specs/pins.md`](../../specs/pins.md)
- **Objective anchors:** issue #376 · PR (same)

> The procedure. The plan wins any conflict.

## 0. The three files
| File | Role |
| :-- | :-- |
| `IMPLEMENTATION.md` | procedure (this) |
| [`TRACKING.md`](./TRACKING.md) | evidence ledger: update after every task |
| [`TODO.md`](./TODO.md) | cursor: the first unchecked box is the next action |

## 1. Preconditions
- `go version` works in the worktree (goenv). Verify: `cd sdk/fleet && go build ./...`
- Worker exists. Verify: `gss feature list --feature pins --json`

## 2. Worker map
| Worker ref | Branch | Worktree |
| :-- | :-- | :-- |
| `pins/edward-raigosa/build` | `feature/pins/edward-raigosa/build` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/pins/edward-raigosa/build` |

Leaves L1 (shell) and L2 (Go) run as concurrent subagents in this one worktree on disjoint paths (plan §6.1).
Subagents commit only their own paths (`git commit -- <paths>`) and never touch TODO/TRACKING. The lead updates the ledgers.

## 3. Execution loop
RED → RUN-RED → GREEN → RUN-GREEN → gates → tee evidence to `evidence/Tn.txt` → commit (explicit paths) → ledger.

## 4. Done-when
Every TRACKING §3 box ticked. `go test ./...` in sdk/fleet passes with ≥60% module and ≥85% `internal/pins` coverage. All lint gates are clean.
Live `fleet pins status` / `--installed` / `fleet status` output is captured.

## 5. Hard rules
- §3 contracts are frozen. Defects go to TRACKING §4, never silent changes.
- Shell follows `docs/mbo/specs/shell-portability.md`; run `make lint-portability`.
- Check every new path against the `.gitignore` allowlist (`git check-ignore -v`), and never use `git add -f`.
- Use `$HOME` in commands (privacy guard). Never run `install.sh` from this worktree.
- Every commit, push and PR goes through an AskUserQuestion confirmation (repo rule). The land flow follows memory `gss-land-flow-no-interrupt`.

## 6. Command cheat-sheet
```
cd sdk/fleet && go test ./... -count=1 -coverprofile=/tmp/c.out && go tool cover -func=/tmp/c.out | tail -1
go test ./internal/pins -cover
bash opt/lib/pins_test.sh ; bash opt/scripts/system/install_sops_test.sh
make lint-shell lint-portability lint-go
gss feature checkpoint --worker pins/edward-raigosa/build
```

## 7. Corrections
(none yet)

## 8. Kickoff prompt
> Resume the `pins` build: read `docs/mbo/plans/pins.md` and this trio, then continue from the first
> unchecked box in `TODO.md`, re-running the last ledger command first. Stop for confirmation before any
> push or PR change.
