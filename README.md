# 🛠️ Dotfiles & Agent Environment

[![Docker Image CI](https://github.com/sfc-gh-eraigosa/dotfiles/actions/workflows/docker-image.yml/badge.svg)](https://github.com/sfc-gh-eraigosa/dotfiles/actions/workflows/docker-image.yml)
[![Antigravity CLI](https://img.shields.io/badge/Antigravity_CLI-1.0.16-blue)](https://antigravity.google)
[![Claude Code](https://img.shields.io/badge/Claude_Code-supported-orange)](https://claude.com/claude-code)

An agent-first development environment for macOS and Linux.

Two things live here: a **shared context and safety layer** that lets
[Antigravity CLI](https://antigravity.google) (`agy`) and
[Claude Code](https://claude.com/claude-code) work from the same skills, hooks,
and rules — and **[the SDK](sdk/README.md)**, a set of small Go tools that make
letting an agent actually *do* things survivable.

---

## 🧰 The SDK

Seven single-binary Go tools covering the loop: *let an agent work → let it
commit without losing anything → see what's happening → roll it out everywhere.*

| Tool | Reach for it when… |
| :--- | :--- |
| [`gss`](sdk/README.md#-gss--never-lose-work-to-git-again) | An agent is about to touch git |
| [`tmux-mgr`](sdk/README.md#-tmux-mgr--run-five-agents-at-once) | One agent isn't fast enough |
| [`gsl`](sdk/README.md#-gsl--know-what-your-agent-is-costing-you) | You can't tell how much context is left |
| [`fleet`](sdk/README.md#-fleet--is-every-machine-actually-updated) | You have more machines than you can `ssh` into |
| [`gff`](sdk/README.md#-gff--flags-that-live-in-git) | "Skip that step on this machine" |
| [`wol`](sdk/README.md#-wol--turn-it-on-from-anywhere) | The machine you need is powered off |
| [`libs`](sdk/README.md#-libs--the-shared-foundation) | You're writing tool #8 |

→ **[What each tool solves, with demos](sdk/README.md)**

---

## 🚀 Quick Start

```bash
git clone https://github.com/sfc-gh-eraigosa/dotfiles.git ~/git/dotfiles
~/git/dotfiles/install.sh
```

The installer is **interactive** — prompts are front-loaded, but on Windows/WSL
the Desktop deploy runs at the end, so stay nearby. It symlinks core configs
(`.zshrc`, `.tmux.conf`, …) into `$HOME`, initializes `nvm`/`pyenv`/`goenv`/`rbenv`,
builds the SDK binaries into `~/opt/bin/`, and installs both assistants
pre-loaded with the shared skills.

---

## 🤖 Agent Configuration

Both assistants read the **same** skills, hooks, and progressive context — write
a skill once, use it in either.

- **Shared skills** — every `SKILL.md` is linked into both assistants by `sync-skills`. [ai/skills/](ai/skills/AGENTS.md)
- **Safety hooks** — the same guard scripts block `rm -rf *` and gate `gss push`. [ai/hooks/](ai/hooks/)
- **Agent teams** — specialized personas installed as native subagents. [ai/teams/](ai/teams/AGENTS.md)
- **Declarative plugins** — extensions defined in code, auto-synced. [docs/ai-plugins.md](docs/ai-plugins.md)
- **Progressive context** — `AGENTS.md` at each level (`CLAUDE.md` is a symlink to it).

**Choosing between them:** Claude Code for long-context multi-file refactors and
slash-command workflows; Antigravity for fast tool-call loops and repos with
`.agents/` config. Either works headless in a tmux pane. You can switch
mid-project — they read the same context.

---

## 📂 Structure

| Path | Description |
| :--- | :--- |
| [`sdk/`](sdk/README.md) | **Go tools** — `gss`, `tmux-mgr`, `gsl`, `fleet`, `gff`, `wol`, `libs`. |
| `opt/bin/` · `opt/profiles/` | Utility scripts (on `$PATH`) and shell configuration. |
| `src/` | Non-Go tooling and agent skills. |
| [`ai/`](ai/) | Assistant config: skills, hooks, teams, plugin manifest. |
| [`docs/`](docs/AGENTS.md) | Documentation; design work starts in [`docs/mbo/`](docs/mbo/AGENTS.md). |

---

## ⌨️ Common Commands

| Command | Action |
| :--- | :--- |
| `/gss` · `/gss-scan` · `/gss-pr` | Repo status · find dirty repos · open a PR |
| `/tmux-agent` | Spawn / list / fan-in parallel agents |
| `/ssh-find <alias>` · `/ssh-keys` | Discover a host's IP · manage keys across hosts |
| `gss push` | Safely backup, sync, and push |
| `fleet status` · `fleet tui` | Which hosts are out of sync · interactive dashboard |

---

## 🖥️ Machine-Local Customization

Host-specific settings that would break other machines go in `~/.zshrc.local` —
sourced last, overrides everything, never tracked.
See [docs/machine-local-overrides.md](docs/machine-local-overrides.md).

---

## 🤝 Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and how to
validate changes with the `Makefile`. Merges go through the Mergify queue —
[docs/mergify.md](docs/mergify.md).

Licensed under the [Apache License, Version 2.0](LICENSE).

---
*Safeguards: 🛡️ Turn Break Mandate, 🛠️ OS-Level GSS Confirmation, 🧪 27-Case Hook Test Suite, 🧪 Automated CI Validation.*
