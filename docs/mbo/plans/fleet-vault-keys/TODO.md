# fleet vault-backed workstation keys — execution cursor

- **Slug:** fleet-vault-keys
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../fleet-vault-keys.md`](../fleet-vault-keys.md) — every task/§ reference points there

> **How to use:** the **first unchecked box is the next action**. Tick a box only after
> you ran the command and read the output. After finishing a `###` task: update
> `TRACKING.md`, commit with the plan's exact message, checkpoint.
>
> **Legend:** `SETUP` prep · `RED` write a failing test · `RUN-RED` run it, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run it, expect PASS · `VERIFY` extra gate ·
> `ALLOWLIST` `.gitignore` check · `DOCS` · `COMMIT` · `LEDGER` update TRACKING.md ·
> `CHECKPOINT` push/PR refresh.

> **Order (operator priority, 2026-11-13):** capture-from-one-node first — T3 → T9 → T10 → T11 → T8 → T12 → T13 → T14 → T15 → T16. **Four stacked PRs:** PR1 #452 = T1, T2, T4–T7, T9, T10 (save/push); PR2 `pull` = T11; PR3 `lifecycle` = T8, T12–T16; PR4 `op` = T3 (1Password, saved branch 953dca5) once an account exists (see IMPLEMENTATION §2). T10 also captures an EXISTING key: encrypted → prompt + verify; unencrypted → generated passphrase encrypts the vault copy, local file untouched unless `--encrypt-local`; captured key enters the roster `active`.

All `go test` commands run from `sdk/fleet/` in the worker worktree.

## Preflight (once)

- [x] `go version` matches `go.mod`
- [x] `ssh -V` ≥ 8.4 (record the version in TRACKING)
- [x] `command -v op lpass` → both present
- [x] `gss feature list --feature fleet-vault-keys --json` → worker `design`, PR #452
- [x] Worker rebased on current `origin/main` (`git -c commit.gpgsign=false rebase origin/main`)
- [x] `./scripts/test.sh` baseline captured to `evidence/baseline.txt` (pre-existing failures noted)

---

### Task 0 — vendor CLI spike  (plan Task 0; operator ruling 2026-11-13: Go live test, not a bash transcript; lpass only, op deferred)

- [x] SETUP: operator runs `lpass login --trust <email>`; `lpass status` → logged in
- [x] RED: `internal/vault/lpass/live_test.go` (`//go:build live`, skips unless `FLEET_VAULT_LIVE=lpass`): `TestLiveRoundTrip` — Put create → Get (version) → Put update with that version → Get (version changed) → Put with the stale version → `ErrConflict` → List contains it → delete in `t.Cleanup`
- [x] RUN (after Task 4): `FLEET_VAULT_LIVE=lpass go test -tags live -run TestLiveRoundTrip -v ./internal/vault/lpass/` → PASS; output (no payload values) teed to `evidence/spike/lpass.txt`
- [x] LEDGER: mark each lpass argv line confirmed/corrected in TRACKING §1; op lines stay unconfirmed
- [x] COMMIT: folded into Task 4's commit

**Done when:** the live round-trip passes against the real lpass and every lpass argv line is marked.

### Task 1 — vault contract  (plan Task 1)

- [x] RED: `TestSecretNeverPrints`, `TestFakeCompareAndSet`, `TestOpenRejectsAnUnknownProvider`
- [x] RUN-RED: `go test ./internal/vault/...` → expect **FAIL** (undefined)
- [x] GREEN: `internal/vault/{store,fake}.go`, `internal/vault/providers/providers.go`
- [x] RUN-GREEN: `go test ./internal/vault/...` → expect **PASS**
- [x] VERIFY: `go test -cover ./internal/vault/...` ≥ 60 %; ALLOWLIST new paths
- [x] COMMIT: `fleet/vault: Store contract, redacting Secret, Fake (fleet-vault-keys T1)`
- [x] LEDGER

