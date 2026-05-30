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
# Windows PowerShell 5.1 defaults to old TLS; force TLS 1.2 for the CDN.
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$AppName       = 'Wispr Flow'
$ResolveLatest = 'https://dl.wisprflow.ai/windows/latest'   # 302 -> ...Setup-v<ver>.exe
$MsiUrlFmt     = 'https://dl.wisprflow.com/wispr-flow/win32/x64/Wispr%20Flow-v{0}.msi'

function Get-Installed {
    # Look the app up across the standard uninstall hives. Returns the registry
    # object (DisplayName/DisplayVersion/UninstallString/PSChildName=ProductCode)
    # or $null.
    $hives = @(
        'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*',
        'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*',
        'HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*'
    )
    Get-ItemProperty $hives -ErrorAction SilentlyContinue |
        Where-Object { $_.DisplayName -match 'Wispr\s*Flow' } |
        Select-Object -First 1
}

function Test-Admin {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    (New-Object Security.Principal.WindowsPrincipal($id)).IsInRole(
        [Security.Principal.WindowsBuiltinRole]::Administrator)
}

function Resolve-LatestVersion {
    # Follow the "latest" redirect WITHOUT auto-following, read Location, parse the
    # version out of the EXE filename, and reuse it for the MSI URL.
    try {
        $req = [System.Net.WebRequest]::Create($ResolveLatest)
        $req.Method = 'HEAD'
        $req.AllowAutoRedirect = $false
        $resp = $req.GetResponse()
        $loc  = $resp.Headers['Location']
        $resp.Close()
        if ($loc -match 'v(\d+\.\d+\.\d+)') { return $Matches[1] }
    } catch {
        Write-Warning "Could not resolve latest Wispr Flow version: $($_.Exception.Message)"
    }
    return $null
}

# --- Status ----------------------------------------------------------------------
if ($Status) {
    $i = Get-Installed
    if ($i) { Write-Host "$AppName installed: version $($i.DisplayVersion)" }
    else    { Write-Host "$AppName not installed." }
    return
}

# --- Uninstall -------------------------------------------------------------------
if ($Uninstall) {
    $i = Get-Installed
    if (-not $i) { Write-Host "$AppName not installed; nothing to uninstall."; return }
    $code = $i.PSChildName   # MSI product code, e.g. {GUID}
    Write-Host "Uninstalling $AppName ($($i.DisplayVersion)) ..."
    $verb = if (Test-Admin) { @{} } else { @{ Verb = 'RunAs' } }
    $p = Start-Process msiexec.exe -ArgumentList @('/x', $code, '/quiet', '/norestart') -PassThru -Wait @verb
    if ($p.ExitCode -in 0, 3010) { Write-Host "Uninstalled." }
    else { throw "msiexec uninstall failed (exit $($p.ExitCode))." }
    return
}

# --- Install ---------------------------------------------------------------------
$existing = Get-Installed
if ($existing -and -not $Force) {
    Write-Host "$AppName already installed (version $($existing.DisplayVersion)); skipping."
    Write-Host "Re-run with -Force to reinstall, or -Latest to update. Setup steps: WISPR-FLOW.md"
    return
}

# Decide which version to fetch.
$ver = $Version
if ($Latest) {
    $resolved = Resolve-LatestVersion
    if ($resolved) { $ver = $resolved; Write-Host "Latest Wispr Flow version: $ver" }
    else { Write-Warning "Falling back to pinned version $ver." }
}

function Save-Msi {
    param([string] $Url, [string] $Dest)
    # Reuse a previously downloaded installer (e.g. after a cancelled UAC prompt)
    # instead of re-fetching ~300 MB.
    if ((Test-Path $Dest) -and ((Get-Item $Dest).Length -gt 50MB)) {
        Write-Host "Using cached installer: $Dest"
        return
    }
    Write-Host "Downloading from $Url ..."
    Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing
}

$msiUrl = [string]::Format($MsiUrlFmt, $ver)
$tmp    = Join-Path $env:TEMP "WisprFlow-v$ver.msi"

