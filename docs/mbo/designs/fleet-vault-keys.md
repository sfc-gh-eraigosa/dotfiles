# fleet vault-backed workstation keys — design

- **Slug:** fleet-vault-keys
- **Date:** 2026-11-13
- **Status:** Draft
- **Relates to:** issue #451 / PR #452 · builds on [`fleet`](./fleet.md), [`fleet-config`](./fleet-config.md); follows PR #450 (scan host-key fix)
- **Author(s):** operator + Claude

## 1. Problem / context

Verified on a fresh Mac on 2026-11-13:

- **A new workstation has nothing to stand on.** With an empty `~/.ssh/config`, no
  `known_hosts`, and a freshly generated `id_ed25519` (created by the OS setup, not by the
  fleet), every fleet node refuses this machine. Joining the fleet took a manual chain:
  `ssh-copy-id` from an authorized host → `fleet config push` → copy this machine's public key
  over → `fleet keys sync`. The operator's reaction: *"the key sync and using this mac's pub key
  didn't seem right"* — the expectation was to **restore** a trusted identity, not mint a new one.
- **There is no backup or restore story.** Keys, the `#fleet` inventory and `known_hosts` live
  only on the machines that happen to hold them. Losing a workstation loses its identity; nothing
  records which keys *should* be authorized.
- **The managed key set is machine-local.** `managedKeys()` (`sdk/fleet/cmd/keys.go`) is "every
  `~/.ssh/*.pub` on the machine running the command", and `fleet keys prune` treats every other
  line in a host's `authorized_keys` as foreign. A prune run from a workstation that doesn't hold
  the other workstations' `.pub` files offers to revoke them. The set is only consistent if the
  operator keeps those files in sync by hand.
- **The shell tool this replaced scp'd the private key to every host** (`syncKeyToHost`'s
  comment), which is why fleet's invariant is *"No private key ever leaves the workstation."*
  Any vault design must keep the property that invariant protects: a private key never lands on
  a fleet **node**.

## 2. Goals & non-goals

**Goals**
1. **One key per workstation** (the machines you ssh *from*), each backed up in a password
   manager and restorable from the CLI in one command.
2. **The vault is the shared source of truth** for four things: each workstation's key pair,
   the **authorized-key roster**, the **`#fleet` inventory** (Host blocks), and the fleet nodes'
   **host keys** (`known_hosts`).
3. **1Password and LastPass now; a third manager is one adapter file.**
4. **Passphrase-protected keys everywhere.** Zero-touch on macOS (Keychain); one
   `fleet keys unlock` per boot on Linux/WSL/Pi, with the passphrase supplied from the vault.
5. **Safe rotation:** overlap → verify the new key logs in on each host → explicit prune. A host
   that fails keeps the old key.
6. **Nothing breaks during rollout.** Without a configured vault, every verb behaves exactly as
   it does today. Fleet nodes need no new software and no new version.
7. **CLI verbs first**, with a TUI entry that runs those verbs (the `p`/`P` pattern), not a
   second implementation.

