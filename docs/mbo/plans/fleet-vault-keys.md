# fleet vault-backed workstation keys Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking; the resumable cursor is [`fleet-vault-keys/TODO.md`](./fleet-vault-keys/TODO.md).

- **Slug:** fleet-vault-keys
- **Date:** 2026-11-13
- **Status:** Approved (design approved by the operator 2026-11-13)
- **Relates to:** spec [`../specs/fleet-vault-keys.md`](../specs/fleet-vault-keys.md) · design [`../designs/fleet-vault-keys.md`](../designs/fleet-vault-keys.md) · issue #451 · PR #452

**Goal:** Each workstation gets its own passphrase-protected SSH key, backed up in 1Password or LastPass along with the fleet's authorized-key roster, inventory and host keys, restorable and rotatable from `fleet` without ever locking a node out.

**Architecture:** A tiny `vault.Store` (versioned opaque blobs) with `op` and `lpass` adapters that shell out to the vendor CLIs; fleet owns the payload format (`fleet-vault/v1` JSON) and every decision, in pure packages with injected `Exec`s. New verbs `fleet vault setup|status|push|pull` and `fleet keys unlock|rotate`; `keys sync|prune` read the roster. All of it is dormant unless `vault.yaml` exists and `fleet.vault.enabled` is true.

**Tech Stack:** Go (module `github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet`), cobra, bubbletea (TUI), `golang.org/x/crypto/ssh`, OpenSSH (`ssh-keygen`, `ssh-add`), 1Password CLI v2 (`op`), LastPass CLI (`lpass`), gff.

**Spec:** [`../specs/fleet-vault-keys.md`](../specs/fleet-vault-keys.md) (+ design). Executors read both.

## Global Constraints

- Secrets (private key bytes, passphrase) never appear in argv, the environment, `fleet.log`, a run capture, or any remote command string. They move only on a child's **stdin** or over the one-shot askpass unix socket (Task 7).
- On disk, the only secret file is the **passphrase-encrypted** private key at the configured path, mode `0600`, `~/.ssh` `0700`.
- Keys are `ed25519`, generated **in-process** with `x/crypto/ssh` (Task 6), never with `ssh-keygen -N`, whose passphrase would be argv.
- Payload schema id: `fleet-vault/v1`. Item logical names: `key/<workstation>`, `roster`, `inventory`, `known_hosts`.
- Config file: `$XDG_CONFIG_HOME/fleet/vault.yaml` (default `~/.config/fleet/vault.yaml`); refused if group/world-writable or not owned by the current uid — the same rule as `readPlanFile` (`cmd/update_plan.go:225`).
- gff key: `fleet.vault.enabled`, bool, **default false, fail-closed** (only a successfully-read `true` enables).
- Fleet coverage floor: 60 % (`scripts/test.sh` `coverage_min`); every new package ≥ 60 %.
- OpenSSH ≥ 8.4 for `SSH_ASKPASS_REQUIRE=force`; older → `keys unlock` falls back to a plain interactive `ssh-add` and says so.
- Commits: unsigned is acceptable while the operator's signing key is expired (`git -c commit.gpgsign=false commit`); trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Never run `install.sh` from a worker worktree.

## Review Focus

1. **Session expires between `Get` and `Put`** (op/lpass sign-in times out mid-`push`): expect `ErrSignedOut` before any write, no partial item, and the verb names the vendor sign-in. → Task 3/4 `TestPutSignedOutWritesNothing`; Task 10 `TestPushStopsCleanlyWhenSignedOutMidway`.
2. **An empty or unparseable roster** must never mean "every key is foreign": prune refuses to run and says why. → Task 14 `TestPruneRefusesWhenTheRosterIsEmptyOrCorrupt`.
3. **No ssh-agent running** (plain WSL shell, `SSH_AUTH_SOCK` unset): pre-flight and `unlock` say how to start one (`eval "$(ssh-agent -s)"`), and nothing is marked `auth-failed`. → Task 8 `TestPreflightWithNoAgentNamesHowToStartOne`; Task 12 `TestUnlockWithNoAgent`.
4. **The workstation was renamed** (hostname changed): `status` says "no vault key for <new>; existing: <list>", and `pull --as <name>` restores under the old name. → Task 9 `TestStatusNamesTheExistingKeysWhenThisNameHasNone`; Task 11 `TestPullAsRestoresAnotherName`.
5. **A different local key already exists at the target path** on `pull`: refuse unless `--replace-key`, and back it up when replacing. → Task 11 `TestPullRefusesToReplaceADifferentKeyWithoutTheFlag`.

