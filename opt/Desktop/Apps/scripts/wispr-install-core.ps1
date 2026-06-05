# wispr-install-core.ps1 — dot-sourceable Wispr Flow MSI primitives.
#
# Functions only, no top-level side effects, so it is safe to dot-source from both
# install-wisprflow.ps1 (standalone installer + customizations) and setup-elevated.ps1
# (the single elevated batch). Wispr Flow is not on winget or the Store; it ships a
# machine-wide MSI we fetch directly (pinned, with a "latest" fallback).
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$script:WisprAppName   = 'Wispr Flow'
$script:WisprResolve   = 'https://dl.wisprflow.ai/windows/latest'    # 302 -> ...Setup-v<ver>.exe
$script:WisprMsiUrlFmt = 'https://dl.wisprflow.com/wispr-flow/win32/x64/Wispr%20Flow-v{0}.msi'
$script:WisprPinned    = '1.5.530'

function Get-WisprInstalled {
    # Returns the uninstall-hive registry object (DisplayName/DisplayVersion/
    # PSChildName=ProductCode) or $null.
    $hives = @(
        'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*',
        'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*',
        'HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*'
    )
    Get-ItemProperty $hives -ErrorAction SilentlyContinue |
        Where-Object { $_.DisplayName -match 'Wispr\s*Flow' } | Select-Object -First 1
}

function Test-WisprAdmin {
    (New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())).IsInRole(
        [Security.Principal.WindowsBuiltinRole]::Administrator)
}

function Resolve-WisprLatestVersion {
    # Follow the "latest" redirect WITHOUT auto-following, read Location, parse the
    # version out of the EXE filename, and reuse it for the MSI URL.
    try {
        $req = [System.Net.WebRequest]::Create($script:WisprResolve)
        $req.Method = 'HEAD'; $req.AllowAutoRedirect = $false
        $resp = $req.GetResponse(); $loc = $resp.Headers['Location']; $resp.Close()
        if ($loc -match 'v(\d+\.\d+\.\d+)') { return $Matches[1] }
    } catch { Write-Warning "Could not resolve latest Wispr Flow version: $($_.Exception.Message)" }
    return $null
}

function Save-WisprMsi {
    param([string]$Url, [string]$Dest)
    # Reuse a previously downloaded installer (e.g. after a cancelled UAC prompt)
    # instead of re-fetching ~300 MB.
    if ((Test-Path $Dest) -and ((Get-Item $Dest).Length -gt 50MB)) {
        Write-Host "Using cached installer: $Dest"; return
    }
    Write-Host "Downloading from $Url ..."
    Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing
}

function Install-WisprMsi {
    # Install Wispr Flow's MSI. Returns $true if installed (or already present),
    # $false on a handled failure (caller decides whether to treat as fatal). Already
    # running as admin -> msiexec runs directly; otherwise self-elevates (one UAC).
    [CmdletBinding()]
    param([switch]$Latest, [string]$Version = $script:WisprPinned, [switch]$Force)

    $existing = Get-WisprInstalled
    if ($existing -and -not $Force) {
        Write-Host "$script:WisprAppName already installed (version $($existing.DisplayVersion)); skipping."
        return $true
    }

    $ver = $Version
    if ($Latest) {
        $r = Resolve-WisprLatestVersion
        if ($r) { $ver = $r; Write-Host "Latest $script:WisprAppName version: $ver" }
        else    { Write-Warning "Falling back to pinned version $ver." }
    }

    $msiUrl = [string]::Format($script:WisprMsiUrlFmt, $ver)
    $tmp    = Join-Path $env:TEMP "WisprFlow-v$ver.msi"
    Write-Host "Preparing $script:WisprAppName v$ver ..."
    Write-Host "  $msiUrl"
    try {
        Save-WisprMsi -Url $msiUrl -Dest $tmp
    } catch {
        # The pin can 404 once it ages out of the CDN; resolve latest and retry once.
        if ($Latest) { throw }
        Write-Warning "Pinned download failed ($($_.Exception.Message)); resolving latest and retrying."
        $r = Resolve-WisprLatestVersion
        if (-not $r) { throw "Could not download v$ver and could not resolve latest." }
        $ver = $r; $msiUrl = [string]::Format($script:WisprMsiUrlFmt, $ver)
        $tmp = Join-Path $env:TEMP "WisprFlow-v$ver.msi"
        Write-Host "Retrying with v${ver}: $msiUrl"
        Save-WisprMsi -Url $msiUrl -Dest $tmp
    }

    # Sanity-check the download (the real MSI is ~300 MB; a tiny file means an error page).
    $sizeMB = [math]::Round((Get-Item $tmp).Length / 1MB, 1)
    if ($sizeMB -lt 50) { throw "MSI is only ${sizeMB} MB - looks like an error page, not the installer." }
    Write-Host "Installer ready (${sizeMB} MB): $tmp"

    Write-Host "Installing $script:WisprAppName v$ver (silent) ..."
    $verb = if (Test-WisprAdmin) { @{} } else { @{ Verb = 'RunAs' } }
    try {
        $p = Start-Process msiexec.exe -ArgumentList @('/i', "`"$tmp`"", '/quiet', '/norestart') -PassThru -Wait @verb
    } catch {
        Write-Warning "Elevation was cancelled or failed: $($_.Exception.Message)"
        Write-Host  "The installer is cached (no re-download needed): $tmp"
        Write-Host  "Finish it from an admin shell:  msiexec /i `"$tmp`" /quiet /norestart"
        return $false
    }
    switch ($p.ExitCode) {
        0     { Write-Host "$script:WisprAppName v$ver installed.";                           Remove-Item $tmp -ErrorAction SilentlyContinue; return $true }
        3010  { Write-Host "$script:WisprAppName v$ver installed (a reboot is recommended)."; Remove-Item $tmp -ErrorAction SilentlyContinue; return $true }
        1638  { Write-Host "Another version of $script:WisprAppName is already installed.";   Remove-Item $tmp -ErrorAction SilentlyContinue; return $true }
        default { Write-Warning "msiexec install failed (exit $($p.ExitCode)). Cached installer kept at $tmp"; return $false }
    }
}
