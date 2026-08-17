# fleet-tui — live state ledger

- **Slug:** fleet-tui
- **Started:** 2026-08-16
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../fleet-tui.md`](../fleet-tui.md) · spec [`../../specs/fleet-tui.md`](../../specs/fleet-tui.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> A row is `done` only with a commit SHA **and** observed evidence.
> **Never write a result you did not observe.**

## 0. Worker registry

| Worker | Worker ref | Branch | PR | State |
| :-- | :-- | :-- | :-- | :-- |
| design (this) | `fleet-tui/<user>/mbo` | `feature/fleet-tui/<user>/mbo` | [#227](https://github.com/sfc-gh-eraigosa/dotfiles/pull/227) | active |
| build | `fleet-tui/<user>/build` | `feature/fleet-tui/<user>/build` | TBD | not created |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 1 — mechanical split + alias cursor | todo | | | zero behavior change; tui.go < 60 lines |
| 2 — theme + styled frames (F3) | todo | | | Ascii-profile goldens |
| 3 — viewport + resize (F4c,d) | todo | | | clampViewport invariant |
| 4 — vim motions (F4a,b) | todo | | | gg pending-key state |
| 5 — streaming collect + spinner (F1) | todo | | | collectOne extraction; headless frozen |
| 6 — refresh `r` (F2) | todo | | | v1's dead key becomes real |
| 7 — `/` search (F5) | todo | | | smartcase; invalid-regex UX |
| 8 — `n`/`N` (F6) | todo | | | wraparound both ways |
| 9 — selection + visual (F7) | todo | | | alias-keyed; survives refresh |
| 10 — batch update + --update-ref (F8,F9) | todo | | | declined = nothing; queue survives failure |
| 11 — `s` ssh action (F10) | todo | | | argv exactly `ssh <alias>` |
| 12 — help overlay + polish (F11,F12) | todo | | | overlay from the keymap table |
| 13 — docs + gate | todo | | | fleet ≥60%, cmd ≥55%, vet, leak sweep |
| 14 — HUMAN STOP: live capture | todo | | | tmux; sudo prompt visible; FAIL advances queue |

## 2. Feature → proof matrix (spec §5)

| Feature | Automated proof | Live proof (Task 14) |
| :-- | :-- | :-- |
| F1 streaming | [ ] F1a/F1b/F1c tests | [ ] rows arrive independently |
| F2 refresh | [ ] F2a | [ ] r re-polls in place |
| F3 theme | [ ] F3a goldens | [ ] colors on a real terminal |
| F4 motion+paging | [ ] F4a–d | [ ] big-fleet paging |
| F5 search | [ ] F5a–c | [ ] /regex highlight |
| F6 n/N | [ ] F6a | [ ] |
| F7 selection | [ ] F7a–b | [ ] visual range |
| F8 batch update | [ ] F8a–c | [ ] sudo prompt + FAIL advances |
| F9 --update-ref | [ ] F9a | [ ] optional |
| F10 ssh action | [ ] F10a | [ ] in and back out |
| F11 help | [ ] F11a | [ ] |
| F12 empty fleet | [ ] F12a | — |

## 3. Done-when — the stop condition

- [ ] Tasks 1–14 `done` with SHAs + evidence
- [ ] `scripts/test.sh` green; fleet ≥ 60%, cmd ≥ 55%
- [ ] `go vet ./...` clean in sdk/fleet
- [ ] Headless status/update tests unmodified (except Task 5's addition)
- [ ] Live capture committed (Task 14)
- [ ] README/AGENTS updated; no identity leak
- [ ] `docs/mbo/index.md` → `in-review`

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |
| | | *(none yet)* | | |

**Pre-identified risks:**

| Risk | Where it bites | What to do |
| :-- | :-- | :-- |
| Stale local main under a new worktree (hit 3× on fleet) | worker creation | preflight §2.1; reset to origin/main |
| Golden frames vary across lipgloss/termenv versions | Tasks 2+ | pin Ascii profile in TestMain; goldens assert the Ascii frame only |
| ExecProcess error paths differ by tea version | Tasks 10–11 | F8c test drives via messages, not a real terminal |
| Spinner ticks make frames nondeterministic | Task 5 | fixed spinner frame injected in tests (no Tick in test path) |

## 5. Session log (append-only)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-08-16 | mbo | Design + spec + plan + trio authored; worktree reset off a stale local main (hazard hit again — 3rd time); index.md registered; issue + draft PR pending. |
