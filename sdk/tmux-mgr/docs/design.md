# Architectural Design: Integrated Agent Orchestration

This document outlines the architecture for `tmux-mgr` to act as a self-contained orchestrator for AI agent teams. The architecture uses an **Integrated Approach**, where `tmux-mgr` manages both OS-level isolation and agent execution within a single binary.

## The Integrated Approach

To solve the challenges of running multiple autonomous agents concurrently, `tmux-mgr` will now be responsible for the entire agent lifecycle, from environment setup to task execution.

1.  **File System Conflicts:** Solved by provisioning unique Git worktrees for each agent session.
2.  **Cognitive Coordination:** Solved by `tmux-mgr` self-invoking in a new "agent execution" mode, making it the central orchestrator.

This new model eliminates the fragile dependency on an external assistant binary (`agy`, formerly `gemini`) and avoids `PATH` and environment-related issues.

## 1. Orchestration and Isolation (`tmux-mgr`)

### 1.1 Command Flow
The entire agent workflow is now handled by `tmux-mgr` subcommands:

1.  **User Request:** The user initiates the process with `tmux-mgr agent start <agent-type> --task-description "Your task..."`.
2.  **Isolation:** The primary `tmux-mgr` process:
    *   Generates a unique session ID.
    *   Creates an isolated Git worktree at `~/.config/tmux-mgr/worktrees/<session-id>`.
    *   Splits the current tmux window to create a new pane.
3.  **Self-Invocation:** Inside the new pane, `tmux-mgr` launches a new instance of itself in agent execution mode: `tmux-mgr agent execute --task-description "..."`.
4.  **Execution:** The `tmux-mgr agent execute` process runs the agent's cognitive loop, performs the work, and writes its final findings to `RESULT.md` in its worktree.
5.  **Fan-In:** The user or primary agent can retrieve this result using `tmux-mgr agent complete <session-id>`.
6.  **Cleanup:** `tmux-mgr agent cleanup <session-id>` removes the worktree and session state files.

### 1.2 Session Tracking & State
Session state (ID, status, worktree path) continues to be managed via JSON files in `~/.config/tmux-mgr/sessions/`, ensuring that the lifecycle of each agent is tracked and can be cleaned up reliably.

## 2. Agent Execution (Internal)

The `tmux-mgr agent execute` command is the new entry point for the spawned agent.

### 2.1 Agent Logic
- This command will house the core logic for the agent. Initially, this can be a simple placeholder.
- In the future, this command will be responsible for:
    1.  Parsing the `--task-description`.
    2.  Interfacing with a hosted model (via the Antigravity CLI, a Go SDK, or direct API calls).
    3.  Executing tools (shell commands, file edits) as required by the task.
    4.  Writing the final, summarized result to `RESULT.md`.

### 2.2 Task Definition
- Tasks are now defined directly via the `--task-description` flag on the `start` command.
- This removes the need for an external tracker system like `tracker_create_task` for simple, fire-and-forget agent tasks. For more complex, multi-agent workflows, a primary agent could still coordinate tasks, but the execution primitive is now self-contained within `tmux-mgr`.

## Summary Workflow

1.  **Spawn:** User runs `tmux-mgr agent start generalist --task-description "Refactor the authentication module."`
2.  **Isolate & Self-Invoke:** `tmux-mgr` creates a worktree and a new tmux pane, then runs `tmux-mgr agent execute --task-description "..."` inside it.
3.  **Execute:** The spawned `tmux-mgr` process performs the refactoring task within its isolated worktree. Upon completion, it writes a summary to `RESULT.md`.
4.  **Fan-In:** User runs `tmux-mgr agent complete <session-id>` to view the `RESULT.md` summary.
5.  **Cleanup:** User runs `tmux-mgr agent cleanup <session-id>` to remove the worktree and session data.

## 3. Host-Aware Execution

The cognitive loop inside `tmux-mgr agent execute` dispatches to the AI CLI matching the host shell that launched the orchestration.

### 3.1 Detection
Detection happens **once in the parent** (`runAgentStart`) via `agent.DetectHost(os.Getenv)`:

- `CLAUDECODE=1` → `AssistantClaude`
- otherwise → `AssistantAntigravity` (default; preserves the original Gemini-era behavior — legacy `"gemini"` values normalize to antigravity)

The child (`runAgentExecute`) never re-detects — the inherited tmux environment is unreliable for this. Instead, the parent encodes its decision into the invocation command for the child.

### 3.2 Parent → Child Contract
The parent serializes two env vars into the shell command it hands to `tmux.CreatePane`:

| Var | Value |
|-----|-------|
| `TMUX_MGR_ASSISTANT` | `"claude"` or `"antigravity"` (legacy `"gemini"` is still accepted and normalized) |
| `TMUX_MGR_ASSISTANT_PATH` | absolute path of the assistant binary (from `exec.LookPath`), or the bare name as a fallback |

For backward compatibility the child still reads the legacy `GEMINI_PATH` env var when `TMUX_MGR_ASSISTANT_PATH` is unset and the host is Antigravity (Gemini CLI's successor).

### 3.3 Per-Host Cognitive Loops
- **`runAntigravityLoop`**: runs `agy -p "<instruction>" --dangerously-skip-permissions` with the existing model-fallback chain (`gemini-3.1-pro-preview` → `gemini-2.5-pro` → `gemini-2.5-flash`) on quota errors. Instruction is plain task text plus the `RESULT.md` write mandate (the Gemini-era `@generalist` extension prefix is gone).
- **`runClaudeLoop`**: runs `claude -p "<instruction>" --dangerously-skip-permissions` once. No model fallback (Claude doesn't surface quota errors in a string-matchable way; one-shot semantics are cleaner). Instruction is plain task text plus the `RESULT.md` write mandate.

Both branches write `RESULT.md` in the worktree, so `runAgentComplete` is host-agnostic.