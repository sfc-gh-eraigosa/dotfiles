#Requires -RunAsAdministrator
<#
.SYNOPSIS
  On-demand Windows-native OpenSSH Server setup: capability, service,
  firewall, authorized_keys from GitHub. Companion to opt/bin/sshd-setup.
.DESCRIPTION
  Installs the OpenSSH.Server capability if missing (native-first: skips
  when present), sets the sshd service to auto-start, opens TCP 22 in the
  Windows firewall, and seeds authorized_keys (user + administrators) from
  the GitHub public keys of the account derived from git credentials
  (gh api user -> origin remote owner -> git config github.user).
  -WslPortProxy additionally forwards a host port (default 2222) to the
  WSL distro's sshd so it is reachable from the LAN.
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File setup-sshd.ps1 -WslPortProxy
#>
param(
    [string]$GithubUser,
    [switch]$WslPortProxy,
    [int]$WslPort = 2222
)
$ErrorActionPreference = 'Stop'

function Get-GithubUser {
    if ($GithubUser) { return $GithubUser }
    $gh = Get-Command gh -ErrorAction SilentlyContinue
    if ($gh) { $u = (& gh api user --jq .login) 2>$null; if ($u) { return $u } }
    $origin = (& git remote get-url origin) 2>$null
    if ($origin -match '(?:[:/])([^/]+)/[^/]+?(?:\.git)?$') { return $Matches[1] }
    $cfg = (& git config github.user) 2>$null
    if ($cfg) { return $cfg }
    throw "Cannot derive GitHub user: pass -GithubUser <login>"
}

# 1. Capability (native-first: skip when present)
$cap = Get-WindowsCapability -Online -Name 'OpenSSH.Server*'
if ($cap.State -ne 'Installed') {
    Write-Host "Installing OpenSSH.Server capability..."
    Add-WindowsCapability -Online -Name $cap.Name | Out-Null
} else { Write-Host "OpenSSH.Server capability already installed." }

# 2. Service: auto-start + start
Set-Service -Name sshd -StartupType Automatic
if ((Get-Service sshd).Status -ne 'Running') { Start-Service sshd }
Write-Host "sshd service: $((Get-Service sshd).Status), startup=Automatic"

# 3. Firewall rule for 22 (idempotent)
if (-not (Get-NetFirewallRule -Name 'OpenSSH-Server-In-TCP' -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -Name 'OpenSSH-Server-In-TCP' -DisplayName 'OpenSSH Server (sshd)' `
        -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22 | Out-Null
    Write-Host "Firewall rule created for TCP 22."
} else { Write-Host "Firewall rule for TCP 22 already present." }

# 4. Keys from GitHub (refuse-empty; seed user + administrators files)
$user = Get-GithubUser
$keys = (Invoke-RestMethod -Uri "https://github.com/$user.keys").Trim()
if (-not $keys) { throw "GitHub returned no public keys for '$user' — refusing to continue" }
$targets = @(
    (Join-Path $env:USERPROFILE '.ssh\authorized_keys'),
    (Join-Path $env:ProgramData 'ssh\administrators_authorized_keys')
)
foreach ($t in $targets) {
    $dir = Split-Path $t
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    $existing = if (Test-Path $t) { Get-Content $t } else { @() }
    $added = 0
    foreach ($k in ($keys -split "`n")) {
        $k = $k.Trim()
        if ($k -and ($existing -notcontains $k)) { Add-Content -Path $t -Value $k; $added++ }
    }
    Write-Host "Seeded $added new key(s) into $t (user: $user)"
}
# Lock down the administrators file per OpenSSH-on-Windows requirements
$admFile = Join-Path $env:ProgramData 'ssh\administrators_authorized_keys'
icacls $admFile /inheritance:r /grant 'Administrators:F' /grant 'SYSTEM:F' | Out-Null

# 5. Optional: portproxy so the WSL sshd is LAN-reachable
if ($WslPortProxy) {
    $wslIp = ((& wsl hostname -I) -split '\s+')[0]
    if (-not $wslIp) { throw "Could not determine WSL IP (is the distro running?)" }
    netsh interface portproxy delete v4tov4 listenport=$WslPort listenaddress=0.0.0.0 2>$null | Out-Null
    netsh interface portproxy add v4tov4 listenport=$WslPort listenaddress=0.0.0.0 `
        connectport=22 connectaddress=$wslIp | Out-Null
    if (-not (Get-NetFirewallRule -Name "WSL-SSH-$WslPort" -ErrorAction SilentlyContinue)) {
        New-NetFirewallRule -Name "WSL-SSH-$WslPort" -DisplayName "WSL sshd ($WslPort)" `
            -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort $WslPort | Out-Null
    }
    Write-Host "Portproxy: host :$WslPort -> WSL ${wslIp}:22 (rerun after WSL IP changes)"
}
Write-Host "Done. Rollback: docs/sshd-setup.md#rollback"
