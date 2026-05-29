---
description: Launch a tmux session running `claude --remote-control` against a chosen GitHub repo so the local session can be steered from claude.ai or the Claude mobile app
allowed-tools: Bash(tmux:*), Bash(git:*), Bash(ls:*)
---

Spin up a tmux-anchored Claude Code session in **remote-control** mode against a repo under `$HOME/github` (or pinned `$HOME/git/dotfiles`, or a newly-cloned repo) so it can be monitored and steered from the web / mobile.

---
Repo target (path, `owner/repo`, full URL, or empty for picker): $ARGUMENTS

Activate the **claude-remote** skill and follow its workflow:

1. If `$ARGUMENTS` is empty, run the launcher with no args so the user gets the interactive picker (fzf or `select`).
2. If `$ARGUMENTS` is a path that exists and contains `.git/`, pass it directly so the picker is skipped.
3. If `$ARGUMENTS` looks like `owner/repo` or a `https://`/`git@` URL, pass it so the launcher clones into `$HOME/github/<name>` (reusing an existing clone if present) and then launches.
4. Run `~/opt/scripts/system/claude-remote $ARGUMENTS` (omit `$ARGUMENTS` if empty).
5. Once the tmux session is up, remind the user they can drive it from [claude.ai/code](https://claude.ai/code) or the Claude mobile app, and that the tmux session persists across terminal disconnects.

Notes:
- The launcher uses `tmux new-session -A`, so re-running on the same repo attaches to the existing session instead of creating a duplicate.
- The pane runs `"$SHELL" -ic 'claude --remote-control || true; exec "$SHELL" -i'`, so the `claude` shell function from `ai/claude/aliases.sh` (YOLO toggle + `tmux-mgr pane anchor`) is in scope, and the pane stays alive after `claude` exits.
- Override defaults with `CLAUDE_REMOTE_GITHUB_DIR`, `CLAUDE_REMOTE_DOTFILES_DIR`, or `CLAUDE_REMOTE_SESSION_PREFIX`.
