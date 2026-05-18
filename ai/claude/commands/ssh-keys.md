---
description: Manage SSH keys across hosts — generate, sync, list status, or delete
allowed-tools: Bash(~/git/dotfiles/src/ssh-key-sync/ssh-key-sync.sh:*)
---

Manage SSH keys across all hosts in `~/.ssh/config` via the `ssh-key-sync` skill.

---
Arguments: $ARGUMENTS

Activate the **ssh-key-sync** skill. Parse `$ARGUMENTS` to determine intent:

- `list` (or no args) — show the management table of all keys and their per-host authorization status:
  `~/git/dotfiles/src/ssh-key-sync/ssh-key-sync.sh --list`
- `generate <name> [--no-sync]` — create a new Ed25519 key. With `--no-sync`, keep it local only; otherwise distribute to every host in `~/.ssh/config`:
  `~/git/dotfiles/src/ssh-key-sync/ssh-key-sync.sh [--no-sync] <name>`
- `delete <name1> [<name2> ...]` — securely revoke and remove keys locally and from every remote `authorized_keys`:
  `~/git/dotfiles/src/ssh-key-sync/ssh-key-sync.sh --delete <names...>`

**Always** confirm with the user before `delete` or before distributing a new key — these are network-wide mutations. List operations are safe to run without confirmation.

After any mutating action, re-run `--list` and show the updated table so the user can verify.
