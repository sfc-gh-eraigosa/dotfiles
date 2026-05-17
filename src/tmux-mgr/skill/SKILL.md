---
name: tmux
description: A powerful, Go-based management tool for tmux, providing structured session management, window arrangements, layout persistence, and autonomous AI Agent Team Orchestration.
---
# Tmux Management Skill

Orchestrate AI agent teams and manage tmux windows/sessions.

## 1. Agent Teams (Isolated Parallel Work)
`tmux-mgr` provides OS-level isolation (Git worktrees + panes). **Always** use native Gemini tools (e.g. `tracker_create_task`) for planning and `tmux-mgr agent` for execution.

- **Start Agent**: `tmux-mgr agent start <agent-name> --task-id <task-id>`
- **Check Progress**: `tmux-mgr agent list`
- **Get Results**: `tmux-mgr agent complete <session-id>` (Reads `RESULT.md` from the worktree)
- **Cleanup**: `tmux-mgr agent cleanup <session-id>`

## 2. Window/Pane Management
- **Split**: `tmux-mgr window split [horizontal|vertical]`
- **Move**: `tmux-mgr window move [left|right|up|down]`
- **Resize**: `tmux-mgr window resize [left|right|up|down] [val]` or `width 50%`
- **Capture**: `tmux-mgr capture [target]` (returns pane content)

## 3. Session Persistence
- **Lifecycle**: `tmux-mgr session [list|new|attach|kill]`
- **Layouts**: `tmux-mgr [save|restore] <name>` (persists to `~/.config/tmux-mgr/`)

## Guidelines
- **Conversation**: "I'll spawn an agent to handle the backend in a separate pane."
- **Efficiency**: Use `tmux-mgr agent` when user wants parallel execution or visual terminal feedback.
- **Fan-In**: Sub-agents MUST write summaries to `RESULT.md`. Retrieve with `agent complete`.