# Security Scan green + required — implementation playbook

- **Slug:** security-scan-green
- **Date:** 2026-10-15
- **Status:** Ready to execute
- **Plan (source of truth):** [`../security-scan-green.md`](../security-scan-green.md) · spec [`../../specs/security-scan-green.md`](../../specs/security-scan-green.md)
- **Objective anchors:** issue #394 · design PR #395 · `docs/mbo/index.md` row `security-scan-green`

> This file is the **procedure**. The plan wins any conflict.

## 0. The three files
| File | Role | Written by |
| :-- | :-- | :-- |
| `IMPLEMENTATION.md` (this) | procedure, worker map, loop, hard rules, kickoff prompt | read-only during the run (§7 corrections allowed) |
| [`TRACKING.md`](./TRACKING.md) | live ledger: task rows with commit + observed output, blockers, session log | after **every** task |
| [`TODO.md`](./TODO.md) | cursor: micro-steps per task; first unchecked box = next action | ticked as you go |

**Resumption rule:** re-run the last gate command before continuing; the ledger is a claim, the command is the proof.

## 1. Preconditions
- Go `1.27.2` available: `GOENV_VERSION=1.27.2 go version` → `go1.27.2`.
- gosec pinned: `go install github.com/securego/gosec/v2/cmd/gosec@v2.29.0` → `$(go env GOPATH)/bin/gosec -version` prints `2.29.0`.
- Registry rows exist: `gss feature list --feature security-scan-green --json` shows workers `design` and `fix`.

## 2. Worker map
| Leaf | worker_ref | branch | worktree (glob — the literal path contains the login name) | base |
| :-- | :-- | :-- | :-- | :-- |
| design | `security-scan-green/edward-raigosa/design` | `feature/security-scan-green/edward-raigosa/design` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/security-scan-green/*/design` | main |
| fix | `security-scan-green/edward-raigosa/fix` | `feature/security-scan-green/edward-raigosa/fix` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/security-scan-green/*/fix` | main |
| promote | (create after fix merges + green main) | — | — | main |

## 3. The execution loop (every task)
1. Locate the first unchecked `TODO.md` box; read the plan task.
2. RED: write the mode-asserting test; run it; record the FAIL line.
3. GREEN: tighten the mode / add the justified `#nosec`; run to PASS.
4. Gates: module `go test ./...`; module gosec with the policy flags → `Issues : 0` (or no output under `-quiet`).
5. Evidence: `tee` the gate output into `evidence/<module>/`.
6. Ledger: tick TODO, update TRACKING row; commit by explicit path; `gss feature checkpoint --worker <ref> --auto` (re-run without `--auto` if it reports "skipped").

## 4. Done-when gates
- Per module: tests green + gosec zero.
- fix leaf: `make security` → `OK: no vulnerability or misuse findings`; CI Security Scan green on the PR.
- promote leaf: green `main` run observed; red proof PR held by the queue.

## 5. Hard rules
- No bare `#nosec`: always `#nosec G30x -- <consumer and why>`.
- Never widen a mode; never change what a file contains.
- No `git add -A`; confirm before publishing; never run `install.sh` from a worktree.
- The privacy hook rejects text that looks like `<Name>Token = "..."` and any literal worktree path — describe, don't paste.

## 6. Command cheat-sheet
```
G=$(go env GOPATH)/bin/gosec
(cd sdk/<m> && $G -quiet -exclude-generated -nosec-require-justification -exclude=G204,G304,G702,G703 ./...)
(cd sdk/<m> && go test ./...)
make security
gss feature checkpoint --worker security-scan-green/edward-raigosa/fix --auto
```

## 7. Resuming, recovery, and corrections
Fix wrong commands here and note it in the TRACKING session log. Contract defects → TRACKING blockers.

## 8. Kickoff prompt (always CURRENT)
> Mission: finish `security-scan-green` task by task. Read first: this file, `TRACKING.md`, `TODO.md`,
> the plan. Scope in order: policy → gsl → gss → tmux-mgr → fleet/gff/gcfg/wlink/ghapp/libs → whole-repo
> → (after merge + green main) promote. Stop for a human at: labelling the fix PR ready-for-merge, the
> ruleset edit, the red proof PR. Blocked → TRACKING §4. Done when the promote leaf's gate is ticked.