---

## 1. Summary & verdict

Builds the design approved on PR #452. One refinement of the design's §4.2 sketch: the vault
interface is a **versioned blob store**, not a field map. The `op --help` evidence (JSON templates
on stdin, `op read` with `?ssh-format=`) and `lpass`'s usage (notes on stdin, no field typing for
multiline values) show that the vendors model SSH keys very differently, and 1Password's "SSH Key"
category imposes its own key-format rules. A field model would leak those semantics into fleet.
With blobs, an adapter is ~5 small methods, fleet owns one JSON schema, and a third manager stays
one file. Compare-and-set uses `op`'s item `version`; LastPass has none, so its adapter compares a
content hash immediately before writing (best-effort, documented).

## 2. File inventory

| Path (under `sdk/fleet/` unless rooted) | Purpose | Implements |
| :-- | :-- | :-- |
| `internal/vault/store.go` | `Store`, `Blob`, `Secret`, `Config`, errors | F1 |
| `internal/vault/fake.go` | in-memory `Store` with versions + call log | F1 |
| `internal/vault/payload.go` | `fleet-vault/v1` codec: `KeyPair`, `Roster`, `Inventory`, `KnownHosts` | F9, F10, F14 |
| `internal/vault/roster.go` | roster state machine | F13, F14 |
| `internal/vault/op/op.go` | 1Password adapter | F2 |
| `internal/vault/lpass/lpass.go` | LastPass adapter | F3 |
| `internal/vault/providers/providers.go` | `Open(cfg, x) (vault.Store, error)` registry (keeps `vault` import-free) | F2, F3 |
| `internal/keystore/keystore.go` | generate / write / encrypted? / fingerprint | F5 |
| `internal/agent/agent.go`, `askpass.go` | loaded? / load via askpass / macOS keychain flag | F6 |
| `main.go` | askpass client mode before cobra | F6 |
| `internal/featflag/featflag.go` | `KeyVaultEnabled`, `Settings.VaultEnabled` (fail-closed) | F4 |
| `/.github/gff/features.yaml` | `fleet.vault.enabled` declaration | F4 |
| `cmd/vault_config.go` | `vault.yaml` load/save + gate | F4 |
| `cmd/preflight.go` | locked-key pre-flight | F12 |
| `cmd/vault.go`, `vault_push.go`, `vault_pull.go` | setup / status / push / pull | F7–F10 |
| `cmd/keys_unlock.go`, `keys_rotate.go` | unlock / rotate | F11, F13 |
| `cmd/keys.go` (modify) | roster-aware managed set, `prune --retire` | F14 |
| `internal/runner/runner.go` (modify) | `Exec.IdentitiesOnly`, `Exec.NoMux` | F13 |
| `cmd/tui_keys.go`, `tui_cmds.go`, `tui_model.go` (modify) | `V` menu + locked banner | F12, F15 |
| `AGENTS.md`, `README.md`, `/sdk/README.md` | invariants (amended private-key rule), verbs, runbook | F16 |
| `/docs/mbo/plans/fleet-vault-keys/evidence/**` | spike + per-task + live-gate captures | §7 |

## 3. Interface contracts (frozen at Task 1; changes go through TRACKING blockers)

