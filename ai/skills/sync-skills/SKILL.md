---
name: sync-skills
description: Synchronize AI agent skills and build binaries from the dotfiles repository.
---

# Sync Skills

This skill allows the agent to refresh the AI agent environment by synchronizing skills from the dotfiles repository to the system and optionally rebuilding core binaries.

## Usage

Always run `sync-skills` using the established alias, which points to the version in `~/opt/scripts/system/sync-skills.sh`. 

Using the `~/opt` path ensures that you are running the scripts as they are installed in the user's environment, maintaining consistency across different machines and installations.

### 1. Synchronize Skills
Link all available skills into `~/.agents/skills`.

```bash
bash ~/opt/scripts/system/sync-skills.sh
```

Or via alias if available:
```bash
sync-skills
```

### 2. Synchronize and Build
Link skills and rebuild core binaries (`gss`, `tmux-mgr`, `wol`).

```bash
bash ~/opt/scripts/system/sync-skills.sh --build
```

## Mandatory Reload

After running `sync-skills`, the agent **MUST** inform the user that they need to manually reload their session for the changes to take effect:

- **Gemini CLI**: Run `/skills reload`
- **Claude Code**: Refresh the session or restart the agent.
