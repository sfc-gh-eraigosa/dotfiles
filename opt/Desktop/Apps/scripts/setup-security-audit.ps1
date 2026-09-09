<#
.SYNOPSIS
  Install (or remove) the unattended security-audit pipeline: a user-level
  Windows scheduled task that collects audit data every evening, plus the Claude
  analysis task prompts (seeded if absent).
  Opt-in: gated at the install.sh call site by gff flag
  install.windows.security-audit (boolDefault: false, fail-closed).

.DESCRIPTION
  Why two halves: Claude's scheduled runs cannot approve computer-use access,
  and terminals are never typeable by Claude, so interactive collection can
  never be unattended. Decoupling fixes it - Windows Task Scheduler runs
  security-audit-collect.ps1 (read-only queries) and writes text reports;
  Claude's tasks just read latest-audit.txt and diff it against the baseline
  kept in their SKILL.md.

  SCHEDULE (the "machine might be off" problem): a single fixed daily time is
  fragile - if the laptop is asleep at that moment the day is simply missed. So
  the task fires EVERY HOUR across an evening window (default 17:00-00:00) and
  the collector's -OncePerDay gate makes all but the first fire ~free (~4ms stat
  vs ~19s CPU for a real collection). Combined with StartWhenAvailable, a machine
  that was off all evening still collects at the next opportunity. Net effect:
  at most one real collection per day, at the first moment the machine is
  actually available.

  CPU HYGIENE: task Priority 7 (background tier), the collector runs
  -LowPriority (BelowNormal), MultipleInstances=IgnoreNew so a slow run is never
  double-started by the next hourly fire, and a 10-minute ExecutionTimeLimit
  caps any runaway.

  This installer (no admin needed - a per-user task):
    1. Copies security-audit-collect.ps1 to %USERPROFILE%\Claude\SecurityAudit
       (NOT run from the Desktop: OneDrive dehydration can break Desktop runs).
    2. Registers user-level task 'ClaudeSecurityAuditCollector'.
    3. Seeds the Claude task prompts from their templates ONLY when missing -
       never overwrites, because audit runs evolve the baseline inside them:
         weekly-security-audit  = the full Saturday summary
         daily-security-triage  = a daily URGENT-ONLY check (quiet when clean)
    4. Runs the first collection immediately.
  Registering the Claude-side schedules is app-managed: see docs/security-audit.md.

.PARAMETER At
  Start of the daily collection window (default 17:00). HH:mm, 24-hour.

.PARAMETER WindowHours
  Hours to keep re-trying hourly from -At (default 7 => 17:00..00:00). 0 disables
  the hourly repetition and leaves a single daily trigger at -At.

.PARAMETER Uninstall
  Remove the scheduled task and installed collector (keeps collected data and
  the Claude task folders; prints what was kept).

.PARAMETER Status
  Report task health, the collection schedule, PROOF of which collector version
  is actually installed vs deployed, report freshness, and the Claude task
  prompts - then exit. Read-only.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File setup-security-audit.ps1
  powershell -ExecutionPolicy Bypass -File setup-security-audit.ps1 -Status
  powershell -ExecutionPolicy Bypass -File setup-security-audit.ps1 -At 18:00 -WindowHours 6
  powershell -ExecutionPolicy Bypass -File setup-security-audit.ps1 -Uninstall
#>
param(
    # Validated here rather than letting New-ScheduledTaskTrigger fail later:
    # -At is parsed culture-sensitively, so an unvalidated string can register a
    # task at an unintended hour instead of erroring. HH:mm, 24-hour, only.
    [ValidatePattern('^([01]\d|2[0-3]):[0-5]\d$')]
    [string]$At = '17:00',
    [ValidateRange(0, 23)]
    [int]$WindowHours = 7,
    [switch]$Uninstall,
    [switch]$Status
)
$ErrorActionPreference = 'Stop'

$taskName   = 'ClaudeSecurityAuditCollector'
$base       = Join-Path $env:USERPROFILE 'Claude\SecurityAudit'
$collector  = Join-Path $base 'security-audit-collect.ps1'
$histDir    = Join-Path $base 'history'
$latest     = Join-Path $base 'latest-audit.txt'
$scheduled  = Join-Path $env:USERPROFILE 'Claude\Scheduled'

# Claude-side task prompts: template file -> task folder name.
$claudeTasks = [ordered]@{
    'weekly-security-audit' = 'security-audit-skill.template.md'
    'daily-security-triage' = 'security-triage-skill.template.md'
}

# --- helpers ----------------------------------------------------------------