```go
// package vault — no imports beyond the standard library
type Exec func(ctx context.Context, name string, stdin []byte, args ...string) (stdout []byte, err error)

type Secret []byte // String / GoString / Format (every verb) / MarshalJSON all yield "[redacted]"

type Blob struct {
    Data    Secret
    Version string // opaque; "" only for a blob never stored
}

type Store interface {
    Name() string                                                  // "op" | "lpass"
    Ready(ctx context.Context) error                               // nil | ErrNotInstalled | ErrSignedOut
    SignIn(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error // vendor owns the terminal
    Get(ctx context.Context, name string) (Blob, error)            // ErrNotFound when absent
    Put(ctx context.Context, name string, data Secret, version string) (newVersion string, err error)
    //   version "" = must not exist yet (create); else must equal the current one, ErrConflict otherwise
    List(ctx context.Context) ([]string, error)                    // logical names of this fleet's items
}
var ErrNotInstalled, ErrSignedOut, ErrNotFound, ErrConflict error

type Config struct {
    Provider    string // "op" | "lpass"
    Vault       string // op: vault name (default "Private"); lpass: folder (default "fleet")
    Workstation string // default: short hostname
    Key         string // default "~/.ssh/id_ed25519"
}

// payload.go — every blob is {"schema":"fleet-vault/v1","kind":K, ...}
type KeyPair struct{ Private, Passphrase Secret; Public, Fingerprint string; Created time.Time }
type KeyState string // "pending" | "active" | "retired"
type RosterEntry struct{ Workstation, Public, Fingerprint string; State KeyState; Changed time.Time; VerifiedOn []string }
type Roster struct{ Entries []RosterEntry }
type Inventory struct{ Config string }   // the #fleet Host blocks, ssh_config text
type KnownHosts struct{ Lines []string } // known_hosts lines for fleet aliases
func Encode(v any) (Secret, error)        // sets schema + kind; Secret fields base64
func Decode[T KeyPair | Roster | Inventory | KnownHosts](b Secret) (T, error) // wrong schema/kind → error

// roster.go
func (r Roster) Managed() []string                                     // Public of every pending|active entry
func (r *Roster) Add(ws, pub, fp string, now time.Time)                // new entry, State pending
func (r *Roster) Verified(fp, host string)                             // appends host to VerifiedOn (dedup)
func (r *Roster) Activate(fp string, now time.Time) (retired []RosterEntry) // fp → active; that workstation's previous active → retired
func (r Roster) RetirableOn(oldFP, host string) bool                    // old retired ∧ that workstation's active key verified on host

// package providers
func Open(cfg vault.Config, x vault.Exec) (vault.Store, error) // unknown provider → error listing "op", "lpass"

// package keystore
func Generate(passphrase []byte, comment string) (private, public []byte, err error) // x/crypto/ssh, OpenSSH format
func Write(path string, private, public []byte, now time.Time) (backup string, err error) // 0600/0644, dir 0700, existing → <path>.bak-<YYYYmmddTHHMMSS>
func Encrypted(path string) (bool, error)        // ssh.ParseRawPrivateKey → *ssh.PassphraseMissingError ⇒ true
func Fingerprint(public []byte) (string, error)  // "SHA256:…"

// package agent
type Exec func(ctx context.Context, name string, env []string, args ...string) ([]byte, error)
var ErrNoAgent error
func Loaded(ctx context.Context, x Exec, fp string) (bool, error) // ErrNoAgent when SSH_AUTH_SOCK unset or dead
func Load(ctx context.Context, x Exec, self, keyPath string, passphrase vault.Secret, goos string) error
//   serves the passphrase ONCE over <0700 tmpdir>/askpass.sock, runs ssh-add [--apple-use-keychain on darwin] keyPath
//   with env SSH_ASKPASS=self, SSH_ASKPASS_REQUIRE=force, FLEET_ASKPASS_SOCK=<sock>
func AskpassClient(sock string, out io.Writer) error // main.go: FLEET_ASKPASS_SOCK set → print passphrase, exit
// (an inherited fd cannot be used: ssh-add closes every fd above stderr at startup)

// package runner — additions; zero values keep today's argv byte for byte
type Exec struct{ /* existing… */ IdentitiesOnly bool; NoMux bool }
//   IdentitiesOnly → "-o IdentitiesOnly=yes"; NoMux → "-o ControlPath=none" in place of muxArgs()
```

**Item naming (adapter-private; fleet only sees logical names):**
- `op`: Secure Note titled `fleet:<logical>` in `Config.Vault`, tag `fleet`, payload in a concealed field `payload`. Version = item `version` from `op item get --format json`.
- `lpass`: Secure Note `<Config.Vault>/fleet:<logical>`, payload = the Notes body. Version = `hex(sha256(notes))`.

**Expected vendor argv** — Task 0 confirms or corrects each line; corrections are recorded in TRACKING, then encoded in the adapter tests:
- `op whoami` (ready) · `op signin` (tty) · `op item get <title> --vault <v> --format json --reveal` (get) · `op item create --vault <v> -` with a Secure Note JSON template on stdin (create) · `op item edit <id> --vault <v>` with the template on stdin; if edit cannot read stdin, a 0600 file in a 0700 temp dir via `--template=<file>`, removed after (update) · `op item list --vault <v> --tags fleet --format json` (list).
- `lpass status -q` (ready) · `lpass login <email>` (tty) · `lpass show --sync=now --notes <name>` (get) · `lpass add --sync=now --non-interactive --notes <name>` with the payload on stdin (create) · `lpass edit --sync=now --non-interactive --notes <name>` (update) · `lpass ls --sync=now <folder>` (list).

## 4. TDD build order

Every task: RED → RUN-RED → GREEN → RUN-GREEN → gate → commit → ledger. Each gate's output is
`tee`'d into `plans/fleet-vault-keys/evidence/T<n>/` with a dated header. Paths below are under
`sdk/fleet/`; run tests from there.

