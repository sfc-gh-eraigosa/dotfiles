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
| PR 1 (docs, T1–T7, T9, T10) | `fleet-vault-keys/${USER}/design` | `feature/fleet-vault-keys/${USER}/design` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/fleet-vault-keys/${USER}/design` | #452 | draft |
| PR 2 (T11) | `fleet-vault-keys/${USER}/pull` | `feature/fleet-vault-keys/${USER}/pull` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/fleet-vault-keys/${USER}/pull` | — | local only |

### Stack (4 stacked PRs — operator ruling 2026-11-13; refresh from `gss feature list --feature fleet-vault-keys --tree`)

| # | PR | Worker | Base | Tasks | Done | State |
| :-- | :-- | :-- | :-- | :-- | :-- | :-- |
| 1 | save/push — #452 | `design` | `main` | docs, T1–T7, T9, T10 | T1–T7, T9, T10 | draft · built, awaiting checkpoint |
| 2 | pull — (not opened) | `pull` | PR 1 | T11 | T11 | built (22daf29), not pushed |
| 3 | rest — (not opened) | `lifecycle` (created at T8) | PR 2 | T8, T12–T16 | — | not started |
| 4 | 1Password live gate — (not opened) | `op` (after PR 3) | PR 3 | G1 + any fixes it finds | adapter already on PR 1 | waiting on a 1Password account |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| T0 vendor spike | done (lpass); op deferred | 40479f9..544d6c5 | `FLEET_VAULT_LIVE=lpass go test -tags live -run TestLiveRoundTrip ./internal/vault/lpass/` → PASS ×3 (evidence/spike/lpass.txt); run 1 found the trailing-\n version bug | Go live test per operator ruling; `fleet/fleet:spike` left in vault, safe to delete after 2026-11-20 |
| T1 vault contract | done | 7414fc8 | `go test -cover ./internal/vault/...` → PASS, vault 94.9 %, providers 100 % | review clean |
| T2 payload + roster | done | 2df7add, a72a737 | `go test -cover ./internal/vault/...` → PASS, vault 96.4 % | JSON field names pinned; retired keys stay retired |
| T3 op adapter | done (unconfirmed live) — on PR 1 as future code | 7067bf4 (cherry-pick of 953dca5) | `go test -cover ./internal/vault/...` → PASS, op 89.7 % | built from real `op` v2.42.0 help; live test written, not run (no account) |
| T4 lpass adapter | done | 40479f9, b29734b, 544d6c5 | `go test -cover ./internal/vault/...` → PASS, lpass 91.1 %, providers 94.1 %; live PASS | best-effort CAS documented; ambiguous names refused |
| T5 vault.yaml + gate | done | 3d78151, dabe23c | `go test -cover ./internal/featflag/ ./cmd/` → PASS (featflag 96.9 %, cmd 77.0 %; only the 3 known cmd failures); `gff lint .github/gff/features.yaml` → 0 | fail-closed; ~ expanded; unknown YAML keys rejected; golden keys-sync argv captured |
| T6 keystore | done | cec71aa, 7065ab2 | `go test -cover ./internal/keystore/` → PASS 75.0 %; ssh-keygen fingerprint match | x/crypto v0.59.0; O_EXCL+counter backups, fsync'd, path returned on error |
| T7 agent + askpass | done | f10db28, 93f3f62 | `go test -race -count=3 -cover ./internal/agent/` → PASS 87.9 %; real ssh-add vs throwaway agent (OpenSSH_10.5p1) PASS | success = ssh-add exit 0 (keychain reloads); /usr/bin/ssh-add on darwin |
| T8 pre-flight | todo | | | |
| T9 vault setup/status | done | d81c2da, 4525bba, 1325557, 7279c29 | `go test ./cmd/` → PASS except the 3 known failures; `go vet ./...` clean | every verb wrapped by `gated()` (walk test); known_hosts lists unresolved aliases (no entry, lookup fail, unreadable file) — 3 fix rounds |
| T10 vault push | done | 65d3af6, 504062b | `go test -cover ./cmd/ ./internal/keystore/` → PASS except the 3 known failures, cmd 78.4 %, keystore 76.3 %; `go vet ./...` clean | captures the EXISTING key (encrypted: typed passphrase verified; plain: vault copy encrypted with a random passphrase); `--allow-partial` merges known_hosts; `--encrypt-local` verifies then deletes the plaintext backup |
| T11 vault pull | done (pull worker) | f50acdc | `go test -cover ./cmd/` → PASS except the 3 known failures, cmd 77.9 %; `go vet ./...` clean | review clean; restore refuses a different key without `--replace-key`; darwin keychain block only after y/N |
| T12 keys unlock | todo | | | |
| T13 runner probe + keys rotate | todo | | | |
| T14 roster-aware sync/prune | todo | | | |
| T15 TUI `V` | todo | | | |
| T16 docs + live gates | todo | | | |

### Vendor argv (plan §3) — confirmed/corrected by Task 0

| Line | Status | Correction |
| :-- | :-- | :-- |
| op ready / signin / get / create / update / list | unconfirmed | no 1Password account here. Open: `op item edit` accepting the full read-back item JSON on stdin; get/list with empty non-tty stdin; signed-out/ambiguous texts. Gate: `FLEET_VAULT_LIVE=op go test -tags live ./internal/vault/op/` |
| lpass ready / login / get / create / update / list | confirmed (live, v1.6.2) | login uses `--trust $LPASS_USER`; `show --notes` appends one `\n` (stripped in Get) |

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
| 2026-11-13 | build 1 | T1, T2, T4 (+T0 lpass live), T5, T6, T7 done, subagent-driven with per-task review. Rulings: lpass-first, op deferred; T0 as a Go `live` test. |
| 2026-11-13 | build 2 | T9 (3 fix rounds), T10 push (1 fix round) on `design`; T11 pull in parallel on new worker `pull`, rebased onto T10. |