### Task 2 — payload codec + roster  (plan Task 2)

- [x] RED: `TestPayloadRoundTripsEveryKind`, `TestDecodeRejectsAWrongKindOrSchema`, `TestRosterLifecycle`, `TestManagedNeverIncludesRetired`
- [x] RUN-RED → **FAIL**
- [x] GREEN: `internal/vault/{payload,roster}.go`
- [x] RUN-GREEN → **PASS**; coverage ≥ 60 %
- [x] COMMIT: `fleet/vault: fleet-vault/v1 payloads and roster lifecycle (fleet-vault-keys T2)`
- [x] LEDGER + CHECKPOINT (confirm first)

### Task 3 — 1Password adapter  (plan Task 3)

- [x] SETUP: `testdata/*.json` from the spike transcript, values replaced
- [x] RED: `TestReadyMapsTheVendorErrors`, `TestGetReturnsPayloadAndVersion`, `TestGetMissingIsErrNotFound`, `TestPutCreateSendsThePayloadOnStdinOnly`, `TestPutUpdateChecksTheVersionFirst`, `TestPutSignedOutWritesNothing`
- [x] RUN-RED: `go test ./internal/vault/op/...` → **FAIL**
- [x] GREEN: `internal/vault/op/op.go`; register in `providers.Open`
- [x] RUN-GREEN → **PASS**; coverage ≥ 60 %
- [x] COMMIT: `fleet/vault/op: 1Password adapter over the op CLI (fleet-vault-keys T3)`
- [x] LEDGER

### Task 4 — LastPass adapter  (plan Task 4)

- [x] RED: Task 3's set for lpass + `TestPutDetectsAChangedNoteBeforeEditing`
- [x] RUN-RED: `go test ./internal/vault/lpass/...` → **FAIL**
- [x] GREEN: `internal/vault/lpass/lpass.go`; register in `providers.Open`
- [x] RUN-GREEN → **PASS**; coverage ≥ 60 %
- [x] COMMIT: `fleet/vault/lpass: LastPass adapter over the lpass CLI (fleet-vault-keys T4)`
- [x] LEDGER + CHECKPOINT (confirm first)

### Task 5 — vault.yaml + fleet.vault.enabled  (plan Task 5)

