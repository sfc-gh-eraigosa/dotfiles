# sshd-setup — on-demand SSH access to your machines

Two companion tools that stand up the **OS-native** sshd only when you ask for it, open the
local firewall, and seed `authorized_keys` from your GitHub public keys:

| Platform | Entry point | Installs | Service | Firewall |
| :-- | :-- | :-- | :-- | :-- |
| Linux | `opt/bin/sshd-setup` | `openssh-server` via `pkg-install` | `systemctl enable --now ssh` | ufw / firewalld (if active) |
| WSL | `opt/bin/sshd-setup` | same as Linux | same as Linux | prints Windows-side steps |
| macOS | `opt/bin/sshd-setup` | nothing (built-in) | `systemsetup -setremotelogin on` | built-in |
| Windows | `opt/Desktop/Apps/scripts/setup-sshd.ps1` | OpenSSH.Server capability | `sshd` service, auto-start | `New-NetFirewallRule` TCP 22 |

Nothing runs at boot or login unless the tool set it up — **invoking it is the opt-in**.
The login-time hook in `opt/profiles/.bashrc` stays disabled by design.

## Quickstart

```bash
# Linux / macOS / WSL
opt/bin/sshd-setup status        # read-only report
opt/bin/sshd-setup --dry-run enable
opt/bin/sshd-setup enable        # install + start + firewall + keys + ~/.sshd.env
opt/bin/sshd-setup keys          # (re)seed authorized_keys only
```

```powershell
# Windows (ADMIN PowerShell)
powershell -ExecutionPolicy Bypass -File opt\Desktop\Apps\scripts\setup-sshd.ps1
```

## Which GitHub account do the keys come from?

Derived automatically, first hit wins:

1. `gh api user --jq .login` (the authenticated GitHub CLI account)
2. the owner segment of `git remote get-url origin`
3. `git config github.user`

Keys are fetched from `https://github.com/<login>.keys`, merged **additively** into
`authorized_keys` (deduplicated, `700`/`600` permissions). An empty response aborts the run —
your existing keys are never truncated. Pass `-GithubUser <login>` to the ps1 to override.

## Windows + WSL: both in one go

1. Inside WSL: `opt/bin/sshd-setup enable` — sets up the distro's sshd on port 22
   (WSL-internal) and prints the Windows handoff.
2. In an **admin** PowerShell:
   `powershell -ExecutionPolicy Bypass -File opt\Desktop\Apps\scripts\setup-sshd.ps1 -WslPortProxy`
   — restores the Windows-native sshd on port 22 (survives reboots) **and** forwards host
   port 2222 → the WSL sshd so it is reachable from your LAN.
3. From another machine: `ssh <winuser>@<host>` (Windows) or `ssh -p 2222 <wsluser>@<host>` (WSL).

Note: the WSL sshd only answers while the distro is running; the Windows one always does.
The portproxy pins the WSL IP at setup time — rerun `setup-sshd.ps1 -WslPortProxy` if it
changes.

## Integration with dotfiles

`enable` writes `SSHD_LOGIN=true` to `~/.sshd.env`, the marker the existing
`opt/scripts/network/sshd_run.sh` (invoked from `install.sh`) checks before (re)starting an
already-installed sshd. `sshd_run.sh` remains start-only; `sshd-setup` is the install path.

## Rollback

```bash
# Linux / WSL
sudo systemctl disable --now ssh && sudo apt-get remove openssh-server
sudo ufw delete allow 22/tcp          # if ufw was configured
rm ~/.sshd.env                        # remove the dotfiles marker
# macOS
sudo systemsetup -setremotelogin off
```

```powershell
# Windows (admin)
Stop-Service sshd; Set-Service sshd -StartupType Disabled
Remove-WindowsCapability -Online -Name (Get-WindowsCapability -Online -Name 'OpenSSH.Server*').Name
Remove-NetFirewallRule -Name 'OpenSSH-Server-In-TCP'
netsh interface portproxy delete v4tov4 listenport=2222 listenaddress=0.0.0.0
Remove-NetFirewallRule -Name 'WSL-SSH-2222'
```

Key seeding is additive only — edit `~/.ssh/authorized_keys` (and on Windows
`%ProgramData%\ssh\administrators_authorized_keys`) by hand to remove entries.
