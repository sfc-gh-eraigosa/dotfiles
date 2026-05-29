---
name: claude-remote
description: Launch a new tmux session running `claude --remote-control` inside a chosen GitHub repo (scans $HOME/github, pins $HOME/git/dotfiles, can clone a new repo into $HOME/github). Use when the user wants a stable LOCAL host running Claude Code that they can drive from claude.ai or the Claude mobile app.
---

# Claude Remote-Control Launcher

This skill spawns a fresh **tmux session** anchored in a chosen GitHub repo and starts **`claude --remote-control`** inside it, exposing a *local* Claude Code session that can be monitored and steered from [claude.ai/code](https://claude.ai/code) or the Claude mobile app.

> `--remote-control` runs Claude on the user's **own hardware** (stable host) and surfaces the session to the web. This is the opposite of `--remote`, which spawns a *new cloud sandbox*. If the user actually wants a cloud session, use `claude --remote "<task>"` directly — this skill is not the right tool.

## When to use this skill

Trigger phrases:

- "Start a remote-controllable Claude session"
- "Pin a Claude session on my laptop so I can drive it from my phone"
- "Open Claude in `<repo>` as a remote-control host"
- "Run Claude on this machine but let me steer it from the web"

## Prerequisites

| Requirement | How it's met |
| --- | --- |
| `tmux`, `git`, `claude` on `$PATH` | Brewfile installs `cask 'claude-code'`; tmux/git ship with `install.sh`. |
| `~/opt/scripts/system/claude-remote` launcher | Provided by this skill. `install.sh` symlinks `${HOME}/opt` to the repo's `opt/`, so the script is reachable at that path with no extra wiring. |
| `claude` shell function in scope | Sourced from `ai/claude/aliases.sh` via `.bashrc` / `.zshrc`. Adds YOLO toggle + tmux pane anchoring on top of the bare `claude` binary. |
| (optional) `fzf` | If present, used as the picker; otherwise the script falls back to a numbered `select` menu. |

## Discovery sources

The launcher offers the user, in this order:

1. Every git repo under **`$HOME/github/`** (scanned to depth 3).
2. **`$HOME/git/dotfiles`**, pinned.
3. A **`[clone new repo into $HOME/github]`** entry that prompts for `owner/repo` or a full URL and clones into `$HOME/github/<name>` before launching.

## Steps

### 1. Confirm intent

If the user hasn't already named a repo, ask whether they want to:

- pick an existing repo from the menu,
- target a specific repo by path (pass it as the argument), or
- clone a new GitHub repo (pass `owner/repo` or a URL as the argument).

If they've named one, skip the picker by passing it as the first argument.

### 2. Run the launcher

```bash
~/opt/scripts/system/claude-remote                          # interactive picker
~/opt/scripts/system/claude-remote ~/github/some-repo       # explicit path
~/opt/scripts/system/claude-remote owner/repo               # clone-and-launch shorthand
~/opt/scripts/system/claude-remote https://github.com/o/r   # clone-and-launch (full URL)
```

For convenience, `~/opt/scripts/system/` is not on `$PATH`; either use the absolute path above, or add a thin alias / symlink (e.g., `ln -s ~/opt/scripts/system/claude-remote ~/opt/bin/claude-remote` if your local `opt/bin` is writable).

### 3. What the launcher does

1. Scans `$HOME/github/` for git repos and pins `$HOME/git/dotfiles`.
2. Presents the picker (`fzf` or `select`).
3. If the clone entry is chosen (or `owner/repo` was passed), clones into `$HOME/github/<name>`. Existing clones are reused.
4. Creates or attaches to a tmux session named `claude-<reponame>` with `tmux new-session -A`, anchored in the repo directory.
5. Inside the pane, runs `"$SHELL" -ic 'claude --remote-control || true; exec "$SHELL" -i'` so the **`claude` shell function** from `ai/claude/aliases.sh` is in scope (preserves YOLO mode + tmux pane anchoring). When `claude` exits, the pane drops into an interactive shell instead of closing.
6. If the user is already inside tmux, the new session is created detached and the current client is switched to it.

### 4. Drive the session from the web

Once the pane shows the Claude prompt, the local session is reachable from claude.ai/code or the Claude mobile app. The tmux session persists across terminal disconnects, so the user can close the laptop's terminal without killing the host.

## Customization

| Variable | Default | Purpose |
| --- | --- | --- |
| `CLAUDE_REMOTE_GITHUB_DIR` | `$HOME/github` | Directory scanned for repos. |
| `CLAUDE_REMOTE_DOTFILES_DIR` | `$HOME/git/dotfiles` | Pinned in the picker. |
| `CLAUDE_REMOTE_SESSION_PREFIX` | `claude` | tmux session name prefix; final name is `<prefix>-<reponame>`. |

## Install orchestration

- **Script location follows the `ssh-host-finder` pattern**: the launcher lives at `opt/scripts/system/claude-remote` (alongside `claude_install.sh`, `gemini_install.sh`, `sync-skills.sh`) and is reachable as `~/opt/scripts/system/claude-remote` because `install.sh` symlinks `${HOME}/opt` to the repo's `opt/` directory.
- **Skill auto-discovery**: this `SKILL.md` is linked into `~/.agents/skills/claude-remote` (Gemini CLI) and `~/.claude/skills/claude-remote` (Claude Code) by `opt/scripts/system/sync-skills.sh`, which `install.sh` calls at the end of its run.
- **Slash-command companion**: `ai/claude/commands/claude-remote.md` provides a `/claude-remote` slash command that activates this skill — mirrors the `ai/claude/commands/ssh-find.md` pattern next to `src/ssh-host-finder/`.
- **`.gitignore`**: covered by the existing `!opt/**` and `!ai/**` and `!src/**` opt-in rules — no per-file allowlist entry needed.

No further wiring is required in `install.sh` itself.

## Troubleshooting

- **`claude --remote-control` exits immediately, pane shows a shell.** The launcher intentionally keeps the pane alive after `claude` exits so any error message stays visible. Re-run `claude --remote-control` from the pane after fixing the cause.
- **`Tip: run inside a tmux pane for AI pane-split support`** — harmless; the `claude` wrapper prints this when it can't find `$TMUX_PANE`. The launcher always runs inside tmux, so this should not appear.
- **Picker is empty.** Confirm there are git repos under `$CLAUDE_REMOTE_GITHUB_DIR`, or use the `[clone new repo …]` entry / pass `owner/repo` as the argument.
