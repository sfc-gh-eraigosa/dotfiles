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
$ErrorActionPreference = 'Continue'

$base    = Join-Path $env:USERPROFILE 'Claude\SecurityAudit'
$taskDir = Join-Path $env:USERPROFILE 'Claude\Scheduled\weekly-security-audit'
$histDir = Join-Path $base 'history'
New-Item -ItemType Directory -Force -Path $base, $histDir | Out-Null

$sections = @()
$sections += "AUDIT TIMESTAMP: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss zzz')"
$sections += "COMPUTER: $env:COMPUTERNAME  USER: $env:USERNAME"

$sections += "`r`n=== 1. NON-MICROSOFT AUTO-START SERVICES ==="
$sections += (Get-CimInstance Win32_Service |
    Where-Object { $_.StartMode -eq 'Auto' -and $_.PathName -notmatch '\\Windows\\' } |
    Sort-Object DisplayName |
    Format-Table -Auto State, DisplayName, PathName | Out-String -Width 400).TrimEnd()

$sections += "`r`n=== 2. NON-MICROSOFT ACTIVE SCHEDULED TASKS ==="
$sections += (Get-ScheduledTask |
    Where-Object { $_.TaskPath -notmatch '^\\Microsoft' -and $_.State -ne 'Disabled' } |
    Format-Table -Auto State, TaskPath, TaskName | Out-String -Width 400).TrimEnd()

$sections += "`r`n=== 3. STARTUP COMMANDS ==="
$sections += (Get-CimInstance Win32_StartupCommand |
    Format-Table -Auto Name, Location | Out-String -Width 400).TrimEnd()

$sections += "`r`n=== 4. PROCESSES FROM APPDATA/TEMP ==="
$sections += ((Get-Process |
    Where-Object { $_.Path -match 'AppData\\(Roaming|Local\\Temp)' } |
    Select-Object -ExpandProperty Path | Sort-Object -Unique) -join "`r`n")

$sections += "`r`n=== 5. DEFENDER ==="
$sections += (Get-MpComputerStatus |
    Format-List RealTimeProtectionEnabled, AntivirusEnabled, AntivirusSignatureLastUpdated, QuickScanEndTime |
    Out-String).TrimEnd()

$report = $sections -join "`r`n"

$report | Set-Content -Path (Join-Path $base 'latest-audit.txt') -Encoding UTF8
if (Test-Path $taskDir) {
    $report | Set-Content -Path (Join-Path $taskDir 'latest-audit.txt') -Encoding UTF8
}
$report | Set-Content -Path (Join-Path $histDir ("audit-{0}.txt" -f (Get-Date -Format 'yyyy-MM-dd'))) -Encoding UTF8
Get-ChildItem $histDir -Filter 'audit-*.txt' | Sort-Object Name -Descending |
    Select-Object -Skip 20 | Remove-Item -Force

# Optional Google Drive fallback copy (only when Drive for Desktop syncs locally).
foreach ($gd in @((Join-Path $env:USERPROFILE 'My Drive'), 'G:\My Drive')) {
    if (Test-Path $gd) {
        Copy-Item (Join-Path $base 'latest-audit.txt') (Join-Path $gd 'security-audit-latest.txt') -Force
        break
    }
}
