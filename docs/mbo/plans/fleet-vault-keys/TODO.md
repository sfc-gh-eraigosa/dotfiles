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

All `go test` commands run from `sdk/fleet/` in the worker worktree.

## Preflight (once)

- [ ] `go version` matches `go.mod`
- [ ] `ssh -V` ≥ 8.4 (record the version in TRACKING)
- [ ] `command -v op lpass` → both present
- [ ] `gss feature list --feature fleet-vault-keys --json` → worker `design`, PR #452
- [ ] Worker rebased on current `origin/main` (`git -c commit.gpgsign=false rebase origin/main`)
- [ ] `./scripts/test.sh` baseline captured to `evidence/baseline.txt` (pre-existing failures noted)

---

### Task 0 — vendor CLI spike  (plan Task 0)

- [ ] SETUP: ask the operator to sign in (`op signin`, `lpass login <email>`)
- [ ] VERIFY: op create→get→update→get→list→delete on `fleet:spike`; tee to `evidence/spike/op.txt` (payload/values redacted)
- [ ] VERIFY: same for lpass → `evidence/spike/lpass.txt`
- [ ] VERIFY: WSL notes (`op --version`, Linux vs `op.exe`, session persistence)
- [ ] LEDGER: mark every vendor-argv line confirmed/corrected in TRACKING §1
- [ ] COMMIT: `docs(fleet-vault-keys): vendor CLI spike transcripts (T0)`

**Done when:** both transcripts committed; every §3 vendor line marked.

### Task 1 — vault contract  (plan Task 1)

- [ ] RED: `TestSecretNeverPrints`, `TestFakeCompareAndSet`, `TestOpenRejectsAnUnknownProvider`
- [ ] RUN-RED: `go test ./internal/vault/...` → expect **FAIL** (undefined)
- [ ] GREEN: `internal/vault/{store,fake}.go`, `internal/vault/providers/providers.go`
- [ ] RUN-GREEN: `go test ./internal/vault/...` → expect **PASS**
- [ ] VERIFY: `go test -cover ./internal/vault/...` ≥ 60 %; ALLOWLIST new paths
- [ ] COMMIT: `fleet/vault: Store contract, redacting Secret, Fake (fleet-vault-keys T1)`
- [ ] LEDGER

### Task 2 — payload codec + roster  (plan Task 2)

- [ ] RED: `TestPayloadRoundTripsEveryKind`, `TestDecodeRejectsAWrongKindOrSchema`, `TestRosterLifecycle`, `TestManagedNeverIncludesRetired`
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `internal/vault/{payload,roster}.go`
- [ ] RUN-GREEN → **PASS**; coverage ≥ 60 %
- [ ] COMMIT: `fleet/vault: fleet-vault/v1 payloads and roster lifecycle (fleet-vault-keys T2)`
- [ ] LEDGER + CHECKPOINT (confirm first)

### Task 3 — 1Password adapter  (plan Task 3)

- [ ] SETUP: `testdata/*.json` from the spike transcript, values replaced
- [ ] RED: `TestReadyMapsTheVendorErrors`, `TestGetReturnsPayloadAndVersion`, `TestGetMissingIsErrNotFound`, `TestPutCreateSendsThePayloadOnStdinOnly`, `TestPutUpdateChecksTheVersionFirst`, `TestPutSignedOutWritesNothing`
- [ ] RUN-RED: `go test ./internal/vault/op/...` → **FAIL**
- [ ] GREEN: `internal/vault/op/op.go`; register in `providers.Open`
- [ ] RUN-GREEN → **PASS**; coverage ≥ 60 %
- [ ] COMMIT: `fleet/vault/op: 1Password adapter over the op CLI (fleet-vault-keys T3)`
- [ ] LEDGER

### Task 4 — LastPass adapter  (plan Task 4)

- [ ] RED: Task 3's set for lpass + `TestPutDetectsAChangedNoteBeforeEditing`
- [ ] RUN-RED: `go test ./internal/vault/lpass/...` → **FAIL**
- [ ] GREEN: `internal/vault/lpass/lpass.go`; register in `providers.Open`
- [ ] RUN-GREEN → **PASS**; coverage ≥ 60 %
- [ ] COMMIT: `fleet/vault/lpass: LastPass adapter over the lpass CLI (fleet-vault-keys T4)`
- [ ] LEDGER + CHECKPOINT (confirm first)

### Task 5 — vault.yaml + fleet.vault.enabled  (plan Task 5)