**Non-goals**
- A vault SDK dependency (no 1Password/LastPass Go SDKs; vendor CLIs only).
- Vault-native SSH agents (1Password's agent): rejected by the operator as vendor lock-in.
- SSH certificates / a CA (`TrustedUserCAKeys`): stronger at scale, but needs sshd changes on
  every node; out of scope for a home fleet. Revisit if workstation count grows.
- Keys for fleet **nodes** that only receive connections: they need none.
- Syncing anything automatically in the background. Every transfer is an explicit, one-way verb.

## 3. Options considered

**A. Vault-backed per-workstation keys via vendor-CLI adapters (recommended).** A small
`Vault` interface in `internal/vault`, with `op` and `lpass` adapters that shell out to the
vendor CLIs (which also own sign-in, Touch ID, sessions). Four item kinds per fleet. Every new
behaviour is opt-in through `fleet vault setup`. **Why:** smallest dependency footprint, the
vendor CLIs already solve authentication, and adding a manager is one file.

**B. External helper protocol** (`fleet-vault-<name>` on PATH, JSON over stdin/stdout, like git
credential helpers). Lets anyone add a manager without rebuilding fleet, but adds a protocol to
version and test before there is a third manager to justify it. **Deferred:** the interface in
A is shaped so a `helper` adapter can implement it later without touching callers.

**C. One shared key on every workstation, restored from the vault.** Simplest onboarding (one
item, already authorized everywhere), but one lost machine exposes every node and forces a
fleet-wide rotation, and logins can't be attributed. **Rejected** by the operator in favour of
per-workstation keys.

## 4. Decision

### 4.1 Units

| Unit | Responsibility | Depends on |
| :-- | :-- | :-- |
| `internal/vault` | The `Vault` interface + item model + `Fake`; pure | — |
| `internal/vault/op`, `internal/vault/lpass` | Adapters: shell out to `op` / `lpass` through an injected `Exec`; secrets only on stdin | `internal/vault` |
| `internal/keystore` | Local key files: generate (with passphrase), write 0600, read pub, "is this key encrypted?", fingerprint; pure but for an injected `Exec` (`ssh-keygen`) | — |
| `internal/agent` | Is a fingerprint loaded? (`ssh-add -l`), load with a passphrase via fleet's own askpass; macOS Keychain flag | `internal/keystore` |
| `cmd/vault*.go` | `fleet vault setup|status|push|pull` | the above + `cfgplan`, `hostkey` |
| `cmd/keys*.go` | `fleet keys unlock|rotate`; `sync`/`prune` read the roster | the above |
| TUI `V` | Suspends the TUI and runs the verb (the `p`/`P` pattern) | `cmd` |

### 4.2 The `Vault` interface (shape; frozen in the plan's §3 with go-goarch)

```go
type Vault interface {
    Name() string                                  // "op", "lpass"
    Ready(ctx) error                               // signed in? (vendor CLI decides)
    SignIn(ctx, tty) error                         // hand the terminal to `op signin` / `lpass login`
    Get(ctx, item string) (Item, error)            // ErrNotFound when absent
    Put(ctx, item string, it Item, rev Rev) error  // compare-and-set on rev; ErrConflict
    List(ctx, prefix string) ([]string, error)
}
type Item struct {
    Kind   Kind              // KeyPair | Roster | Inventory | KnownHosts
    Fields map[string]Secret // private_key, passphrase, public_key, body …
    Rev    Rev               // opaque, vendor revision/hash, for compare-and-set
}
```

`Secret` is a byte slice with a redacting `String()`/`MarshalJSON`, so a stray `%v` can never print
a key. Adapters pass secrets to the vendor CLI on **stdin only**, never in argv or the environment
(`/proc/<pid>/{cmdline,environ}` are world-readable — the same rule the sudo credential follows).

### 4.3 Vault layout (one folder/vault per fleet, default name `fleet`)

| Item | Kind | Contents | Written by |
| :-- | :-- | :-- | :-- |
| `fleet/key/<workstation>` | KeyPair | private key (encrypted), passphrase, public key, fingerprint, created | `vault push` from that workstation; `keys rotate` |
| `fleet/roster` | Roster | the authorized public keys, one per workstation, with state `active` / `pending` / `retired` | `vault push`, `keys rotate`, `keys prune --retire` |
| `fleet/inventory` | Inventory | the `#fleet` Host blocks (inert fields only — `cfgplan` already guarantees no exec directive can travel) | `vault push` |
| `fleet/known_hosts` | KnownHosts | the fleet nodes' host keys | `vault push` |

`<workstation>` defaults to the short hostname and is stored in `~/.config/fleet/vault.yaml`
(provider, vault/folder name, workstation name — no secrets).

### 4.4 Verbs (minimal surface: six new, two extended)

| Verb | Does |
| :-- | :-- |
| `fleet vault setup` | Pick provider (`op`/`lpass`), sign in, choose vault/folder; writes `vault.yaml`. Offers to `push` (existing workstation) or `pull` (new workstation) |
| `fleet vault status` | Provider, signed in?, this workstation's key: present locally / in vault / in roster / loaded in agent; inventory + known_hosts drift vs local |
| `fleet vault push` | One-way, diff-first, confirm: this workstation's key pair → its item; local `#fleet` blocks → inventory (merge, never replace); known_hosts entries for fleet aliases → known_hosts; this workstation's pub → roster |
| `fleet vault pull` | One-way, diff-first, confirm: this workstation's key pair → `~/.ssh` (passphrase-protected, 0600), loaded into the agent; inventory → `~/.ssh/config` (merge via `cfgplan`); known_hosts → `~/.ssh/known_hosts`. A brand-new workstation with no item is offered `keys rotate --new` |
| `fleet keys unlock` | If the key is encrypted and not in the agent: fetch the passphrase from the vault and `ssh-add` it (macOS: into Keychain too). No-op when already loaded |
| `fleet keys rotate [--new]` | Generate a new passphrase-protected key → store in vault as `pending` → authorize it on every host (overlap) → verify it logs in on each host with `IdentitiesOnly=yes` → mark `active`; the old key is NOT removed |
| `fleet keys sync` *(extended)* | Authorizes the roster's `active`+`pending` keys when a vault is configured; else today's `~/.ssh/*.pub` |
| `fleet keys prune` *(extended)* | The managed set becomes roster (`active`+`pending`) ∪ local `*.pub`; `--retire <key>` removes exactly one retired key, only from hosts where the replacement verified |

TUI: `V` opens a small menu (status / pull / push / unlock / rotate) and runs the chosen verb with
the TUI suspended, so its diff and confirmation are the CLI's. The status line's row tip gains
`press V → unlock` when the pre-flight check (§4.6) finds the key locked.

### 4.5 Amended invariant

*"No private key ever leaves the workstation"* becomes: **"A private key is never sent to a fleet
node. The only place it may go is the operator's vault, encrypted by its passphrase, and it
travels on a vendor CLI's stdin."** `syncKeyToHost` keeps authorizing public keys only, and a test
pins that no remote command string can contain `PRIVATE KEY`.

### 4.6 Locked-key pre-flight

Every probe runs `BatchMode=yes`, which cannot prompt for a passphrase. After a Linux reboot the
agent is empty, so without a check every host would read `auth-failed` — exactly the misdiagnosis
`internal/sshfail` exists to prevent. Before fanning out (`status`, `update`, `tui`, `keys sync`),
fleet checks once per process: is the configured identity encrypted (`keystore`) and its
fingerprint absent from `ssh-add -l` (`agent`)? If so the run stops before dialling and says
`key locked: run fleet keys unlock` (TUI: a banner + `V`). Hosts are never marked `auth-failed`
for a locked local key.

### 4.7 Rollout and fallback

- **Opt-in by configuration.** No `vault.yaml` ⇒ every code path is today's, byte for byte.
  A `fleet.vault.enabled` gff flag (default **off**, fail-open to off) gates the new verbs while
  they're proven, so the TUI menu and the extended `sync`/`prune` behaviour stay dormant.
- **Nodes don't change.** Everything new runs on the workstation; nodes still only see an
  `authorized_keys` append, the same script as today. There is no "update every node at once"
  moment to get wrong.
- **Overlap rotation** keeps the old key valid until every host has verified the new one; a host
  that fails verification keeps the old key and is named in the report. `--retire` acts only on
  hosts that verified.
- **The vault is never the only copy during a change.** `pull` backs up every local file it
  rewrites (`~/.ssh/config`, `known_hosts`, an existing key) with the timestamped-backup pattern
  config writes already use.

## 5. Risks & blast radius

| Risk | Mitigation |
| :-- | :-- |
| Vault compromise exposes every workstation key | Keys are passphrase-encrypted *and* the passphrase is a separate field; a vault breach already means full compromise — documented, not hidden. Per-workstation keys keep a single **laptop** loss to one `--retire` |
| A secret leaks via argv/env/logs | stdin-only transport; `Secret` redacts on print; tests assert no `PRIVATE KEY`/passphrase reaches argv, env, the capture log or `fleet.log` |
| Prune from a stale roster revokes a valid key | Prune's managed set is roster ∪ local `*.pub`; `--retire` names one key and requires the replacement verified on that host |
| Two workstations push at once (lost update) | `Put` is compare-and-set on the item's revision; a conflict re-reads, re-diffs and re-asks |
| A rogue inventory/known_hosts in the vault | Inventory goes through `cfgplan` (no exec directive can travel) + PR #450's `sshconf` control-character guard; pull is diff-first with confirmation |
| Vendor CLI behaviour drifts (field names, flags) | Adapter integration tests against recorded CLI transcripts + a live gate per adapter |
| Rotation verification gives a false pass because the agent offers the old key | Verification probes use `IdentitiesOnly=yes -i <new>` and `ControlPath=none` (a live master would skip auth) |

## 6. Rollback

Delete `~/.config/fleet/vault.yaml` (or set `fleet.vault.enabled=false`): fleet reverts to local
behaviour immediately, with nothing on any node to undo. Keys already authorized stay authorized;
`fleet keys prune` removes them like any other foreign key. Each `pull` leaves timestamped
backups of every file it rewrote.

## 7. Evidence expectations

- **Unit:** each adapter against recorded `op`/`lpass` transcripts (fake `Exec`); `Secret`
  redaction; compare-and-set conflict path; roster state machine; prune managed-set union; the
  pre-flight short-circuit; argv/env secret scans.
- **Live gates (operator):** G1 `vault setup` + `push` from an existing workstation on **each**
  provider; G2 `vault pull` onto the fresh Mac → `fleet status` all green with no manual key
  copy; G3 Linux reboot → pre-flight says locked → `keys unlock` → green; G4 `keys rotate` across
  the fleet with one host deliberately unreachable → that host keeps the old key, others verified;
  G5 `--retire` removes the old key only where the new one verified.
- **Transcripts** under `plans/fleet-vault-keys/evidence/`, real output only (sdk rule: an
  invented transcript is worse than none).

> Produced via `superpowers:brainstorming` (operator Q&A in-session, 2026-11-13). Interface
> freeze in the plan with `go-goarch`. Spec: [`../specs/fleet-vault-keys.md`](../specs/fleet-vault-keys.md).
