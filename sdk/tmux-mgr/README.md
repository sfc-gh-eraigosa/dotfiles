# tmux-mgr 🧩

`tmux-mgr` is a powerful, Go-based management tool for `tmux`, designed to bridge the gap between your terminal and your AI CLI (Gemini or Claude Code). It provides structured session management, window arrangements, layout persistence, and a host-aware Agent Team Orchestrator — spawned panes run `claude` when invoked from Claude Code and `gemini` otherwise, automatically.

## 🚀 Key Features

- **Structured CLI**: Built with `cobra` for a professional, subcommand-based experience.
- **Session Management**: Full lifecycle control (`list`, `new`, `attach`, `kill`).
- **Smart Windowing**: Directional focus and percentage-based resizing (e.g., `width 50%`).
- **Layout Persistence**: Save and restore complex window arrangements to `~/.config/tmux-mgr/`.
- **Introspection**: A `capture` feature that allows Gemini to "see" into any tmux pane.
- **Logging**: All actions are logged to `~/.config/tmux-mgr/tmux-mgr.log` for troubleshooting.

## 🛠 Installation

`tmux-mgr` is integrated into the `dotfiles` repository. 

### Standard Install
Run the main installation script from the repository root:
```bash
cd ~/git/dotfiles
./install.sh
```

### Manual/Developer Build
If you have Go installed, you can build and install the tool and its Gemini skill directly:
```bash
cd ~/git/dotfiles/sdk/tmux-mgr
./build.sh
```
*This will install the binary to `~/opt/bin/tmux-mgr` and link the Gemini skill to `~/.agents/skills/tmux`.*

## 📖 Getting Started

### 1. Explore the Help
The best way to learn `tmux-mgr` is through its built-in help system:
```bash
tmux-mgr --help
tmux-mgr session --help
tmux-mgr window resize --help
```

### 2. Start a New Session
Create and attach to a new managed session:
```bash
tmux-mgr session new my-project
tmux-mgr session attach my-project
```

### 3. Arrange Your Windows
Once inside tmux, use `tmux-mgr` to move focus or resize panes:
```bash
# Move focus left
tmux-mgr window move left

# Make current pane exactly 50% width
tmux-mgr window resize width 50%
```

### 4. Save Your Work
If you love your current layout, save it for later:
```bash
tmux-mgr save dev-layout
```
*Restore it anytime with `tmux-mgr restore dev-layout`.*

## 🤖 AI CLI Integration

Once installed, you can talk to tmux via either Gemini CLI or Claude Code:
- `tmux: what's running in my other window?`
- `tmux: resize this pane to be 30% height`
- `tmux: list all my active sessions`

When you spawn agents with `tmux-mgr agent start`, the assistant inside each pane is auto-selected based on the host CLI:
- Inside Claude Code (`CLAUDECODE=1`): each pane runs `claude -p ... --dangerously-skip-permissions`. The spawned Claude can use its native `Task()` subagents for further fan-out.
- Otherwise: each pane runs `gemini -y -p ...` with model fallback.

No flags or config — detection is automatic.

## 📄 Documentation

For detailed information on project requirements, implementation details, user guides, and coding standards, please refer to:
- [User Guide](docs/user_guide.md)
- [Requirements & Architecture](docs/requirements.md)
- [Agent Coordination Design](docs/design.md)

---
*Maintained in the `~/git/dotfiles` repository.*