- [ ] SETUP: capture today's `keys sync` argv golden from `main` before editing
- [ ] RED: `TestResolveVaultEnabledIsFailClosed`, `TestVaultConfigRefusesAGroupWritableFile`, `TestVaultConfigMissingIsUnconfigured`, `TestVaultConfigDefaults`, `TestNoVaultMeansTodaysKeysSyncArgv`
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `featflag.go`, `cmd/vault_config.go`, `/.github/gff/features.yaml`
- [ ] RUN-GREEN → **PASS**
- [ ] VERIFY: `gff` validates `features.yaml` (repo's gff check)
- [ ] COMMIT: `fleet: vault.yaml + fleet.vault.enabled gate, fail-closed (fleet-vault-keys T5)`
- [ ] LEDGER

### Task 6 — keystore  (plan Task 6)

- [ ] RED: `TestGenerateIsEncryptedWithThePassphrase`, `TestWriteBacksUpAnExistingKeyAndSetsModes`, `TestEncryptedFalseForAPlainKey`, `TestFingerprintMatchesSshKeygen`
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `internal/keystore/keystore.go` (+ `golang.org/x/crypto` if needed; `go mod tidy`)
- [ ] RUN-GREEN → **PASS**; coverage ≥ 60 %
- [ ] COMMIT: `fleet/keystore: passphrase-protected ed25519 generate/write/inspect (fleet-vault-keys T6)`
- [ ] LEDGER

### Task 7 — agent + askpass  (plan Task 7)

- [ ] RED: `TestLoadedParsesSshAddList`, `TestLoadPassesThePassphraseOnlyOverTheSocket`, `TestAskpassServesOnceThenCloses`, `TestAskpassSocketDirIs0700`
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `internal/agent/{agent,askpass}.go`; `main.go` askpass client mode
- [ ] RUN-GREEN → **PASS**; coverage ≥ 60 %
- [ ] COMMIT: `fleet/agent: load a key via fleet-as-askpass, never argv/env (fleet-vault-keys T7)`
- [ ] LEDGER + CHECKPOINT (confirm first)

### Task 8 — locked-key pre-flight  (plan Task 8)

- [ ] RED: `TestPreflightStopsBeforeAnyDial`, `TestPreflightIgnoresAPlainKey`, `TestPreflightIgnoresALoadedKey`, `TestPreflightWithNoAgentNamesHowToStartOne`, `TestPreflightNeverMarksAHostAuthFailed`
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `cmd/preflight.go`; wire into `status`, `update`, `keys sync`, `tui`
- [ ] RUN-GREEN → **PASS**; full `go test ./cmd/...` still green
- [ ] COMMIT: `fleet: stop before dialling when the key is locked (fleet-vault-keys T8)`
- [ ] LEDGER

### Task 9 — vault setup|status  (plan Task 9)

- [ ] RED: `TestSetupWritesConfigAfterSignIn`, `TestVaultVerbsRefuseWhenDisabled`, `TestStatusReportsEachFacet`, `TestStatusNamesTheExistingKeysWhenThisNameHasNone`
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `cmd/vault.go`
- [ ] RUN-GREEN → **PASS**
- [ ] COMMIT: `fleet: vault setup + status (fleet-vault-keys T9)`
- [ ] LEDGER

### Task 10 — vault push  (plan Task 10)

- [ ] RED: `TestPushNothingChangedWritesNothing`, `TestPushShowsADiffAndWritesAfterConfirm`, `TestPushConflictRereadsAndAsksAgain`, `TestPushStopsCleanlyWhenSignedOutMidway`, `TestPushNeverIncludesAnExecDirective`
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `cmd/vault_push.go`
- [ ] RUN-GREEN → **PASS**
- [ ] COMMIT: `fleet: vault push, one-way and diff-first (fleet-vault-keys T10)`
- [ ] LEDGER

### Task 11 — vault pull  (plan Task 11)

- [ ] RED: `TestPullRestoresKeyConfigAndKnownHosts`, `TestPullRefusesToReplaceADifferentKeyWithoutTheFlag`, `TestPullAsRestoresAnotherName`, `TestPullWithNoKeyOffersRotateNew`, `TestPullOnDarwinOffersUseKeychain`, `TestPullLoadsTheKeyIntoTheAgent`
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `cmd/vault_pull.go`
- [ ] RUN-GREEN → **PASS**
- [ ] COMMIT: `fleet: vault pull — restore key, inventory, host keys (fleet-vault-keys T11)`
- [ ] LEDGER + CHECKPOINT (confirm first)

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
