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

The most powerful feature of `tmux-mgr` is its ability to spawn independent `gemini-cli` agent sessions. This allows you to fan out complex tasks to specialized agents without them stepping on each other's toes.

### How it Works (Hybrid Architecture)
Gemini natively tracks tasks and routes agent requests, but it doesn't automatically isolate file systems for parallel OS processes. When you start an agent, `tmux-mgr` handles the physical isolation:
1. It creates a new, isolated **Git Worktree** from your current repository branch.
2. It creates a new **tmux pane** and launches the agent inside it using the native command: `gemini-cli --agent <name> --task <id>`.
3. The newly spawned agent connects to Gemini's native tracking system to understand its assignment.

### 1. Spawning an Agent (Fan-Out)
To give a task to an agent, you must first create a task using Gemini's native `tracker_create_task` tool to get a Task ID. Then, use the `agent start` command:
```bash
tmux-mgr agent start generalist --task-id 12345
```
*This command drops the `generalist` subagent directly into an isolated worktree and tells it to fulfill task `12345`.*

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