# Identity of a collector copy: the version marker it declares + a content hash.
# This is what makes "am I running the latest script?" answerable rather than
# assumed - the hash proves byte-identity, the version proves capability.
function Get-CollectorInfo([string]$path) {
    if (-not (Test-Path -LiteralPath $path)) { return $null }
    $ver = '(no version marker)'
    try {
        $m = Select-String -LiteralPath $path -Pattern "COLLECTOR_VERSION\s*=\s*'([^']+)'" | Select-Object -First 1
        if ($m) { $ver = 'v' + $m.Matches[0].Groups[1].Value }
    } catch { }
    $sha = '(unhashable)'
    try { $sha = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.Substring(0, 16) } catch { }
    return [pscustomobject]@{
        Path = $path; Version = $ver; Sha = $sha
        Written = (Get-Item -LiteralPath $path).LastWriteTime
    }
}

# Task Scheduler reports its own status as HRESULTs; '267011' means nothing to a
# reader, so decode the ones that actually show up.
function Format-TaskResult($code) {
    $c = [int64]$code
    $hex = '0x{0:X8}' -f $c
    $txt = switch ($c) {
        0          { 'success' }
        1           { 'generic failure (the script exited 1)' }
        267008      { 'task is ready' }
        267009      { 'task is currently running' }
        267010      { 'task is disabled' }
        267011      { 'task has NOT YET RUN' }
        267012      { 'no more runs scheduled' }
        267014      { 'task was terminated by the user' }
        2147750687  { 'an instance is already running' }
        2147943645  { 'the Task Scheduler service did not respond' }
        default     { 'see the Task Scheduler history for detail' }
    }
    return "$c ($hex : $txt)"
}

function Get-DeployedCollectorPath {
    return (Join-Path ([Environment]::GetFolderPath('Desktop')) 'Apps\scripts\security-audit-collect.ps1')
}

# --- -Status -----------------------------------------------------------------

