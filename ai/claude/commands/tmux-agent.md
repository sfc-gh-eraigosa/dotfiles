---
description: Orchestrate parallel agents via tmux-mgr (start / list / complete / cleanup)
allowed-tools: Bash(tmux-mgr agent:*), Bash(tmux-mgr session:*), Bash(tmux-mgr capture:*)
---

Delegate or inspect work running in isolated `tmux-mgr` agent sessions.

## Currently running agents
!`tmux-mgr agent list 2>/dev/null || echo "no agents running"`

---
Arguments: $ARGUMENTS

Parse the arguments to decide intent:

- `start <name> <task description...>` — spawn a new agent in an isolated git worktree:
  `tmux-mgr agent start <name> --task-description "<task>"`
- `list` — show all active agent sessions and their worktree paths.
- `complete <session-id>` — fan-in: read `RESULT.md` from the agent's worktree and summarize.
- `cleanup <session-id>` — remove the worktree and tracking file (only after results are saved).
- `capture <session-id>` — inspect the live terminal of a running agent.

If the user gave a free-form task without a verb, default to **start** with an auto-generated name.

Per the **tmux** skill: prefer agent delegation for any task that is parallelizable, repository-isolated, or expected to take more than a few minutes. Sub-agents MUST write `RESULT.md` before being cleaned up.
