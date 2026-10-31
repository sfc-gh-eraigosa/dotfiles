<#
.SYNOPSIS
  Sign-in recovery for the wslgiga1 workstation. Run by the PlaygroundLogon task
  (at logon, the operator's account, not elevated) registered by install-wslgiga1.ps1.
.DESCRIPTION
  1. Take a named mutex; a second run exits at once.
  2. Read the pause flag <StateDir>\paused (spec 3.1: the file existing means
     paused, whatever its content). Only author "watchdog" may be auto-resumed.
     "person", an unknown author, or a file that cannot be read or parsed all mean
     paused: start the app and stop, WSL left alone.
  3. Otherwise: Start-DevStack.ps1, then
     `k3sops node resume --if-author watchdog` inside the distro, then the app.
  Every step is logged with a timestamp to <StateDir>\logs\logon.log. A failed
  step never stops the app starting.

  The app path is `exePath` in <StateDir>\desktop.json, else
  <StateDir>\wslgiga1-status.exe. A missing exe is logged and skipped.
.PARAMETER StateDir
  Default: $env:LOCALAPPDATA\playground.
.PARAMETER DryRun
  Read the flag and decide, print every step as DRYRUN, run nothing, write no log.
#>
param(
    [string]$StateDir,
    [switch]$DryRun
)
$ErrorActionPreference = 'Continue'
$env:WSL_UTF8 = '1'
if (-not $StateDir) { $StateDir = Join-Path $env:LOCALAPPDATA 'playground' }
$logFile = Join-Path (Join-Path $StateDir 'logs') 'logon.log'
. (Join-Path $PSScriptRoot 'lib\wsl.ps1')   # sets $WslDistro

function Log($m) {
    $line = '{0} {1}' -f (Get-Date -Format 'yyyy-MM-ddTHH:mm:sszzz'), $m
    Write-Host $line
    if ($DryRun) { return }
    try {
        $null = New-Item -ItemType Directory -Force -Path (Split-Path $logFile) -ErrorAction Stop
        $line | Out-File -FilePath $logFile -Append -Encoding ascii
    } catch { }
}

# Get-PauseState: 'none' (no flag), 'watchdog' (auto-resumable), or 'hold'
# (person, unknown author, or unreadable: never auto-resume).
function Get-PauseState([string]$dir) {
    $file = Join-Path $dir 'paused'
    # Only "not found" means no flag. Any other error (access denied, parent not
    # traversable) cannot show the flag is absent, so it holds.
    try { $null = Get-Item -LiteralPath $file -Force -ErrorAction Stop }
    catch [System.Management.Automation.ItemNotFoundException] { return @{ State = 'none'; Why = 'no paused flag' } }
    catch { return @{ State = 'hold'; Why = "paused flag cannot be checked ($($_.Exception.GetType().Name)); treated as paused" } }
    try {
        $raw = Get-Content -LiteralPath $file -Raw -ErrorAction Stop
        # PS 5.1 unwraps a one-element JSON array into its member: require a literal object.
        if ($raw.TrimStart() -notmatch '^\{') { throw 'not a JSON object' }
        $obj = $raw | ConvertFrom-Json -ErrorAction Stop
        if ($obj -isnot [pscustomobject]) { throw 'not a JSON object' }
        $author = [string]$obj.author
    } catch {
        return @{ State = 'hold'; Why = 'paused flag exists but cannot be read; treated as paused' }
    }
    if ($author -ceq 'watchdog') { return @{ State = 'watchdog'; Why = 'paused by the watchdog' } }
    if ($author -ceq 'person') { return @{ State = 'hold'; Why = 'paused by a person' } }
    return @{ State = 'hold'; Why = "paused flag has author '$author'; treated as paused" }
}

# Step: run a scriptblock, log its output and exit code, never throw.
function Step([string]$name, [scriptblock]$body) {
    if ($DryRun) { Log "DRYRUN step: $name"; return }
    Log "step: $name"
    try {
        $global:LASTEXITCODE = 0
        $out = & $body 2>&1
        $rc = $LASTEXITCODE
        foreach ($l in @($out)) { $t = "$l".Trim(); if ($t) { Log "  $t" } }
        if ($rc -ne 0 -and $null -ne $rc) { Log "step failed: $name (exit $rc)" } else { Log "step ok: $name" }
    } catch {
        Log "step failed: $name ($($_.Exception.Message))"
    }
}

function Get-AppExe([string]$dir) {
    $exe = Join-Path $dir 'wslgiga1-status.exe'
    $cfg = Join-Path $dir 'desktop.json'
    if (Test-Path -LiteralPath $cfg) {
        try {
            $c = Get-Content -LiteralPath $cfg -Raw -ErrorAction Stop | ConvertFrom-Json -ErrorAction Stop
            if ($c.exePath -is [string] -and $c.exePath.Trim()) { $exe = $c.exePath.Trim() }
        } catch { Log "desktop.json unreadable ($($_.Exception.Message)); using the default app path" }
    }
    return $exe
}

function Start-App {
    $ErrorActionPreference = 'Stop'   # this function scope only: any error is caught and logged
    try { Start-AppUnchecked } catch { Log "app step failed: $($_.Exception.Message)" }
}
function Start-AppUnchecked {
    $exe = Get-AppExe $StateDir
    if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) { Log "app not found: $exe (skipped)"; return }
    $name = [IO.Path]::GetFileNameWithoutExtension($exe)
    if (Get-Process -Name $name -ErrorAction SilentlyContinue) { Log "app already running: $name"; return }
    if ($DryRun) { Log "DRYRUN would start app: $exe"; return }
    try { Start-Process -FilePath $exe; Log "app started: $exe" }
    catch { Log "app failed to start: $($_.Exception.Message)" }
}

$created = $false
$mutex = New-Object System.Threading.Mutex($false, 'Local\PlaygroundLogon', [ref]$created)
$held = $false
try {
    try { $held = $mutex.WaitOne(0) } catch [System.Threading.AbandonedMutexException] { $held = $true }
    if (-not $held) { Log 'another PlaygroundLogon run holds the lock; exiting'; exit 0 }

    Log "logon recovery start (distro $WslDistro, state dir $StateDir)$(if ($DryRun) { ' [DRYRUN]' })"
    $p = Get-PauseState $StateDir
    Log "pause state: $($p.State) ($($p.Why))"

    if ($p.State -eq 'hold') {
        Log 'leaving WSL alone; starting the app only'
        Start-App
        Log 'logon recovery done'
        exit 0
    }

    $startScript = Join-Path $PSScriptRoot 'Start-DevStack.ps1'
    Step 'Start-DevStack.ps1' {
        & (Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe') -NoProfile -ExecutionPolicy Bypass -File $startScript
    }
    Step 'k3sops node resume --if-author watchdog' {
        & (Join-Path $env:SystemRoot 'System32\wsl.exe') -d $WslDistro --exec bash -lc '$HOME/.local/bin/k3sops node resume --if-author watchdog'
    }
    Start-App
    Log 'logon recovery done'
    exit 0
} finally {
    if ($held) { $mutex.ReleaseMutex() }
    $mutex.Dispose()
}
