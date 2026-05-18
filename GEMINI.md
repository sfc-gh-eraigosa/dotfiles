# Dotfiles Repository

Welcome to the dotfiles repository. This file serves as the entry point for agent-based discovery of the tools, configurations, and workflows contained within this repo. It is read by both **Gemini CLI** (as `GEMINI.md`) and **Claude Code** (as `CLAUDE.md` — a symlink to this file).

## Repository Structure

- `opt/bin/`: A collection of utility scripts and binaries. [See opt/bin/GEMINI.md](./opt/bin/GEMINI.md) for a categorized registry of these tools.
- `opt/profiles/`: Shell configuration files (.zshrc, .bashrc, .tmux.conf, etc.). [See opt/profiles/GEMINI.md](./opt/profiles/GEMINI.md) for details.
- `opt/docs/`: Legacy and reference documentation for various tools and setups. [See opt/docs/GEMINI.md](./opt/docs/GEMINI.md).
- `src/`: Source code for custom tools and agent skills. [See src/GEMINI.md](./src/GEMINI.md).
- `ai/gemini/`: Gemini-specific commands, TOML policies, and settings.
- `ai/claude/`: Claude-specific commands, settings, and the `safety_guard.sh` PreToolUse hook.

## Usage Guidelines

- **Tool Discovery**: Check `opt/scripts/GEMINI.md` for available shell scripts.
- **Configuration**: Shell profiles and aliases are maintained in `opt/profiles/`.
- **Progressive Loading**: Only read subdirectory `GEMINI.md` (or `CLAUDE.md` — same file) when specifically needing information about that section, to conserve context.
- **Skills are shared**: `SKILL.md` files under `src/*/skill/` and `src/ssh-*/` drive both assistants. Edit once, benefit twice.

## Portability & Best Practices

- **Use $HOME**: Always use `${HOME}` or `~` instead of absolute home paths (e.g., `/Users/eraigosa`) in scripts, aliases, and configuration files to ensure they are portable across different systems and users.
- **Avoid Hardcoded Paths**: Use relative paths or environment variables (like `BASE_DIR` in `install.sh`) whenever possible.
