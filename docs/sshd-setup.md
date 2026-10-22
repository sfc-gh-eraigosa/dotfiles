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
opt/bin/sshd-setup enable        # install + start + firewall + keys + gff flag (+ WSL networking mode)
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

On a machine that runs `install.sh`, steps 2's Windows side is normally automatic: see
[WSL networking is a flag](#wsl-networking-is-a-flag) below. The manual commands here stay
valid for a machine that does not use `install.sh`.

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

> **This is now a gff flag.** `install.windows.wsl-network` (`off|nat|bridged|mirrored`,
> default `off`) records the design; `install.sh` applies it. See
> [WSL networking is a flag](#wsl-networking-is-a-flag). The tables and manual steps below
> explain what each mode does.

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
| Set up by | `install.windows.wsl-network=nat` (or `refresh-wsl-portproxy.ps1`) | `=bridged` (the `WSLBridge` switch is by hand) | `=mirrored` |

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

`install.sh` never creates the `WSLBridge` switch (step 1 stays manual) and never runs
`wsl --shutdown` (step 2's restart is yours). An existing `networkingMode=bridged` is recorded
and left byte for byte as it is. Rollback: remove the `networkingMode`/`vmSwitch`/`macAddress`/`dhcp`
lines, `wsl --shutdown`, `Remove-VMSwitch WSLBridge`, then `gff unset install.windows.wsl-network`.

## WSL networking is a flag

`install.windows.wsl-network` holds the design for this host. Set it, or override it per
machine, like any gff flag; the resolution layers and provenance are in `gff get`:

```bash
gff get install.windows.wsl-network         # value + where it came from
gff set install.windows.wsl-network bridged # pin it on this host
gff unset install.windows.wsl-network       # back to the default (off)
```

How it gets its value:

1. **Existing setup is recorded, never changed.** While sshd is enabled
   (`install.network.sshd`), detection reads `.wslconfig` (`networkingMode=bridged|mirrored`) and
   the WSL portproxy / `RefreshWslPortProxy` task (`nat`), records that with `gff set`, and marks
   it as found (`~/.local/state/dotfiles/wsl-network-detected`). `install.sh` then only reports a
   found setup, even an incomplete one. A machine that works today keeps working, untouched, until
   you hand it over: run `install_windows.sh <repo> --wsl-network` yourself, or set another mode.
   With sshd off nothing is detected or recorded, and Windows is not probed.
2. **Fresh enablement asks once.** With nothing detected, `sshd-setup enable` on WSL recommends a
   mode (k3s present and a wired adapter: `bridged`; otherwise `nat`) and asks. Pre-answer with
   `WSLNET_ANSWER=<off|nat|bridged|mirrored|skip>`; with no TTY and no answer nothing is
   recorded. Answering `skip` at the prompt is remembered (`wsl-network-skipped`): `install.sh`
   does not ask again, and `sshd-setup enable` does. `fleet` sends `WSLNET_ANSWER` (default
   `skip`), set per session in the fleet answers form and never saved, because the mode is a
   per-host choice.
3. **`install.sh` applies a chosen mode** (see [install-windows.md](./install-windows.md)). `off` is
   hands off, and so is a found one (1).

Behaviors worth knowing:

- `nat` keeps a `dhcp=` key and `mirrored` keeps an existing `vmSwitch`/`macAddress`: WSL ignores
  them in those modes, so they are left alone rather than churned.
- A fresh `bridged` setup asks for one UAC prompt per `install.sh` run until the `WSLBridge`
  switch exists; without it the step only prints the `New-VMSwitch` command.
- On `mirrored`, a Hyper-V firewall rule that cannot be read without admin is treated as
  current (the read-only `-Status` check never elevates).
- Changing mode cleans up the previous mode's artifacts (portproxy, task, firewall rule) only
  when this run changed `networkingMode`; a mode that was merely recorded is never cleaned up.
- The apply script logs to `%LOCALAPPDATA%\dotfiles\set-wsl-network.log` and backs up
  `.wslconfig` to `.wslconfig.bak-<timestamp>` before any edit.

## Integration with dotfiles

sshd is switched by the gff flag `install.network.sshd` (default `false`).
`sshd-setup enable` runs `gff set install.network.sshd true`; `install.sh` gates
`opt/scripts/network/sshd_run.sh` with it. `sshd_run.sh` remains start-only and no longer reads
any file; `sshd-setup` is the install path. `sshd-setup status` prints `sshd flag: <value>`.

**Migration from `~/.sshd.env`.** The old marker (`SSHD_LOGIN=true` in `~/.sshd.env`) is
migrated once by `install.sh`: it runs `gff set install.network.sshd true` and deletes the
file. Nothing else reads `~/.sshd.env`. `install.sh` now requires a working gff (freshly built,
or an existing `~/opt/bin/gff` that runs) and exits 1 otherwise.

## Rollback

```bash
# Linux / WSL
sudo systemctl disable --now ssh && sudo apt-get remove openssh-server
sudo ufw delete allow 22/tcp          # if ufw was configured
gff unset install.network.sshd        # drop the dotfiles flag (back to false)
# macOS
sudo systemsetup -setremotelogin off
```

WSL networking, per mode (then `gff unset install.windows.wsl-network`, and `wsl --shutdown`
where `.wslconfig` changes):

| Mode | Undo |
| :-- | :-- |
| `nat` | the `refresh-wsl-portproxy.ps1 -UnregisterTask` + `netsh ... portproxy delete` + `Remove-NetFirewallRule WSL-SSH-<port>` steps below |
| `bridged` | remove the `networkingMode`/`vmSwitch`/`macAddress`/`dhcp` lines (or restore a `.wslconfig.bak-<timestamp>`); `Remove-VMSwitch WSLBridge` |
| `mirrored` | remove `networkingMode=mirrored`; `Remove-NetFirewallHyperVRule` for the rule the apply step added |

Unsetting the flag alone changes nothing on Windows; it only stops `install.sh` from applying it.

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
