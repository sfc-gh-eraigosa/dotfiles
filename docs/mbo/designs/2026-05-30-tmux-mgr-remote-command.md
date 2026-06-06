# Design: `tmux-mgr remote` — Persistent Remote Claude Session Management

**Date:** 2026-05-30  
**Status:** Draft  
**Relates to:** `ai/skills/remote-claude-session/SKILL.md`

---

## Problem

Starting a `claude --remote-control` session on a remote host today requires a
manual multi-step sequence: validate the SSH alias, detect how Claude is installed
(PATH vs nvm), check for a stale tmux session, construct the right launch command,
handle the trust prompt, and verify startup. This is error-prone and not repeatable
without Claude mediating it.

`tmux-mgr` already manages local session lifecycle. Adding a `remote` subcommand
makes the same workflow a single CLI call, usable from the shell or from within
the `remote-claude-session` skill.

---

## Command Design

### Subcommand tree

```
tmux-mgr remote start   <ssh-alias>  [flags]   # start or reuse a session
tmux-mgr remote stop    <ssh-alias>  [flags]   # kill the session
tmux-mgr remote status  <ssh-alias>  [flags]   # check if session + Claude are live
tmux-mgr remote attach  <ssh-alias>  [flags]   # print the attach command (or exec it)
```

### `remote start` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--repo <path>` | auto-detect | Working directory on the remote. When omitted, probe `~/git/<local-repo-name>` then `~/github/<local-repo-name>` on the remote and use the first that exists. Both layouts are in active use across the user's hosts. |
| `--session <name>` | `claude-remote` | tmux session name |
| `--width <cols>` | `220` | Initial terminal width |
| `--height <rows>` | `50` | Initial terminal height |
| `--force` | false | Kill any existing session before starting |
| `--no-verify` | false | Skip the "Remote Control active" verification poll |

### `remote status` output (JSON with `--json`)

```json
{
  "alias": "wenlockgigabyte",
  "session": "claude-remote",
  "repo": "~/git/dotfiles",
  "session_alive": true,
  "claude_active": true,
  "attach_cmd": "ssh wenlockgigabyte -t tmux attach -t claude-remote"
}
```

---

## Implementation Plan

### Phase 1 — `remote start` / `remote status`

1. **SSH alias resolution** — run `ssh -G <alias>` to extract `hostname` and `user`;
   fail fast with a clear message if the alias is not in `~/.ssh/config`.

2. **Remote capability check** (single SSH round-trip):
   ```bash
   ssh <alias> "command -v tmux; command -v claude 2>/dev/null || \
     find ~/.nvm/versions/node/*/bin -name claude 2>/dev/null | sort -V | tail -1"
   ```
   - Missing tmux → error with install hint.
   - Claude via nvm → build an nvm-source prefix for the launch command.
   - Claude not found → error with install hint.

3. **Repo-path resolution** (skip if `--repo` was passed explicitly):
   ```bash
   ssh <alias> "for d in ~/git/<repo> ~/github/<repo>; do \
     [ -d \"\$d/.git\" ] && echo \"\$d\" && break; done"
   ```
   - Empty output → error listing both candidate paths and asking the caller to
     pass `--repo` explicitly.
   - A path is printed → use it as the working directory.
   - Implementation note: this can be folded into the capability check above as a
     single SSH round-trip to keep startup latency down.

4. **Idempotent session start**:
   - Check if session exists and Claude is healthy (`grep 'Remote Control active'`).
   - If healthy, print the attach command and exit 0 (no-op).
   - If session exists but Claude is dead, kill it (unless `--force` is not set, in
     which case ask the user).
   - Create the session with `tmux new-session -d -s <name> -c <repo> -x W -y H`.
   - Send the launch command via `tmux send-keys`.

5. **Startup verification** — poll `tmux capture-pane` every 1 s for up to 15 s:
   - Auto-confirm the trust dialog if seen (`send-keys '1' Enter`).
   - Succeed when `Remote Control active` appears in the status bar.
   - On timeout: print the pane content and exit non-zero.

6. **Output** — on success print the attach command (human-readable or `--json`).

### Phase 2 — `remote stop` / `remote attach`

- `stop`: `ssh <alias> "tmux kill-session -t <session>"` with a confirmation prompt
  unless `--force`.
- `attach`: exec `ssh <alias> -t tmux attach -t <session>` (replaces the shell
  process so the user lands directly in the session).

### Phase 3 — Registry integration

Persist remote sessions in `~/.config/tmux-mgr/sessions/` alongside local agent
sessions so `tmux-mgr agent list` can show remote Claude instances alongside local
ones.

---

## Files to create / modify

| Path | Change |
|------|--------|
| `sdk/tmux-mgr/cmd/remote.go` | New file — `remote` command + subcommands |
| `sdk/tmux-mgr/pkg/remote/session.go` | New package — SSH + session logic |
| `sdk/tmux-mgr/pkg/remote/session_test.go` | Unit tests (mock SSH runner) |
| `sdk/tmux-mgr/cmd/root.go` | Register `remoteCmd` |
| `sdk/tmux-mgr/GEMINI.md` | Add `tmux-mgr remote` to the command summary |
| `ai/skills/remote-claude-session/SKILL.md` | Update "Future path" note to "use `tmux-mgr remote start`" once merged |

---

## Non-goals

- Managing multiple simultaneous remote sessions per host (Phase 3 registry handles
  awareness; the user can run multiple `tmux-mgr remote start` calls with different
  `--session` names).
- Tunnelling/proxying network traffic.
- Supporting non-tmux remote session managers.
