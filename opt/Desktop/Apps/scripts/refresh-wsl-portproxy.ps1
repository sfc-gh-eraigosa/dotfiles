#Requires -RunAsAdministrator
<#
.SYNOPSIS
  Refresh the host->WSL sshd portproxy after WSL's NAT IP changes (it does on
  most reboots). Companion to setup-sshd.ps1 -WslPortProxy, which creates the
  forward initially but leaves it stale once the distro gets a new address.

.DESCRIPTION
  Re-resolves the WSL distro's current IP and rewrites the
  0.0.0.0:<ListenPort> -> <wslIp>:22 v4tov4 portproxy rule if it points
  elsewhere (no-op when already current). Appends every decision to
  %LOCALAPPDATA%\dotfiles\refresh-wsl-portproxy.log.

  -RegisterTask installs an elevated at-logon Scheduled Task (named
  RefreshWslPortProxy, 30s delay) that re-runs this script with the same
  parameters, so the forward heals itself on every boot without UAC prompts.

.PARAMETER ListenPort
  Host port the forward listens on (default 22).

.PARAMETER Distro
  WSL distro whose sshd is the target (default Ubuntu).

.PARAMETER RegisterTask
  Also register the at-logon refresh Scheduled Task.

.PARAMETER UnregisterTask
  Remove the Scheduled Task and exit (no refresh).

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File refresh-wsl-portproxy.ps1 -RegisterTask
  powershell -ExecutionPolicy Bypass -File refresh-wsl-portproxy.ps1 -UnregisterTask
#>
param(
    [int]$ListenPort = 22,
    [string]$Distro = 'Ubuntu',
    [switch]$RegisterTask,
    [switch]$UnregisterTask
)
$ErrorActionPreference = 'Stop'
$env:WSL_UTF8 = '1'

$taskName = 'RefreshWslPortProxy'
$logDir   = Join-Path $env:LOCALAPPDATA 'dotfiles'
$logFile  = Join-Path $logDir 'refresh-wsl-portproxy.log'

function Write-Log([string]$msg) {
    if (-not (Test-Path $logDir)) { New-Item -ItemType Directory -Path $logDir -Force | Out-Null }
    "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') $msg" | Add-Content -Path $logFile
    Write-Host $msg
}

if ($UnregisterTask) {
    if (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue) {
        Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
        Write-Log "unregistered scheduled task '$taskName'"
    } else { Write-Host "No scheduled task '$taskName' (nothing to remove)" }
    return
}

$wslIp = ((& wsl.exe -d $Distro -- hostname -I) -split '\s+')[0]
if (-not $wslIp) { Write-Log "ERROR: could not determine WSL IP for distro '$Distro'"; exit 1 }

# Current connect address of our rule, if any (netsh has no queryable output;
# parse the fixed-width table).
$current = $null
foreach ($line in (netsh interface portproxy show v4tov4)) {
    if ($line -match "^\s*0\.0\.0\.0\s+$ListenPort\s+(\S+)\s+22\s*$") { $current = $Matches[1]; break }
}

if ($current -eq $wslIp) {
    Write-Log "portproxy :$ListenPort -> ${wslIp}:22 already current; nothing to do"
} else {
    netsh interface portproxy delete v4tov4 listenport=$ListenPort listenaddress=0.0.0.0 2>$null | Out-Null
    netsh interface portproxy add v4tov4 listenport=$ListenPort listenaddress=0.0.0.0 `
        connectport=22 connectaddress=$wslIp | Out-Null
    $was = if ($current) { $current } else { '<none>' }
    Write-Log "portproxy :$ListenPort -> ${wslIp}:22 updated (was $was)"
}

if ($RegisterTask) {
    $action = New-ScheduledTaskAction -Execute 'powershell.exe' `
        -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$PSCommandPath`" -ListenPort $ListenPort -Distro $Distro"
    $trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
    $trigger.Delay = 'PT30S'   # let WSL/autostart settle before probing the IP
    $principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Highest
    Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Principal $principal -Force | Out-Null
    Write-Log "registered scheduled task '$taskName' (at logon +30s, elevated, port $ListenPort, distro $Distro)"
}