- [x] SETUP: capture today's `keys sync` argv golden from `main` before editing
- [x] RED: `TestResolveVaultEnabledIsFailClosed`, `TestVaultConfigRefusesAGroupWritableFile`, `TestVaultConfigMissingIsUnconfigured`, `TestVaultConfigDefaults`, `TestNoVaultMeansTodaysKeysSyncArgv`
- [x] RUN-RED → **FAIL**
- [x] GREEN: `featflag.go`, `cmd/vault_config.go`, `/.github/gff/features.yaml`
- [x] RUN-GREEN → **PASS**
- [x] VERIFY: `gff` validates `features.yaml` (repo's gff check)
- [x] COMMIT: `fleet: vault.yaml + fleet.vault.enabled gate, fail-closed (fleet-vault-keys T5)`
- [x] LEDGER

### Task 6 — keystore  (plan Task 6)

- [x] RED: `TestGenerateIsEncryptedWithThePassphrase`, `TestWriteBacksUpAnExistingKeyAndSetsModes`, `TestEncryptedFalseForAPlainKey`, `TestFingerprintMatchesSshKeygen`
- [x] RUN-RED → **FAIL**
- [x] GREEN: `internal/keystore/keystore.go` (+ `golang.org/x/crypto` if needed; `go mod tidy`)
- [x] RUN-GREEN → **PASS**; coverage ≥ 60 %
- [x] COMMIT: `fleet/keystore: passphrase-protected ed25519 generate/write/inspect (fleet-vault-keys T6)`
- [x] LEDGER

### Task 7 — agent + askpass  (plan Task 7)

- [x] RED: `TestLoadedParsesSshAddList`, `TestLoadPassesThePassphraseOnlyOverTheSocket`, `TestAskpassServesOnceThenCloses`, `TestAskpassSocketDirIs0700`
- [x] RUN-RED → **FAIL**
- [x] GREEN: `internal/agent/{agent,askpass}.go`; `main.go` askpass client mode
- [x] RUN-GREEN → **PASS**; coverage ≥ 60 %
- [x] COMMIT: `fleet/agent: load a key via fleet-as-askpass, never argv/env (fleet-vault-keys T7)`
- [x] LEDGER + CHECKPOINT (confirm first)

### Task 8 — locked-key pre-flight  (plan Task 8)

- [x] RED: `TestPreflightStopsBeforeAnyDial`, `TestPreflightIgnoresAPlainKey`, `TestPreflightIgnoresALoadedKey`, `TestPreflightWithNoAgentNamesHowToStartOne`, `TestPreflightNeverMarksAHostAuthFailed`
- [x] RUN-RED → **FAIL**
- [x] GREEN: `cmd/preflight.go`; wire into `status`, `update`, `keys sync`, `tui`
- [x] RUN-GREEN → **PASS**; full `go test ./cmd/...` still green
- [x] COMMIT: `fleet: stop before dialling when the key is locked (fleet-vault-keys T8)`
- [ ] FIX: stub `keyLocked` in the existing status/update/keys-sync/tui tests so none reach the real ~/.ssh or agent (test-isolation rule); full `go test ./...` = only the 3 known failures
- [ ] REVIEW: task review of 81b7b23 + the fix (spec + quality); fix loop until clean
- [ ] LEDGER

### Task 9 — vault setup|status  (plan Task 9)

- [x] RED: `TestSetupWritesConfigAfterSignIn`, `TestVaultVerbsRefuseWhenDisabled`, `TestStatusReportsEachFacet`, `TestStatusNamesTheExistingKeysWhenThisNameHasNone`
- [x] RUN-RED → **FAIL**
- [x] GREEN: `cmd/vault.go`
- [x] RUN-GREEN → **PASS**
- [x] COMMIT: `fleet: vault setup + status (fleet-vault-keys T9)`
- [x] LEDGER

### Task 10 — vault push  (plan Task 10)

- [x] RED: `TestPushNothingChangedWritesNothing`, `TestPushShowsADiffAndWritesAfterConfirm`, `TestPushConflictRereadsAndAsksAgain`, `TestPushStopsCleanlyWhenSignedOutMidway`, `TestPushNeverIncludesAnExecDirective`
- [x] RUN-RED → **FAIL**
- [x] GREEN: `cmd/vault_push.go`
- [x] RUN-GREEN → **PASS**
- [x] COMMIT: `fleet: vault push, one-way and diff-first (fleet-vault-keys T10)`
- [x] LEDGER

### Task 11 — vault pull  (plan Task 11)

- [x] RED: `TestPullRestoresKeyConfigAndKnownHosts`, `TestPullRefusesToReplaceADifferentKeyWithoutTheFlag`, `TestPullAsRestoresAnotherName`, `TestPullWithNoKeyOffersRotateNew`, `TestPullOnDarwinOffersUseKeychain`, `TestPullLoadsTheKeyIntoTheAgent`
- [x] RUN-RED → **FAIL**
- [x] GREEN: `cmd/vault_pull.go`
- [x] RUN-GREEN → **PASS**
- [x] COMMIT: `fleet: vault pull — restore key, inventory, host keys (fleet-vault-keys T11)`
- [x] LEDGER + CHECKPOINT (confirm first)

### Task 12 — keys unlock  (plan Task 12)

- [ ] RED: `TestUnlockIsANoOpWhenLoaded`, `TestUnlockFetchesThePassphraseAndLoads`, `TestUnlockSignsInFirstWhenSignedOut`, `TestUnlockWithNoAgent`, `TestUnlockWithoutAVaultFallsBackToSshAdd`
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `cmd/keys_unlock.go`
- [ ] RUN-GREEN → **PASS**
- [ ] COMMIT: `fleet: keys unlock from the vault passphrase (fleet-vault-keys T12)`
- [ ] LEDGER

### Task 13 — runner probe + keys rotate  (plan Task 13)

- [ ] RED: `TestIdentitiesOnlyAndNoMuxArgs`, `TestDefaultArgvUnchanged`
- [ ] RUN-RED: `go test ./internal/runner/...` → **FAIL**
- [ ] GREEN: `runner.Exec.IdentitiesOnly`, `NoMux`
- [ ] RUN-GREEN → **PASS**
- [ ] RED: `TestRotateOverlapsVerifiesAndActivates`, `TestRotateVerificationUsesOnlyTheNewKey`, `TestRotateKeepsTheOldKeyOnAFailedHost`, `TestRotateNewForABrandNewWorkstation`, `TestNoRemoteCommandCarriesAPrivateKey`
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `cmd/keys_rotate.go` (incl. `--resume`, `--new`)
- [ ] RUN-GREEN → **PASS**
- [ ] COMMIT: `fleet: keys rotate — overlap, verify with the new key only, activate (fleet-vault-keys T13)`
- [ ] LEDGER

### Task 14 — roster-aware sync/prune  (plan Task 14)

- [ ] RED: `TestSyncAuthorizesTheRosterAndLocalKeys`, `TestPruneNeverProposesARosterKey`, `TestPruneRefusesWhenTheRosterIsEmptyOrCorrupt`, `TestRetireRemovesOnlyWhereTheReplacementVerified`, `TestKeysWithoutAVaultAreUnchanged`
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `cmd/keys.go` (`managedSet`, `prune --retire`)
- [ ] RUN-GREEN → **PASS**
- [ ] COMMIT: `fleet: keys sync/prune read the vault roster; prune --retire (fleet-vault-keys T14)`
- [ ] LEDGER

### Task 15 — TUI `V`  (plan Task 15)

- [ ] RED: `TestVKeyOpensTheVaultMenu`, `TestVaultMenuRunsTheCLIVerb`, `TestVRefusedWhileAnUpdateOwnsTheRow`, `TestLockedBannerNamesV`
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `tui_keys.go`, `tui_cmds.go`, `tui_model.go`
- [ ] RUN-GREEN → **PASS**; existing keymap/help/view-size tests green
- [ ] COMMIT: `fleet/tui: V vault menu delegating to the CLI verbs (fleet-vault-keys T15)`
- [ ] LEDGER + CHECKPOINT (confirm first)

### Task 16 — docs, invariants, live gates  (plan Task 16)

- [ ] VERIFY: G1 push from WSL (op) → `evidence/e2e/G1.txt` (operator)
- [ ] VERIFY: G2 lpass round-trip → `evidence/e2e/G2.txt` (operator)
- [ ] VERIFY: G3 pull onto the Mac → `evidence/e2e/G3.txt` (operator)
- [ ] VERIFY: G4 unlock after a Linux reboot → `evidence/e2e/G4.txt` (operator)
- [ ] VERIFY: G5 rotate with one host down → `evidence/e2e/G5.txt` (operator)
- [ ] VERIFY: G6 retire → `evidence/e2e/G6.txt` (operator)
- [ ] DOCS: `sdk/fleet/AGENTS.md`, `sdk/fleet/README.md`, `sdk/README.md` — demos from the transcripts
- [ ] VERIFY: `./scripts/test.sh` and `make lint-go` green; captured
- [ ] DOCS: `docs/mbo/index.md` state → `in-review`
- [ ] COMMIT: `docs(fleet): vault-backed keys — verbs, invariants, runbook (fleet-vault-keys T16)`
- [ ] LEDGER + CHECKPOINT (confirm first)

**Done when:** TRACKING §3 fully ticked.
