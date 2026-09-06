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

## 🍎 macOS-Style Keyboard (on by default)

**Heads up:** this repo deliberately makes every machine's keyboard behave like a
Mac. `install.sh` turns this on by default, so a fresh install *will* change your
shortcuts. The goal is one set of muscle memory everywhere — the same `Cmd+C`
works on the Windows box, the Linux desktop, and the Mac.

**The Command key is `Super`** (the ⊞/⌘ key next to Alt). `Ctrl` is left alone, so
`Ctrl+C` is still SIGINT in a terminal.

| Shortcut | Action | | Shortcut | Action |
| :--- | :--- | :-- | :--- | :--- |
| `Cmd+C` `V` `X` | Copy / paste / cut | | `Cmd+←` `→` | Start / end of line |
| `Cmd+A` `Z` `⇧Z` | Select all / undo / redo | | `Cmd+↑` `↓` | Top / bottom of document |
| `Cmd+S` `F` `G` | Save / find / find next | | `Cmd+⇧+←→↑↓` | …and select while moving |
| `Cmd+T` `⇧T` `W` | New tab / reopen / close | | `Opt+⌫` | Delete previous word |
| `Cmd+1…9` | Jump to tab N | | `Cmd+⌫` | Delete to start of line |
| `Cmd+Q` `M` `H` | Quit / minimize / hide | | `Cmd+[` `]` | Browser back / forward |
| `Cmd+Tab` | Switch application | | `Cmd+Opt+←→↑↓` | Tile / maximize window |
| `Cmd+Space` | Spotlight (Activities search) | | `Cmd+L` | Lock screen |
| *(tap `Cmd` alone)* | Nothing, as on macOS | | | |

In a **terminal** the destructive chords are handled for you: `Cmd+C` copies
(never SIGINT), and `Cmd+S` / `Cmd+Z` / `Cmd+D` are inert rather than freezing,
suspending, or logging out of your shell.

### Turning it off

One flag covers **every OS** — Linux (keyd + GNOME) and Windows (AutoHotkey +
the Copilot-key remap):

```bash
gff set keyboard.macos.enabled false && ./install.sh   # leave every keyboard stock
```

To remove it from a Linux machine that already has it:

```bash
~/opt/scripts/system/macos-keys-linux.sh --uninstall
```

If a remap ever misbehaves, hold **Backspace + Esc + Enter** together to
panic-quit the keyd daemon and get a stock keyboard back instantly.

Full key table, platform coverage, and troubleshooting:
[docs/macos-keys.md](docs/macos-keys.md).

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
