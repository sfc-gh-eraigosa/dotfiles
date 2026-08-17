<#
.SYNOPSIS
  Install (or remove) the unattended weekly security-audit pipeline:
  a user-level Windows scheduled task that collects audit data daily, plus
  the Claude "weekly-security-audit" analysis task prompt (seeded if absent).
  Opt-in: gated at the install.sh call site by gff flag
  install.windows.security-audit (boolDefault: false, fail-closed).

.DESCRIPTION
  Why two halves: Claude's scheduled runs cannot approve computer-use access,
  and terminals are never typeable by Claude, so interactive collection can
  never be unattended. Decoupling fixes it - Windows Task Scheduler runs
  security-audit-collect.ps1 (read-only queries) and writes text reports;
  Claude's weekly task just reads latest-audit.txt from its own task folder
  and diffs it against the baseline kept in its SKILL.md.

  This installer (no admin needed - a per-user task):
    1. Copies security-audit-collect.ps1 to %USERPROFILE%\Claude\SecurityAudit
       (NOT run from the Desktop: OneDrive dehydration can break Desktop runs).
    2. Registers user-level task 'ClaudeSecurityAuditCollector'
       (daily at -At, hidden window, battery-friendly, StartWhenAvailable).
    3. Seeds %USERPROFILE%\Claude\Scheduled\weekly-security-audit\SKILL.md from
       security-audit-skill.template.md ONLY when missing - never overwrites,
       because audit runs evolve the baseline inside that file.
    4. Runs the first collection immediately.
  Registering the Claude-side schedule itself is app-managed: after install,
  tell Claude "schedule the weekly-security-audit task for Saturday 10:00".

.PARAMETER At
  Daily collection time (default 09:00 - keep it before the Claude analysis).

.PARAMETER Uninstall
  Remove the scheduled task and installed collector (keeps collected data and
  the Claude task folder; prints what was kept).

.PARAMETER Status
  Report task state, last/next run times, and report freshness, then exit.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File setup-security-audit.ps1
  powershell -ExecutionPolicy Bypass -File setup-security-audit.ps1 -Status
  powershell -ExecutionPolicy Bypass -File setup-security-audit.ps1 -Uninstall
#>
param(
    # Validated here rather than letting New-ScheduledTaskTrigger fail later:
    # -At is parsed culture-sensitively, so an unvalidated string can register a
    # task at an unintended hour instead of erroring. HH:mm, 24-hour, only.
    [ValidatePattern('^([01]\d|2[0-3]):[0-5]\d$')]
    [string]$At = '09:00',
    [switch]$Uninstall,
    [switch]$Status
)
$ErrorActionPreference = 'Stop'

$taskName  = 'ClaudeSecurityAuditCollector'
$base      = Join-Path $env:USERPROFILE 'Claude\SecurityAudit'
$collector = Join-Path $base 'security-audit-collect.ps1'
$claudeDir = Join-Path $env:USERPROFILE 'Claude\Scheduled\weekly-security-audit'
$skillMd   = Join-Path $claudeDir 'SKILL.md'
$latest    = Join-Path $base 'latest-audit.txt'

if ($Status) {
    $t = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    if ($t) {
        $info = $t | Get-ScheduledTaskInfo
        Write-Host "Task    : $taskName  [$($t.State)]"
        Write-Host "LastRun : $($info.LastRunTime)  (result $($info.LastTaskResult))"
        Write-Host "NextRun : $($info.NextRunTime)"
    } else { Write-Host "Task    : $taskName NOT registered" }
    if (Test-Path $latest) {
        $f = Get-Item $latest
        Write-Host "Report  : $latest"
        Write-Host "Written : $($f.LastWriteTime)  ($([int]((Get-Date) - $f.LastWriteTime).TotalHours)h ago)"
    } else { Write-Host "Report  : none at $latest" }
    Write-Host "Claude  : $(if (Test-Path $skillMd) { "SKILL.md present at $claudeDir" } else { 'SKILL.md not seeded' })"
    return
}

if ($Uninstall) {
    if (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue) {
        Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
        Write-Host "Removed scheduled task $taskName"
    } else { Write-Host "No scheduled task $taskName (nothing to remove)" }
    if (Test-Path $collector) { Remove-Item $collector -Force; Write-Host "Removed $collector" }
    Write-Host "Kept: $base (history/reports) and $claudeDir (Claude task + baseline)."
    Write-Host "Delete those folders manually if you want a full wipe."
    return
}

# --- 1. Install the collector to its permanent, OneDrive-free home ----------
New-Item -ItemType Directory -Force -Path $base | Out-Null
Copy-Item -Force (Join-Path $PSScriptRoot 'security-audit-collect.ps1') $collector
Write-Host "Installed collector: $collector"

# --- 2. Register the user-level daily task (no admin) -----------------------
$action   = New-ScheduledTaskAction -Execute 'powershell.exe' `
            -Argument ('-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "{0}"' -f $collector)
$trigger  = New-ScheduledTaskTrigger -Daily -At $At
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
            -StartWhenAvailable -ExecutionTimeLimit (New-TimeSpan -Minutes 10) -MultipleInstances IgnoreNew
Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Settings $settings -Force | Out-Null
Write-Host "Registered task '$taskName' (daily at $At, StartWhenAvailable)."

# --- 3. Seed the Claude analysis task prompt (never overwrite) ---------------
if (-not (Test-Path $skillMd)) {
    New-Item -ItemType Directory -Force -Path $claudeDir | Out-Null
    Copy-Item (Join-Path $PSScriptRoot 'security-audit-skill.template.md') $skillMd
    Write-Host "Seeded Claude analysis task prompt: $skillMd"
    Write-Host "  -> Finish once in the Claude app: 'schedule the weekly-security-audit task for Saturday 10:00'."
} else {
    Write-Host "Claude task prompt already present (left untouched): $skillMd"
}

# --- 4. First collection now, so the pipeline is verified end-to-end --------
Write-Host "Running first collection..."
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $collector
if (Test-Path $latest) {
    $f = Get-Item $latest
    Write-Host "OK: $($f.FullName) ($($f.Length) bytes, $($f.LastWriteTime))"
} else {
    Write-Warning "collector ran but $latest was not created - investigate before relying on it."
}
Write-Host "Done. Check state anytime with: setup-security-audit.ps1 -Status"
