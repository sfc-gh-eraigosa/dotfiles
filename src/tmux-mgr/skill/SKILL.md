---
name: tmux
description: A powerful, Go-based management tool for tmux, providing structured session management, window arrangements, layout persistence, and autonomous AI Agent Team Orchestration.
---
# Tmux Management Skill

This skill provides expertise in managing tmux sessions and windows using the `tmux-mgr` tool. It lets your AI coding assistant interact with tmux sessions, manage layouts, introspect pane content, and orchestrate teams of AI agents to work on complex, multi-step tasks in parallel.

## Capabilities

### 1. Integrated Agent Team Orchestration

`tmux-mgr` is a self-contained orchestrator that provides OS-level isolation for parallel agent tasks. By provisioning an isolated working directory for each agent and spawning new instances of itself in dedicated tmux panes, multiple agents can work on the same repository without file collisions.

**Host-aware assistant selection.** `tmux-mgr` auto-detects which AI CLI is driving the orchestration and spawns the matching assistant inside each agent pane:

- Inside Claude Code (`CLAUDECODE=1`): each pane runs `claude -p "<task>" --dangerously-skip-permissions` and exits when the task completes. The spawned Claude can further fan out via its native `Task()` subagents inside its own pane — so a single `agent start` invocation already gives you a full agent team.
- Otherwise (default): each pane runs `gemini -y -p "<task>"` with model fallback, preserving the original behavior.

No flag is needed — detection is automatic from the host shell's environment.

**You (the primary agent) can now delegate tasks directly.** Use the `tmux-mgr agent start` command with a natural language description of the work to be done.

- **Start Agent**: `tmux-mgr agent start <agent-name> --task-description "<task>"`
  - The session is tagged with the current repo's git root so `agent list` can scope to it later. Sessions live globally under `~/.config/tmux-mgr/sessions/`, so the same binary works from any repo without per-repo setup.
  - **Agent definitions**: `<agent-name>` is matched against `./.ai/agents/<name>.md` first, then `~/.ai/agents/<name>.md`; both directories are also scanned for files whose `# Aliases:` frontmatter contains the name. When matched, the file's `# Persona:` becomes the pane label, `# Symbol:` becomes the pane emoji, and `# Model:` (Ollama-style, e.g. `qwen2.5:1.5b` or `smollm:360m`) is mapped by parameter size to a Claude/Gemini tier (Haiku/Flash for <3B, Sonnet/Pro for 3–7B, Opus/Pro for ≥8B). Unrecognized models cause the spawn to inherit the host CLI's default model. When no file matches, `generalist` defaults apply: the cheapest tier (Haiku 4.5 / gemini-2.5-flash) and the 🤖 emoji.
- **Check Progress**: `tmux-mgr agent list`
  - Defaults to sessions started from the current repo; pass `--all` to see every session (including global / legacy ones with no repo binding). Status reconciles live: `RUNNING` while the pane is alive, `COMPLETED` once `RESULT.md` is written, `FAILED` if the pane exits without one.
- **Get Results (Fan-In)**: `tmux-mgr agent complete <session-id>`
  - Retrieves the final summary from the `RESULT.md` file in the agent's isolated working directory.
- **Cleanup**: `tmux-mgr agent cleanup <session-id>`
  - Tears down the agent's isolated working directory and deletes the session tracking file.

### 2. Evaluation Suite
- **Evaluate Agent Orchestration**: "tmux evaluate the agent"
  - This command instructs your assistant to run the self-validation suite located in `src/tmux-mgr/evaluation/AGENT_EVAL.md`.
  - It proves the ability to spawn agents, isolate working directories, and fan-in results.

### 3. Session Management
- **List sessions**: `tmux-mgr session list`
- **Create new session**: `tmux-mgr session new [name] [-a|--attach]` (Use -a to automatically attach)
- **Attach to session**: `tmux-mgr session attach [name]`
- **Detach from session**: `tmux-mgr session detach`
- **Kill session**: `tmux-mgr session kill [name]`

### 4. Window & Pane Arrangements
- **Anchor root pane**: `tmux-mgr pane anchor [title]`
  - Run once from your terminal to mark the active pane as the orchestration root.
  - Stores the pane ID in the tmux global environment (`TMUX_MGR_ROOT_PANE`) so any AI process can target it.
  - Required before any AI process (Claude, Gemini) can split or spawn panes correctly from outside tmux.
  - Default title is `root`; pass a custom label (e.g. `claude`, `gemini`) if desired.
- **Adopt pane from outside tmux**: `tmux-mgr pane adopt`
  - Creates a new tmux window from a non-tmux shell, registers it as the root anchor, and prints its pane ID.
  - Use this when an AI process needs an anchor but was not launched from inside tmux.
  - Requires a running tmux server.
- **Split (anchor-aware)**: `tmux-mgr pane split [horizontal|vertical|left|right|up|down]`
  - Splits the root anchor pane. Errors with an actionable message if no anchor is set.
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
- **Isolate for Safety**: Explain that `tmux-mgr` gives each agent its own isolated working directory to prevent file-system conflicts.
- **Durable Progress**: Each agent's work is automatically checkpointed to a draft pull request as it runs, so an interrupted or failed agent still leaves a recoverable trail you can inspect — you don't have to babysit a pane to avoid losing work.
- **Fan-In**: Sub-agents MUST write their final summaries to `RESULT.md`. Retrieve these with `agent complete` before summarizing for the user.
- **Introspect First**: Use `tmux-mgr capture` to understand what's running in other windows or to troubleshoot terminal output.
- **Anchor before splitting**: If `tmux-mgr window split` or `tmux-mgr pane split` returns an error like `not running inside a tmux pane and no anchor established`, respond with the following message to the user:

  > To let me manage panes, I need a one-time anchor from your terminal. Run this from inside your tmux pane:
  > ```
  > tmux-mgr pane anchor
  > ```
  > Once that's done, ask me again and I'll split the pane correctly.

  From outside tmux, use `tmux-mgr pane adopt` to create and register an anchor window automatically.
- **Be Conversational**: "I'll spawn an agent to handle the backend in a separate pane so we can work on the frontend here."
