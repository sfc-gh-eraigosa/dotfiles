---
name: find_ssh_host
description: Find an SSH host on the local network and update ~/.ssh/config with its current IP address
---

# Find SSH Host

This skill uses the `ssh-find` script to scan the local network for an SSH host
defined in `~/.ssh/config`, identify it by hostname, and update the config file
with the host's current IP address.

## Prerequisites

- `nmap` must be installed (`brew install nmap` on macOS, `sudo apt-get install nmap` on Linux)
- The `~/opt/bin/ssh-find` script must be available (installed via dotfiles)
- The target host alias must already exist in `~/.ssh/config`
- The SSH key referenced in the config must be present locally

## Steps

### 1. Prompt for the Host Alias

Ask the user which SSH host alias they want to find. The alias must match a
`Host` entry in `~/.ssh/config`.

If the user has already provided a host alias in their request, use that value
directly without prompting.

### 2. Validate the Host Alias

Run the following command to confirm the alias exists in the SSH config:

```bash
ssh -G <HOST_ALIAS> 2>/dev/null | head -5
```

- If the command succeeds (exit code 0), the alias is valid. Extract and display
  the current `hostname` and `user` from the output for the user's reference.
- If the command fails, inform the user that the alias was not found in
  `~/.ssh/config` and stop.

### 3. Run ssh-find

Execute the `ssh-find` script via the terminal. This script is interactive — it
will scan the network and prompt for confirmation before updating the config.

```bash
~/opt/bin/ssh-find <HOST_ALIAS>
```

**Important:** This command is interactive. It will:
1. Scan the local subnet for SSH-enabled hosts using `nmap` (may require `sudo`)
2. Test each discovered IP by connecting with the configured SSH key
3. For each successful connection, display the remote hostname and ask:
   `Is this the correct host for '<HOST_ALIAS>'? (y/n):`

You must monitor the terminal output and:
- When prompted with `(y/n)`, ask the user whether the displayed hostname matches
  the host they are looking for.
- Send `y` or `n` based on the user's answer.

### 4. Report Results

After the script completes, report the outcome to the user:

- **Success**: The SSH config was updated. Show the new IP address and note that
  a backup was created at `~/.ssh/config.bak`.
- **No match found / No SSH hosts found**: The script scanned all discovered
  hosts but none were confirmed.
  - If a MAC address for `<HOST_ALIAS>` exists in `~/.ssh/wol_map.json`, the
    `ssh-find` script will automatically offer to send a Wake-on-LAN magic
    packet. 
  - Follow the script's interactive prompts (confirm with the user if they want
    to wake the host).
  - If the script doesn't offer WOL but the user provides a MAC address, you
    can run `wol <MAC_ADDRESS>` manually.
  - Inform the user if a packet was sent and suggest waiting a minute before
    retrying.

### 5. Verify the Update (on success)

If the config was updated, verify by running:

```bash
ssh -G <HOST_ALIAS> 2>/dev/null | grep "^hostname "
```

Display the updated hostname value to confirm the change took effect.
