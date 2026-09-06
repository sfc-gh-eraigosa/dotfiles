# gcfg — live state ledger

- **Slug:** gcfg
- **Started:** 2026-09-05 (design)
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../gcfg.md`](../gcfg.md) · spec [`../../specs/gcfg.md`](../../specs/gcfg.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a
> commit SHA **and** evidence. Never write a result you did not observe.

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| design | `gcfg/edward-raigosa/design` | `feature/gcfg/edward-raigosa/design` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gcfg/edward-raigosa/design` | #285 | designing |
| build (P0+P1 sequential) | `gcfg/edward-raigosa/build` | `feature/gcfg/edward-raigosa/build` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gcfg/edward-raigosa/build` | [#287](https://github.com/sfc-gh-eraigosa/dotfiles/pull/287) (draft, base = design) | building |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| P0-T1 ghapp scaffold + version | done | `e2b78f2` | `go test ./... -count=1 -cover` → 3 pkgs ok (main 90.9%, cmd 83.3%, version 100%); CI formula total 88.2% ≥80; `go run . version` → `ghapp vdev …`; actionlint clean → [evidence/P0-T1/2026-09-05.txt](./evidence/P0-T1/2026-09-05.txt) | ghapp-ci.yml added (no PR path filter, like gff-ci) |
| P0-T2 store + JWT | done | `02edb2d` | `go test ./pkg/ghapp/ -count=1 -cover` → ok 87.5% (12 tests: 0700 dir, 0600 PEM+apps.json, round-trip, 0644 PEM refused naming file+mode, missing PEM, bad slug, corrupt JSON, XDG dir; RS256 iss/iat-60s/exp+9m verified with pubkey, wrong key rejected, PKCS1+PKCS8, non-RSA rejected); module total 87.7% → [evidence/P0-T2/2026-09-05.txt](./evidence/P0-T2/2026-09-05.txt) | golang-jwt/jwt/v5 v5.3.1 |
| P0-T3 installation tokens | done | (this commit) | `go test ./pkg/ghapp/ -count=1 -cover` → ok 89.2% (JWT-bearer stub; 2-page installations; scoped body permissions/repositories, no body when unscoped; cache hit at expiry−3m, miss at expiry−1m30s, per-inst/scope keys; Token redacts under String/%v/%+v/%s/JSON; 401/403 errors carry status+message; lazy PEM load); leak grep clean; module total see evidence → [evidence/P0-T3/2026-09-05.txt](./evidence/P0-T3/2026-09-05.txt) | ghapp-ci: `pull_request` base filter dropped (stacked PR #287 got no run) |
| P0-T4 manifest flow + CLI | todo | | | human: one real create (redacted) |
| P1-T1 gcfg scaffold + CI | todo | | | |
| P1-T2 schema load + lint + JSON Schema | todo | | | |
| P1-T3 gh client + credential chain | todo | | | |
| P1-T4 family model + general + security | todo | | | |
| P1-T5 engine + renderers | todo | | | |
| P1-T6 verbs export/verify/plan/apply/init | todo | | | live UC1–UC3 evidence |
| P2-T1 rulesets family (+ import) | todo | | | |
| P2-T2 actions family | todo | | | |
| P2-T3 labels family | todo | | | |
| P2-T4 autolinks family | todo | | | |
| P2-T5 environments family | todo | | | |
| P2-T6 secrets (names) + webhooks families | todo | | | |
| P2-T7 collaborators + pages families | todo | | | |
| P2-T8 org profile + members + security_defaults | todo | | | |
| P2-T9 org actions + org rulesets | todo | | | |
| P2-T10 org apps (report) | todo | | | |
| P3-T1 auth doctor + pat | todo | | | |
| P3-T2 auth app wrappers | todo | | | |
| P3-T3 actions install/uninstall | todo | | | |
| P3-T4 e2e harness | todo | | | |
| P4-T1 TUI navigation + search | todo | | | |
| P4-T2 TUI editors + write-back | todo | | | |
| P4-T3 TUI verify/apply | todo | | | |
| P5-T1 wiring (flag, install.sh, Makefile, docs) | todo | | | |
| P5-T2 this repo's gcfg.yaml + schema + CI verify | todo | | | |
| P5-T3 workflows installed + red→green evidence | todo | | | |
| P5-T4 retire one-off scripts | todo | | | only after P5-T2 green |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 schema & lint | [ ] | [ ] n/a | |
| F2 export | [ ] | [ ] live export of this repo | |
| F3 verify | [ ] | [ ] UI drift caught (exit 1) | |
| F4 plan & apply | [ ] | [ ] apply + re-read on this repo | |
| F5 init & templates | [ ] | [ ] n/a | |
| F6 TUI | [ ] | [ ] short recording | |
| F7 actions install | [ ] | [ ] verify red→green; apply run log | |
| F8 auth | [ ] | [ ] doctor for gh token + App token | |
| F9 ownership | [ ] | [ ] n/a | |
| F10 families | [ ] | [ ] export covers every family present live | |
| F11 ghapp | [ ] | [ ] one real create (redacted) + token mint | |
| F12 adoption | [ ] | [ ] `make gcfg-verify` green in CI | |

## 3. Validation done-when — the stop condition

- [ ] `gcfg-ci` + `ghapp-ci` green with 80/90/90 bars
- [ ] `scripts/e2e.sh` green
- [ ] `.github/gcfg.yaml` exported, reviewed, `make gcfg-verify` green in CI
- [ ] `gcfg-verify.yml` + `gcfg-apply.yml` installed; red→green evidence captured
- [ ] `gcfg auth doctor` evidence for gh token and App token
- [ ] `github_secret_scanning.sh` + `ruleset_snapshot.sh` + `.github/rulesets/main.json` removed
- [ ] every task row `done` with SHA + evidence; `index.md` state advanced

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |
| 2026-09-05 | P0-T3 | Plan §3.2 contract defect: `App` declares field `Installations map[string]int64` **and** method `Installations(ctx)`; Go cannot have both | `go test ./pkg/ghapp/` → `cannot call s.app().Installations (value of type map[string]int64): map[string]int64 is not a function` | Field renamed `Installs` (JSON key `installations` unchanged); method keeps the contract name. Plan text left as-is per §5 (escalate, never edit) — owner to amend §3.2 when the design PR is next touched |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-09-05 | design | POC research (gh token scopes, GITHUB_TOKEN has no administration permission, fine-grained Administration covers all repo endpoints, org endpoints need admin:org / org Administration, non-provider patterns not honoured on this plan, safe-settings/Terraform/Probot prior art); design + spec + plan + trio written; feature `gcfg` + design worker created |
| 2026-09-05 | build (P0 start) | registry row for `gcfg` re-created (`gss feature start`, dropped by an earlier shared-registry `audit --repair`); worker `gcfg/edward-raigosa/build` added, base = design branch (#285 not merged; owner chose to build on it); P0-T1 RED→GREEN, main refactored to a testable `run()` to clear the 80% bar |