### Task 0: Vendor CLI spike (operator-assisted, no code)

**Files:** Create `docs/mbo/plans/fleet-vault-keys/evidence/spike/{op,lpass}.txt`.

- [ ] **Step 1:** Operator signs in (`op signin` / `lpass login <email>`). Against a throwaway item `fleet:spike`, run for each CLI: create → get (record version) → update → get (version changed) → list → delete, payload `{"schema":"fleet-vault/v1","kind":"spike"}` piped on stdin. Record argv, exit codes, where the version lives in the JSON, the exact signed-out error text, and whether `op item edit` reads a template from stdin.
- [ ] **Step 2:** On the WSL workstation record `op --version`, whether it is Linux `op` or `op.exe`, and how `op signin` persists the session (`OP_SESSION_*` eval vs desktop-app integration).
- [ ] **Step 3:** Mark every §3 vendor line confirmed/corrected in TRACKING; commit the transcripts. **Gate:** both transcripts committed, every line marked.

### Task 1: `vault` contract

**Files:** Create `internal/vault/{store,fake}.go`, `internal/vault/providers/providers.go`; Test `internal/vault/store_test.go`, `providers/providers_test.go`.
**Interfaces:** Produces §3 `package vault` (minus payload/roster) and `providers.Open`.

- [ ] **Step 1: Failing tests**
  - `TestSecretNeverPrints`: `s := Secret("PRIVATE-MATERIAL-123")`; each of `fmt.Sprint(s)`, `fmt.Sprintf("%v %+v %#v %s %q %x", s, s, s, s, s, s)`, `json.Marshal(struct{ S Secret }{s})`, `fmt.Sprintf("%+v", Blob{Data: s})` contains `[redacted]` and not `PRIVATE-MATERIAL`.
  - `TestFakeCompareAndSet`: `Put(n, d, "")` → `"1"`; `Put(n, d, "")` → `ErrConflict`; `Put(n, d, "1")` → `"2"`; `Put(n, d, "1")` → `ErrConflict`; `Get("missing")` → `ErrNotFound`.
  - `TestOpenRejectsAnUnknownProvider`: `Open(Config{Provider: "keepass"}, nil)` error contains `"op"` and `"lpass"`.
- [ ] **Step 2:** `go test ./internal/vault/...` → FAIL (undefined).
- [ ] **Step 3:** Implement per §3. `Fake.Calls` records `"<op> <name>"` only, never data. Until Tasks 3/4, `providers.Open` returns `ErrNotInstalled` for both names.
- [ ] **Step 4:** `go test ./internal/vault/...` → PASS.
- [ ] **Step 5:** Commit `fleet/vault: Store contract, redacting Secret, Fake (fleet-vault-keys T1)`.

### Task 2: payload codec + roster state machine

**Files:** Create `internal/vault/{payload,roster}.go`; Test `payload_test.go`, `roster_test.go`.

- [ ] **Step 1: Failing tests**
  - `TestPayloadRoundTripsEveryKind`: each of the four types survives `Encode`→`Decode` deep-equal; JSON contains `"schema":"fleet-vault/v1"` and the right `"kind"`.
  - `TestDecodeRejectsAWrongKindOrSchema`: `Decode[Roster]` of a KeyPair blob → error naming both kinds; schema `fleet-vault/v2` → error naming it.
  - `TestRosterLifecycle`: start with A(active); `Add(ws, pubB, fpB)` → `Managed()` = {A, B}; `Verified(fpB, "h1")`; `Activate(fpB)` returns [A]; `Managed()` = {B}; `RetirableOn(fpA, "h1")` true, `RetirableOn(fpA, "h2")` false.
  - `TestManagedNeverIncludesRetired`.
- [ ] **Step 2–4:** FAIL → implement → PASS.
- [ ] **Step 5:** Commit `fleet/vault: fleet-vault/v1 payloads and roster lifecycle (fleet-vault-keys T2)`.

### Task 3: 1Password adapter

**Files:** Create `internal/vault/op/op.go`, `op_test.go`, `testdata/*.json` (from the spike transcript, values replaced).
**Interfaces:** Produces `func New(cfg vault.Config, x vault.Exec) *Store`; register in `providers.Open`.

