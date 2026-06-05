---
name: remote-claude-session
description: Start, restart, or attach to a persistent Claude --remote-control session on any remote SSH host inside a specified git repository. Use this skill whenever the user asks to "start a remote Claude session", "spin up Claude on [host]", "set up remote control on [host]", "launch Claude remotely in [repo]", "start an AI session on [machine]", "restart the remote Claude session", or any variation of establishing or checking a remote Claude Code instance. Also triggers when the user asks to check whether a remote session is running or to get the attach command for an existing one.
---

# Remote Claude Session

Sets up a named tmux session on a remote SSH host with `claude --remote-control`
running inside a specified git repository, so the session survives disconnects and
can be attached or controlled at any time.

> **Future path:** `tmux-mgr remote start <alias> --repo <path>` will wrap this
> workflow once the `remote` subcommand lands in tmux-mgr. Until then follow the
> steps below directly.

## Inputs

Collect these from the user's message, conversation context, or by asking:

| Input | Default | Notes |
|-------|---------|-------|
| `SSH_ALIAS` | (required) | Must match a `Host` entry in `~/.ssh/config` |
| `REPO_PATH` | auto-detect | Path to the repo on the **remote** host. If not given, probe `~/git/<repo-name>` then `~/github/<repo-name>` on the remote — both layouts exist across this user's hosts. |
| `SESSION_NAME` | `claude-remote` | tmux session name on the remote |

If `REPO_PATH` is not specified, derive `<repo-name>` from the basename of the active local repo (or ask). The actual path on the remote is then resolved in step 2.

## Steps

### 1. Validate the SSH alias

```bash
ssh -G <SSH_ALIAS> 2>/dev/null | grep -E "^(hostname|user) "
```

Stop and report the error if the alias is not found in `~/.ssh/config`.

### 2. Check the remote (tmux, Claude, and REPO_PATH)

```bash
# tmux + Claude detection
ssh <SSH_ALIAS> "command -v tmux 2>/dev/null && \
  (command -v claude 2>/dev/null || \
   find ~/.nvm/versions/node/*/bin -name claude 2>/dev/null | sort -V | tail -1)"

# REPO_PATH probe (skip if an explicit path was supplied)
ssh <SSH_ALIAS> "for d in ~/git/<repo-name> ~/github/<repo-name>; do \
  [ -d \"\$d/.git\" ] && echo \"\$d\" && break; done"
```

- **tmux missing** → stop; tell the user to run `sudo apt install tmux` on the remote.
- **claude on PATH** → use `claude` directly.
- **claude found via nvm** → prefix the launch command with `export NVM_DIR=~/.nvm && source ~/.nvm/nvm.sh && `.
- **claude not found** → stop; tell the user to install Claude Code on the remote (`npm i -g @anthropic-ai/claude-code` after sourcing nvm).
- **REPO_PATH probe printed a path** → use it.
- **REPO_PATH probe empty** → stop; show both candidates (`~/git/<repo-name>`, `~/github/<repo-name>`) and ask the user which path to use.

### 3. Check for an existing session

```bash
ssh <SSH_ALIAS> "tmux ls 2>/dev/null | grep '^<SESSION_NAME>:'"
```

If the session exists, check whether Claude is already healthy:

```bash
ssh <SSH_ALIAS> "tmux capture-pane -t <SESSION_NAME> -p 2>/dev/null | grep -c 'Remote Control active'"
```

- **Claude is active** → report the session as healthy, show the attach command, and stop. Nothing more to do.
- **Session exists but Claude is not running** → kill it and proceed to step 4.

### 4. Start the session

Kill any stale session then create a fresh one in the target repo:

```bash
ssh <SSH_ALIAS> "tmux kill-session -t <SESSION_NAME> 2>/dev/null; \
  tmux new-session -d -s <SESSION_NAME> -c <REPO_PATH> -x 220 -y 50 && \
  tmux send-keys -t <SESSION_NAME> '<NVM_PREFIX>claude --remote-control' Enter"
```

### 5. Verify startup (poll up to 15 s)

```bash
ssh <SSH_ALIAS> "tmux capture-pane -t <SESSION_NAME> -p 2>/dev/null | tail -5"
```

- If a **trust prompt** appears ("Is this a project you trust?"), send confirmation:
  ```bash
  ssh <SSH_ALIAS> "tmux send-keys -t <SESSION_NAME> '1' Enter"
  ```
  Then re-poll.
- Success when the status bar contains **`Remote Control active`**.
- If not seen within 15 s, capture and show the full pane to help diagnose.

### 6. Report success

Tell the user:
- Claude Remote Control is active on `<SSH_ALIAS>` in `<REPO_PATH>`
- Session: `<SESSION_NAME>` (persists after disconnect — tmux keeps it alive)
- Attach any time with:
  ```
  ssh <SSH_ALIAS> -t tmux attach -t <SESSION_NAME>
  ```

## Convention checklist (when creating this skill's directory)

- [ ] `ai/skills/remote-claude-session/SKILL.md` — this file
- [ ] `ai/skills/remote-claude-session/GEMINI.md` — brief agent context
- [ ] `ai/skills/remote-claude-session/CLAUDE.md -> GEMINI.md` — symlink
