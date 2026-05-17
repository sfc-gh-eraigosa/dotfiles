# tmux-mgr Project Documentation

Welcome to the `tmux-mgr` project. This tool is a Go-based CLI for managing tmux sessions, layout persistence, and most notably, orchestrating multi-agent teams using Git worktrees and isolated `gemini-cli` panes.

## Documentation Structure

For maintaining codebase standards, user guides, and architecture, refer to the following documents located in the `docs/` directory:

- [Requirements & Architecture](./docs/requirements.md): Details the tool's requirements, including Agent Orchestration, TDD standards, and Go implementation specifics.
- [Design Details](./docs/design.md): In-depth architectural designs for complex features (e.g., the Agent Team Orchestration plan).
- [User Guide](./docs/user_guide.md): The primary manual on how to use `tmux-mgr`, covering session management, window resizing, and the full Fan-Out/Fan-In agent workflow.

## Skill Integration

This project compiles not only a CLI but also exports a Gemini Skill to instruct autonomous agents on how to leverage it. 

- [Agent Skill Instructions](./skill/SKILL.md): The core prompt injected into Gemini when the "tmux" skill is activated.

## Core Mandates for Modifying `tmux-mgr`

1. **Test-Driven Development (TDD)**: All changes MUST maintain a minimum test coverage of >60%. See `src/GEMINI.md` for broader repository standards. 
2. **Standard Go Layout**: Follow the existing structure (`cmd/` for CLI handlers, `pkg/` for reusable business logic).
3. **Docs Maintenance**: Whenever a new feature is added (like a new CLI subcommand), ensure the `docs/user_guide.md` and `README.md` are updated accordingly.