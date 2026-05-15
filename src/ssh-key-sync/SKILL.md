# SSH Key Sync Skill

This skill allows for managing SSH keys across a network, including generation, synchronization, status reporting, and secure deletion.

## Capabilities

### 1. Generate and Sync Keys
- **Generate a new key pair**: Automatically creates a new Ed25519 key with a unique name.
- **Optional Distribute**: Copies the new private/public keys to all hosts in `~/.ssh/config` unless `--no-sync` is specified.
- **Authorize keys**: Appends the new public key to `~/.ssh/authorized_keys` on the local machine and remote hosts.
- **Sync Config**: Ensures the `~/.ssh/config` file is consistent across all hosts during synchronization.

### 2. List Key Status
- **Management Table**: Generates a table showing all managed keys, their types, and which hosts (local/remote) have them authorized.

### 3. Delete/Cleanup Keys
- **Secure Deletion**: Removes specified keys from the local machine and all remote hosts.
- **Revoke Authorization**: Automatically removes the public key entries from `authorized_keys` files locally and remotely to ensure the keys can no longer be used.

## Usage

### Direct Execution
You can run the script directly:
```bash
# Generate and sync to all hosts
/home/wenlock/git/dotfiles/src/ssh-key-sync/ssh-key-sync.sh <key_name>

# Generate locally only
/home/wenlock/git/dotfiles/src/ssh-key-sync/ssh-key-sync.sh --no-sync <key_name>

# List all keys and their sync status
/home/wenlock/git/dotfiles/src/ssh-key-sync/ssh-key-sync.sh --list

# Delete one or more keys
/home/wenlock/git/dotfiles/src/ssh-key-sync/ssh-key-sync.sh --delete <key_name1> <key_name2>
```

### Natural Language Instructions
- "Show me a table of all my SSH keys."
- "Generate a new key called 'temp-key' and sync it."
- "Clean up the test keys 'id_ed25519_new' and 'local_test_key' across all my hosts."

## Workflow
1. **Research**: The skill identifies the user's intent (generate, sync, list, or delete).
2. **Action**: It executes `ssh-key-sync.sh` with the appropriate flags and key names.
3. **Verification**: It confirms the success of the requested management action.
