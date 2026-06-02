<#
.SYNOPSIS
    Install (or update / uninstall / report) the Wispr Flow dictation app on Windows.

.DESCRIPTION
    Wispr Flow is not on winget or the Microsoft Store, so we install its official
    machine-wide MSI with msiexec. By default we pin a known-good version for
    reproducibility and fall back to resolving "latest" only if the pin 404s
    (pass -Latest to always resolve latest).

    Idempotent: if Wispr Flow is already installed it does nothing (unless -Force).
    Machine-context MSIs need elevation; if this isn't already an admin session the
    install step self-elevates (one UAC prompt).

    This installs the APP only. A few one-time steps cannot be scripted (Wispr's
    own MDM guide says so): sign-in, microphone permission, "Start at login", and
    binding the activation hotkey. See WISPR-FLOW.md in this folder.

.PARAMETER Latest
    Resolve and install the latest version instead of the pinned one.

.PARAMETER Version
    Install a specific version (e.g. 1.5.530). Overrides the pin.

.PARAMETER Status
    Report whether Wispr Flow is installed (and its version), then exit.

.PARAMETER Uninstall
    Silently uninstall Wispr Flow using its registered product code.

.PARAMETER Force
    Reinstall even if an installed copy is detected.

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File install-wisprflow.ps1
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File install-wisprflow.ps1 -Status
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File install-wisprflow.ps1 -Latest
#>
[CmdletBinding(DefaultParameterSetName = 'Install')]
param(
    [Parameter(ParameterSetName = 'Install')] [switch] $Latest,
    [Parameter(ParameterSetName = 'Install')] [string] $Version = '1.5.530',
    [Parameter(ParameterSetName = 'Install')] [switch] $Force,
    [Parameter(ParameterSetName = 'Status')]  [switch] $Status,
    [Parameter(ParameterSetName = 'Uninstall')] [switch] $Uninstall
)

$ErrorActionPreference = 'Stop'

# Shared MSI primitives (Get-WisprInstalled / Resolve-WisprLatestVersion /
# Save-WisprMsi / Install-WisprMsi / Test-WisprAdmin), also used by setup-elevated.ps1
# so the unified app step can install Wispr inside the single elevated batch.
. (Join-Path $PSScriptRoot 'wispr-install-core.ps1')
$AppName = $script:WisprAppName

# --- Status ----------------------------------------------------------------------
if ($Status) {
    $i = Get-WisprInstalled
    if ($i) { Write-Host "$AppName installed: version $($i.DisplayVersion)" }
    else    { Write-Host "$AppName not installed." }
    return
}

# --- Uninstall -------------------------------------------------------------------
if ($Uninstall) {
    $i = Get-WisprInstalled
    if (-not $i) { Write-Host "$AppName not installed; nothing to uninstall."; return }
    $code = $i.PSChildName   # MSI product code, e.g. {GUID}
    Write-Host "Uninstalling $AppName ($($i.DisplayVersion)) ..."
    $verb = if (Test-WisprAdmin) { @{} } else { @{ Verb = 'RunAs' } }
    $p = Start-Process msiexec.exe -ArgumentList @('/x', $code, '/quiet', '/norestart') -PassThru -Wait @verb
    if ($p.ExitCode -in 0, 3010) { Write-Host "Uninstalled." }
    else { throw "msiexec uninstall failed (exit $($p.ExitCode))." }
    return
}

# --- Install (shared core) -------------------------------------------------------
$ok = Install-WisprMsi -Latest:$Latest -Version $Version -Force:$Force
if (-not $ok) {
    Write-Host "Install did not complete. Re-run this script and approve UAC, or run msiexec from an admin shell."
    exit 1
}

# --- Configure PowerToys so the Copilot key drives dictation ---------------------
# The Copilot key emits Win(Left)+Shift(Left)+F23. Windows opens its "Customize
# Copilot key" Settings page on that chord (and Flow ignores injected keys, so AHK
# can't drive Flow directly). The sibling script uses PowerToys Keyboard Manager to
# remap the chord to F24 BEFORE Windows sees it (and disables the Win-key modules
# that fight macos.ahk); macos.ahk then binds F24 and drives Flow by clicking its
# overlay. Idempotent + backs up. Best-effort: missing PowerToys is a warning, not a
# failure, so a machine without it still finishes installing Flow.
$suppressor = Join-Path $PSScriptRoot 'suppress-copilot-key.ps1'
if (Test-Path -LiteralPath $suppressor) {
    Write-Host ''
    Write-Host 'Configuring PowerToys (Copilot key Win+Shift+F23 -> F24) ...'
    try { & $suppressor }
    catch {
        Write-Warning "PowerToys Copilot-key setup failed: $($_.Exception.Message)"
        Write-Warning "Flow is installed, but the Copilot key is NOT wired up yet. Once PowerToys is installed, re-run:"
        Write-Warning "  suppress-copilot-key.ps1            (then -Status to verify the F24 remap)"
    }
} else {
    Write-Warning "suppress-copilot-key.ps1 not found next to this script; skipping PowerToys setup."
}

Write-Host ''
Write-Host '------------------------------------------------------------------'
Write-Host 'One-time manual setup (cannot be scripted - see WISPR-FLOW.md):'
Write-Host '  1. Launch Wispr Flow and sign in (browser).'
Write-Host '  2. Settings -> Privacy: allow microphone access.'
Write-Host '  3. Flow Settings -> General -> Shortcuts: move ALL Flow shortcuts OFF'
Write-Host '     the Win key (any non-Win combo). Flows Win-key hook otherwise breaks'
Write-Host '     the macOS-style Cmd shortcuts. (The Copilot key drives Flow via its'
Write-Host '     overlay, not via Flows hotkey - macos.ahk handles that.)'
Write-Host '  4. Keep Flows on-screen overlay visible (macos.ahk clicks it).'
Write-Host '  5. Enable "Start at login" in Flow.'
Write-Host '------------------------------------------------------------------'
