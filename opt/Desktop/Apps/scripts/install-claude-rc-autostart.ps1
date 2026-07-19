<#
.SYNOPSIS
  Install (or remove) a Windows logon shortcut that opens a WSL terminal and
  auto-starts a named Claude remote-control session via opt/bin/claude-rc-boot.
  Companion to setup-sshd.ps1. Not wired into install.sh — run it when you want it.

.DESCRIPTION
  Creates ~\...\Startup\claude-remote.lnk pointing at
  Windows Terminal -> wsl -d <Distro> -> bash -c -> claude-rc-boot.
  The launcher's absolute path inside WSL is resolved at install time (prefers
  the dotfiles PATH copy, falls back to ~/.local/bin/claude-rc-boot).

.PARAMETER Prefix
  Session-name prefix (default: the launcher's own default = short hostname).

.PARAMETER Distro
  WSL distro to launch (default: Ubuntu).

.PARAMETER Uninstall
  Remove the shortcut instead of creating it.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File install-claude-rc-autostart.ps1 -Prefix gigabyte
  powershell -ExecutionPolicy Bypass -File install-claude-rc-autostart.ps1 -Uninstall
#>
param(
    [string]$Prefix,
    [string]$Distro = 'Ubuntu',
    [switch]$Uninstall
)
$ErrorActionPreference = 'Stop'
$env:WSL_UTF8 = '1'   # make wsl.exe emit clean UTF-8 we can capture

$startup = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Startup'
$lnkPath = Join-Path $startup 'claude-remote.lnk'

if ($Uninstall) {
    if (Test-Path $lnkPath) { Remove-Item $lnkPath -Force; Write-Host "Removed $lnkPath" }
    else { Write-Host "No shortcut at $lnkPath (nothing to remove)" }
    return
}

$wt = Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps\wt.exe'
if (-not (Test-Path $wt)) { throw "Windows Terminal (wt.exe) not found; install it from the Microsoft Store." }

# Resolve the launcher's absolute path inside WSL (dotfiles PATH copy first,
# then ~/.local/bin fallback).
$resolve = 'command -v claude-rc-boot 2>/dev/null || { [ -x "$HOME/.local/bin/claude-rc-boot" ] && echo "$HOME/.local/bin/claude-rc-boot"; }'
$launcher = (& wsl.exe -d $Distro -- bash -lc $resolve | Select-Object -First 1)
if ($launcher) { $launcher = $launcher.Trim() }
if (-not $launcher) {
    throw "claude-rc-boot not found in $Distro (not on PATH and no ~/.local/bin/claude-rc-boot). Install the dotfiles bin or copy opt/bin/claude-rc-boot to ~/.local/bin first."
}

$prefixArg = if ($Prefix) { " " + $Prefix } else { '' }
$inner  = "exec '$launcher'$prefixArg"
$wtArgs = "wsl.exe -d $Distro -- bash -c `"$inner`""

$ws  = New-Object -ComObject WScript.Shell
$lnk = $ws.CreateShortcut($lnkPath)
$lnk.TargetPath       = $wt
$lnk.Arguments        = $wtArgs
$lnk.Description       = 'Auto-start a Claude remote-control session when WSL comes up'
$lnk.WorkingDirectory = $env:USERPROFILE
$lnk.Save()

Write-Host "Installed logon shortcut: $lnkPath"
Write-Host "  launcher: $launcher"
Write-Host "  command : $wt $wtArgs"
Write-Host "Test now:  double-click the shortcut, or reboot/sign out and back in."
Write-Host "Remove  :  re-run with -Uninstall"
