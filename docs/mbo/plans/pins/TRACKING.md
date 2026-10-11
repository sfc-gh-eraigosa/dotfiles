# pins — live state ledger

- **Slug:** pins
- **Started:** 2026-10-10
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)

## 0. Worker registry
| Leaf/worker | Worker ref | Branch | PR | State |
| :-- | :-- | :-- | :-- | :-- |
| build (L1+L2+L3) | `pins/edward-raigosa/build` | `feature/pins/edward-raigosa/build` | — | building |

## 1. Task ledger
| Task | Status | Commit | Evidence | Notes |
| :-- | :-- | :-- | :-- | :-- |
| T1 manifest + pins.sh + flag | todo | | | L1 |
| T2 converge + manifest defaults | todo | | | L1 |
| T3 internal/pins manifest/version | todo | | | L2 |
| T4 upstream resolver | todo | | | L2 |
| T5 pins status/bump | todo | | | L2 |
| T6 installed + apply | todo | | | L2 |
| T7 status PINS column | todo | | | L3 |
| T8 pins.yaml + workflow + docs | todo | | | L3 |
| T9 go bump + live evidence | todo | | | L3 |

## 2. Feature → proof matrix
| Feature | Automated | Live | Notes |
| :-- | :-- | :-- | :-- |
| F1 manifest | [ ] | | |
| F2 pins_want | [ ] | | |
| F3 converge | [ ] | | |
| F4 defaults | [ ] | | |
| F5 status | [ ] | [ ] | |
| F6 --installed | [ ] | [ ] | |
| F7 bump | [ ] | [ ] | |
| F8 apply | [ ] | | |
| F9 PINS column | [ ] | [ ] | |
| F10 pins.yaml | [ ] | | |
| F11 workflow | [ ] | | |
| F12 flag | [ ] | | |

## 3. Stop condition
- [ ] T1–T9 done with evidence
- [ ] coverage gates (≥85% internal/pins, ≥60% module)
- [ ] `make lint-shell lint-portability lint-go` clean
- [ ] code review pass, confirmed findings fixed

## 4. Blockers
| Date | Task | Blocker | Command + output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log
| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-10-10 | 1 | design/spec/plan/trio written; feature `pins` + worker created |