if ($Status) {
    # 1. Is the job set up, and is it actually running?
    $t = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    if ($t) {
        Write-Host "Task     : $taskName  [$($t.State)]"
        foreach ($trg in $t.Triggers) {
            $start = try { ([datetime]$trg.StartBoundary).ToString('HH:mm') } catch { $trg.StartBoundary }
            $rep = if ($trg.Repetition -and $trg.Repetition.Interval) {
                       "  repeating every $($trg.Repetition.Interval) for $($trg.Repetition.Duration)"
                   } else { '  (single daily fire)' }
            Write-Host "Schedule : daily at $start$rep"
        }
        $info = $t | Get-ScheduledTaskInfo
        Write-Host "LastRun  : $($info.LastRunTime)"
        Write-Host "LastResult: $(Format-TaskResult $info.LastTaskResult)"
        Write-Host "NextRun  : $($info.NextRunTime)"
        $act = @($t.Actions)[0]
        Write-Host "Runs     : $($act.Execute) $($act.Arguments)"
    } else {
        Write-Host "Task     : $taskName NOT registered  (run this script with no arguments to install it)"
    }

    # 2. PROOF of which collector version is actually in play.
    Write-Host ""
    Write-Host "Collector identity (what the task actually executes):"
    $inst = Get-CollectorInfo $collector
    $dep  = Get-CollectorInfo (Get-DeployedCollectorPath)
    if ($inst) { Write-Host ("  installed : {0,-6} sha256 {1}  {2:yyyy-MM-dd HH:mm}" -f $inst.Version, $inst.Sha, $inst.Written) }
    else       { Write-Host "  installed : MISSING ($collector)" }
    if ($dep)  { Write-Host ("  deployed  : {0,-6} sha256 {1}  {2:yyyy-MM-dd HH:mm}" -f $dep.Version, $dep.Sha, $dep.Written) }
    else       { Write-Host "  deployed  : MISSING ($(Get-DeployedCollectorPath))" }
    if ($inst -and $dep) {
        if ($inst.Sha -eq $dep.Sha) {
            Write-Host "  => UP TO DATE (installed is byte-identical to the deployed copy)"
        } else {
            Write-Host "  => STALE: installed differs from deployed. Re-run this installer to refresh it:"
            Write-Host "     powershell -ExecutionPolicy Bypass -File `"$(Join-Path ([Environment]::GetFolderPath('Desktop')) 'Apps\scripts\setup-security-audit.ps1')`""
        }
    }

    # 3. Is it producing data?
    Write-Host ""
    if (Test-Path -LiteralPath $latest) {
        $f = Get-Item -LiteralPath $latest
        $age = [int]((Get-Date) - $f.LastWriteTime).TotalHours
        $producedBy = '(unknown)'
        try {
            $hdr = Get-Content -LiteralPath $latest -TotalCount 4
            $vl = $hdr | Where-Object { $_ -match '^COLLECTOR:\s*(v\S+)' } | Select-Object -First 1
            if ($vl -and $vl -match '^COLLECTOR:\s*(v\S+)') { $producedBy = $Matches[1] }
        } catch { }
        Write-Host "Report   : $latest"
        Write-Host "           produced by $producedBy, written $($f.LastWriteTime) (${age}h ago, $($f.Length) bytes)"
        if ($age -gt 72) { Write-Host "           WARNING: older than 3 days - the collector may not be running." }
    } else {
        Write-Host "Report   : none at $latest"
    }
    $hist = @(Get-ChildItem -LiteralPath $histDir -Filter 'audit-*.txt' -ErrorAction SilentlyContinue)
    Write-Host "History  : $($hist.Count) dated report(s)$(if ($hist.Count) { ' (newest ' + ($hist | Sort-Object Name -Descending | Select-Object -First 1).Name + ')' })"

    # 4. Claude-side prompts.
    Write-Host ""
    Write-Host "Claude task prompts:"
    foreach ($name in $claudeTasks.Keys) {
        $md = Join-Path (Join-Path $scheduled $name) 'SKILL.md'
        Write-Host ("  {0,-22} {1}" -f $name, $(if (Test-Path -LiteralPath $md) { 'SKILL.md present' } else { 'not seeded' }))
    }
    return
}

# --- -Uninstall --------------------------------------------------------------

if ($Uninstall) {
    if (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue) {
        Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
        Write-Host "Removed scheduled task $taskName"
    } else { Write-Host "No scheduled task $taskName (nothing to remove)" }
    if (Test-Path $collector) { Remove-Item $collector -Force; Write-Host "Removed $collector" }
    Write-Host "Kept: $base (history/reports) and the Claude task folders under $scheduled."
    Write-Host "Delete those folders manually if you want a full wipe."
    return
}

# --- 1. Install the collector to its permanent, OneDrive-free home ----------
New-Item -ItemType Directory -Force -Path $base | Out-Null
Copy-Item -Force (Join-Path $PSScriptRoot 'security-audit-collect.ps1') $collector
$installed = Get-CollectorInfo $collector
Write-Host "Installed collector: $collector  [$($installed.Version) sha256 $($installed.Sha)]"

# --- 2. Register the user-level task (no admin) -----------------------------
# -OncePerDay makes the hourly fires cheap; -LowPriority keeps the real run off
# the foreground's back.
$action = New-ScheduledTaskAction -Execute 'powershell.exe' `
          -Argument ('-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "{0}" -OncePerDay -LowPriority' -f $collector)

# A Daily trigger cannot take -RepetitionInterval directly (parameter sets do not
# compose), so build the repetition on a throwaway -Once trigger and graft it on.
# The result is ONE readable trigger in Task Scheduler: daily at $At, repeating
# hourly for $WindowHours.
$trigger = New-ScheduledTaskTrigger -Daily -At $At
if ($WindowHours -gt 0) {
    $trigger.Repetition = (New-ScheduledTaskTrigger -Once -At $At `
        -RepetitionInterval (New-TimeSpan -Hours 1) `
        -RepetitionDuration (New-TimeSpan -Hours $WindowHours)).Repetition
}

$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
            -StartWhenAvailable -ExecutionTimeLimit (New-TimeSpan -Minutes 10) `
            -MultipleInstances IgnoreNew -Priority 7
Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Settings $settings -Force | Out-Null
$windowEnd = ([datetime]::ParseExact($At, 'HH:mm', $null).AddHours($WindowHours)).ToString('HH:mm')
if ($WindowHours -gt 0) {
    Write-Host "Registered '$taskName': daily $At, retrying hourly until $windowEnd (StartWhenAvailable, one real run/day)."
} else {
    Write-Host "Registered '$taskName': daily at $At (StartWhenAvailable)."
}

# --- 3. Seed the Claude analysis prompts (never overwrite) ------------------
foreach ($name in $claudeTasks.Keys) {
    $dir = Join-Path $scheduled $name
    $md  = Join-Path $dir 'SKILL.md'
    $tpl = Join-Path $PSScriptRoot $claudeTasks[$name]
    if (Test-Path -LiteralPath $md) {
        Write-Host "Claude '$name' prompt already present (left untouched): $md"
        continue
    }
    if (-not (Test-Path -LiteralPath $tpl)) {
        Write-Warning "template missing for '$name': $tpl"
        continue
    }
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Copy-Item $tpl $md
    Write-Host "Seeded Claude '$name' prompt: $md"
}
Write-Host "  -> Finish once in the Claude app (see docs/security-audit.md):"
Write-Host "     'schedule the weekly-security-audit task for Saturdays at 10:00'"
Write-Host "     'schedule the daily-security-triage task daily at 23:30'"

# --- 4. First collection now, so the pipeline is verified end-to-end --------
# No -OncePerDay here: an install should always prove the pipeline works.
Write-Host "Running first collection..."
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $collector
if (Test-Path $latest) {
    $f = Get-Item $latest
    Write-Host "OK: $($f.FullName) ($($f.Length) bytes, $($f.LastWriteTime))"
} else {
    Write-Warning "collector ran but $latest was not created - investigate before relying on it."
}
Write-Host "Done. Check state anytime with: setup-security-audit.ps1 -Status"
