# mono-gdash — live state ledger

- **Slug:** mono-gdash
- **Started:** 2026-10-18
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../mono-gdash.md`](../mono-gdash.md) · spec [`../../specs/mono-gdash.md`](../../specs/mono-gdash.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`. A row is `done` only
> with a commit SHA **and** evidence (the command plus its real result).

## 0. Worker registry

| Leaf | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| design | `mono-gdash/edward-raigosa/design` | `feature/mono-gdash/edward-raigosa/design` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/mono-gdash/edward-raigosa/design` | #411 | design + spec + plan + trio |
| gdash | `mono-gdash/edward-raigosa/gdash` | `feature/mono-gdash/edward-raigosa/gdash` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/mono-gdash/edward-raigosa/gdash` | — | Task 0 |
| deck | — | — | — | — | not created |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 0 lab facts + fixtures | done | (this commit) | `capture.py` → 4 dashboards, 10 panel queries, all with frames; scrub greps → 0 files (`evidence/task0/capture.txt`) | issued by the service account `gdash-viewer` (Viewer; admin API → 403), created 2026-10-19 with the owner's go-ahead |
| A1 scaffold | done | (this commit) | RED: `TestVersionPrintsStampedVersion` printed the root help (no version cmd); GREEN: `go test -cover ./...` → cmd 81.8%; `build.sh` builds, `gdash version` prints the stamped version; golangci-lint 0 issues (`evidence/a1/a1.txt`) | |
| A2 config + auth | done | (this commit) | RED: build failures (no package) + `unknown command "config"`; GREEN: cmd 81.1%, auth 90.6% (≥ 80 bar), config 75.1%; E10/E11/E13 tests pass; lint 0 issues; gosec (repo flags) exit 0 (`evidence/a2/a2.txt`) | |
| A3 client | todo | | | |
| A4 dash + vars | todo | | | |
| A5 render | todo | | | record the charting choice |
| A6 app | todo | | | |
| A7 doctor + list | todo | | | |
| A8 security + install + docs | todo | | | |
| B1 config schema | todo | | | |
| B2 probe + doctor | todo | | | |
| B3 deck runtime | todo | | | |
| B4 nav mode | todo | | | |
| B5 config verbs + UI | todo | | | |
| B6 boundary + docs | todo | | | |
| C1 default mid deck | todo | | | after both leaves merge |
| C2 live evidence | todo | | | |

**Lab facts (Task 0, 2026-10-19):**
- **Grafana:** `13.1.3`; dashboard schemaVersion 39; every target's datasource is `prometheus`, via the `$datasource` variable.
- **conductor:** `conductor-metrics`, "Conductor — metrics against goals". 17 panels: 3 row, 6 stat, 8 timeseries.
- **CI:** `ci-lab`, "CI — lab runners". 37 panels: 5 row, 16 stat, 10 timeseries, 4 table, 2 bargauge.
- **k3s-nodes and nodes (owner, 2026-10-19): use both.** `k3s-nodes` is the Nodes Overview, and Node Exporter becomes the extra view `nodes`. The spec amendment lists all four views.
  - `7d57716318ee0dddbac5a7f451fb7753` "Node Exporter / Nodes": 12 panels (4 row, 6 timeseries, 1 gauge, 1 table); vars `cluster`, `instance`.
  - `058020e04168bfdea0c52269cb699df2` "Kubernetes / Compute Resources / Nodes Overview": 5 timeseries; var `cluster`.
- **Not supported in v1:** `table` panels (4 in CI, 1 in Node Exporter) become placeholders under spec F3, so A5 must render the placeholder for them.

## 2. Feature → proof matrix (spec §5)

| Feature | Automated proof | Live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 fetch | [ ] E1 E2 | [ ] `gdash conductor` on spark1 | |
| F2 vars | [ ] E3 E4 | [ ] k3s-nodes `$node` panels draw | |
| F3 panels/layout | [ ] E5 E6 | [ ] capture at 80 and 120+ cols | |
| F4 refresh/stale | [ ] E7 E8 | [ ] Grafana unreachable → stale row | |
| F5 keys | [ ] E9 | — | |
| F6 config/auth | [~] E10 E11 E13 (E12 is A8) | [ ] `config get` shows ‹redacted› | |
| F7 doctor/list | [ ] E14 E15 | [ ] doctor pass on spark1, exit 1 without credentials | |
| F8 decks | [ ] E16–E19 E24 | [ ] one gdash process while cycling | |
| F9 nav | [ ] E20–E23 | [ ] capture of the deck cycle | |

## 3. Validation done-when — the stop condition

- [ ] gdash leaf merged (coverage ≥ 60%; auth/vars/render ≥ 80%; lint + security green)
- [ ] deck leaf merged (coverage ≥ 60%; E16 compat golden green)
- [ ] C1 default `mid` deck merged
- [ ] C2 evidence in `evidence/e2e/`
- [ ] `index.md` row → `done`

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log (append-only)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-10-18 | design session (claude-mono) | design #411; owner answered nav mode |
| 2026-10-18 | playground session | Grafana via LAN NodePort (Q2/Q3 closed); spec + plan + trio written; owner approved |
| 2026-10-19 | playground session | #411 merged; gdash worker created; Viewer token issued; Task 0 fixtures captured + scrubbed |
| 2026-10-19 | playground session | owner picked k3s-nodes = Nodes Overview and kept Node Exporter as a 4th view `nodes`; spec F8 amended |
