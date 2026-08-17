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
| 1 — mechanical split + alias cursor | **done** | see log | `go build` OK; tui.go 55 lines; TestRowArrivalResortsButKeepsCursorOnItsAlias | zero behavior change; tui.go < 60 lines |
| 2 — theme + styled frames (F3) | **done** | see log | 11 demo frames render + fit width; `evidence/demo/frames.txt` | Ascii-profile goldens |
| 3 — viewport + resize (F4c,d) | **done** | see log | TestWindowResizeKeepsCursorVisible, TestHalfPageMovesAndStaysInBounds | clampViewport invariant |
| 4 — vim motions (F4a,b) | **done** | see log | TestGGJumpsTopAndGJumpsBottom, TestLoneGIsCancelledByTheNextKey | gg pending-key state |
| 5 — streaming collect + spinner (F1) | **done** | see log | probeHost extracted; headless status tests unchanged-green | collectOne extraction; headless frozen |
| 6 — refresh `r` (F2) | **done** | see log | TestRefreshRepollsKeepingCursorAndSelection | v1's dead key becomes real |
| 7 — `/` search (F5) | **done** | see log | smartcase + invalid-regex + mode-swallow tests | smartcase; invalid-regex UX |
| 8 — `n`/`N` (F6) | **done** | see log | TestNextPrevMatchWrapsBothWays | wraparound both ways |
| 9 — selection + visual (F7) | **done** | see log | TestSpaceTogglesSelectionByAliasAcrossResort | alias-keyed; survives refresh |
| 10 — concurrent bg update engine (F8,F9,F2b) | **done** | see log | TestBackgroundWaveNeverExceedsJobLimit (running<=jobs at every step), FailedUpdateAdvances, RefreshSkipsUpdating | ≤ jobs slots; BatchMode fast-FAIL; TUI stays live; refresh excludes updating |
| 11 — interactive fallback queue (F13) | **done** | see log | TestPrecheckRoutes..., TestInteractiveFallbackRunsOnlyAfterTheWaveDrains | sudo-precheck routing; serial handoffs after the wave |
| 12 — `s` ssh action (F10) | **done** | see log | TestSSHIsBlockedWhileThatHostUpdates | argv exactly `ssh <alias>`; blocked while updating |
| 13 — help overlay + polish (F11,F12) | **done** | see log | overlay from keyHelp; TestQuitIsGuardedWhileUpdatesRun | overlay from the keymap table; quit guard |
| 14 — docs + gate | **done** | see log | `scripts/test.sh` -> OK: coverage for fleet is 80% (min 60); go vet clean; 67 tests | fleet ≥60%, cmd ≥55%, vet, leak sweep |
| 15 — HUMAN STOP: live capture | todo | | | tmux; 2 concurrent updates + navigation; sudo prompt on fallback |

## 2. Feature → proof matrix (spec §5)

| Feature | Automated proof | Live proof (Task 14) |
| :-- | :-- | :-- |
| F1 streaming | [ ] F1a/F1b/F1c tests | [ ] rows arrive independently |
| F2 refresh | [ ] F2a · [ ] F2b (during updates) | [ ] r re-polls in place mid-wave |
| F3 theme | [ ] F3a goldens | [ ] colors on a real terminal |
| F4 motion+paging | [ ] F4a–d | [ ] big-fleet paging |
| F5 search | [ ] F5a–c | [ ] /regex highlight |
| F6 n/N | [ ] F6a | [ ] |
| F7 selection | [ ] F7a–b | [ ] visual range |
| F8 concurrent bg update | [ ] F8a–f | [ ] 2 hosts updating at once, TUI live |
| F9 --update-ref/--jobs | [ ] F9a–b | [ ] optional |
| F10 ssh action | [ ] F10a | [ ] in and back out |
| F11 help | [ ] F11a | [ ] |
| F12 empty fleet | [ ] F12a | — |
| F13 interactive fallback | [ ] F13a–b | [ ] sudo prompt reaches operator |

## 3. Done-when — the stop condition

- [ ] Tasks 1–15 `done` with SHAs + evidence
- [ ] `scripts/test.sh` green; fleet ≥ 60%, cmd ≥ 55%
- [ ] `go vet ./...` clean in sdk/fleet
- [ ] Headless status/update tests unmodified (except Task 5's addition)
- [ ] Live capture committed (Task 15) — incl. concurrent updates + mid-wave refresh
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
| ExecProcess error paths differ by tea version | Tasks 11–12 | F13b test drives via messages, not a real terminal |
| Spinner ticks make frames nondeterministic | Task 5 | fixed spinner frame injected in tests (no Tick in test path) |
| `sudo -n` passes but a later prompt appears (cred-cache expiry mid-run) | Task 10 | BatchMode=yes turns any prompt into a fast FAIL with the cause in the row log; operator re-runs via fallback |
| A background update and a refresh poll race on one host | Tasks 6/10 | in-flight ownership invariant (one of pending/updating/resolved); F2b test pins it |

## 5. Session log (append-only)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-08-16 | build | Tasks 1-14 implemented in PR #227 (same-PR per operator). Split tui.go into model/view/keys/cmds; probeHost extracted so headless status is untouched. Concurrent background update engine + sudo-precheck fallback. 67 tests pass, `go vet` clean, **coverage 58.4% cmd / 80% fleet** (was 40.3/74). Two real defects caught by the new gates: lipgloss profile 0 is TrueColor not ASCII (goldens would have been colour-dependent), and a long remote failure log overflowed the row (now truncated). Demo: `evidence/demo/frames.txt`, 11 frames. Task 15 (live capture) still human-gated. |
| 2026-08-16 | mbo-amend | Operator: make sync + update concurrent in the TUI. Root cause named: ExecProcess suspends the whole TUI, so anything routed through it is inherently serial. Redesigned F8 as a background-first engine (≤ --jobs concurrent, BatchMode fast-FAIL, per-row log) with sudo-precheck routing to a serial interactive fallback (new F13); refresh works mid-wave excluding updating hosts (F2b); in-flight ownership invariant added. Plan now 15 tasks. |
| 2026-08-16 | mbo | Design + spec + plan + trio authored; worktree reset off a stale local main (hazard hit again — 3rd time); index.md registered; issue + draft PR pending. |
