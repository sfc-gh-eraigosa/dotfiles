# 🛠️ Dotfiles & Agent Environment

[![Docker Image CI](https://github.com/sfc-gh-eraigosa/dotfiles/actions/workflows/docker-image.yml/badge.svg)](https://github.com/sfc-gh-eraigosa/dotfiles/actions/workflows/docker-image.yml)
[![Gemini CLI](https://img.shields.io/badge/Gemini_CLI-0.42.0-blue)](https://geminicli.com)

A modernized, agent-first development environment for macOS and Linux. This repository bridges the gap between traditional terminal tools and **AI-assisted engineering**.

---

## ✨ Core Pillars

### 🤖 Agent-First Workflow
Native integration with [Gemini CLI](https://geminicli.com) turns your terminal into a collaborative workspace.
- **Custom Skills**: Specialized instructions for Git (`gss`), Tmux (`tmux-mgr`), and SSH management.
- **Slash Commands**: Instant health checks via `/gss`.
- **Automated Maintenance**: Use Gemini to manage, update, and troubleshoot your dotfiles autonomously.

### 🔄 Safe Repository Management (`gss`)
Stop worrying about broken rebases or lost work.
- **Safety Backups**: Every push triggers an automatic timestamped backup branch.
- **Workspace Scanning**: Instantly identify uncommitted changes across your entire `~/git` tree.

### 🪟 Terminal Introspection (`tmux-mgr`)
Gives your AI agents "eyes" into your terminal state.
- **Content Capture**: Agents can capture and analyze the history of any pane.
- **Layout Persistence**: Save and restore complex project environments with one command.

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
- **Agent Activation**: Gemini CLI is installed and pre-loaded with the custom skills from `src/`.

---

## 🛠️ Development with Gemini

This repository is designed to be maintained **with** Gemini. 

- **Syncing**: Tell Gemini: *"Sync my dotfiles"* — it will use `gss` to backup and push safely.
- **Updating**: Ask Gemini: *"Add a new alias to my zshrc"* or *"Fix my brew permissions"*.
- **Discovery**: Use the `/gss` command inside Gemini to see current status and help.

---

## 📂 Structure

| Path | Description |
| :--- | :--- |
| `opt/bin/` | Specialized scripts and binaries (in your `$PATH`). |
| `opt/profiles/` | Core shell and tool configurations. |
| `src/` | Source code for custom tools and **Agent Skills**. |
| `opt/conf/` | Templates for Gemini policies and commands. |

---

## ⌨️ Common Commands

| Command | Action |
| :--- | :--- |
| `/gss` | (Inside Gemini) Quick status summary and help. |
| `gss push` | Safely backup, sync, and push the current repo. |
| `tmux-mgr save` | Save current tmux window layout. |
| `vimw` / `vimg` | Open Vim with White or Green color profiles. |

---

## 📄 License

Licensed under the [Apache License, Version 2.0](LICENSE).

---
*Safeguards: 🛡️ Turn Break Mandate & 🛠️ OS-Level GSS Confirmation active.*