Write-Host "Preparing $AppName v$ver ..."
Write-Host "  $msiUrl"
try {
    Save-Msi -Url $msiUrl -Dest $tmp
} catch {
    # The pin can 404 once it ages out of the CDN; resolve latest and retry once.
    if (-not $Latest) {
        Write-Warning "Pinned download failed ($($_.Exception.Message)); resolving latest and retrying."
        $resolved = Resolve-LatestVersion
        if (-not $resolved) { throw "Could not download v$ver and could not resolve latest." }
        $ver = $resolved; $msiUrl = [string]::Format($MsiUrlFmt, $ver)
        $tmp = Join-Path $env:TEMP "WisprFlow-v$ver.msi"
        Write-Host "Retrying with v${ver}: $msiUrl"
        Save-Msi -Url $msiUrl -Dest $tmp
    } else { throw }
}

# Sanity-check the download (the real MSI is ~300 MB; a tiny file means an error page).
$sizeMB = [math]::Round((Get-Item $tmp).Length / 1MB, 1)
if ($sizeMB -lt 50) { throw "MSI is only ${sizeMB} MB - looks like an error page, not the installer." }
Write-Host "Installer ready (${sizeMB} MB): $tmp"

Write-Host "Installing $AppName v$ver (silent; APPROVE the UAC prompt that appears) ..."
$msiArgs = @('/i', "`"$tmp`"", '/quiet', '/norestart')
$verb = if (Test-Admin) { @{} } else { @{ Verb = 'RunAs' } }
try {
    $p = Start-Process msiexec.exe -ArgumentList $msiArgs -PassThru -Wait @verb
} catch {
    # Most common cause: the UAC elevation prompt was dismissed/denied.
    Write-Warning "Elevation was cancelled or failed: $($_.Exception.Message)"
    Write-Host "The installer is cached (no re-download needed):"
    Write-Host "  $tmp"
    Write-Host "Finish it by re-running this script and approving UAC, or from an admin shell:"
    Write-Host "  msiexec /i `"$tmp`" /quiet /norestart"
    exit 1
}

switch ($p.ExitCode) {
    0     { Write-Host "$AppName v$ver installed.";                              Remove-Item $tmp -ErrorAction SilentlyContinue }
    3010  { Write-Host "$AppName v$ver installed (a reboot is recommended).";    Remove-Item $tmp -ErrorAction SilentlyContinue }
    1638  { Write-Host "Another version of $AppName is already installed.";      Remove-Item $tmp -ErrorAction SilentlyContinue }
    default { throw "msiexec install failed (exit $($p.ExitCode)). Cached installer kept at $tmp" }
}

# --- Tame the Copilot key so Wispr Flow can own it -------------------------------
# The Copilot key emits Win(Left)+Shift(Left)+F23. Left alone, Windows acts on that
# chord (launches Copilot) and can swallow it before Flow sees it, and Flow often
# won't bind F23 directly. The sibling script remaps the chord to Ctrl+Shift+F12 in
# PowerToys Keyboard Manager (idempotent; backs up the existing config). It is
# best-effort: a missing PowerToys is a warning, not a failure, so a machine
# without PowerToys still finishes installing Flow.
$suppressor = Join-Path $PSScriptRoot 'suppress-copilot-key.ps1'
if (Test-Path -LiteralPath $suppressor) {
    Write-Host ''
    Write-Host 'Remapping the Copilot key (Win+Shift+F23 -> Ctrl+Shift+F12) via PowerToys ...'
    try { & $suppressor }
    catch { Write-Warning "Copilot-key remap step failed: $($_.Exception.Message)" }
} else {
    Write-Warning "suppress-copilot-key.ps1 not found next to this script; skipping Copilot-key remap."
}

Write-Host ''
Write-Host '------------------------------------------------------------------'
Write-Host 'One-time manual setup (cannot be scripted - see WISPR-FLOW.md):'
Write-Host '  1. Launch Wispr Flow and sign in (browser).'
Write-Host '  2. Settings -> Privacy: allow microphone access.'
Write-Host '  3. Tray icon -> "Edit shortcut" -> press Ctrl+Shift+F12 (the combo the'
Write-Host '     Copilot key now sends, courtesy of the PowerToys remap above).'
Write-Host '     Restart PowerToys first if the remap has not taken effect yet.'
Write-Host '  4. Enable "Start at login" in Flow.'
Write-Host '------------------------------------------------------------------'