- [ ] **Step 1: Failing tests** (fake `Exec` keyed on argv, replaying spike-recorded output):
  - `TestReadyMapsTheVendorErrors`: exec "not found" → `ErrNotInstalled`; the recorded signed-out text → `ErrSignedOut`.
  - `TestGetReturnsPayloadAndVersion`, `TestGetMissingIsErrNotFound`.
  - `TestPutCreateSendsThePayloadOnStdinOnly`: the joined argv contains neither the payload nor its base64; stdin does.
  - `TestPutUpdateChecksTheVersionFirst`: current version `3`, `Put(…, "2")` → `ErrConflict`, no edit call recorded.
  - `TestPutSignedOutWritesNothing` (Review Focus 1).
- [ ] **Step 2–4:** FAIL → implement with the confirmed argv → PASS.
- [ ] **Step 5:** Commit `fleet/vault/op: 1Password adapter over the op CLI (fleet-vault-keys T3)`.

### Task 4: LastPass adapter

Same shape as Task 3 in `internal/vault/lpass/`. Version = `hex(sha256(notes))`; `Put` re-reads and compares immediately before `edit`. Extra test `TestPutDetectsAChangedNoteBeforeEditing`. Commit `fleet/vault/lpass: LastPass adapter over the lpass CLI (fleet-vault-keys T4)`.

### Task 5: `vault.yaml` + `fleet.vault.enabled`

**Files:** Modify `internal/featflag/featflag.go` (+test), `/.github/gff/features.yaml`; Create `cmd/vault_config.go` (+test).
**Interfaces:** `func loadVaultConfig(path string) (vault.Config, bool, error)` (bool = configured); `func saveVaultConfig(path string, c vault.Config) error` (0600); `func vaultActive(s featflag.Settings, configured bool) bool`.

- [ ] **Step 1: Failing tests**
  - `TestResolveVaultEnabledIsFailClosed`: source error / unknown key / typed-nil → false; explicit true → true.
  - `TestVaultConfigRefusesAGroupWritableFile` (0664 → error naming the mode), `TestVaultConfigMissingIsUnconfigured` (no error, false), `TestVaultConfigDefaults` (Workstation = short hostname, Key = `~/.ssh/id_ed25519`, op Vault `Private`, lpass Vault `fleet`).
  - `TestNoVaultMeansTodaysKeysSyncArgv` (UC8): with no config, `keys sync` against `runner.Fake` records argv equal to a golden captured from `main` before this task.
- [ ] **Step 2–4:** FAIL → implement → PASS. `features.yaml`: under `area: fleet`, `path: fleet.vault.enabled`, `boolDefault: false`, a description saying it is fail-closed and gates `fleet vault`, `keys unlock|rotate` and the roster-aware `sync`/`prune`.
- [ ] **Step 5:** Commit `fleet: vault.yaml + fleet.vault.enabled gate, fail-closed (fleet-vault-keys T5)`.

### Task 6: `keystore`

**Files:** Create `internal/keystore/keystore.go` (+test). `go.mod`: add `golang.org/x/crypto` if not already a direct dependency.

- [ ] **Step 1: Failing tests** `TestGenerateIsEncryptedWithThePassphrase` (`Encrypted` true; `ssh.ParseRawPrivateKeyWithPassphrase(priv, pass)` ok; a wrong passphrase fails), `TestWriteBacksUpAnExistingKeyAndSetsModes` (0600 / 0644 / dir 0700; the `.bak-<ts>` file holds the old bytes), `TestEncryptedFalseForAPlainKey`, `TestFingerprintMatchesSshKeygen` (compare with `ssh-keygen -lf` when the binary exists; skip otherwise).
- [ ] **Step 2–4.** **Step 5:** Commit `fleet/keystore: passphrase-protected ed25519 generate/write/inspect (fleet-vault-keys T6)`.

### Task 7: `agent` + askpass

**Files:** Create `internal/agent/{agent,askpass}.go` (+tests); Modify `main.go`.

- [ ] **Step 1: Failing tests**
  - `TestLoadedParsesSshAddList`: fingerprint present/absent; exit 1 "The agent has no identities" → false; exit 2 / unset socket → `ErrNoAgent`.
  - `TestLoadPassesThePassphraseOnlyOverTheSocket`: spy `Exec` records env + args; none contains the passphrase; env has `SSH_ASKPASS_REQUIRE=force` and `FLEET_ASKPASS_SOCK`; on `darwin` args include `--apple-use-keychain`, on `linux` they don't.
  - `TestAskpassServesOnceThenCloses` (a second client gets an error), `TestAskpassSocketDirIs0700`.
