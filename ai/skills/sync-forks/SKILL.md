---
name: sync-forks
description: Check how many of your GitHub forks are out of date and sync them.
---

# Sync Forks

This skill checks all GitHub forks associated with the authenticated user, identifies which ones are out of date compared to their upstream parent, and optionally syncs them using the GitHub CLI (`gh`).

## Usage

When the user asks to check or sync their forks, use the provided script.

### 1. Check for Out-of-Date Forks

Run the script without any arguments to get a summary of forks that are behind their parent repositories.

```bash
bash /home/wenlock/git/dotfiles/ai/skills/sync-forks/scripts/check_and_sync_forks.sh
```

### 2. Sync Out-of-Date Forks

Run the script with the `--sync` flag to automatically sync any forks that are behind.

```bash
bash /home/wenlock/git/dotfiles/ai/skills/sync-forks/scripts/check_and_sync_forks.sh --sync
```

### 3. Force Sync Diverged Forks

If a fork has diverged from its parent and the user wants to forcefully overwrite local changes to match the parent, use the `--force` flag along with `--sync`.

```bash
bash /home/wenlock/git/dotfiles/ai/skills/sync-forks/scripts/check_and_sync_forks.sh --sync --force
```

## Note

Some repositories might fail to sync if the upstream parent has added or modified GitHub Actions workflows. The user will need to grant the `workflow` scope to the GitHub CLI by running `gh auth refresh -s workflow` manually in their terminal.
