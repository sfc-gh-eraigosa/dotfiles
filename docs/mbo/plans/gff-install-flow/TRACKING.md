# gff-install-flow — live state ledger

- **Slug:** gff-install-flow
- **Started:** 2026-07-26
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../gff-install-flow.md`](../gff-install-flow.md) · spec [`../../specs/gff-install-flow.md`](../../specs/gff-install-flow.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a
> commit SHA **and** evidence. Never write a result you did not observe.

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| impl | `gff-install-flow/edward-raigosa/impl` | `feature/gff-install-flow/edward-raigosa/impl` | `${HOME}/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff-install-flow/edward-raigosa/impl` | [#193](https://github.com/sfc-gh-eraigosa/dotfiles/pull/193) | planning → building |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| T1 `opt/lib/winsetup.sh` + test driver (TDD) | todo | | | 9 cases, bash AND dash; RED observed first |
| T2 `install_windows.sh` `--ask`/`--deferred` split | todo | | | per-run deploy stays answer-independent |
| T3 `install.sh` early export + Windows-last | todo | | | 3 surgical edits; order-grep verify |
| T4 PowerShell `-GffEnv` + log + loud UAC failure | todo | | | pwsh parse check or defer to T6 (not faked) |
| T5 docs + ledgers | todo | | | root AGENTS.md bullet, index.md, gff §10 row-5 closure |
| T6 human validation matrix (owner, real WSL) | todo | — | | 4 runs; capstone = elevated-log wispr SKIP |
| **Objective gate** | todo | — | | see §3 |

## 2. Feature → proof matrix (spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| **F1** early export | [ ] T3 order-grep | [ ] matrix runs 2–4 (no manual eval anywhere) | |
| **F2** prompt-early / Windows-last | [ ] winsetup tests 1–2, 9 | [ ] matrix run ordering (prompt ≤1 min, exec last) | |
| **F3** gff-owned skip state | [ ] winsetup tests 3–8 | [ ] matrix run 3 (`gff list` override, no sentinel) | |
| **F4** UAC argument hand-off | [ ] T4 parse check | [ ] matrix run 1 elevated-log SKIP | |
| **F5** readable elevated log | — (path change) | [ ] matrix run 1 `cat` via /mnt/c, no elevation | |
| **F6** loud UAC failure | [ ] T4 code (PassThru + catch) | [ ] optional live decline probe | |

## 3. Validation done-when — the stop condition

- [ ] Tasks 1–5 done with evidence; winsetup driver green under bash AND dash
- [ ] Both lint gates clean on the final tree (`make lint-shell && make lint-portability`)
- [ ] Four-run human matrix captured → `../gff/evidence/F09-gating/gff-install-flow-matrix.txt`
- [ ] Run 1 shows `SKIP (gff: install.windows.wispr-flow=false)` in the **elevated** log
- [ ] gff TRACKING §10 row 5 (UAC env-strip) resolution updated; root AGENTS.md flow text updated
- [ ] `docs/mbo/index.md` state advanced (building → in-review → merged)
- [ ] PR #193 ready (owner-confirmed) and merged (owner-gated)

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-07-26 | planning | Spec + plan authored and approved (owner); worker + draft PR #193 created; execution trio (this file, IMPLEMENTATION, TODO) scaffolded; templates added to `docs/mbo/templates/` and the mbo-plan skill now requires the trio for build objectives |
