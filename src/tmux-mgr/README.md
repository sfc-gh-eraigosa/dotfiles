# tmux-mgr 🧩

`tmux-mgr` is a powerful, Go-based management tool for `tmux`, designed to bridge the gap between your terminal and the Gemini CLI. It provides structured session management, window arrangements, and layout persistence, along with a dedicated Gemini skill for natural language control.

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
cd ~/git/dotfiles/src/tmux-mgr
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

## 🤖 Gemini CLI Integration

Once installed, you can talk to tmux via the Gemini CLI:
- `tmux: what's running in my other window?`
- `tmux: resize this pane to be 30% height`
- `tmux: list all my active sessions`

## 📄 Documentation

For detailed information on project requirements, implementation details, and coding standards, please refer to:
- [Requirements & Implementation](docs/requirements.md)

---
*Maintained in the `~/git/dotfiles` repository.*
