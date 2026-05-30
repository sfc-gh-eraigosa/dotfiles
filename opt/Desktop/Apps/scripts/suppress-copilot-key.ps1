<#
.SYNOPSIS
    Tame the Windows Copilot key (LWin+LShift+F23) via Microsoft PowerToys Keyboard
    Manager so Windows stops reacting to it and Wispr Flow can own it exclusively.

.DESCRIPTION
    The Copilot key on newer keyboards emits the chord Win(Left)+Shift(Left)+F23.
    Left alone, Windows acts on that chord (e.g. launches Windows Copilot) and may
    swallow it before Wispr Flow's hotkey listener sees it. Wispr Flow also often
    refuses to bind F23 directly (its docs only cover function keys up to F12).

    This script remaps the Copilot chord to Ctrl+Alt+F12 in PowerToys Keyboard
    Manager. That kills the Windows Copilot behaviour AND turns the physical key
    into a normal, Flow-bindable combo: bind Ctrl+Alt+F12 inside Wispr Flow
    ("Edit shortcut") and the Copilot key drives dictation. Ctrl+Alt+F12 is the
    same combo the AHK fallback shim in WISPR-FLOW.md uses, so they stay in sync.

    It edits PowerToys' Keyboard Manager config directly:
        %LOCALAPPDATA%\Microsoft\PowerToys\Keyboard Manager\default.json
    The existing file (if any) is backed up first, the remap is *merged* (other
    remaps are preserved), and the operation is idempotent - re-running it never
    creates a duplicate entry. PowerToys must be restarted (or Keyboard Manager
    toggled off/on) for a file edit to take effect.

    If PowerToys is not installed, the script prints a warning with the download
    link and exits without error (it is a no-op, so the installer can call it
    unconditionally).

.PARAMETER Status
    Report whether PowerToys is installed and whether the Copilot key is currently
    remapped to the Flow combo, then exit. Changes nothing.

.PARAMETER Remove
    Remove the Copilot-key remap (restore default Copilot-key behaviour), backing
    up the config first. Other remaps are left untouched.

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File suppress-copilot-key.ps1
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File suppress-copilot-key.ps1 -Status
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File suppress-copilot-key.ps1 -Remove
#>
[CmdletBinding(DefaultParameterSetName = 'Apply')]
param(
    [Parameter(ParameterSetName = 'Status')] [switch] $Status,
    [Parameter(ParameterSetName = 'Remove')] [switch] $Remove
)

$ErrorActionPreference = 'Stop'

# --- Constants -------------------------------------------------------------------
# PowerToys stores each shortcut as a ';'-joined string of *decimal* virtual-key
# codes, modifiers first and the action key last.
#
# Original chord = the Copilot key:
#   91  = VK_LWIN   (Left Windows)
#   160 = VK_LSHIFT (Left Shift)
#   134 = VK_F23
$CopilotChord = '91;160;134'

# Target chord = Ctrl+Alt+F12, a combo Wispr Flow accepts. Remapping (not
# disabling) means the key still produces something Flow can bind, while Windows
# Copilot - which only listens for Win+Shift+F23 - never fires.
#   162 = VK_LCONTROL (Left Ctrl)
#   164 = VK_LMENU    (Left Alt)
#   123 = VK_F12
$FlowChord = '162;164;123'

# Human-readable labels for log lines.
$CopilotLabel = 'LWin+LShift+F23'
$FlowLabel    = 'Ctrl+Alt+F12'

# Per-user Keyboard Manager config. This lives under LOCALAPPDATA regardless of
# whether PowerToys itself was installed per-user or machine-wide.
$KbmConfig   = Join-Path $env:LOCALAPPDATA 'Microsoft\PowerToys\Keyboard Manager\default.json'
$ReleasesUrl = 'https://github.com/microsoft/PowerToys/releases'

function Test-PowerToysInstalled {
    # PowerToys can be installed per-user (LOCALAPPDATA) or machine-wide (Program
    # Files / Program Files (x86)) depending on the installer used. Any of these
    # folders existing means PowerToys is present.
    $paths = @(
        (Join-Path $env:LOCALAPPDATA 'PowerToys'),
        (Join-Path $env:ProgramFiles 'PowerToys')
    )
    $pf86 = ${env:ProgramFiles(x86)}
    if ($pf86) { $paths += (Join-Path $pf86 'PowerToys') }
    foreach ($p in $paths) { if (Test-Path -LiteralPath $p) { return $true } }
    return $false
}

function Get-KbmConfigObject {
    # Read default.json into an object, or return an empty object if the file is
    # missing/blank so callers can build the structure from scratch.
    if (Test-Path -LiteralPath $KbmConfig) {
        $raw = Get-Content -LiteralPath $KbmConfig -Raw -ErrorAction Stop
        if (-not [string]::IsNullOrWhiteSpace($raw)) { return ($raw | ConvertFrom-Json) }
    }
    return [pscustomobject]@{}
}

function Get-GlobalRemaps {
    # Return remapShortcuts.global as an array (always materialise to an array so a
    # single-object value from ConvertFrom-Json still behaves like a list).
    param($Config)
    # Callers always coerce the result with @(...), which both rebuilds an array
    # from an unrolled scalar and turns a $null/absent value into an empty array.
    if (($Config.PSObject.Properties.Name -contains 'remapShortcuts') -and $Config.remapShortcuts -and
        ($Config.remapShortcuts.PSObject.Properties.Name -contains 'global')) {
        return @($Config.remapShortcuts.global)
    }
    return @()
}

