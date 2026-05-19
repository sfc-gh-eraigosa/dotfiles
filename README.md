# 🛠️ Dotfiles & Agent Environment

[![Docker Image CI](https://github.com/sfc-gh-eraigosa/dotfiles/actions/workflows/docker-image.yml/badge.svg)](https://github.com/sfc-gh-eraigosa/dotfiles/actions/workflows/docker-image.yml)
[![Gemini CLI](https://img.shields.io/badge/Gemini_CLI-0.42.0-blue)](https://geminicli.com)
[![Claude Code](https://img.shields.io/badge/Claude_Code-supported-orange)](https://claude.com/claude-code)

A modernized, agent-first development environment for macOS and Linux. This repository bridges the gap between traditional terminal tools and **AI-assisted engineering** — and supports more than one assistant.

---

## ✨ Core Pillars

### 🤖 Multi-Assistant Workflow
First-class integration with both [Gemini CLI](https://geminicli.com) and [Claude Code](https://claude.com/claude-code). Each assistant reads the **same** skills, the **same** progressive context, and the **same** safety rules — driven by a single source of truth in `src/` and `ai/`.
- **Custom Skills**: Specialized instructions for Git (`gss`), Tmux (`tmux-mgr`), and SSH management — shared across assistants via symlinks.
- **Slash Commands**: `/gss`, `/gss-scan`, `/gss-pr`, `/tmux-agent`, `/ssh-find`, `/ssh-keys` (Claude); `/gss` (Gemini).
- **Safety Hooks**: A `PreToolUse` hook (Claude) and TOML policies (Gemini) enforce the same rules — block `rm -rf *`, require explicit confirmation for `gss push`, etc.
- **Continuous Validation**: CI/CD pipeline runs unit tests, integration tests, and a 27-case hook test suite on every push.

### 🔄 Safe Repository Management (`gss`)
Stop worrying about broken rebases or lost work.
- **Safety Backups**: Every push triggers an automatic timestamped backup branch.
- **Workspace Scanning**: Instantly identify uncommitted changes across your entire `~/git` tree.
- **Approval-Token Handshake**: Assistants cannot push autonomously — a fresh `~/.config/gss/approval.token` is required and matched against current `HEAD`.

### 🪟 Terminal Introspection (`tmux-mgr`)
Gives your AI agents "eyes" into your terminal state.
- **Content Capture**: Agents can capture and analyze the history of any pane.
- **Layout Persistence**: Save and restore complex project environments with one command.
- **Parallel Agent Orchestration**: `tmux-mgr agent start` provisions isolated git worktrees for fan-out work.

---

## 🚀 Quick Start

### 1. Installation
Clone the repo and run the idempotent installer:

```bash
git clone https://github.com/sfc-gh-eraigosa/dotfiles.git ~/git/dotfiles
~/git/dotfiles/install.sh
```

### 2. What to Expect
- **Seamless Linking**: Core configs (`.zshrc`, `.tmux.conf`, etc.) are symlinked to your `$HOME`.
- **Toolchain Readiness**: `nvm`, `pyenv`, `goenv`, and `rbenv` are initialized and ready.
- **Assistant Activation**: Both Gemini CLI and Claude Code are installed and pre-loaded with the custom skills from `src/`.

---

## 🤔 Choosing Your Assistant

Both Gemini and Claude have access to the same skills and tools — pick by workflow fit.

| Use case | Best fit | Why |
| :--- | :--- | :--- |
| Long-context refactor across many files | **Claude Code** | Larger context window, strong multi-file editing |
| Fast tool-call loops with short prompts | **Gemini CLI** | Lower latency per turn |
| Headless autonomous runs in tmux panes | Either | `tmux-mgr agent start` works with both |
| Slash-command driven workflows | **Claude Code** | More slash commands wired (`/gss-scan`, `/gss-pr`, `/tmux-agent`, `/ssh-find`, `/ssh-keys`) |
| Repos with `.gemini/` policies already present | **Gemini CLI** | Native TOML policy support |

You can switch between them in the same project — they read the same `GEMINI.md`/`CLAUDE.md` context (the latter is a symlink to the former).

---

## 🛠️ Development with Your Assistant

This repository is designed to be maintained **with** your AI assistant.

- **Syncing**: *"Sync my dotfiles"* — the assistant uses `gss` to backup and push safely (always asks first).
- **Updating**: *"Add a new alias to my zshrc"* or *"Fix my brew permissions"*.
- **Discovery**: Use `/gss` in either assistant for current repo status; Claude also has `/gss-scan`, `/gss-pr`.

---

## 📂 Structure

| Path | Description |
| :--- | :--- |
| `opt/bin/` | Specialized scripts and binaries (in your `$PATH`). |
| `opt/profiles/` | Core shell and tool configurations. |
| `src/` | Source code for custom tools and **Agent Skills** (`SKILL.md` files, shared across assistants). |
| `ai/gemini/` | Gemini-specific commands, policies, settings. |
| `ai/claude/` | Claude-specific commands, settings, hooks. |
| `GEMINI.md` / `CLAUDE.md` | Progressive context (CLAUDE.md is a symlink to GEMINI.md at each level). |

---

## 🖥️ Machine-Local Customization

Some settings differ per host — a different CLI launcher, work credentials, or
function overrides that would break other machines if committed. Use
`~/.zshrc.local` for this: it is sourced last by `.zshrc`, overrides anything
in the shared config, and is never tracked in this repo.

See **[docs/machine-local-overrides.md](docs/machine-local-overrides.md)** for
usage and examples.

---

## ⌨️ Common Commands

| Command | Action |
| :--- | :--- |
| `/gss` | (In either assistant) Quick status summary and help. |
| `/gss-scan` | (Claude) Find dirty repos across `~/git`. |
| `/gss-pr` | (Claude) Create a feature branch and open a PR. |
| `/tmux-agent` | (Claude) Spawn / list / fan-in parallel agents. |
| `/ssh-find <alias>` | (Claude) Discover SSH host IP on local subnet. |
| `/ssh-keys` | (Claude) List / generate / delete keys across hosts. |
| `gss push` | Safely backup, sync, and push the current repo. |
| `tmux-mgr save` | Save current tmux window layout. |
| `vimw` / `vimg` | Open Vim with White or Green color profiles. |

---

## 🤝 Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details on our development workflow, testing procedures, and how to use the `Makefile` to validate your changes.

---

## 📄 License

Licensed under the [Apache License, Version 2.0](LICENSE).

---
*Safeguards: 🛡️ Turn Break Mandate, 🛠️ OS-Level GSS Confirmation, 🧪 27-Case Hook Test Suite, and 🧪 Automated CI Validation active.*
