<#
.SYNOPSIS
  Install (or remove) a Windows logon shortcut that opens a WSL terminal and
  auto-starts a named Claude remote-control session via opt/bin/claude-rc-boot.
  Companion to setup-sshd.ps1. Not wired into install.sh — run it when you want it.

.DESCRIPTION
  Creates ~\...\Startup\claude-remote.lnk pointing at
  Windows Terminal -> wsl -d <Distro> -> bash -lc -> claude-rc-boot.
  Also drops a copy of the shortcut on the Desktop so the session can be
  (re)started manually without digging into shell:startup.
  The launcher's absolute path inside WSL is resolved at install time (prefers
  the dotfiles PATH copy, falls back to ~/.local/bin/claude-rc-boot).
  claude-rc-boot is idempotent, so the Startup shortcut racing another launch
  source (e.g. Windows Terminal's restored window layout) starts one session.

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
# GetFolderPath follows OneDrive Desktop redirection; $env:USERPROFILE\Desktop does not.
$desktopLnk = Join-Path ([Environment]::GetFolderPath('Desktop')) 'claude-remote.lnk'

if ($Uninstall) {
    foreach ($p in @($lnkPath, $desktopLnk)) {
        if (Test-Path $p) { Remove-Item $p -Force; Write-Host "Removed $p" }
        else { Write-Host "No shortcut at $p (nothing to remove)" }
    }
    return
}

$wt = Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps\wt.exe'
if (-not (Test-Path $wt)) { throw "Windows Terminal (wt.exe) not found; install it from the Microsoft Store." }

# Resolve the launcher's absolute path inside WSL (dotfiles PATH copy first,
# then ~/.local/bin fallback). The login shell (-l) may print profile banners to
# stdout, so the real answer is sentinel-prefixed and everything else discarded.
# No double quotes in the snippet: PowerShell 5.1 re-quotes native args naively
# and embedded quotes get mangled on the way into wsl.exe.
$resolve = 'p=$(command -v claude-rc-boot 2>/dev/null) || p=$HOME/.local/bin/claude-rc-boot; [ -x $p ] && echo RCBOOT:$p'
$launcher = (& wsl.exe -d $Distro -- bash -lc $resolve) |
    Where-Object { $_ -match '^RCBOOT:/' } | Select-Object -First 1
if ($launcher) { $launcher = $launcher.Trim().Substring(7) }
if (-not $launcher) {
    # Multi-word snippets don't survive every PS->wsl.exe arg-quoting path (e.g.
    # when this script is itself invoked from inside WSL). Probe ~/.local/bin
    # through the \\wsl$ share instead — no shell quoting involved.
    $unc = Get-ChildItem "\\wsl$\$Distro\home\*\.local\bin\claude-rc-boot" -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($unc -and $unc.FullName -match '\\home\\([^\\]+)\\') {
        $launcher = "/home/$($Matches[1])/.local/bin/claude-rc-boot"
    }
}
if (-not $launcher) {
    throw "claude-rc-boot not found in $Distro (not on PATH and no ~/.local/bin/claude-rc-boot). Install the dotfiles bin or copy opt/bin/claude-rc-boot to ~/.local/bin first."
}

$prefixArg = if ($Prefix) { " " + $Prefix } else { '' }
$inner  = "exec '$launcher'$prefixArg"
# -l (login) is load-bearing: it makes bash source ~/.profile, so the Claude
# session (and everything it spawns) inherits the full dotfiles environment.
# A plain `bash -c` here is how autostarted sessions ended up with a bare PATH.
$wtArgs = "wsl.exe -d $Distro -- bash -lc `"$inner`""

# Distinct icon (WSL penguin) so the shortcut doesn't blend in with plain
# Windows Terminal launchers; skipped silently on hosts without System32\wsl.exe.
$icon = Join-Path $env:SystemRoot 'System32\wsl.exe'

$ws = New-Object -ComObject WScript.Shell
foreach ($p in @($lnkPath, $desktopLnk)) {
    $lnk = $ws.CreateShortcut($p)
    $lnk.TargetPath       = $wt
    $lnk.Arguments        = $wtArgs
    $lnk.Description       = 'Auto-start a Claude remote-control session when WSL comes up'
    $lnk.WorkingDirectory = $env:USERPROFILE
    if (Test-Path $icon) { $lnk.IconLocation = "$icon,0" }
    $lnk.Save()
    Write-Host "Installed shortcut: $p"
}
Write-Host "  launcher: $launcher"
Write-Host "  command : $wt $wtArgs"
Write-Host "Test now:  double-click a shortcut, or reboot/sign out and back in."
Write-Host "Remove  :  re-run with -Uninstall"
