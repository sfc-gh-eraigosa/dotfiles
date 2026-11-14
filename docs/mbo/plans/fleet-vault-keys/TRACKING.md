# fleet vault-backed workstation keys — live state ledger

- **Slug:** fleet-vault-keys
- **Started:** 2026-11-13
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../fleet-vault-keys.md`](../fleet-vault-keys.md) · spec [`../../specs/fleet-vault-keys.md`](../../specs/fleet-vault-keys.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a
> commit SHA **and** evidence. Never write a result you did not observe.

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| all | `fleet-vault-keys/${USER}/design` | `feature/fleet-vault-keys/${USER}/design` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/fleet-vault-keys/${USER}/design` | #452 | draft |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| T0 vendor spike | todo | | | needs operator sign-in |
| T1 vault contract | todo | | | |
| T2 payload + roster | todo | | | |
| T3 op adapter | todo | | | |
| T4 lpass adapter | todo | | | |
| T5 vault.yaml + gate | todo | | | |
| T6 keystore | todo | | | |
| T7 agent + askpass | todo | | | |
| T8 pre-flight | todo | | | |
| T9 vault setup/status | todo | | | |
| T10 vault push | todo | | | |
| T11 vault pull | todo | | | |
| T12 keys unlock | todo | | | |
| T13 runner probe + keys rotate | todo | | | |
| T14 roster-aware sync/prune | todo | | | |
| T15 TUI `V` | todo | | | |
| T16 docs + live gates | todo | | | |

### Vendor argv (plan §3) — confirmed/corrected by Task 0

| Line | Status | Correction |
| :-- | :-- | :-- |
| op ready / signin / get / create / update / list | unconfirmed | |
| lpass ready / login / get / create / update / list | unconfirmed | |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 Store + Secret redaction | [ ] `TestSecretNeverPrints` | — | |
| F2 op adapter | [ ] T3 suite | [ ] G1 | |
| F3 lpass adapter | [ ] T4 suite | [ ] G2 | |
| F4 config + gate + fallback | [ ] `TestNoVaultMeansTodaysKeysSyncArgv` | — | |
| F5 keystore | [ ] T6 suite | — | |
| F6 agent/askpass | [ ] T7 suite | [ ] G4 | |
| F7/F8 setup/status | [ ] T9 suite | [ ] G1 | |
| F9 push | [ ] T10 suite | [ ] G1 | |
| F10 pull | [ ] T11 suite | [ ] G3 | |
| F11 unlock | [ ] T12 suite | [ ] G4 | |
| F12 pre-flight | [ ] T8 suite | [ ] G4 | |
| F13 rotate | [ ] T13 suite | [ ] G5 | |
| F14 roster sync/prune | [ ] T14 suite | [ ] G6 | |
| F15 TUI | [ ] T15 suite | [ ] G3 | |
| F16 docs | — | [ ] G1–G6 transcripts | |

## 3. Validation done-when — the stop condition

- [ ] Every Task 0–16 row `done` with SHA + evidence.
- [ ] `./scripts/test.sh` green; every new package ≥ 60 %; `cmd` not below its prior figure.
- [ ] `make lint-go` green.
- [ ] `TestNoRemoteCommandCarriesAPrivateKey` and the argv/env secret scans green.
- [ ] Without `vault.yaml`, `keys sync`/`prune` argv unchanged (golden).
- [ ] Live gates G1–G6 transcripts in `evidence/e2e/`.
- [ ] README/AGENTS demos are real captured output.
- [ ] `docs/mbo/index.md` state advanced to `in-review`.

## 4. Blockers & escalations

Failing command + its **real** output. Contract defects go here and get escalated —
never silently patched.

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-11-13 | planning | Plan + execution trio written on worker `design` (PR #452). |
