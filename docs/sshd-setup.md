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

The portproxy pins the WSL IP at setup time, and **WSL's NAT IP changes on most
reboots**, silently breaking the forward. `refresh-wsl-portproxy.ps1` re-resolves
the IP and rewrites the rule (no-op when current), logging each decision to
`%LOCALAPPDATA%\dotfiles\refresh-wsl-portproxy.log`. Run once in an admin
PowerShell to make it self-healing:

```powershell
powershell -ExecutionPolicy Bypass -File opt\Desktop\Apps\scripts\refresh-wsl-portproxy.ps1 -RegisterTask
```

`-RegisterTask` installs an elevated at-logon Scheduled Task (`RefreshWslPortProxy`,
30s delay) that repeats the refresh on every boot without UAC prompts; remove it
with `-UnregisterTask`. `-ListenPort`/`-Distro` mirror the setup defaults.
The script also opens the listen port inbound (`WSL-SSH-<port>`, scoped to
`LocalSubnet`). Without that rule, a Wi-Fi network marked **Public** drops every LAN
client even though the forward is correct. When Windows runs no sshd of its own,
`-ListenPort 22` forwards the standard port straight to WSL, so the fleet
`~/.ssh/config` entry needs no `Port` line.

## WSL networking: pick one of three designs

How a LAN client (fleet) reaches a WSL sshd depends on the distro's
`networkingMode` in `%USERPROFILE%\.wslconfig`. The fleet machines use two of these.

| | NAT + portproxy (default) | Bridged | Mirrored |
| :-- | :-- | :-- | :-- |
| `.wslconfig` | nothing (WSL default) | `networkingMode=bridged` + `vmSwitch=<switch>` | `networkingMode=mirrored` |
| WSL's address | private NAT IP behind the Windows host | its own LAN IP from DHCP | shares the Windows host's IPs |
| Inbound path | Windows `portproxy` -> WSL `:22` | straight to the VM; Windows is not in the path | the host's port, delivered into WSL |
| Firewall | Windows rule `WSL-SSH-<port>` | none on the Windows host | Hyper-V firewall (`New-NetFirewallHyperVRule`) |
| IP churn | changes most boots -> `RefreshWslPortProxy` task | none with a fixed MAC + DHCP reservation | none |
| Inbound UDP | n/a | yes | **no** (kernel sockets get no inbound UDP) |
| Works over Wi-Fi | yes | poorly: needs a Hyper-V external switch; most Wi-Fi adapters cannot carry a second MAC | yes |
| Used by | Wi-Fi laptops | the k3s workstation | — (untested here) |
| Set up by | `refresh-wsl-portproxy.ps1` (above) | by hand (below) | by hand |

**Rule of thumb:** use NAT + portproxy. Use bridged only when the VM must be a real LAN
peer that receives inbound UDP. A k3s node is the case that needs it: flannel's VXLAN
overlay is UDP, and in mirrored mode those packets never reached the node (2026-10-11).
Mirrored is the simplest option for TCP-only use (no forward, no IP churn), but no fleet
machine runs it yet.

### Bridged (the k3s workstation)

The VM gets its own NIC on a Hyper-V **external** switch, so sshd answers on the VM's own
LAN address and Windows' firewall and portproxy never see the traffic. On that machine:

```ini
# %USERPROFILE%\.wslconfig
[wsl2]
networkingMode=bridged
vmSwitch=WSLBridge
# Fixed, locally administered MAC (02:...) so DHCP hands out the same address each boot.
macAddress=02:xx:xx:xx:xx:xx
dhcp=true
```

1. Create the external switch once, bound to the **wired** adapter (admin PowerShell,
   Hyper-V feature required): `New-VMSwitch -Name WSLBridge -NetAdapterName <Ethernet> -AllowManagementOS $true`.
   (This is the standard command. How the workstation's own switch was created was not
   recorded; check it there with `Get-VMSwitch WSLBridge`.)
2. Write the `.wslconfig` above, then `wsl --shutdown` and start the distro again. Inside WSL,
   `ip -4 addr show eth0` should now show a LAN address, not `172.x`.
3. Optionally, reserve that MAC's address on the router. fleet only needs `HostName` in
   `~/.ssh/config` to resolve.
4. Skip `refresh-wsl-portproxy.ps1` and `setup-sshd.ps1 -WslPortProxy` on a bridged
   machine. The VM is already a LAN peer, so a forward would only add a second path
   through the Windows host and claim the host's port 22 for nothing.

This setup is not automated by `install.sh`. It is hand-configured, with the rationale
recorded in that machine's `.wslconfig`. Rollback: remove the three `networkingMode`/`vmSwitch`/`macAddress`
lines, `wsl --shutdown`, and `Remove-VMSwitch WSLBridge`.

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
Unregister-ScheduledTask -TaskName RefreshWslPortProxy -Confirm:$false   # if -RegisterTask was used
```

If you skipped the Windows sshd and forwarded port 22 straight to WSL
(`refresh-wsl-portproxy.ps1 -ListenPort 22`), undo just that, then delete the
distro marker `install.sh` deployed (`lib\wsl.ps1` falls back to the next source):

```powershell
# Windows (admin)
powershell -ExecutionPolicy Bypass -File refresh-wsl-portproxy.ps1 -UnregisterTask
netsh interface portproxy delete v4tov4 listenport=22 listenaddress=0.0.0.0
Remove-NetFirewallRule -Name 'WSL-SSH-22'
Remove-Item "$([Environment]::GetFolderPath('Desktop'))\Apps\scripts\lib\wsl-distro.txt"
```

Key seeding is additive only — edit `~/.ssh/authorized_keys` (and on Windows
`%ProgramData%\ssh\administrators_authorized_keys`) by hand to remove entries.
