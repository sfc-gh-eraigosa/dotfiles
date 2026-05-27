<#
.SYNOPSIS
    Keep the Windows system awake (no system sleep) while letting the monitors
    sleep normally. Optionally blank the displays now and schedule a reminder to
    revert the change.

.NOTES
    Created for the dotfiles "keep-awake" skill. Targets the Windows host via
    powercfg + Task Scheduler. Safe to re-run; the reminder task is replaced.
#>
[CmdletBinding()]
param(
    # Local time (24h HH:mm) for the revert reminder. Fires today if still in the
    # future, otherwise tomorrow. Ignored when -NoReminder is set.
    [string]$RemindAt = '08:00',

    # Turn the monitors off immediately.
    [switch]$SleepMonitors,

    # Also prevent sleep while on battery (DC), not just on AC.
    [switch]$IncludeBattery,

    # Skip scheduling the revert reminder.
    [switch]$NoReminder
)

$ErrorActionPreference = 'Stop'

function Get-StandbyTimeoutAcSeconds {
    $line = (powercfg /query SCHEME_CURRENT SUB_SLEEP STANDBYIDLE |
             Select-String 'Current AC Power Setting Index').ToString()
    $hex = $line.Split(':')[1].Trim()
    return [Convert]::ToInt32($hex, 16)
}

# 1. Remember the current AC sleep timeout so the reminder can restore it.
$origMinutes = [math]::Round((Get-StandbyTimeoutAcSeconds) / 60)
if ($origMinutes -lt 1) { $origMinutes = 5 }  # was already "never" -> sane default

# 2. Keep the system awake (never sleep) on AC.
powercfg /change standby-timeout-ac 0
if ($IncludeBattery) { powercfg /change standby-timeout-dc 0 }
$scope = if ($IncludeBattery) { 'AC + battery' } else { 'AC' }
Write-Host "Keep-awake ON: system sleep disabled ($scope). Monitor timeout unchanged."

# 3. Schedule the revert reminder.
if (-not $NoReminder -and $RemindAt) {
    $parsed = [datetime]::ParseExact($RemindAt, 'HH:mm', $null)
    $when = (Get-Date).Date.AddHours($parsed.Hour).AddMinutes($parsed.Minute)
    if ($when -le (Get-Date)) { $when = $when.AddDays(1) }

    $revert = Join-Path $PSScriptRoot 'revert-keepawake.ps1'
    $argLine = '-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "{0}" -Minutes {1}' -f $revert, $origMinutes
    if ($IncludeBattery) { $argLine += ' -IncludeBattery' }

    $action  = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument $argLine
    $trigger = New-ScheduledTaskTrigger -Once -At $when
    $set     = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
    Register-ScheduledTask -TaskName 'Claude - Revert keep-awake' `
        -Action $action -Trigger $trigger -Settings $set `
        -Description 'Reminds/offers to revert the AC system-sleep timeout after a keep-awake session.' `
        -Force | Out-Null
    Write-Host ("Revert reminder scheduled for {0} (restores {1} min)." -f $when.ToString('ddd MMM d, h:mm tt'), $origMinutes)
}

# 4. Optionally put the monitors to sleep now.
if ($SleepMonitors) {
    Add-Type -Name Mon -Namespace Win -MemberDefinition '[DllImport("user32.dll")] public static extern int SendMessage(int hWnd, int hMsg, int wParam, int lParam);'
    Start-Sleep -Milliseconds 800   # let the launching keystroke settle
    [Win.Mon]::SendMessage(0xffff, 0x0112, 0xF170, 2) | Out-Null  # HWND_BROADCAST, WM_SYSCOMMAND, SC_MONITORPOWER, OFF
    Write-Host 'Monitors put to sleep.'
}
