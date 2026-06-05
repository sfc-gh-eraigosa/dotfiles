<#
.SYNOPSIS
    Configure PowerToys so the Copilot key drives Wispr Flow "hover-dictation"
    (via macos.ahk) instead of Windows hijacking it.

.DESCRIPTION
    The Copilot key emits LWin+LShift+F23. Two facts shaped this design:
      * Windows opens its "Customize Copilot key" Settings page on that chord, and
        suppressing it in AutoHotkey is an unreliable race.
      * Wispr Flow ignores INJECTED keystrokes, so AHK can't drive Flow's hotkey;
        instead macos.ahk drives Flow by clicking its on-screen overlay.
    So we let PowerToys Keyboard Manager remap the chord to a clean unused key (F24)
    BEFORE Windows sees it; macos.ahk binds F24 and runs the overlay-click flow.

    This script is idempotent and backs up every file before changing it. It:
      1. writes the KBM remap   Win+Shift+F23 (91;160;134) -> F24 (135)
      2. in PowerToys settings.json: enables Keyboard Manager and disables the
         modules whose Win-key shortcuts fight macos.ahk's Cmd = Left-Win layer
         (FancyZones, PowerToys Run, Shortcut Guide)
      3. restarts PowerToys so the changes take effect
    If PowerToys isn't installed it prints a warning + link and exits without error.

.PARAMETER Status
    Report PowerToys install state, whether the F24 remap is present, and the
    relevant module enabled-states. Changes nothing.

.PARAMETER Remove
    Remove the Copilot->F24 KBM remap (other remaps untouched); module toggles
    are left as-is.

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File suppress-copilot-key.ps1
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File suppress-copilot-key.ps1 -Status
#>
[CmdletBinding(DefaultParameterSetName = 'Apply')]
param(
    [Parameter(ParameterSetName = 'Status')] [switch] $Status,
    [Parameter(ParameterSetName = 'Remove')] [switch] $Remove
)

$ErrorActionPreference = 'Stop'

# --- Constants -------------------------------------------------------------------
# PowerToys stores shortcut keys as ';'-joined decimal virtual-key codes.
#   91 = VK_LWIN, 160 = VK_LSHIFT, 134 = VK_F23  -> the Copilot chord.
$CopilotChord  = '91;160;134'
# Remap target: a clean key nothing else uses. 135 = VK_F24. macos.ahk binds F24.
$TargetKey     = '135'
$TargetLabel   = 'F24'
# Modules whose global Win-key shortcuts collide with macos.ahk's Cmd = Left-Win
# mappings; disabled so KBM can run without breaking the macOS layer.
$DisableModules = @('FancyZones', 'PowerToys Run', 'Shortcut Guide')

$LocalApp    = $env:LOCALAPPDATA
$PtSettings  = Join-Path $LocalApp 'Microsoft\PowerToys\settings.json'
$KbmConfig   = Join-Path $LocalApp 'Microsoft\PowerToys\Keyboard Manager\default.json'
$ReleasesUrl = 'https://github.com/microsoft/PowerToys/releases'

function Test-PowerToysInstalled {
    $paths = @((Join-Path $LocalApp 'PowerToys'), (Join-Path $env:ProgramFiles 'PowerToys'))
    $pf86 = ${env:ProgramFiles(x86)}
    if ($pf86) { $paths += (Join-Path $pf86 'PowerToys') }
    foreach ($p in $paths) { if (Test-Path -LiteralPath $p) { return $true } }
    return $false
}

function Backup-FileOnce([string]$Path) {
    if (Test-Path -LiteralPath $Path) {
        $bak = "$Path.bak-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
        Copy-Item -LiteralPath $Path -Destination $bak -Force
        Write-Host "  backed up -> $bak"
    }
}

