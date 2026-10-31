<#
.SYNOPSIS
  Apply the GPU power cap (watts) at boot. Run by the SYSTEM startup task
  PlaygroundGpuCapApply, registered by install-wslgiga1.ps1.
.DESCRIPTION
  Reads <StateDir>\gpu-cap-w (an integer 100-350; default 280 when the file is
  missing or invalid), runs `nvidia-smi.exe -pl <n>`, reads `power.limit` back
  from every GPU and exits non-zero if any differs. Logs to <StateDir>\logs\.

  StateDir is the OPERATOR's %LOCALAPPDATA%\playground. Under SYSTEM,
  %LOCALAPPDATA% is SYSTEM's own profile, so the installer passes the operator's
  absolute path as -StateDir in the task action. Without it the script falls back
  to $env:LOCALAPPDATA and says so in the log.

  Exit: 0 applied and verified (or dry run), 1 mismatch or nvidia-smi failed,
  2 nvidia-smi.exe not found.
.PARAMETER StateDir
  Folder holding gpu-cap-w and logs\. Default: $env:LOCALAPPDATA\playground.
.PARAMETER DryRun
  Resolve and print the cap and the command; do not run nvidia-smi, do not write a log.
.EXAMPLE
  .\playground-gpu-cap.ps1 -StateDir C:\Temp\cap-test -DryRun
#>
param(
    [string]$StateDir,
    [switch]$DryRun
)
$ErrorActionPreference = 'Stop'
$DefaultWatts = 280
$MinWatts = 100
$MaxWatts = 350

# Get-CapWatts: the cap in <dir>\gpu-cap-w, or the default. Returns Watts and Note.
function Get-CapWatts([string]$dir) {
    $file = Join-Path $dir 'gpu-cap-w'
    if (-not (Test-Path -LiteralPath $file -PathType Leaf)) {
        return @{ Watts = $DefaultWatts; Note = "no gpu-cap-w, default $DefaultWatts" }
    }
    $text = ''
    try { $text = ([string](Get-Content -LiteralPath $file -TotalCount 1 -ErrorAction Stop)).Trim() }
    catch { return @{ Watts = $DefaultWatts; Note = "gpu-cap-w unreadable or empty, default $DefaultWatts" } }
    if ($text -match '^[0-9]{1,4}$') {
        $n = [int]$text
        if ($n -ge $MinWatts -and $n -le $MaxWatts) { return @{ Watts = $n; Note = 'from gpu-cap-w' } }
    }
    return @{ Watts = $DefaultWatts; Note = "gpu-cap-w invalid (want integer $MinWatts-$MaxWatts), default $DefaultWatts" }
}

$fellBack = $false
if (-not $StateDir) { $StateDir = Join-Path $env:LOCALAPPDATA 'playground'; $fellBack = $true }
$logFile = Join-Path (Join-Path $StateDir 'logs') 'gpu-cap.log'
function Log($m) {
    $line = '{0} {1}' -f (Get-Date -Format 'yyyy-MM-ddTHH:mm:sszzz'), $m
    Write-Host $line
    if ($DryRun) { return }
    try {
        $null = New-Item -ItemType Directory -Force -Path (Split-Path $logFile) -ErrorAction Stop
        $line | Out-File -FilePath $logFile -Append -Encoding ascii
    } catch { }   # a log that cannot be written must not stop the cap
}

if ($fellBack) { Log "StateDir not given; using $StateDir (under SYSTEM that is SYSTEM's profile, not the operator's)" }
$cap = Get-CapWatts $StateDir
Log "cap $($cap.Watts) W ($($cap.Note)); state dir $StateDir"

$smi = Join-Path $env:SystemRoot 'System32\nvidia-smi.exe'
if (-not (Test-Path -LiteralPath $smi)) {
    $found = Get-Command nvidia-smi.exe -ErrorAction SilentlyContinue
    if ($found) { $smi = $found.Source } else { $smi = $null }
}
if ($DryRun) {
    Log "DRYRUN would run: $(if ($smi) { $smi } else { 'nvidia-smi.exe (not found)' }) -pl $($cap.Watts), then read power.limit back"
    exit 0
}
if (-not $smi) { Log 'ERROR nvidia-smi.exe not found'; exit 2 }

try {
    $out = & $smi -pl $cap.Watts 2>&1
    $rc = $LASTEXITCODE
    foreach ($l in @($out)) { if ("$l".Trim()) { Log "smi: $l" } }
    if ($rc -ne 0) { Log "ERROR nvidia-smi -pl exited $rc"; exit 1 }

    $read = @(& $smi --query-gpu=power.limit --format=csv,noheader,nounits 2>&1)
    if ($LASTEXITCODE -ne 0) { Log "ERROR power.limit query exited $LASTEXITCODE"; exit 1 }
    $bad = 0
    $i = 0
    foreach ($l in $read) {
        $s = "$l".Trim()
        if (-not $s) { continue }
        $v = 0.0
        $ok = [double]::TryParse($s, [Globalization.NumberStyles]::Float, [Globalization.CultureInfo]::InvariantCulture, [ref]$v)
        if ($ok -and [math]::Round($v) -eq $cap.Watts) { Log "gpu $i power.limit $s W matches" }
        else { Log "ERROR gpu $i power.limit '$s' does not match $($cap.Watts)"; $bad++ }
        $i++
    }
    if ($i -eq 0) { Log 'ERROR power.limit read back returned nothing'; exit 1 }
    if ($bad -gt 0) { exit 1 }
    Log 'cap applied and verified'
    exit 0
} catch {
    Log "ERROR $($_.Exception.Message)"
    exit 1
}
