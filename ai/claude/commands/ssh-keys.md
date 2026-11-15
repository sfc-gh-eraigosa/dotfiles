---
description: Manage SSH keys across fleet hosts — list, sync, enroll a new machine, or delete
allowed-tools: Bash(fleet keys:*), Bash(ssh-keygen:*)
---

Manage SSH public keys across the `#fleet` hosts in `~/.ssh/config` via the `ssh-key-sync` skill (`fleet keys`).

---
Arguments: $ARGUMENTS

Activate the **ssh-key-sync** skill. Parse `$ARGUMENTS` to determine intent:

- `list` (or no args) — which hosts authorize each key: `fleet keys list`
- `generate <name> [--no-sync]` — `ssh-keygen -t ed25519 -f ~/.ssh/<name>`; unless `--no-sync`, then `fleet keys sync <name>`
- `sync [name...]` — authorize this machine's public key(s) where missing: `fleet keys sync [name...]`
- `enroll <alias-or-user@ip>` — authorize a NEW machine's key (one every host refuses) from this machine, which already has access: `fleet keys enroll <dest>`
- `delete <name>` — revoke a key on every host, per-host confirmation: `fleet keys delete <name>`

**Always** confirm with the user before `sync`, `enroll`, or `delete` — these are network-wide mutations. For `enroll`, show the fingerprint fleet prints and ask the user to check it on the new machine. `list` is safe to run without confirmation.

After any mutating action, re-run `fleet keys list` and show the result.
