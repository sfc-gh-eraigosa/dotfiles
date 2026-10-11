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
| T1 manifest + pins.sh + flag | done | 0e2f995 | `bash opt/lib/pins_test.sh` → PASS 44 FAIL 0 (also dash); `gff lint` rc=0 — evidence/T1.txt | L1; flag is `pins.track.mode` (gff needs 3-part keys), see §4 |
| T2 converge + manifest defaults | done | f9d640b, fb9f8e2 | sops 26/0, yq 22/0, k8s 28/0, gitleaks 42/0; uv/glow/rust/herdr/keyd/fonts no regressions; `make shell-test` 61/0; lint-shell + lint-portability rc=0 — evidence/T2.txt | L1 |
| T3 internal/pins manifest/version | done | a5ba8f2 | `go test ./internal/pins -cover` — evidence/T3.txt | L2 |
| T4 upstream resolver | done | b12ff6a | httptest suite green — evidence/T4.txt | L2 |
| T5 pins status/bump | done | 40b6a7f | `go test ./cmd -run Pins` + -race green — evidence/T5.txt | L2 |
| T6 installed + apply | done | fcba5ee, 8ec3df4 | internal/pins 94.3%, module 87.6%, golangci-lint 0 — evidence/T6.txt | L2; floor-under-pin fixed by lead |
| T7 status PINS column | done | 1fdd5fa | `go test ./...` green, module 87.6%, lint 0 — evidence/T7.txt | TUI column deferred (width), plan §3.5 |
| T8 pins.yaml + workflow + docs | done | 936626c, 3bde2ce | actionlint clean; `fleet update --file opt/etc/fleet/pins.yaml --dry-run` prints sync + pins steps — evidence/T8.txt |  |
| T9 go bump + live evidence | done | 154eee8 | `fleet pins bump go --to 1.26.3`; live `pins status --installed` ok 10 drift 1 missing 1 unknown 2; `fleet status` PINS "-" on hosts still on main — evidence/T9.txt |  |

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
- [x] T1–T9 done with evidence
- [x] coverage gates (≥85% internal/pins, ≥60% module)
- [x] lint clean (`make lint-shell`, `make lint-portability`, golangci-lint; `lint-go` target does not exist)
- [x] code review pass, confirmed findings fixed (d02af4d): 6 fixed, #5 kept by design (fleet hosts all BEHIND the pins, no helm 3→4), #8 installer-bootstrap dedupe + TUI PINS column → follow-up, #9 pool dedupe skipped (minor)

## 4. Blockers
| Date | Task | Blocker | Command + output | Resolution |
| :-- | :-- | :-- | :-- | :-- |
| 2026-10-10 | T1 | contract name `pins.track` invalid | `gff lint`: path "pins.track" must have exactly 3 dotted segments, got 2 | renamed `pins.track.mode` everywhere (3bde2ce) |

## 5. Session log
| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-10-10 | 1 | design/spec/plan/trio written; feature `pins` + worker created |
| 2026-10-10 | 1 | L1 + L2 built by parallel subagents; L3 (PINS column, pins.yaml, workflow, docs, go bump) by lead; code review → 6 fixes (d02af4d) |
