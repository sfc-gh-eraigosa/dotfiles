<#
.SYNOPSIS
  Read-only weekly security-audit collector. Snapshots the five audit lenses
  and writes them to text files that the Claude "weekly-security-audit"
  scheduled task analyzes unattended (no computer-use, no approvals).

.DESCRIPTION
  Sections (keep formats/filters stable - the analysis baseline depends on them):
    1. Non-Microsoft auto-start services
    2. Non-Microsoft active scheduled tasks
    3. Startup commands (Win32_StartupCommand lens)
    4. Processes running from AppData\Roaming or AppData\Local\Temp
    5. Microsoft Defender status

  Every section resolves to exactly one of three states, so the analysis run can
  never mistake a broken lens for a clean machine:
    <data>                  - the lens ran and found items
    (none)                  - the lens ran and found nothing
    !! COLLECTION ERROR: .. - the lens itself failed (report it, do not read as clean)

  Writes:
    %USERPROFILE%\Claude\SecurityAudit\latest-audit.txt          (canonical)
    %USERPROFILE%\Claude\Scheduled\weekly-security-audit\latest-audit.txt
        (the Claude task folder - delivered to the analysis run as its uploads)
    %USERPROFILE%\Claude\SecurityAudit\history\audit-YYYY-MM-DD.txt (keeps 20)
    <Google Drive>\security-audit-latest.txt                      (fallback,
        only when a local Drive sync folder exists)
  Registered by setup-security-audit.ps1 as user-level task
  'ClaudeSecurityAuditCollector' (daily, hidden). Runs fine standalone too.
  Read-only WMI/CIM + Defender queries; no admin, no network calls.
#>

# 'Stop' (not 'Continue') so a failing cmdlet raises a catchable terminating
# error that Add-Section turns into a visible "!! COLLECTION ERROR" marker.
# Under 'Continue' the error went to a stderr nobody reads and the section was
# written EMPTY - indistinguishable from "nothing suspicious found".
$ErrorActionPreference = 'Stop'

$base    = Join-Path $env:USERPROFILE 'Claude\SecurityAudit'
$taskDir = Join-Path $env:USERPROFILE 'Claude\Scheduled\weekly-security-audit'
$histDir = Join-Path $base 'history'
New-Item -ItemType Directory -Force -Path $base, $histDir | Out-Null

$sections = @()

# Add-Section - run one lens, and record its outcome unambiguously. A lens that
# throws must never collapse into an empty section (see .DESCRIPTION).
function Add-Section {
    param(
        [Parameter(Mandatory)][string]$Title,
        [Parameter(Mandatory)][scriptblock]$Body
    )
    $script:sections += "`r`n=== $Title ==="
    try {
        $out = (& $Body | Out-String -Width 400).TrimEnd()
        if ([string]::IsNullOrWhiteSpace($out)) { $out = '(none)' }
        $script:sections += $out
    } catch {
        $script:sections += "!! COLLECTION ERROR: $($_.Exception.Message)"
    }
}

$sections += "AUDIT TIMESTAMP: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss zzz')"
$sections += "COMPUTER: $env:COMPUTERNAME  USER: $env:USERNAME"

# Path-based "non-Microsoft" heuristic: skip services under \Windows\, EXCEPT
# \Windows\Temp\ - that directory is user-writable by default and is a classic
# persistence location, so excluding it would blind the audit's most valuable
# lens. Services with a null PathName fall through and stay visible on purpose.
Add-Section '1. NON-MICROSOFT AUTO-START SERVICES' {
    Get-CimInstance Win32_Service |
        Where-Object {
            $_.StartMode -eq 'Auto' -and
            ($_.PathName -notmatch '\\Windows\\' -or $_.PathName -match '\\Windows\\Temp\\')
        } |
        Sort-Object DisplayName |
        Format-Table -Auto State, DisplayName, PathName
}

Add-Section '2. NON-MICROSOFT ACTIVE SCHEDULED TASKS' {
    Get-ScheduledTask |
        Where-Object { $_.TaskPath -notmatch '^\\Microsoft' -and $_.State -ne 'Disabled' } |
        Format-Table -Auto State, TaskPath, TaskName
}

Add-Section '3. STARTUP COMMANDS' {
    Get-CimInstance Win32_StartupCommand | Format-Table -Auto Name, Location
}

# .Path throws on processes this user cannot open (protected/elevated). Read it
# per-process inside try/catch so those become $null instead of failing the
# whole lens - an access-denied on one PID must not hide every other match.
Add-Section '4. PROCESSES FROM APPDATA/TEMP' {
    Get-Process -ErrorAction SilentlyContinue |
        ForEach-Object { try { $_.Path } catch { $null } } |
        Where-Object { $_ -and $_ -match 'AppData\\(Roaming|Local\\Temp)' } |
        Sort-Object -Unique
}

Add-Section '5. DEFENDER' {
    Get-MpComputerStatus |
        Format-List RealTimeProtectionEnabled, AntivirusEnabled, AntivirusSignatureLastUpdated, QuickScanEndTime
}

$report = $sections -join "`r`n"

# The canonical copy is the one the pipeline is allowed to fail on: if it cannot
# be written there is nothing to analyze, so surface a non-zero task result.
$report | Set-Content -Path (Join-Path $base 'latest-audit.txt') -Encoding UTF8

# Secondary copies are best-effort - a full disk, a locked file, or a missing
# Drive folder must not discard the canonical report that already succeeded.
try {
    if (Test-Path $taskDir) {
        $report | Set-Content -Path (Join-Path $taskDir 'latest-audit.txt') -Encoding UTF8
    }
} catch { Write-Warning "could not write the Claude task-folder copy: $($_.Exception.Message)" }

try {
    $report | Set-Content -Path (Join-Path $histDir ("audit-{0}.txt" -f (Get-Date -Format 'yyyy-MM-dd'))) -Encoding UTF8
    # Names are ISO-dated, so a descending lexical sort is newest-first: keep 20.
    Get-ChildItem $histDir -Filter 'audit-*.txt' | Sort-Object Name -Descending |
        Select-Object -Skip 20 | Remove-Item -Force
} catch { Write-Warning "could not update the history folder: $($_.Exception.Message)" }

# Optional Google Drive fallback copy (only when Drive for Desktop syncs locally).
try {
    foreach ($gd in @((Join-Path $env:USERPROFILE 'My Drive'), 'G:\My Drive')) {
        if (Test-Path $gd) {
            Copy-Item (Join-Path $base 'latest-audit.txt') (Join-Path $gd 'security-audit-latest.txt') -Force
            break
        }
    }
} catch { Write-Warning "could not write the Google Drive fallback copy: $($_.Exception.Message)" }
