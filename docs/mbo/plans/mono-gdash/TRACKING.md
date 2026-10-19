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
| gdash | — | — | — | — | not created |
| deck | — | — | — | — | not created |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 0 lab facts + fixtures | todo | | | needs the owner's Viewer token |
| A1 scaffold | todo | | | |
| A2 config + auth | todo | | | |
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

**Lab facts (Task 0 fills these):** Grafana version `13.1.3` (checked 2026-10-18 via
`/api/health`) · conductor uid `—` · k3s-nodes uid `—` · CI uid `—` · datasource types `—`.

## 2. Feature → proof matrix (spec §5)

| Feature | Automated proof | Live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 fetch | [ ] E1 E2 | [ ] `gdash conductor` on spark1 | |
| F2 vars | [ ] E3 E4 | [ ] k3s-nodes `$node` panels draw | |
| F3 panels/layout | [ ] E5 E6 | [ ] capture at 80 and 120+ cols | |
| F4 refresh/stale | [ ] E7 E8 | [ ] Grafana unreachable → stale row | |
| F5 keys | [ ] E9 | — | |
| F6 config/auth | [ ] E10–E13 | [ ] `config get` shows ‹redacted› | |
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
