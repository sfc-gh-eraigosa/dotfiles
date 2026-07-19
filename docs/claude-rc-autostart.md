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
| `opt/Desktop/Apps/scripts/install-claude-rc-autostart.ps1` | Creates/removes the Windows logon shortcut |

The session name records **when it came up**, e.g. `gigabyte-2026-07-19_0948`.
Prefix precedence: argument → `$CLAUDE_RC_PREFIX` → short hostname.

## Install

The launcher must be reachable inside WSL — either on your PATH (via the
dotfiles `opt/bin`) or copied to `~/.local/bin/claude-rc-boot`. Then, in
**PowerShell** (no admin needed — it writes to your own Startup folder):

```powershell
powershell -ExecutionPolicy Bypass -File opt\Desktop\Apps\scripts\install-claude-rc-autostart.ps1 -Prefix gigabyte
```

Options: `-Distro <name>` (default `Ubuntu`), `-Prefix <name>` (default: hostname).

## Test / use

- Double-click `…\Startup\claude-remote.lnk`, or sign out and back in — a
  Windows Terminal window opens with Claude in remote-control mode.
- Preview the name without launching: `claude-rc-boot --print-name gigabyte`.

## Notes & caveats

- Runs at **logon**, not at the lock screen — Windows must reach your desktop
  first. On a passwordless/auto-login account that's automatic after boot.
- Opening the WSL window also **boots the distro**, which starts any enabled
  systemd services (e.g. `ssh` from `sshd-setup`). That's the intended side
  effect that makes SSH reachable after a reboot on this setup.
- Remove with `-Uninstall`, or delete `claude-remote.lnk` from the Startup folder.
