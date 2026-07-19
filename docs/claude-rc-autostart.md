# claude-rc-autostart — auto-start a Claude remote-control session on WSL logon

On Windows+WSL hosts, this opens a WSL terminal at logon and starts a named
Claude **remote-control** session so you can drive the machine from claude.ai
(phone/web) without leaving a terminal open by hand. Pairs well with
[`sshd-setup`](./sshd-setup.md) (shell access) — SSH for the terminal, remote
control for Claude. **Opt-in: not wired into `install.sh`.**

## Pieces

| File | Role |
| :-- | :-- |
| `opt/bin/claude-rc-boot` | WSL launcher: runs `claude --remote-control "<prefix>-<date>_<time>"` |
| `opt/Desktop/Apps/scripts/install-claude-rc-autostart.ps1` | Creates/removes the logon + Desktop shortcuts |

The session name records **when it came up**, e.g. `gigabyte-2026-07-19_0948`.
Prefix precedence: argument → `$CLAUDE_RC_PREFIX` → short hostname.

The launcher is **idempotent**: if a `claude --remote-control` session is already
running for your user it exits 0 without starting a second one. This matters at
logon, where more than one thing can race to launch it (the Startup shortcut plus
e.g. Windows Terminal's restored window layout — with
`"firstWindowPreference": "persistedWindowLayout"`, WT replays the saved tab's
commandline, which is a second launch). Every decision is appended to
`~/.local/state/claude-rc-boot.log` (override with `$CLAUDE_RC_LOG`).

## Install

The launcher must be reachable inside WSL — either on your PATH (via the
dotfiles `opt/bin`) or copied to `~/.local/bin/claude-rc-boot`. Then, in
**PowerShell** (no admin needed — it writes to your own Startup folder):

```powershell
powershell -ExecutionPolicy Bypass -File opt\Desktop\Apps\scripts\install-claude-rc-autostart.ps1 -Prefix gigabyte
```

Options: `-Distro <name>` (default `Ubuntu`), `-Prefix <name>` (default: hostname).

The installer creates **two** shortcuts from the same template: the Startup-folder
one (runs at logon) and a Desktop copy (manual start/restart), both with the WSL
penguin icon so they're easy to spot. `-Uninstall` removes both. The Desktop path
follows OneDrive redirection.

## Test / use

- Double-click the `claude-remote` Desktop shortcut (or the Startup one), or sign
  out and back in — a Windows Terminal window opens with Claude in remote-control
  mode. If a session is already running, the launcher says so and exits.
- Preview the name without launching: `claude-rc-boot --print-name gigabyte`.
- Audit what happened at logon: `cat ~/.local/state/claude-rc-boot.log`.

## Notes & caveats

- Runs at **logon**, not at the lock screen — Windows must reach your desktop
  first. On a passwordless/auto-login account that's automatic after boot.
- Opening the WSL window also **boots the distro**, which starts any enabled
  systemd services (e.g. `ssh` from `sshd-setup`). That's the intended side
  effect that makes SSH reachable after a reboot on this setup.
- Remove with `-Uninstall`, or delete `claude-remote.lnk` from the Startup folder.
