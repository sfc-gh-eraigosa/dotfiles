---
name: tmux
description: A powerful, Go-based management tool for tmux, providing structured session management, window arrangements, and layout persistence.
---
# Tmux Management Skill

This skill provides expertise in managing tmux sessions and windows using the `tmux-mgr` tool. It allows the Gemini CLI to interact with tmux sessions, manage layouts, and introspect pane content using natural language.

## Capabilities

The `tmux-mgr` tool is the primary interface for this skill. It is located in `~/opt/bin/tmux-mgr`.

### 1. Session Management
- **List sessions**: `tmux-mgr session list`
- **Create new session**: `tmux-mgr session new [name]`
- **Attach to session**: `tmux-mgr session attach [name]`
- **Kill session**: `tmux-mgr session kill [name]`

### 2. Window & Pane Arrangements
- **Move focus**: `tmux-mgr window move [left|right|up|down]`
- **Resize panes**: 
  - Incremental: `tmux-mgr window resize [left|right|up|down] [val]`
  - Percentage: `tmux-mgr window resize width 50%` or `tmux-mgr window resize height 25%`

### 3. Layout Persistence
- **Save layout**: `tmux-mgr save [name]` (saves to `~/.config/tmux-mgr/[name].json`)
- **Restore layout**: `tmux-mgr restore [name]` (restores window layout and names)

### 4. Desktop Navigation
- **List windows**: `tmux-mgr desktop list`
- **Switch window**: `tmux-mgr desktop switch [name|index]`

### 5. Introspection (Eyes for Gemini)
- **Capture content**: `tmux-mgr capture [target]` (returns the text content of a pane)
- Use this to understand what's running in other windows or to troubleshoot terminal output.

## Guidelines for Natural Language Interaction

When a user asks you to "manage tmux" or "fix my windows," use these strategies:
- **Be Conversational**: "I can help you arrange your tmux windows. Should I move this pane to the right or resize it?"
- **Interpret Intent**: If the user says "make this wider," use `tmux-mgr window resize right 10`. If they say "half width," use `tmux-mgr window resize width 50%`.
- **Introspect First**: If asked "what's going on in the other session?", use `tmux-mgr capture` before answering.
- **Cheap & Efficient**: This skill is designed to be "cheap" by using direct shell commands. Any Gemini model (Flash or Pro) can handle these tasks effectively.

## Help for New Users
If a user is unfamiliar with the tool, explain that it's a Go-based manager built into their dotfiles that helps bridge the gap between their terminal and the Gemini CLI.