- [ ] **Step 2–4.** `main.go`: if `FLEET_ASKPASS_SOCK` is set, call `agent.AskpassClient(sock, os.Stdout)` and exit with 0/1, before `cmd.Execute()`.
- [ ] **Step 5:** Commit `fleet/agent: load a key via fleet-as-askpass, never argv/env (fleet-vault-keys T7)`.

### Task 8: locked-key pre-flight

**Files:** Create `cmd/preflight.go` (+test); Modify the entry points of `status`, `update`, `keys sync` and `tui` to call it before any dial.
**Interfaces:** `func keyLocked(keyPath, fp string, enc func(string) (bool, error), loaded func(string) (bool, error)) (locked bool, hint string)`; `const lockedHint = "key locked: run fleet keys unlock"`.

- [ ] **Step 1: Failing tests** `TestPreflightStopsBeforeAnyDial` (runner.Fake records zero calls; stderr has `lockedHint`; non-zero exit), `TestPreflightIgnoresAPlainKey`, `TestPreflightIgnoresALoadedKey`, `TestPreflightWithNoAgentNamesHowToStartOne` (Review Focus 3), `TestPreflightNeverMarksAHostAuthFailed`.
- [ ] **Step 2–4.** Applies with or without a vault (an encrypted key with an empty agent is the same fault either way); without a vault the hint is `ssh-add <key>`.
- [ ] **Step 5:** Commit `fleet: stop before dialling when the key is locked (fleet-vault-keys T8)`.

### Task 9: `fleet vault setup|status`

**Files:** Create `cmd/vault.go` (+test).

- [ ] **Step 1: Failing tests** `TestSetupWritesConfigAfterSignIn` (Fake store; config written 0600; provider choice read from stdin), `TestVaultVerbsRefuseWhenDisabled` (flag off → non-zero, message names `gff set fleet.vault.enabled true`, zero store calls), `TestStatusReportsEachFacet` (signed in · key local/vault/roster/agent · inventory drift count · known_hosts drift count), `TestStatusNamesTheExistingKeysWhenThisNameHasNone` (Review Focus 4).
- [ ] **Step 2–5.** Commit `fleet: vault setup + status (fleet-vault-keys T9)`.

### Task 10: `fleet vault push`

**Files:** Create `cmd/vault_push.go` (+test).

- [ ] **Step 1: Failing tests** `TestPushNothingChangedWritesNothing`, `TestPushShowsADiffAndWritesAfterConfirm` (four items; inventory built with `cfgplan` from local `#fleet` blocks; known_hosts lines for fleet aliases only), `TestPushConflictRereadsAndAsksAgain`, `TestPushStopsCleanlyWhenSignedOutMidway` (Review Focus 1), `TestPushNeverIncludesAnExecDirective` (a local block with `ProxyCommand` → absent from the inventory blob; the withheld line is printed).
- [ ] **Step 2–5.** Commit `fleet: vault push, one-way and diff-first (fleet-vault-keys T10)`.

### Task 11: `fleet vault pull`

**Files:** Create `cmd/vault_pull.go` (+test).

- [ ] **Step 1: Failing tests** `TestPullRestoresKeyConfigAndKnownHosts` (temp HOME; key 0600 and encrypted; `~/.ssh/config` merged, not replaced; backups exist), `TestPullRefusesToReplaceADifferentKeyWithoutTheFlag` (Review Focus 5), `TestPullAsRestoresAnotherName` (Review Focus 4), `TestPullWithNoKeyOffersRotateNew` (prints `fleet keys rotate --new` and "then run `fleet keys sync` on a trusted workstation"), `TestPullOnDarwinOffersUseKeychain` (adds a `Host *` block with `UseKeychain yes` + `AddKeysToAgent yes` only after confirmation; never on linux), `TestPullLoadsTheKeyIntoTheAgent` (agent spy receives the vault passphrase as a `Secret`).
- [ ] **Step 2–5.** Commit `fleet: vault pull — restore key, inventory, host keys (fleet-vault-keys T11)`.

### Task 12: `fleet keys unlock`

**Files:** Create `cmd/keys_unlock.go` (+test).

- [ ] **Step 1: Failing tests** `TestUnlockIsANoOpWhenLoaded`, `TestUnlockFetchesThePassphraseAndLoads`, `TestUnlockSignsInFirstWhenSignedOut` (`SignIn` once, then `Get`), `TestUnlockWithNoAgent` (Review Focus 3), `TestUnlockWithoutAVaultFallsBackToSshAdd` (runs a plain interactive `ssh-add <key>`).
- [ ] **Step 2–5.** Commit `fleet: keys unlock from the vault passphrase (fleet-vault-keys T12)`.

### Task 13: verification probe + `fleet keys rotate`