function Find-CopilotIndex {
    # Index of the existing Copilot-chord remap in $GlobalRemaps, or -1 if absent.
    param($GlobalRemaps)
    $arr = @($GlobalRemaps)   # re-wrap defensively in case a scalar was passed in
    for ($i = 0; $i -lt $arr.Count; $i++) {
        if ($arr[$i].originalKeys -eq $CopilotChord) { return $i }
    }
    return -1
}

function Backup-Config {
    # Timestamped copy alongside the original before we modify it (matches the
    # ".bak-<timestamp>" convention used elsewhere in these scripts).
    if (Test-Path -LiteralPath $KbmConfig) {
        $bak = "$KbmConfig.bak-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
        Copy-Item -LiteralPath $KbmConfig -Destination $bak -Force
        Write-Host "Backed up existing config -> $bak"
    }
}

function Save-Config {
    param($Config)
    $dir = Split-Path -Parent $KbmConfig
    if (-not (Test-Path -LiteralPath $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    # Write UTF-8 *without* a BOM - PowerToys' parser expects plain UTF-8.
    $json = $Config | ConvertTo-Json -Depth 10
    [System.IO.File]::WriteAllText($KbmConfig, $json, (New-Object System.Text.UTF8Encoding($false)))
}

function Invoke-Status {
    if (-not (Test-PowerToysInstalled)) {
        Write-Host "PowerToys: not installed ($ReleasesUrl)"
        return
    }
    Write-Host "PowerToys: installed."
    if (-not (Test-Path -LiteralPath $KbmConfig)) {
        Write-Host "Keyboard Manager: no config yet - Copilot key NOT remapped."
        return
    }
    $global = @(Get-GlobalRemaps -Config (Get-KbmConfigObject))
    $idx = Find-CopilotIndex -GlobalRemaps $global
    if ($idx -lt 0) {
        Write-Host "Copilot key ($CopilotLabel): NOT remapped."
    } elseif ($global[$idx].newRemapKeys -eq $FlowChord) {
        Write-Host "Copilot key ($CopilotLabel): remapped to $FlowLabel (bind this in Wispr Flow)."
    } else {
        Write-Host "Copilot key ($CopilotLabel): remapped to '$($global[$idx].newRemapKeys)' (not the expected $FlowLabel)."
    }
}

function Invoke-Remove {
    if (-not (Test-Path -LiteralPath $KbmConfig)) {
        Write-Host "No Keyboard Manager config found; nothing to remove."
        return
    }
    $config = Get-KbmConfigObject
    $global = @(Get-GlobalRemaps -Config $config)
    if ((Find-CopilotIndex -GlobalRemaps $global) -lt 0) {
        Write-Host "Copilot-key remap not present; nothing to remove."
        return
    }
    Backup-Config
    $config.remapShortcuts.global = @($global | Where-Object { $_.originalKeys -ne $CopilotChord })
    Save-Config -Config $config
    Write-Host "Removed Copilot-key remap. Restart PowerToys to apply."
}

function Invoke-Apply {
    if (-not (Test-PowerToysInstalled)) {
        # Soft warning, not an error: callers (e.g. the installer) invoke this
        # unconditionally, and a machine without PowerToys simply skips the remap.
        Write-Warning @"
PowerToys is not installed, so the Copilot key cannot be remapped at the OS level.
Install PowerToys, then re-run this script:
  $ReleasesUrl
Without it, Windows keeps acting on $CopilotLabel and Wispr Flow may never see the key.
"@
        return
    }

    $config = Get-KbmConfigObject

    # Ensure the remapShortcuts.global container exists without clobbering any
    # remaps the user already configured (remapKeys, appSpecific, etc.).
    if (-not ($config.PSObject.Properties.Name -contains 'remapShortcuts') -or -not $config.remapShortcuts) {
        $config | Add-Member -MemberType NoteProperty -Name 'remapShortcuts' -Value ([pscustomobject]@{}) -Force
    }
    $rs = $config.remapShortcuts
    if (-not ($rs.PSObject.Properties.Name -contains 'global') -or $null -eq $rs.global) {
        $rs | Add-Member -MemberType NoteProperty -Name 'global' -Value @() -Force
    }

    $global = @(Get-GlobalRemaps -Config $config)
    $idx = Find-CopilotIndex -GlobalRemaps $global

    # Idempotent: already remapped to the Flow combo -> do nothing.
    if ($idx -ge 0 -and $global[$idx].newRemapKeys -eq $FlowChord) {
        Write-Host "Copilot key ($CopilotLabel) is already remapped to $FlowLabel; nothing to do."
        return
    }

    Backup-Config
    $entry = [pscustomobject]@{ originalKeys = $CopilotChord; newRemapKeys = $FlowChord }
    if ($idx -ge 0) {
        Write-Host "Updating existing $CopilotLabel remap -> $FlowLabel."
        $global[$idx] = $entry
    } else {
        Write-Host "Adding $CopilotLabel -> $FlowLabel remap."
        $global += $entry
    }
    $rs.global = @($global)

    Save-Config -Config $config
    Write-Host "Copilot key remapped to $FlowLabel in PowerToys Keyboard Manager: $KbmConfig"
    Write-Host "Restart PowerToys (or toggle Keyboard Manager off/on), then bind $FlowLabel in Wispr Flow."
}

# --- Dispatch --------------------------------------------------------------------
if ($Status) { Invoke-Status; return }
if ($Remove) { Invoke-Remove; return }
Invoke-Apply
