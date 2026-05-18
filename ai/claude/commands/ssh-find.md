---
description: Find an SSH host on the local network and update ~/.ssh/config with its current IP
allowed-tools: Bash(ssh -G:*), Bash(grep:*)
---

Locate an SSH host alias on the local network using the `ssh-find` script, then update `~/.ssh/config` with the discovered IP.

---
Host alias: $ARGUMENTS

Activate the **ssh-host-finder** skill and follow its workflow:

1. If no alias was provided, ask the user which `Host` entry to find.
2. Validate the alias resolves via `ssh -G <alias>` and display the current `hostname`/`user`.
3. Run `~/opt/scripts/network/ssh-find <alias>` interactively. The script will:
   - Scan the local subnet with `nmap` (may require `sudo`).
   - Test each candidate IP with the configured SSH key.
   - Prompt `(y/n)` for each remote hostname found — relay each prompt to the user.
4. If no match is found and a MAC address exists in `~/.ssh/wol_map.json`, the script offers WOL. Confirm with the user before sending the magic packet.
5. On success, run `ssh -G <alias> | grep "^hostname "` to verify, and note that `~/.ssh/config.bak` was created.
