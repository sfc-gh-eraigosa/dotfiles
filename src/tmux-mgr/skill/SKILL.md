---
name: tmux
description: A powerful, Go-based management tool for tmux, providing structured session management, window arrangements, layout persistence, and autonomous AI Agent Team Orchestration.
---
---
name: tmux
description: A powerful, Go-based management tool for tmux, providing structured session management, window arrangements, layout persistence, and autonomous AI Agent Team Orchestration.
---
# Tmux Management Skill

This skill provides expertise in managing tmux sessions and windows using the `tmux-mgr` tool. It allows the Gemini CLI to interact with tmux sessions, manage layouts, introspect pane content, and orchestrate teams of AI agents to work on complex, multi-step tasks in parallel.

## Capabilities

### 1. Integrated Agent Team Orchestration

`tmux-mgr` is a self-contained orchestrator that provides OS-level isolation for parallel agent tasks. By provisioning independent Git worktrees and spawning new instances of itself in dedicated tmux panes, multiple agents can work on the same repository without file collisions.

**You (the primary agent) can now delegate tasks directly.** Use the `tmux-mgr agent start` command with a natural language description of the work to be done.

- **Start Agent**: `tmux-mgr agent start <agent-name> --task-description "<task>"`
- **Check Progress**: `tmux-mgr agent list`
  - Shows all currently active agent sessions and the paths to their isolated worktrees.
- **Get Results (Fan-In)**: `tmux-mgr agent complete <session-id>`
  - Retrieves the final summary from the `RESULT.md` file in the agent's isolated worktree.
- **Cleanup**: `tmux-mgr agent cleanup <session-id>`
  - Removes the Git worktree and deletes the session tracking file.

### 2. Evaluation Suite
- **Evaluate Agent Orchestration**: "tmux evaluate the agent"
  - This command instructs the Gemini CLI to run the self-validation suite located in `src/tmux-mgr/evaluation/AGENT_EVAL.md`.
  - It proves the ability to spawn agents, isolate worktrees, and fan-in results.

### 3. Session Management
- **List sessions**: `tmux-mgr session list`
- **Create new session**: `tmux-mgr session new [name] [-a|--attach]` (Use -a to automatically attach)
- **Attach to session**: `tmux-mgr session attach [name]`
- **Detach from session**: `tmux-mgr session detach`
- **Kill session**: `tmux-mgr session kill [name]`

### 4. Window & Pane Arrangements
- **Split window**: `tmux-mgr window split [horizontal|vertical|left|right|up|down]`
  - Use `vertical` or `down` when requested to "open a pane below" or "under" (splits top-to-bottom).
  - Use `horizontal` or `right` when requested to "open a pane to the side" (splits left-to-right).
- **Move focus**: `tmux-mgr window move [left|right|up|down]`
- **Resize panes**: 
  - Directional: `tmux-mgr window resize [left|right|up|down] [val]`
  - Percentage: `tmux-mgr window resize width 50%` or `tmux-mgr window resize height 25%`
- **Capture content**: `tmux-mgr capture [target]` (returns the text content of a pane)

### 5. Layout Persistence
- **Save layout**: `tmux-mgr save [name]` (saves to `~/.config/tmux-mgr/[name].json`)
- **Restore layout**: `tmux-mgr restore [name]` (restores window layout and names)

### 6. Desktop Navigation
- **List windows**: `tmux-mgr desktop list`
- **Switch window**: `tmux-mgr desktop switch [name|index]`

### 7. Shell Environment
- **Install aliases & completions**: `tmux-mgr alias install`
  - Sets up `tmux-a`, `tmux-ls`, `tmux-new`, `tmux-kill`, and `tmux-start`.
  - Configures shell completions for Bash and Zsh.

## Guidelines for Natural Language Interaction

- **Self-Evaluation**: If a user asks to "evaluate the agent" or "test your team features," activate this skill and follow the instructions in `src/tmux-mgr/evaluation/AGENT_EVAL.md`.
- **Orchestrate Effectively**: If a task is complex or requires parallel execution, proactively suggest using the `tmux-mgr agent` features with a direct task description.
- **Isolate for Safety**: Explain that `tmux-mgr` creates a separate `git worktree` for each agent to prevent file-system conflicts.
- **Fan-In**: Sub-agents MUST write their final summaries to `RESULT.md`. Retrieve these with `agent complete` before summarizing for the user.
- **Introspect First**: Use `tmux-mgr capture` to understand what's running in other windows or to troubleshoot terminal output.
- **Be Conversational**: "I'll spawn an agent to handle the backend in a separate pane so we can work on the frontend here."
