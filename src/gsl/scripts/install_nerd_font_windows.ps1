<#
.SYNOPSIS
    gsl-packaged Windows Nerd Font installer. Installs the pinned MesloLGS NF
    faces from the ryanoasis/nerd-fonts release and activates them for the
    running session via GDI. Idempotent.
.OUTPUTS
    Returns the installed font family name ('MesloLGS NF') so callers (e.g.
    setup-apps.ps1) can use it for terminal profiles.
#>
[CmdletBinding()]
param()

# Suppress interactive prompts and download progress bars so this script is
# safe to invoke unattended (e.g. from a WSL bash script via powershell.exe).
$ProgressPreference   = 'SilentlyContinue'
$ConfirmPreference    = 'None'
$ErrorActionPreference = 'Stop'

$NerdFontsVersion = 'v3.4.0'
$NerdFontsAsset   = 'Meslo.zip'
$NerdFontsUrl     = "https://github.com/ryanoasis/nerd-fonts/releases/download/$NerdFontsVersion/$NerdFontsAsset"
$FontFamily       = 'MesloLGS NF'

# ── Preflight ────────────────────────────────────────────────────────────────
$fontDir = "$env:LOCALAPPDATA\Microsoft\Windows\Fonts"
$regKey  = 'HKCU:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts'
try { New-Item -ItemType Directory -Force -Path $fontDir -ErrorAction Stop | Out-Null }
catch {
    Write-Host "ERROR: cannot write to user font dir '$fontDir'; run from a normal (non-elevated) PowerShell." -ForegroundColor Red
    return $null
}
if (-not (Get-Command Invoke-WebRequest -ErrorAction SilentlyContinue)) {
    Write-Host "ERROR: Invoke-WebRequest unavailable; PowerShell 5.1+ required." -ForegroundColor Red
    return $null
}

Write-Host "`n=== gsl Nerd Font ($FontFamily, nerd-fonts $NerdFontsVersion) ===" -ForegroundColor Cyan

$faces = @(
    @{ File = 'MesloLGS NF Regular.ttf';     Reg = 'MesloLGS NF (TrueType)' }
    @{ File = 'MesloLGS NF Bold.ttf';        Reg = 'MesloLGS NF Bold (TrueType)' }
    @{ File = 'MesloLGS NF Italic.ttf';      Reg = 'MesloLGS NF Italic (TrueType)' }
    @{ File = 'MesloLGS NF Bold Italic.ttf'; Reg = 'MesloLGS NF Bold Italic (TrueType)' }
)

# Drop stale Ubuntu Mono registry entries whose files no longer exist (migration).
$ErrorActionPreference = 'SilentlyContinue'
$existing = Get-ItemProperty $regKey
$ErrorActionPreference = 'Stop'
if ($existing) {
    foreach ($p in $existing.PSObject.Properties) {
        if ($p.Name -match 'Ubuntu Mono' -and $p.Value -and -not (Test-Path $p.Value)) {
            Remove-ItemProperty -Path $regKey -Name $p.Name -ErrorAction SilentlyContinue
        }
    }
}

# Download + extract once if any face is missing.
$needDownload = $faces | Where-Object { -not (Test-Path (Join-Path $fontDir $_.File)) }
if ($needDownload) {
    $tmp = Join-Path $env:TEMP ("meslo-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Force -Path $tmp | Out-Null
    $zip = Join-Path $tmp $NerdFontsAsset
    Write-Host "Downloading $NerdFontsAsset..." -ForegroundColor Cyan
    Invoke-WebRequest -Uri $NerdFontsUrl -OutFile $zip -UseBasicParsing
    # Remove Zone.Identifier ADS so Expand-Archive does not trigger SmartScreen.
    Unblock-File -Path $zip -ErrorAction SilentlyContinue
    Expand-Archive -Path $zip -DestinationPath $tmp -Force
    foreach ($f in $faces) {
        $src = Get-ChildItem -Path $tmp -Filter $f.File -Recurse | Select-Object -First 1
        if ($src) { Copy-Item $src.FullName (Join-Path $fontDir $f.File) -Force }
    }
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

$installed = @()
foreach ($f in $faces) {
    $dest = Join-Path $fontDir $f.File
    if (Test-Path $dest) {
        New-ItemProperty -Path $regKey -Name $f.Reg -Value $dest -PropertyType String -Force | Out-Null
        $installed += $dest
    }
}

# Activate for the running session via GDI + broadcast WM_FONTCHANGE.
$sig = @'
[DllImport("gdi32.dll", CharSet=CharSet.Unicode)]
public static extern int AddFontResourceW(string lpFileName);
[DllImport("user32.dll", CharSet=CharSet.Auto)]
public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, IntPtr wParam, IntPtr lParam, uint fuFlags, uint uTimeout, out IntPtr lpdwResult);
'@
$api = Add-Type -MemberDefinition $sig -Name FontApi -Namespace Win32 -PassThru
foreach ($p in $installed) { [void]$api::AddFontResourceW($p) }
$res = [IntPtr]::Zero
[void]$api::SendMessageTimeout([IntPtr]0xffff, 0x001D, [IntPtr]::Zero, [IntPtr]::Zero, 2, 2000, [ref]$res)
Write-Host "Activated $FontFamily for this session (restart Terminal to pick it up)." -ForegroundColor Green
return $FontFamily
