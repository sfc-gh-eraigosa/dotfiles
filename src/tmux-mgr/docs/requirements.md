# tmux-mgr Requirements and Implementation

This document outlines the requirements for the `tmux-mgr` tool and how each is met in the implementation.

## Requirements Checklist

| Requirement | Description | Implementation Status |
| :--- | :--- | :--- |
| **1. Help & Documentation** | Explanation of all features via CLI help and docs. | **Met**: `tmux-mgr help` provides detailed usage; `README.md` and `SKILL.md` offer further guidance. |
| **2. Window Arrangements** | Move focus (L/R/U/D), resize (L/R/U/D), and percentage-based sizing. | **Met**: `window move` and `window resize` commands handle focus and directional/percentage resizing. |
| **3. Save States** | Save/Restore layouts to/from `$HOME/.config/tmux-mgr`. | **Met**: `save` and `restore` commands persist window names and layouts as JSON files in `~/.config/tmux-mgr/`. |
| **4. Session Management** | List, create, attach, and kill tmux sessions. | **Met**: `session [list\|new\|attach\|kill]` provides full lifecycle management. |
| **5. Windowing Desktop** | Navigate between windows/layouts easily. | **Met**: `desktop [list\|switch]` allows quick navigation by window name or index. |
| **6. Gemini CLI Skill** | "tmux" skill for natural language control and introspection. | **Met**: `SKILL.md` instructs Gemini on using `tmux-mgr` for management and `capture` for introspection. |
| **7. Go Implementation** | Written in Go, compiled to `opt/bin`. | **Met**: Source in `src/tmux-mgr`, binary installed to `opt/bin/tmux-mgr`. |
| **8. Build System** | Script to compile and install without checking in binary. | **Met**: `build.sh` handles compilation and skill linking; `.gitignore` excludes the binary. |
| **9. Repository Structure** | Source and resources in `~/git/dotfiles/src/tmux-mgr`. | **Met**: All code, build scripts, docs, and skills are encapsulated in the requested directory. |

## Implementation Details

### Window Management & Resizing
The tool uses `tmux select-pane` for movement and `tmux resize-pane` for resizing. For percentage-based resizing, the tool queries the terminal dimensions (`terminal_width`/`terminal_height`) and calculates absolute values for `-x` and `-y` flags.

### Layout Persistence
Layouts are stored as JSON objects containing window indices, names, and their corresponding tmux layout strings (retrieved via `#{window_layout}`). This ensures that `restore` can accurately recreate the visual arrangement.

### Gemini Introspection
The `capture` command uses `tmux capture-pane -pt` to stream the contents of any pane directly to the Gemini CLI. This allows the model to "see" and analyze what is happening in background windows or other sessions.

### Installation & Build
The `build.sh` script is designed to be idempotent and safe. It checks for the `go` compiler before attempting a build and manages the symlinking of the Gemini skill to the user's global `.agents/skills` directory.

## Coding Standards and Go Best Practices

The `tmux-mgr` implementation adheres to high-quality software engineering standards and idiomatic Go practices:

### 1. Project Layout (Standard Go Project Layout)
- **`main.go`**: Sparce entry point that delegates execution to the command package.
- **`cmd/`**: Contains the CLI implementation using the `cobra` library. Each subcommand is isolated in its own file (`session.go`, `window.go`, etc.) for better maintainability.
- **`pkg/`**: Core business logic is encapsulated in `pkg/tmux`, allowing the `Manager` struct and its methods to be reused independently of the CLI.

### 2. Separation of Concerns
- The CLI layer (`cmd/`) is strictly responsible for parsing flags and arguments.
- The business logic layer (`pkg/tmux`) handles the actual interaction with tmux, layout management, and data structures.

### 3. Error Handling
- Functions in the `pkg` package return meaningful errors rather than logging and exiting.
- Errors are wrapped with context (e.g., `fmt.Errorf("...: %w", err)`) to provide a clear trace of failure.

### 4. Logging and Introspection
- Centralized logging to `~/.config/tmux-mgr/tmux-mgr.log` ensures that all tool actions are traceable without cluttering the user's terminal.
- A `--verbose` flag enables execution tracing, logging the exact `tmux` commands being run.

### 5. Idiomatic Go
- Use of standard library features where appropriate.
- Adherence to Go naming conventions (e.g., PascalCase for exported symbols, camelCase for internal ones).
- Clean, documented code with clear struct definitions and method receivers.
