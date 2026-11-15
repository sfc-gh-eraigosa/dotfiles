---
name: ssh-key-sync
description: Audit, authorize, enroll, and revoke SSH public keys across the `#fleet` hosts in ~/.ssh/config using `fleet keys`. Use for "which hosts accept my key", "sync my key to the fleet", "a new machine gets Permission denied (publickey) everywhere", "enroll/authorize this machine's key", or "revoke/delete key X".
---
# SSH Key Sync Skill

Every operation runs through **`fleet keys`** (the `fleet` CLI in `sdk/fleet`).
The old `ssh-key-sync.sh` script was retired: it copied the PRIVATE key to every
host, its `--prune` overwrote every `authorized_keys` with one machine's keys,
`--delete` silently generated and distributed the key instead, and it hid
per-host failures. `fleet keys` fixes all four (see `docs/mbo/designs/fleet.md`
§3 "Absorbing ssh-key-sync").

Scope: only hosts marked `#fleet` in `~/.ssh/config` (`fleet list`), not every
`Host` block.

## Commands

| Intent | Command | Mutates? |
| :-- | :-- | :-- |
| Which hosts authorize each of my keys | `fleet keys list` | no |
| Authorize **this** machine's public key(s) where missing | `fleet keys sync [name...] [--host A]` | yes |
| A **new** machine is refused everywhere (keys-only hosts) | on a machine that already has access: `fleet keys enroll <alias-or-user@ip>` | yes |
| Revoke one of my keys on every host | `fleet keys delete <name> [--host A]` | yes, per-host confirm |
| Remove keys not in my managed set | `fleet keys prune` | yes, per-host confirm |

Generating a key is plain `ssh-keygen` (no private key ever leaves the machine):

```bash
ssh-keygen -t ed25519 -f ~/.ssh/<name> -C "$USER@$(hostname)-<name>"
fleet keys sync <name>
```

## Choosing between `sync` and `enroll`

- **`sync`** pushes keys from the machine you are on. It needs that machine to
  *already* log in to each host.
- **`enroll`** is for a machine whose key nothing accepts yet — typically one
  that just ran `fleet` for the first time and generated `~/.ssh/id_ed25519`.
  Run it **from a machine that already has access**. It reads the new machine's
  public key over SSH, shows the fingerprint and every host that would change,
  appends it (backup first, existing keys kept) here and on every fleet host,
  then logs in from the new machine to verify. `fleet status` / `fleet tui`
  print the exact `fleet keys enroll …` line for a keys-only refusal.
- `ssh-copy-id` (or `A` in `fleet tui`) only helps a host that accepts a
  password; on keys-only hosts it cannot work.

## Rules

1. `list` is safe to run without asking.
2. **Always confirm with the user before `sync`, `enroll`, `delete`, or `prune`**
   — they change `authorized_keys` across the network. Show the target hosts and,
   for `enroll`, the fingerprint `fleet` prints, and ask the user to check it on
   the new machine (`ssh-keygen -lf ~/.ssh/id_ed25519.pub`).
3. Never pass `--yes` unless the user explicitly asked for a non-interactive run.
4. Warn before `delete` of the key this machine uses to reach the fleet (fleet
   prints the warning too): it ends this machine's access to those hosts.
5. Never copy a private key anywhere. Never edit `authorized_keys` by hand when a
   `fleet keys` verb covers the change.
6. After any mutation, re-run `fleet keys list` (and for `enroll`, show the
   "verified from …" block) so the user can see the result. Report per-host
   failures verbatim — never summarize a partial run as success.
