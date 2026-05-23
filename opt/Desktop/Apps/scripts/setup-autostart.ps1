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
$alias    = "$env:LOCALAPPDATA\Microsoft\WindowsApps\AutoHotkey.exe"
$script   = Join-Path $PSScriptRoot 'macos.ahk'
$log      = 'C:\Windows\Temp\macos-hotkeys-setup.log'

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
    if (-not (Test-Path $alias))  { throw "AutoHotkey alias not found: $alias" }
    if (-not (Test-Path $script)) { throw "Script not found: $script" }

    $action    = New-ScheduledTaskAction -Execute $alias -Argument ('"{0}"' -f $script)
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
}