**Files:** Modify `internal/runner/runner.go` (+test); Create `cmd/keys_rotate.go` (+test).

- [ ] **Step 1: Failing tests**
  - runner: `TestIdentitiesOnlyAndNoMuxArgs` (`-o IdentitiesOnly=yes`, `-o ControlPath=none`, no `ControlMaster=auto`); `TestDefaultArgvUnchanged` (zero-value fields → today's argv).
  - rotate: `TestRotateOverlapsVerifiesAndActivates` (new key written to `<key>.new`, then swapped to `<key>`, the old one kept as `<key>.retired-<ts>`; roster: new pending → verified on every host → active, old retired), `TestRotateVerificationUsesOnlyTheNewKey` (every verify argv has `IdentitiesOnly=yes`, `-i <new>`, `ControlPath=none`), `TestRotateKeepsTheOldKeyOnAFailedHost` (one host fails → report names it with its cause, non-zero exit, `VerifiedOn` excludes it, the old key still in the roster as active until every host verified), `TestRotateNewForABrandNewWorkstation` (`--new`: no old key; stores KeyPair + roster pending; prints the "run `fleet keys sync` on a trusted workstation" step), `TestNoRemoteCommandCarriesAPrivateKey` (scan every `runner.Fake` call for `PRIVATE KEY` and the passphrase).
- [ ] **Step 2–4.** Activation rule: the new key becomes `active` only when it verified on every reachable fleet host; otherwise it stays `pending` (still authorized and managed), the report says which hosts are missing, and re-running `rotate --resume` re-verifies just those.
- [ ] **Step 5:** Commit `fleet: keys rotate — overlap, verify with the new key only, activate (fleet-vault-keys T13)`.

### Task 14: roster-aware `sync` / `prune` + `--retire`

**Files:** Modify `cmd/keys.go` (+`keys_test.go`).
**Interfaces:** `func managedSet(local map[string]string, r *vault.Roster) map[string]string` — roster pending|active ∪ local; a roster entry wins a name collision.

- [ ] **Step 1: Failing tests** `TestSyncAuthorizesTheRosterAndLocalKeys`, `TestPruneNeverProposesARosterKey` (UC7), `TestPruneRefusesWhenTheRosterIsEmptyOrCorrupt` (Review Focus 2), `TestRetireRemovesOnlyWhereTheReplacementVerified` (UC6), `TestKeysWithoutAVaultAreUnchanged` (golden).
- [ ] **Step 2–5.** Commit `fleet: keys sync/prune read the vault roster; prune --retire (fleet-vault-keys T14)`.

### Task 15: TUI `V`

**Files:** Modify `cmd/tui_keys.go` (`keyHelp` entry in the SSH & access group), `tui_cmds.go` (`vaultShell(verb string) tea.Cmd`, mirroring `configShell`), `tui_model.go` (menu state, locked banner, row tip `press V → unlock`).

- [ ] **Step 1: Failing tests** `TestVKeyOpensTheVaultMenu`, `TestVaultMenuRunsTheCLIVerb` (argv `[self, "vault", "pull"]`, `[self, "keys", "unlock"]`, …), `TestVRefusedWhileAnUpdateOwnsTheRow`, `TestLockedBannerNamesV`; the existing `TestEveryFleetKeyIsReservedAgainstProviders`, `TestHelpDescriptionsFitTheirColumn` and `TestViewFitsAcrossEveryTerminalSize` stay green.
- [ ] **Step 2–5.** Commit `fleet/tui: V vault menu delegating to the CLI verbs (fleet-vault-keys T15)`.

### Task 16: docs, invariants, live gates

**Files:** Modify `AGENTS.md` (Commands + Layout rows; invariants: the amended private-key rule, `Secret` redaction, the fail-closed gate, the pre-flight, overlap rotation), `README.md` ("Vault-backed keys" section + new-workstation runbook), `/sdk/README.md` fleet blurb, `/docs/mbo/index.md` state.

- [ ] **Step 1:** Docs written; every demo is **real output** captured during G1–G6 (sdk rule).
- [ ] **Step 2:** Live gates G1–G6 (spec §6) with the operator; transcripts in `evidence/e2e/`.
- [ ] **Step 3:** `./scripts/test.sh` (fleet ≥ 60 %), `make lint-go` → green; captured.
- [ ] **Step 4:** Commit `docs(fleet): vault-backed keys — verbs, invariants, runbook (fleet-vault-keys T16)`.

## 5. Verification mapping

| Spec rule (§5) | Test |
| :-- | :-- |
| F1 redaction | `TestSecretNeverPrints` |
| F2/F3 stdin-only, CAS, signed-out, not-installed | `TestPutCreateSendsThePayloadOnStdinOnly`, `TestPutUpdateChecksTheVersionFirst`, `TestPutDetectsAChangedNoteBeforeEditing`, `TestReadyMapsTheVendorErrors`, `TestPutSignedOutWritesNothing` |
| F4 fallback, mode, gate | `TestNoVaultMeansTodaysKeysSyncArgv`, `TestVaultConfigRefusesAGroupWritableFile`, `TestResolveVaultEnabledIsFailClosed` |
| F5 generate/write | `TestGenerateIsEncryptedWithThePassphrase`, `TestWriteBacksUpAnExistingKeyAndSetsModes` |
| F6 agent | `TestLoadPassesThePassphraseOnlyOverTheSocket`, `TestAskpassServesOnceThenCloses` |
| F7/F8 setup/status | `TestSetupWritesConfigAfterSignIn`, `TestStatusReportsEachFacet` |
| F9 push | `TestPushNothingChangedWritesNothing`, `TestPushConflictRereadsAndAsksAgain`, `TestPushNeverIncludesAnExecDirective` |
| F10 pull | `TestPullRestoresKeyConfigAndKnownHosts`, `TestPullRefusesToReplaceADifferentKeyWithoutTheFlag` |
| F11 unlock | `TestUnlockFetchesThePassphraseAndLoads` |
| F12 pre-flight | `TestPreflightStopsBeforeAnyDial`, `TestPreflightNeverMarksAHostAuthFailed` |
| F13 rotate | `TestRotateVerificationUsesOnlyTheNewKey`, `TestRotateKeepsTheOldKeyOnAFailedHost` |
| F14 sync/prune | `TestPruneNeverProposesARosterKey`, `TestRetireRemovesOnlyWhereTheReplacementVerified` |
| F15 TUI | `TestVaultMenuRunsTheCLIVerb`, `TestVRefusedWhileAnUpdateOwnsTheRow` |
| F16 docs | live gates G1–G6 transcripts |
| all: no private key to a node | `TestNoRemoteCommandCarriesAPrivateKey` |

## 6. Integration & rollout

- No `install.sh` change: fleet is already built and installed by it, and the feature ships
  dormant (`fleet.vault.enabled` false). The operator enables it per workstation with
  `gff set fleet.vault.enabled true` + `fleet vault setup`.
- `scripts/test.sh` discovers new packages by directory; the 60 % floor applies.
- Rollout order (operator): G1 push from the WSL workstation → G3 pull onto the Mac → G4 unlock
  after a Linux reboot → G5 rotate the Mac's key with one host down → G6 retire.

### 6.1 Build leaves / DAG

Default is **one PR, built sequentially** (Tasks 0–16): Tasks 8–15 all edit `cmd/` and would be
a false split. If parallelized, the only clean cut is below Task 8:

| Leaf | Owns (paths) | Consumes (← edge) | `done-when` gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| iface (T1–T2) | `internal/vault/{store,fake,payload,roster}.go`, `internal/vault/providers/` | — | `go test ./internal/vault/...` ≥ 60 % | yes (base) |
| adapters (T0, T3–T4) | `internal/vault/{op,lpass}/**`, `evidence/spike/` | iface | adapter tests green on spike-recorded transcripts | no |
| local (T5–T7) | `internal/{keystore,agent,featflag}/**`, `cmd/vault_config.go`, `main.go`, `features.yaml` | iface | `go test ./internal/{keystore,agent,featflag}` ≥ 60 % | no |
| verbs (T8–T16) | the rest of `cmd/`, `internal/runner`, docs | adapters, local | full suite + lint + G1–G6 | — |

## 7. Validation & evidence

- **Coverage:** every new package ≥ 60 %; `cmd` must not drop below its current figure.
- **Adversarial cases:** the Review Focus tests, `TestNoRemoteCommandCarriesAPrivateKey`, and the
  argv/env secret scans in Tasks 3, 4 and 7.
- **Evidence tree:** `plans/fleet-vault-keys/evidence/{spike,T1…T16,e2e,demo}/`, each file with a
  dated header, append-only, committed with its task. A feature without captured evidence is not done.
- **Live gates** G1–G6 (spec §6) are operator-run; their transcripts become the README demos.

> Produced via `superpowers:writing-plans`. Execute with `superpowers:subagent-driven-development`
> or `superpowers:executing-plans`, TDD throughout. Update `../index.md` state as it moves.
