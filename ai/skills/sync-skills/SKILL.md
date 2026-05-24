---
name: sync-skills
description: Synchronize AI agent skills and build binaries from the dotfiles repository.
---

# Sync Skills

This skill allows the agent to refresh the AI agent environment by synchronizing skills from the dotfiles repository to the system and optionally rebuilding core binaries.

## Usage

When the user asks to "sync skills" or "rebuild tools", use the `sync-skills` alias.

### 1. Synchronize Skills
Link all available skills into `~/.agents/skills`.

```bash
sync-skills
```

### 2. Synchronize and Build
Link skills and rebuild core binaries (`gss`, `tmux-mgr`, `wol`).

```bash
sync-skills --build
```

## Mandatory Reload

After running `sync-skills`, the agent **MUST** inform the user that they need to manually reload their session for the changes to take effect:

- **Gemini CLI**: Run `/skills reload`
- **Claude Code**: Refresh the session or restart the agent.
