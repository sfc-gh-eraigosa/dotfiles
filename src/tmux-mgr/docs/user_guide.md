# tmux-mgr User Guide

`tmux-mgr` is your command-line interface for complex tmux interactions, saving layouts, and acting as a central hub for spawning AI Agent Teams.

## Managing Windows and Sessions

### Session Lifecycle
- Start a new session: `tmux-mgr session new my-project -a` (The `-a` flag attaches you immediately).
- See all sessions: `tmux-mgr session list`
- Kill a session: `tmux-mgr session kill my-project`

### Moving and Resizing
- To move your cursor to a different pane: `tmux-mgr window move [left|right|up|down]`
- To resize a pane by a percentage of the screen: `tmux-mgr window resize width 50%`

### Saving Layouts
If you have a complex setup of windows and panes that you use every day, you can save it:
1. Arrange your windows.
2. Run `tmux-mgr save daily-dev`
3. Later, you can recreate this setup by running `tmux-mgr restore daily-dev`

## AI Agent Team Orchestration

The most powerful feature of `tmux-mgr` is its ability to spawn independent, autonomous agents directly from the command line. This allows you to delegate complex tasks to specialized agents that run in parallel without conflicting with your work.

### How it Works (Integrated Architecture)
`tmux-mgr` is a self-contained orchestrator. When you start an agent, `tmux-mgr` handles the entire lifecycle:

1.  It creates a new, isolated **Git Worktree** from your current repository branch.
2.  It creates a new **tmux pane** for the agent to run in.
3.  It launches a new instance of **itself** in that pane in a special agent execution mode.
4.  The new `tmux-mgr` instance then performs the task, writing its results to a `RESULT.md` file.

This integrated model removes external dependencies and ensures the agent has the exact same environment and tooling as the main `tmux-mgr` process.

### Host-Aware Assistant Selection
`tmux-mgr` auto-detects which AI CLI is driving you and runs the matching assistant inside each spawned pane — no flags required:

- **Inside Claude Code** (detected via `CLAUDECODE=1`): each pane runs `claude -p "<task>" --dangerously-skip-permissions`. Because Claude has built-in `Task()` sub-agents, a single `agent start` invocation already gives you a full fan-out / fan-in team inside that pane.
- **Otherwise (default)**: each pane runs `gemini -y -p "<task>"` with model fallback — the original behavior, untouched.

`tmux-mgr agent start` prints the detected host (e.g. `assistant=claude`) so you can confirm which branch fired.

### 1. Spawning an Agent (Fan-Out)
To give a task to an agent, use the `agent start` command with a clear, natural language description of the task.

```bash
tmux-mgr agent start generalist --task-description "Refactor the user authentication Go module to improve error handling."
```
*This command spawns a `generalist` agent in an isolated worktree and gives it the specified task.*

### 2. Checking Status
You can see all currently active agent sessions (and the paths to their worktrees) using:
```bash
tmux-mgr agent list
```
This will show the Session ID, the Agent Name, its status, and when it started.

### 3. Collecting Results (Fan-In)
Spawned agents are instructed to write their final findings to a `RESULT.md` file. Once the task is complete, you can retrieve the detailed summary using:
```bash
tmux-mgr agent complete <session-id>
```

### 4. Cleaning Up
When the task is completely finished, you should clean up the isolated environment to save space and remove old tracking data:
```bash
tmux-mgr agent cleanup <session-id>
```
*Note: You must manually close the `tmux` pane (e.g., by typing `exit` or `Ctrl+D` in that pane) after the agent finishes.*
