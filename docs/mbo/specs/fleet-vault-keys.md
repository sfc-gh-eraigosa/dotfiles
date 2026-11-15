# fleet vault-backed workstation keys — spec

- **Slug:** fleet-vault-keys
- **Date:** 2026-11-13
- **Status:** Draft
- **Relates to:** issue #451 / PR #452 / design [`../designs/fleet-vault-keys.md`](../designs/fleet-vault-keys.md)

## 1. Goal

Each machine the operator works *from* has its own passphrase-protected SSH key, backed up in
1Password or LastPass. A new or rebuilt workstation joins the fleet with `fleet vault setup` +
`fleet vault pull`: its key, the `#fleet` inventory, and the nodes' host keys arrive from the
vault, and every node already trusts it. Keys rotate without ever locking a node out. With no
vault configured, fleet behaves exactly as it does today.

## 2. Use cases

**UC1 — Back up an existing workstation.** *Actor:* operator on a workstation already in the
fleet. *Trigger:* `fleet vault setup` (first time), then `fleet vault push`. *Flow:* pick
provider → vendor sign-in → choose vault/folder → diff of what will be written (key item,
roster entry, inventory blocks, known_hosts entries) → confirm → written. *Accept:* the four
items exist in the vault; re-running `push` with nothing changed writes nothing and says so.

**UC2 — Restore a rebuilt workstation.** *Actor:* operator on a reinstalled machine whose item
exists. *Trigger:* `fleet vault setup` → `fleet vault pull`. *Flow:* sign in → diff → confirm →
key written to `~/.ssh` (0600, passphrase-protected) and loaded into the agent (macOS: Keychain),
inventory merged into `~/.ssh/config`, host keys merged into `known_hosts`, a backup of every
rewritten file. *Accept:* `fleet status` reaches every reachable node with no manual key copy,
no `ssh-copy-id`, and no host-key prompt.

**UC3 — Join a brand-new workstation (the 2026-11-13 Mac).** *Trigger:* `fleet vault pull`
finds no `fleet/key/<this-host>`. *Flow:* offers `fleet keys rotate --new`: generate a key,
store it, add it to the roster as `pending`. Nodes don't trust it yet, so the command prints the
one step that is still needed: **any already-trusted workstation runs `fleet keys sync`** (it
reads the roster). *Accept:* after that one sync, UC2's acceptance holds on the new machine.

**UC4 — Unlock after reboot (Linux/WSL/Pi).** *Trigger:* any fan-out verb or `fleet tui` while
the key is encrypted and not in the agent. *Flow:* the run stops before dialling:
`key locked: run fleet keys unlock` (TUI: banner, `V` → unlock). `fleet keys unlock` signs in to
the vault if needed and loads the key with the vault's passphrase. *Accept:* no host is ever
shown `auth-failed` because of a locked local key; after unlock, status is green.

**UC5 — Rotate a workstation's key.** *Trigger:* `fleet keys rotate`. *Flow:* new key → vault
(`pending`) → authorized on every fleet host (overlap) → each host verified with the new key
alone → report `verified N / failed M (named)` → roster `active` for the new key, old key marked
`retired`. *Accept:* the old key still works on every host; failed hosts are named with their
cause.

**UC6 — Retire the old key.** *Trigger:* `fleet keys prune --retire <old>`. *Flow:* diff per
host → confirm → removes that exact line only on hosts where the replacement verified; others
are listed as skipped with the reason. *Accept:* no host loses its last working key for this
workstation.

**UC7 — Keep the fleet consistent from any workstation.** *Trigger:* `fleet keys sync` /
`fleet keys prune` on any workstation with a vault configured. *Flow:* the managed set is the
roster (`active`+`pending`) ∪ local `~/.ssh/*.pub`. *Accept:* a prune from a workstation that
holds none of the others' `.pub` files proposes no removal of a roster key.