function Save-Json($Object, [string]$Path) {
    $dir = Split-Path -Parent $Path
    if (-not (Test-Path -LiteralPath $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    $json = $Object | ConvertTo-Json -Depth 20
    [System.IO.File]::WriteAllText($Path, $json, (New-Object System.Text.UTF8Encoding($false)))  # no BOM
}

function Get-KbmGlobals {
    if (-not (Test-Path -LiteralPath $KbmConfig)) { return @() }
    $raw = Get-Content -LiteralPath $KbmConfig -Raw -ErrorAction Stop
    if ([string]::IsNullOrWhiteSpace($raw)) { return @() }
    $cfg = $raw | ConvertFrom-Json
    if (($cfg.PSObject.Properties.Name -contains 'remapShortcuts') -and $cfg.remapShortcuts -and
        ($cfg.remapShortcuts.PSObject.Properties.Name -contains 'global')) {
        return @($cfg.remapShortcuts.global)
    }
    return @()
}

function Test-RemapPresent {
    foreach ($e in (Get-KbmGlobals)) {
        if ($e.originalKeys -eq $CopilotChord -and $e.newRemapKeys -eq $TargetKey) { return $true }
    }
    return $false
}

function Restart-PowerToys {
    Write-Host "Restarting PowerToys to apply changes..."
    foreach ($n in @('PowerToys', 'PowerToys.KeyboardManagerEngine', 'PowerToys.Settings')) {
        Get-Process $n -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
    }
    Start-Sleep -Seconds 1
    $exe = Join-Path $LocalApp 'PowerToys\PowerToys.exe'
    if (Test-Path -LiteralPath $exe) { Start-Process -FilePath $exe; return $true }
    Write-Warning "PowerToys.exe not found at $exe; start PowerToys manually."
    return $false
}

# --- Status ----------------------------------------------------------------------
if ($Status) {
    if (-not (Test-PowerToysInstalled)) { Write-Host "PowerToys: not installed ($ReleasesUrl)"; return }
    Write-Host "PowerToys: installed."
    Write-Host ("Copilot->{0} KBM remap: {1}" -f $TargetLabel, $(if (Test-RemapPresent) { 'present' } else { 'MISSING' }))
    if (Test-Path -LiteralPath $PtSettings) {
        $en = (Get-Content -LiteralPath $PtSettings -Raw | ConvertFrom-Json).enabled
        Write-Host ("Keyboard Manager enabled: {0} (want True)" -f $en.'Keyboard Manager')
        foreach ($m in $DisableModules) { Write-Host ("  {0} enabled: {1} (want False)" -f $m, $en.$m) }
    }
    return
}

# --- Remove ----------------------------------------------------------------------
if ($Remove) {
    if (-not (Test-Path -LiteralPath $KbmConfig)) { Write-Host "No KBM config; nothing to remove."; return }
    $cfg = Get-Content -LiteralPath $KbmConfig -Raw | ConvertFrom-Json
    if (-not (@(Get-KbmGlobals) | Where-Object { $_.originalKeys -eq $CopilotChord })) {
        Write-Host "Copilot remap not present; nothing to remove."; return
    }
    Backup-FileOnce $KbmConfig
    $cfg.remapShortcuts.global = @(@(Get-KbmGlobals) | Where-Object { $_.originalKeys -ne $CopilotChord })
    Save-Json $cfg $KbmConfig
    Write-Host "Removed Copilot->$TargetLabel remap. Restart PowerToys to apply."
    return
}

# --- Apply -----------------------------------------------------------------------
if (-not (Test-PowerToysInstalled)) {
    Write-Warning @"
PowerToys is not installed, so the Copilot key can't be wired to Wispr Flow dictation.
Install PowerToys, then re-run this script:
  $ReleasesUrl
"@
    return
}

$changed = $false

# 1) KBM remap: Win+Shift+F23 -> F24 (merge; never duplicate) --------------------
if (Test-RemapPresent) {
    Write-Host "KBM remap Copilot->$TargetLabel already present."
} else {
    if (Test-Path -LiteralPath $KbmConfig) {
        $cfg = Get-Content -LiteralPath $KbmConfig -Raw | ConvertFrom-Json
        Backup-FileOnce $KbmConfig
    } else {
        $cfg = [pscustomobject]@{ remapKeys = [pscustomobject]@{ inProcess = @() } }
    }
    if (-not ($cfg.PSObject.Properties.Name -contains 'remapShortcuts') -or -not $cfg.remapShortcuts) {
        $cfg | Add-Member -NotePropertyName 'remapShortcuts' -NotePropertyValue ([pscustomobject]@{ global = @(); appSpecific = @() }) -Force
    }
    if (-not ($cfg.remapShortcuts.PSObject.Properties.Name -contains 'global') -or $null -eq $cfg.remapShortcuts.global) {
        $cfg.remapShortcuts | Add-Member -NotePropertyName 'global' -NotePropertyValue @() -Force
    }
    $kept  = @(@($cfg.remapShortcuts.global) | Where-Object { $_.originalKeys -ne $CopilotChord })
    $entry = [pscustomobject]@{ originalKeys = $CopilotChord; newRemapKeys = $TargetKey }
    $cfg.remapShortcuts.global = @($kept + $entry)
    Save-Json $cfg $KbmConfig
    Write-Host "Wrote KBM remap Copilot ($CopilotChord) -> $TargetLabel ($TargetKey)."
    $changed = $true
}

# 2) Module toggles in settings.json --------------------------------------------
if (Test-Path -LiteralPath $PtSettings) {
    $s = Get-Content -LiteralPath $PtSettings -Raw | ConvertFrom-Json
    $needs = ($s.enabled.'Keyboard Manager' -ne $true)
    foreach ($m in $DisableModules) { if ($s.enabled.$m -ne $false) { $needs = $true } }
    if ($needs) {
        Backup-FileOnce $PtSettings
        $s.enabled.'Keyboard Manager' = $true
        foreach ($m in $DisableModules) {
            if ($s.enabled.PSObject.Properties.Name -contains $m) { $s.enabled.$m = $false }
        }
        Save-Json $s $PtSettings
        Write-Host "Enabled Keyboard Manager; disabled: $($DisableModules -join ', ')."
        $changed = $true
    } else {
        Write-Host "PowerToys modules already configured (KBM on; conflicters off)."
    }
} else {
    Write-Warning "PowerToys settings.json not found ($PtSettings); run PowerToys once, then re-run."
}

# 3) Apply ------------------------------------------------------------------------
if ($changed) {
    if (Restart-PowerToys) {
        Write-Host "Done. The Copilot key now sends $TargetLabel; macos.ahk drives Wispr Flow from there."
    } else {
        Write-Warning "Config written, but PowerToys was not auto-restarted. Start PowerToys manually for the $TargetLabel remap to take effect."
    }
} else {
    Write-Host "Already configured; no restart needed."
}
