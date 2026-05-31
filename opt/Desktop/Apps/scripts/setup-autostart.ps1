# ============================================================================
#  Registers a Task Scheduler job that auto-starts the macOS-style hotkeys
#  (macos.ahk) at logon, elevated, so the shortcuts also work in apps that
#  run as administrator (Task Manager, installers, etc.).
#
#  Re-run this any time you need to recreate the task. It self-elevates
#  (you'll see one UAC prompt). Right-click -> "Run with PowerShell", or just
#  double-click is fine.
# ============================================================================

$ErrorActionPreference = 'Stop'

$taskName = 'macOS Hotkeys'
$user     = "$env:USERDOMAIN\$env:USERNAME"
$log      = 'C:\Windows\Temp\macos-hotkeys-setup.log'

# AutoHotkey launcher: prefer the Store-edition execution alias, fall back to the
# standard per-user v2 install (the alias can be missing under elevation).
$alias  = "$env:LOCALAPPDATA\Microsoft\WindowsApps\AutoHotkey.exe"
$altExe = "$env:LOCALAPPDATA\Programs\AutoHotkey\v2\AutoHotkey64.exe"
$exe    = if (Test-Path $alias) { $alias } elseif (Test-Path $altExe) { $altExe } else { $alias }

# Resolve macos.ahk to a LOCAL path. A logon task must NOT point at a network
# path: if this script is run from \\wsl.localhost\... (the WSL repo mounted into
# Windows), that share is usually unavailable early at logon, so AutoHotkey starts
# with no script loaded and EVERY shortcut silently dies. macos.ahk also loads a
# sibling (screenshot-window.ps1) via A_ScriptDir, so we point at the DEPLOYED
# Desktop folder that keeps both files together rather than an isolated copy.
$deployed = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Apps\scripts\macos.ahk'
$here     = Join-Path $PSScriptRoot 'macos.ahk'
if ($PSScriptRoot -like '\\*') {
    # Invoked from a UNC/network path (e.g. \\wsl.localhost\Ubuntu\...): never bake
    # that into a logon task. Require the locally-deployed copy instead.
    if (-not (Test-Path $deployed)) {
        throw "Running from a network path ($PSScriptRoot); a logon task needs a LOCAL copy. Deploy the scripts to your Desktop (Apps\scripts) first and re-run, or ensure '$deployed' exists."
    }
    $script = $deployed
} elseif (Test-Path $deployed) {
    $script = $deployed     # prefer the canonical deployed copy
} else {
    $script = $here         # running from a local checkout with no deploy yet
}

# --- self-elevate if not already running as administrator -------------------
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
            ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Start-Process powershell -Verb RunAs -ArgumentList @(
        '-NoProfile','-ExecutionPolicy','Bypass','-File',"`"$PSCommandPath`""
    )
    return
}

try {
    if (-not (Test-Path $exe))    { throw "AutoHotkey not found: $exe" }
    if (-not (Test-Path $script)) { throw "Script not found: $script" }

    Write-Output "Task will launch: $exe `"$script`""
    $action    = New-ScheduledTaskAction -Execute $exe -Argument ('"{0}"' -f $script)
    $trigger   = New-ScheduledTaskTrigger -AtLogOn -User $user
    $principal = New-ScheduledTaskPrincipal -UserId $user -LogonType Interactive -RunLevel Highest
    $settings  = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
                    -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew -StartWhenAvailable

    Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger `
        -Principal $principal -Settings $settings `
        -Description 'Starts macOS-style keyboard shortcuts (macos.ahk) at logon.' -Force | Out-Null

    "OK $(Get-Date -Format o): registered task '$taskName' for $user" | Out-File $log -Encoding utf8
    Write-Output "Registered scheduled task '$taskName'."
} catch {
    "ERROR $(Get-Date -Format o): $($_.Exception.Message)" | Out-File $log -Encoding utf8
    Write-Output "FAILED: $($_.Exception.Message)"
    exit 1   # non-zero so the caller (install_windows.sh, set -e) sees the failure
}
