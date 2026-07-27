# setup-elevated.ps1 — ALL elevation-requiring Windows setup, run once under a
# single UAC prompt. setup-apps.ps1 launches this with Start-Process -Verb RunAs
# (local CWD + local -File) after its non-elevated app pass, so the user approves
# ONE prompt instead of one per installer. Safe to run standalone too: if launched
# non-elevated it self-elevates (one UAC), then performs every admin task here.
#
# Each step is best-effort and logged to %USERPROFILE%\setup-elevated.log (user-
# readable — no second elevation needed to inspect it) so one failure never
# aborts the rest. Tasks:
#   1. macOS-style hotkeys logon task  (setup-autostart.ps1)
#   2. iTunes Win32 build              (winget Apple.iTunes; Store build removed by setup-apps.ps1)
#   3. Wispr Flow MSI                  (wispr-install-core.ps1)
#   4. PowerToys Copilot-key -> F24    (suppress-copilot-key.ps1)
param([string]$GffEnv = '')

$ErrorActionPreference = 'Continue'   # one failing step must not abort the rest
$log = Join-Path $env:USERPROFILE 'setup-elevated.log'
"=== setup-elevated $(Get-Date -Format o) ===" | Set-Content $log -Encoding utf8

function Log($m) { Write-Host $m; "$m" | Add-Content $log -Encoding utf8 }

# --- self-elevate if launched standalone without admin ----------------------
# Use a LOCAL deployed copy + local CWD so the elevated child does not inherit an
# inaccessible \\wsl.localhost\... path/CWD (which would kill it on startup).
# -GffEnv is forwarded: the elevated child gets a FRESH environment, so the flag
# state must ride the command line (see the seeding block below).
$admin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
          ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $admin) {
    $selfDeployed = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Apps\scripts\setup-elevated.ps1'
    $self = if ($PSCommandPath -like '\\*' -and (Test-Path $selfDeployed)) { $selfDeployed } else { $PSCommandPath }
    Start-Process powershell -Verb RunAs -WorkingDirectory $env:SystemRoot -ArgumentList @(
        '-NoProfile','-ExecutionPolicy','Bypass','-File',"`"$self`"",'-GffEnv',"`"$GffEnv`"") -Wait
    return
}

# Local deployed scripts dir (this file is admin-launched from a local path).
$dir = $PSScriptRoot

# gff feature gates (fail-open): flags cross from WSL via WSLENV (see
# opt/bin/install_windows.sh). A missing lib must never block setup, so fall
# back to an always-on Test-GffOn when lib\gff.ps1 is absent.
$gffLib = Join-Path $dir 'lib\gff.ps1'
if (Test-Path $gffLib) { . $gffLib }
else { function Test-GffOn([string]$Key) { return $true } }

# GFF_* does NOT survive the UAC boundary as environment (Start-Process -Verb
# RunAs starts the child with a fresh env — dotfiles gff TRACKING §10, proven
# 2026-07-26). The parent passes the flag states as -GffEnv "NAME=value;…";
# seed them into $env: so Test-GffOn works unchanged. Strictly validated;
# malformed pairs are logged and ignored (fail-open).
foreach ($pair in ($GffEnv -split ';' | Where-Object { $_ })) {
    if ($pair -match '^(GFF_INSTALL_WINDOWS_[A-Z_]+)=(true|false|[a-z0-9,-]+)$') {
        Set-Item -Path ('env:' + $Matches[1]) -Value $Matches[2]
    } else { Log "  WARNING: ignored malformed -GffEnv pair: $pair" }
}

# 1) macOS-style hotkeys logon task. setup-autostart.ps1 skips its own self-elevate
#    because we are already admin, registers the task, and reloads AutoHotkey.
if (Test-GffOn 'install.windows.ahk-autostart') {
    try {
        Log '== [1/4] macOS hotkeys logon task =='
        & (Join-Path $dir 'setup-autostart.ps1')
        Log "  setup-autostart.ps1 exit=$LASTEXITCODE"
    } catch { Log "  task registration FAILED: $($_.Exception.Message)" }
} else { Log 'SKIP (gff: install.windows.ahk-autostart=false)' }

# 2) iTunes Win32 (the Store build was removed non-elevated by setup-apps.ps1).
try {
    Log '== [2/4] iTunes (Win32) =='
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        # Quiet winget's spinner in this captured/elevated context (own winget settings).
        winget settings --set visual.progressBar disabled 2>$null | Out-Null
        winget install --id Apple.iTunes -e --source winget `
            --accept-package-agreements --accept-source-agreements --disable-interactivity 2>&1 |
            ForEach-Object { Log "  $_" }
        Log "  winget exit=$LASTEXITCODE"
    } else { Log '  winget not available -- skipping iTunes.' }
} catch { Log "  iTunes install FAILED: $($_.Exception.Message)" }

# 3) Wispr Flow MSI via the shared core (already admin -> no nested UAC).
if (Test-GffOn 'install.windows.wispr-flow') {
    try {
        Log '== [3/4] Wispr Flow =='
        . (Join-Path $dir 'wispr-install-core.ps1')
        if (Install-WisprMsi) { Log '  Wispr Flow install ok.' } else { Log '  Wispr Flow install reported a failure (see above).' }
    } catch { Log "  Wispr install FAILED: $($_.Exception.Message)" }
} else { Log 'SKIP (gff: install.windows.wispr-flow=false)' }

# 4) PowerToys Copilot-key remap (best-effort; warns if PowerToys absent).
if (Test-GffOn 'install.windows.copilot-key') {
    try {
        Log '== [4/4] PowerToys Copilot-key remap =='
        $suppressor = Join-Path $dir 'suppress-copilot-key.ps1'
        if (Test-Path $suppressor) { & $suppressor } else { Log '  suppress-copilot-key.ps1 not found -- skipping.' }
    } catch { Log "  suppress-copilot-key FAILED: $($_.Exception.Message)" }
} else { Log 'SKIP (gff: install.windows.copilot-key=false)' }

Log "=== done $(Get-Date -Format o) ==="
Write-Host ''
Write-Host 'Elevated setup complete. Wispr Flow one-time manual steps (sign-in, mic,'
Write-Host 'shortcuts off the Win key, start-at-login) are documented in WISPR-FLOW.md.'
