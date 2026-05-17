# Architectural Design: Hybrid Agent Orchestration

This document outlines the architecture for adapting the `tmux-mgr` CLI to orchestrate AI agent teams. The architecture employs a **Hybrid Approach**, cleanly dividing responsibilities between OS-level isolation and cognitive state management.

## The Hybrid Approach

When spawning multiple autonomous agents concurrently on the same project, two distinct challenges arise:
1.  **File System Conflicts:** Parallel processes modifying the same files will cause race conditions and data corruption.
2.  **Cognitive Coordination:** Agents need to know what to do, report their status, and share findings.

**The Solution:**
*   `tmux-mgr` handles **Physical Isolation**: It provisions unique Git worktrees and spawns isolated tmux panes.
*   The `gemini-cli` handles **Cognitive Coordination**: It uses its native `.gemini/agents/` routing, `invoke_agent`, and `tracker_*` tools to maintain a Directed Acyclic Graph (DAG) of tasks.

## 1. OS-Level Isolation (`tmux-mgr`)

### 1.1 Isolated Worktree Setup
When an agent team member is spawned, `tmux-mgr` ensures a clean environment:
1.  Generates a unique session ID (e.g., `generalist-1678886400`).
2.  Creates a new `git worktree` branched from the current repository state.
3.  **Destination:** `~/.config/tmux-mgr/worktrees/<session-id>`
4.  This isolation guarantees that parallel agents can edit code, run tests, and format files without breaking the primary workspace or other agents.

### 1.2 Tmux Pane Management
1.  `tmux-mgr` executes `tmux split-window` to create a new, visible pane in the current window.
2.  The pane drops the agent directly into its isolated worktree.

### 1.3 Session Tracking & Cleanup
1.  To ensure no orphaned worktrees are left behind, `tmux-mgr` tracks active sessions by writing JSON state files to `~/.config/tmux-mgr/sessions/`.
2.  The `tmux-mgr agent cleanup <session-id>` command is responsible for forcing the removal of the Git worktree and deleting the tracking file once the task is complete.

## 2. Cognitive Coordination (Native Gemini)

We intentionally do *not* implement custom task files (like `TASK.md`) or custom agent routing in `tmux-mgr`. Instead, the invocation bridges to native Gemini features.

### 2.1 Agent Invocation
The `tmux-mgr agent start` command bridges the gap by formatting the correct shell command for the new pane:
*   `gemini-cli --agent <agent-name> --task <task-id>`
*   This relies on Gemini's built-in subagent definitions (`.gemini/agents/*.md`), eliminating the need for duplicate configuration.

### 2.2 Task Tracking & Results (The Fan-In)
While the primary agent uses the native Gemini `tracker` for high-level status, a robust **shell-level Fan-In** is required to retrieve the sub-agent's detailed findings:

1.  **Standardized Output:** Every spawned agent is instructed to write its final summary to a `RESULT.md` file in the root of its isolated worktree.
2.  **Retrieval:** The primary agent uses the `tmux-mgr agent complete <session-id>` command. This command reads the `RESULT.md` from the corresponding worktree and prints it to the console, allowing the primary agent to "roll up" the findings.

## 3. Reliability & Visibility
To ensure the user can visually monitor agents:
*   `tmux-mgr` uses a simplified `tmux split-window -v` targeting the current window, ensuring new panes are always visible and positioned predictably.
*   Agents are launched with the `gemini-cli`, which remains open until the task is complete, allowing the user to see the "live" reasoning process.

## Summary Workflow

1.  **Plan:** Primary agent runs `tracker_create_task` -> yields Task ID `123`.
2.  **Spawn:** Primary agent runs `tmux-mgr agent start generalist --task-id 123`.
3.  **Isolate:** `tmux-mgr` creates worktree, splits pane, and runs `gemini-cli --agent generalist --task 123`.
4.  **Execute:** Subagent does work in the worktree, natively updates tracker state.
5.  **Fan-In:** Primary agent sees task `123` is done in its tracker.
6.  **Cleanup:** Primary agent runs `tmux-mgr agent cleanup <session-id>` to remove the worktree.