**UC8 — No vault (fallback).** *Trigger:* any verb with no `vault.yaml`, or `fleet.vault.enabled`
off. *Accept:* behaviour, output and argv identical to today; the new verbs say how to enable
the feature and exit non-zero without side effects.

## 3. Architecture

```
cmd/vault*.go ─┬─► internal/vault (Vault iface, Item, Secret, Fake)
cmd/keys*.go  ─┤        ▲            ▲
cmd/tui (V)  ──┘   vault/op      vault/lpass        (shell out via injected Exec; secrets on stdin)
               ├─► internal/keystore (ssh-keygen: generate, encrypted?, pubkey, fingerprint)
               ├─► internal/agent    (ssh-add -l / ssh-add via fleet askpass; macOS Keychain)
               ├─► internal/cfgplan  (existing: inventory merge, inert fields only)
               ├─► internal/hostkey  (existing: known_hosts read/merge, KnownIn)
               └─► internal/runner   (existing: authorized_keys append; verification probe)
```

Each `internal/*` unit is pure except for its injected `Exec`, so every decision is tested
without a vault, an agent, or a socket. Interfaces are frozen in the plan's §3 with go-goarch.

**askpass:** `keys unlock` loads the key by running `ssh-add` with `SSH_ASKPASS` pointing at the
fleet binary itself (`SSH_ASKPASS_REQUIRE=force`), which reads the passphrase from an inherited
pipe file descriptor — not argv, not env, not disk.

## 4. Behavior / features

- **F1** `Vault` interface + `Secret` redaction + `Fake`.
- **F2** `op` adapter (1Password CLI v2): ready / sign-in / get / put (compare-and-set) / list.
- **F3** `lpass` adapter (LastPass CLI): same contract.
- **F4** `vault.yaml` (provider, vault/folder, workstation name; no secrets; 0600) + `fleet.vault.enabled` gate.
- **F5** `keystore`: generate a passphrase-protected ed25519, write 0600 with a backup of any existing key, detect encrypted, read pub/fingerprint.
- **F6** `agent`: loaded?, load via askpass pipe, macOS `--apple-use-keychain`.
- **F7** `fleet vault setup`.
- **F8** `fleet vault status`.
- **F9** `fleet vault push` (key, roster, inventory, known_hosts; diff-first; compare-and-set).
- **F10** `fleet vault pull` (same four, merge, backups, agent load; UC3 offer).
- **F11** `fleet keys unlock`.
- **F12** Locked-key pre-flight on every fan-out verb + TUI banner.
- **F13** `fleet keys rotate [--new]` with `IdentitiesOnly` verification.
- **F14** `keys sync` / `keys prune` read the roster; `prune --retire`.
- **F15** TUI `V` menu delegating to the verbs; row tip names it.
- **F16** Docs: fleet README section, AGENTS.md invariants (amended private-key rule), a "new workstation" runbook.

## 5. Evaluation criteria (per feature)

