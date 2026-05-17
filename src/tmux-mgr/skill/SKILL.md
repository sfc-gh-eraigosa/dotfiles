---
name: tmux
description: A powerful, Go-based management tool for tmux, providing structured session management, window arrangements, layout persistence, and autonomous AI Agent Team Orchestration.
---
# Tmux Management Skill

This skill provides expertise in managing tmux sessions and windows using the `tmux-mgr` tool. It allows the Gemini CLI to interact with tmux sessions, manage layouts, introspect pane content, and orchestrate teams of AI agents to work on complex, multi-step tasks in parallel.

## Capabilities

### 1. Hybrid Agent Team Orchestration (Isolated Parallel Work)

`tmux-mgr` provides OS-level isolation for parallel agent tasks. By provisioning independent Git worktrees and starting new `gemini-cli` sessions in dedicated tmux panes, multiple agents can work on the same repository without file collisions.

**Crucially, `tmux-mgr` does not manage cognitive state.** You (the primary agent) must use your native Gemini tools (like `tracker_create_task`) to define the work and track progress, while using `tmux-mgr agent start` simply to spawn the isolated workspace.

- **Start Agent**: `tmux-mgr agent start <agent-name> --task-id <task-id>`
- **Check Progress**: `tmux-mgr agent list`
  - Shows all currently active agent sessions and the paths to their isolated worktrees.
- **Get Results (Fan-In)**: `tmux-mgr agent complete <session-id>`
  - Retrieves the final summary from the `RESULT.md` file in the agent's isolated worktree.
- **Cleanup**: `tmux-mgr agent cleanup <session-id>`
  - Removes the Git worktree and deletes the session tracking file.

### 2. Session Management
- **List sessions**: `tmux-mgr session list`
- **Create new session**: `tmux-mgr session new [name] [-a|--attach]` (Use -a to automatically attach)
- **Attach to session**: `tmux-mgr session attach [name]`
- **Detach from session**: `tmux-mgr session detach`
- **Kill session**: `tmux-mgr session kill [name]`

### 3. Window & Pane Arrangements
- **Split window**: `tmux-mgr window split [horizontal|vertical]`
- **Move focus**: `tmux-mgr window move [left|right|up|down]`
- **Resize panes**: 
  - Directional: `tmux-mgr window resize [left|right|up|down] [val]`
  - Percentage: `tmux-mgr window resize width 50%` or `tmux-mgr window resize height 25%`
- **Capture content**: `tmux-mgr capture [target]` (returns the text content of a pane)

### 4. Layout Persistence
- **Save layout**: `tmux-mgr save [name]` (saves to `~/.config/tmux-mgr/[name].json`)
- **Restore layout**: `tmux-mgr restore [name]` (restores window layout and names)

### 5. Desktop Navigation
- **List windows**: `tmux-mgr desktop list`
- **Switch window**: `tmux-mgr desktop switch [name|index]`

### 6. Shell Environment
- **Install aliases & completions**: `tmux-mgr alias install`
  - Sets up `tmux-a`, `tmux-ls`, `tmux-new`, `tmux-kill`, and `tmux-start`.
  - Configures shell completions for Bash and Zsh.

## Guidelines for Natural Language Interaction

- **Orchestrate Effectively**: If a task is complex or requires parallel execution, proactively suggest using the `tmux-mgr agent` features.
- **Isolate for Safety**: Explain that `tmux-mgr` creates a separate `git worktree` for each agent to prevent file-system conflicts.
- **Fan-In**: Sub-agents MUST write their final summaries to `RESULT.md`. Retrieve these with `agent complete` before summarizing for the user.
- **Introspect First**: Use `tmux-mgr capture` to understand what's running in other windows or to troubleshoot terminal output.
- **Be Conversational**: "I'll spawn an agent to handle the backend in a separate pane so we can work on the frontend here."