| F | Trigger predicate | Fires | Must not fire | Edge | Pass |
| :-- | :-- | :-- | :-- | :-- | :-- |
| F1 | an `Item` with a `private_key` is formatted with `%v`/`%+v`/JSON | prints `[redacted]` | never prints key bytes | nil/empty `Secret` | test over every fmt verb + `json.Marshal` |
| F2/F3 | `Put` of a KeyPair | vendor CLI invoked with the secret on stdin | secret in argv or env | item exists with a newer rev → `ErrConflict` | recorded-transcript tests; argv/env scan for the key's base64 |
| F2/F3 | not signed in | `Ready` returns `ErrSignedOut`; `SignIn` hands over the tty | silent hang | vendor CLI missing → `ErrNotInstalled` naming the install step | fake `Exec` cases |
| F4 | no `vault.yaml` or flag off | every existing verb unchanged | any vault call, any new output | `vault.yaml` with mode 0644 → refused, like `fleet.yaml` | golden output + argv of `status`/`keys sync` unchanged with and without the file |
| F5 | generate | key encrypted with the passphrase, mode 0600 | overwriting an existing key without a backup | `~/.ssh` missing → created 0700 | `ssh-keygen -y -P ''` fails on the file; backup exists |
| F6 | key not loaded | `ssh-add` run with askpass + passphrase via pipe | passphrase in argv/env | macOS → `--apple-use-keychain` present; Linux → absent | spy on the built `exec.Cmd` |
| F9 | `push` with no changes | "nothing to push", no `Put` | any write | concurrent push → re-diff and re-ask on conflict | Fake vault counts writes |
| F10 | `pull` | backups of every rewritten file, then merge | replacing `~/.ssh/config` wholesale; any exec directive | an existing different local key → refuse unless `--replace-key`, with backup | cfgplan merge tests + a backup assertion |
| F12 | key encrypted ∧ fingerprint not in `ssh-add -l` | stop before any dial, name `fleet keys unlock` | marking any host `auth-failed`; firing for an unencrypted key or a loaded one; firing when no vault and the key isn't encrypted | no agent running → same message plus how to start one | runner Fake records zero dials |
| F13 | `rotate` | per-host verify with `IdentitiesOnly=yes`, `ControlPath=none`, `-i <new>` | marking verified via a mux master or the agent's old key | one host unreachable → keeps old key, named in the report, exit non-zero | argv assertions + mixed-result Fake |
| F14 | `prune` with a vault | managed set = roster(`active`,`pending`) ∪ local pubs | proposing removal of a roster key | `--retire` on a host where the new key did not verify → skipped with reason | table test over roster states × hosts |
| F15 | `V` | TUI suspends, runs the verb | a second confirm flow inside the TUI | `V` while an update owns the row → refused like `s` | key-routing test; `TestEveryFleetKeyIsReservedAgainstProviders` |
| all | any remote command string | — | contains `PRIVATE KEY` or the passphrase | — | invariant test over every builder that reaches `runner` |

## 6. Verification harness

- **Automated:** unit tests per `internal/*` package (≥ the fleet coverage floor in
  `scripts/test.sh`), adapter tests over recorded vendor-CLI transcripts, `cmd` tests with
  `vault.Fake` + `runner.Fake`, and the argv/env secret scans. `make lint-go`, `go vet`.
- **Live gates (operator, evidenced under `plans/fleet-vault-keys/evidence/`):** G1 setup+push on
  1Password; G2 setup+push on LastPass; G3 pull onto the fresh Mac → all green; G4 Linux reboot →
  locked → unlock → green; G5 rotate with one host down; G6 retire. Real output only.

## 7. Prerequisites / dependencies

- `op` (1Password CLI v2) and/or `lpass` installed on the workstation; **verify at plan time**
  the exact item/field commands for an SSH-key item on each (field names, stdin template form,
  revision field for compare-and-set), and how `op` behaves under WSL (Linux `op` vs `op.exe`).
- OpenSSH ≥ 8.4 on workstations for `SSH_ASKPASS_REQUIRE` (macOS, Ubuntu 22.04+, Pi OS 12 ship
  newer). Older clients fall back to a typed `ssh-add` prompt.
- PR #450 merged (its `sshconf` control-character guard is relied on by F10).

## 8. Out of scope (and why)

- Vault-native SSH agents: vendor lock-in (operator decision).
- External helper protocol for third-party managers: deferred until a third manager exists;
  the interface is shaped for it.
- SSH certificates / CA: needs sshd changes on every node.
- Background or automatic sync: every transfer stays an explicit one-way verb.
- Node-side keys: nodes that never ssh out need none.

## 9. Rollback

Remove `~/.config/fleet/vault.yaml` or set `fleet.vault.enabled=false`; nothing on any node
changes. `pull` leaves timestamped backups of every file it rewrote.

> Produced via `superpowers:brainstorming` (operator Q&A, 2026-11-13). Next: plan via
> `superpowers:writing-plans`, interfaces frozen with `go-goarch`. Register / update `../index.md`.
