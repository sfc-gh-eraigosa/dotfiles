<#
.SYNOPSIS
    Provisions this Windows machine: WSL + Ubuntu distros, fonts, Windows
    Terminal themes/profiles, and a standard set of desktop apps (winget).
.DESCRIPTION
    Default mode runs every phase, in order, idempotently:
      1. WSL            - install the WSL platform if missing (skips if ready).
      2. Distros        - ensure the two Ubuntu distros exist; create the user.
      3. GitHub link    - link the Windows GitHub folder into each distro.
      4. Font           - (re)install Ubuntu Mono so Terminal can find it.
      5. Windows Terminal - add color schemes + per-distro profiles.
      6. Apps           - winget-install the app list, skipping what's present.

    -Status mode installs nothing; it reports WSL state plus each app's
    installed version and install folder location.

    Run from an ordinary (non-elevated) PowerShell prompt -- some packages
    (e.g. Spotify) refuse to install when run elevated. Installing the WSL
    *platform* on a machine that lacks it needs an elevated run + reboot;
    everything else works un-elevated.
.PARAMETER Status
    Report what is installed (WSL + apps) instead of installing.
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\setup-apps.ps1
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\setup-apps.ps1 -Status
#>
[CmdletBinding()]
param(
    [switch]$Status,
    # Skip the single elevated batch at the end (setup-elevated.ps1). Use for CI or
    # when you only want the non-elevated app pass.
    [switch]$SkipElevated
)

$ErrorActionPreference = 'Continue'
$env:WSL_UTF8 = '1'  # make wsl.exe emit parseable UTF-8 instead of UTF-16
[Net.ServicePointManager]::SecurityProtocol =
    [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

# ============================ Configuration =================================

# The two (and only two) Ubuntu distributions. Key = wsl distro name,
# value = friendly label used in Windows Terminal profile names.
$LatestDistro = 'Ubuntu'         # pre-existing Ubuntu 26.04 LTS  -> "Latest"
$LtsDistro    = 'Ubuntu-24.04'   # added                          -> "LTS"
$Distros      = [ordered]@{
    $LatestDistro = 'Ubuntu 26.04 (Latest)'
    $LtsDistro    = 'Ubuntu 24.04 (LTS)'
}

$DistroUser   = 'wenlock'        # Linux user to ensure on newly-created distros
$GitHubLink   = "$env:USERPROFILE\GitHub"  # Windows folder (a symlink) to expose in WSL
$TerminalFont = 'MesloLGS NF'
$HistorySize  = 100000

# Color schemes to publish, and the order in which a profile is made per distro.
$Themes = 'Solarized Dark', 'Solarized Light', 'Ocean', 'Green', 'GitHub Dark'

# Load application configuration from apps.json
$appsPath = Join-Path $PSScriptRoot 'apps.json'
if (-not (Test-Path $appsPath)) {
    Write-Host "ERROR: apps.json not found at $appsPath" -ForegroundColor Red
    exit 1
}
$apps = Get-Content $appsPath -Raw | ConvertFrom-Json

# Color-scheme palettes (Windows Terminal "schemes" entries).
$SchemeDefs = @(
    @{ name='Solarized Dark'; background='#002B36'; foreground='#839496'; cursorColor='#839496'; selectionBackground='#073642'
       black='#073642'; red='#DC322F'; green='#859900'; yellow='#B58900'; blue='#268BD2'; purple='#D33682'; cyan='#2AA198'; white='#EEE8D5'
       brightBlack='#002B36'; brightRed='#CB4B16'; brightGreen='#586E75'; brightYellow='#657B83'; brightBlue='#839496'; brightPurple='#6C71C4'; brightCyan='#93A1A1'; brightWhite='#FDF6E3' }
    @{ name='Solarized Light'; background='#FDF6E3'; foreground='#657B83'; cursorColor='#657B83'; selectionBackground='#EEE8D5'
       black='#073642'; red='#DC322F'; green='#859900'; yellow='#B58900'; blue='#268BD2'; purple='#D33682'; cyan='#2AA198'; white='#EEE8D5'
       brightBlack='#002B36'; brightRed='#CB4B16'; brightGreen='#586E75'; brightYellow='#657B83'; brightBlue='#839496'; brightPurple='#6C71C4'; brightCyan='#93A1A1'; brightWhite='#FDF6E3' }
    @{ name='Ocean'; background='#2B303B'; foreground='#C0C5CE'; cursorColor='#C0C5CE'; selectionBackground='#4F5B66'
       black='#2B303B'; red='#BF616A'; green='#A3BE8C'; yellow='#EBCB8B'; blue='#8FA1B3'; purple='#B48EAD'; cyan='#96B5B4'; white='#C0C5CE'
       brightBlack='#65737E'; brightRed='#BF616A'; brightGreen='#A3BE8C'; brightYellow='#EBCB8B'; brightBlue='#8FA1B3'; brightPurple='#B48EAD'; brightCyan='#96B5B4'; brightWhite='#EFF1F5' }
    @{ name='Green'; background='#000000'; foreground='#00FF00'; cursorColor='#00FF00'; selectionBackground='#B5D5FF'
       black='#000000'; red='#990000'; green='#00A600'; yellow='#999900'; blue='#0000B2'; purple='#B200B2'; cyan='#00A6B2'; white='#BFBFBF'
       brightBlack='#666666'; brightRed='#E50000'; brightGreen='#00D900'; brightYellow='#E5E500'; brightBlue='#0000FF'; brightPurple='#E500E5'; brightCyan='#00E5E5'; brightWhite='#E5E5E5' }
    @{ name='GitHub Dark'; background='#24292E'; foreground='#D1D5DA'; cursorColor='#C9D1D9'; selectionBackground='#444D56'
       black='#586069'; red='#EA4A5A'; green='#34D058'; yellow='#FFEA7F'; blue='#2188FF'; purple='#B392F0'; cyan='#39C5CF'; white='#D1D5DA'
       brightBlack='#959DA5'; brightRed='#F97583'; brightGreen='#85E89D'; brightYellow='#FFEA7F'; brightBlue='#79B8FF'; brightPurple='#B392F0'; brightCyan='#56D4DD'; brightWhite='#FAFBFC' }
)

# =============================== Helpers ====================================

function Test-Admin {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    (New-Object Security.Principal.WindowsPrincipal($id)).IsInRole(
        [Security.Principal.WindowsBuiltinRole]::Administrator)
}

function New-GuidFromString {
    param([string]$Text)
    $md5 = [Security.Cryptography.MD5]::Create()
    $bytes = $md5.ComputeHash([Text.Encoding]::UTF8.GetBytes($Text))
    return "{$([Guid]::new($bytes))}"
}

# ---- WSL -------------------------------------------------------------------

function Test-WslReady {
    if (-not (Get-Command wsl.exe -ErrorAction SilentlyContinue)) { return $false }
    wsl.exe --status *> $null
    return ($LASTEXITCODE -eq 0)
}

function Get-WslDistros {
    if (-not (Get-Command wsl.exe -ErrorAction SilentlyContinue)) { return @() }
    (wsl.exe --list --quiet 2>$null) | ForEach-Object { $_.Trim() } | Where-Object { $_ }
}

function Ensure-Wsl {
    Write-Host "`n=== [1/6] WSL platform ===" -ForegroundColor Cyan
    if (Test-WslReady) {
        Write-Host "WSL already installed -- skipping." -ForegroundColor DarkGray
        return $true
    }
    if (-not (Test-Admin)) {
        Write-Host "WSL is not installed and this step needs an elevated prompt." -ForegroundColor Red
        Write-Host "Re-run once from an *Administrator* PowerShell: wsl --install --no-distribution" -ForegroundColor Red
        return $false
    }
    Write-Host "Installing WSL platform (a reboot may be required)..." -ForegroundColor Cyan
    wsl.exe --install --no-distribution
    return ($LASTEXITCODE -eq 0)
}

function Ensure-Distro {
    param([string]$Name)
    $existing = Get-WslDistros
    if ($existing -contains $Name) {
        Write-Host "Distro '$Name' present -- skipping install." -ForegroundColor DarkGray
        return
    }
    Write-Host "Installing distro '$Name' (this downloads several hundred MB)..." -ForegroundColor Cyan
    wsl.exe --install -d $Name --no-launch
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Failed to install '$Name' (exit $LASTEXITCODE)." -ForegroundColor Red
        return
    }

    # Create the user, give passwordless sudo, set as the distro default.
    Write-Host "Creating user '$DistroUser' on '$Name'..." -ForegroundColor Cyan
    $setup = @"
set -e
id -u $DistroUser >/dev/null 2>&1 || { useradd -m -s /bin/bash $DistroUser; }
usermod -aG sudo $DistroUser
echo '$DistroUser ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/90-$DistroUser
chmod 440 /etc/sudoers.d/90-$DistroUser
printf '[user]\ndefault=%s\n' '$DistroUser' > /etc/wsl.conf
"@
    wsl.exe -d $Name -u root -- bash -lc $setup
    wsl.exe --terminate $Name *> $null   # apply default-user change
}

function Link-GitHubInto {
    param([string]$Name, [string]$WslPath)
    $cmd = "ln -sfn '$WslPath' `"`$HOME/GitHub`" && readlink -f `"`$HOME/GitHub`""
    Write-Host "Linking GitHub into '$Name'..." -ForegroundColor Cyan
    wsl.exe -d $Name -- bash -lc $cmd
}

function Get-WslReport {
    if (-not (Test-WslReady)) { Write-Host "WSL: not installed" -ForegroundColor Yellow; return }
    $ver = (wsl.exe --version 2>$null | Select-Object -First 1)
    Write-Host "WSL: $ver" -ForegroundColor Green
    wsl.exe --list --verbose 2>$null | Write-Host
}

# ---- Fonts -----------------------------------------------------------------
# Font install/activation now lives in the gsl skill so it stays in sync with
# the codepoints gsl renders. setup-apps.ps1 only delegates.
function Install-NerdFont {
    Write-Host "`n=== [4/6] Nerd Font (MesloLGS NF) ===" -ForegroundColor Cyan
    # setup-apps.ps1 sits at opt/Desktop/Apps/scripts/; the gsl script is at
    # sdk/gsl/scripts/ — four levels up, then into sdk/gsl/scripts.
    $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..\..\..')).Path
    $fontScript = Join-Path $repoRoot 'src\gsl\scripts\install_nerd_font_windows.ps1'
    if (-not (Test-Path $fontScript)) {
        Write-Host "Nerd Font installer not found at $fontScript -- skipping." -ForegroundColor Yellow
        return
    }
    $family = & $fontScript
    if ($family) { $script:TerminalFont = $family }
}

# ---- Windows Terminal ------------------------------------------------------

function Get-TerminalSettingsPath {
    $stable  = "$env:LOCALAPPDATA\Packages\Microsoft.WindowsTerminal_8wekyb3d8bbwe\LocalState\settings.json"
    $preview = "$env:LOCALAPPDATA\Packages\Microsoft.WindowsTerminalPreview_8wekyb3d8bbwe\LocalState\settings.json"

    # Prefer a settings.json Windows Terminal has already generated.
    foreach ($p in @($stable, $preview)) { if (Test-Path $p) { return $p } }

    # A freshly winget-installed Terminal only writes settings.json on first
    # launch, so on a clean machine this step would otherwise find nothing and
    # silently skip. Seed a minimal valid file when a WT package is present;
    # Terminal merges its own defaults + dynamic profiles on launch and keeps
    # the keys we set here.
    $pkg = Get-AppxPackage -Name 'Microsoft.WindowsTerminal*' -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $pkg) { return $null }
    $target = if ($pkg.Name -like '*Preview*') { $preview } else { $stable }
    New-Item -ItemType Directory -Force -Path (Split-Path $target -Parent) | Out-Null
    $seed = [pscustomobject]@{
        '$schema' = 'https://aka.ms/terminal-profiles-schema'
        profiles  = [pscustomobject]@{ defaults = [pscustomobject]@{}; list = @() }
        schemes   = @()
    }
    [IO.File]::WriteAllText($target, ($seed | ConvertTo-Json -Depth 32), [Text.UTF8Encoding]::new($false))
    Write-Host "Seeded minimal settings.json at $target (Terminal not yet launched)." -ForegroundColor DarkGray
    return $target
}

function Configure-Terminal {
    Write-Host "`n=== [5/6] Windows Terminal themes + profiles ===" -ForegroundColor Cyan
    $path = Get-TerminalSettingsPath
    if (-not $path) {
        Write-Host "Windows Terminal settings.json not found -- skipping." -ForegroundColor Yellow
        return
    }

    Copy-Item $path "$path.bak-$(Get-Date -Format yyyyMMdd-HHmmss)" -Force
    $json = Get-Content $path -Raw | ConvertFrom-Json

    # --- global settings ---
    if ($null -eq $json.PSObject.Properties['focusFollowMouse']) {
        $json | Add-Member -MemberType NoteProperty -Name "focusFollowMouse" -Value $true
    } else {
        $json.focusFollowMouse = $true
    }

    # --- schemes (idempotent by name) ---
    $schemeObjs = $SchemeDefs | ForEach-Object { [pscustomobject]$_ }
    $mineNames  = $schemeObjs.name
    $keptSchemes = @()
    if ($json.schemes) { $keptSchemes = @($json.schemes | Where-Object { $_.name -notin $mineNames }) }
    $json.schemes = @($keptSchemes + $schemeObjs)

    # --- profiles (idempotent by deterministic guid) ---
    $newProfiles = foreach ($distro in $Distros.Keys) {
        $label = $Distros[$distro]
        foreach ($theme in $Themes) {
            $pname = "$label - $theme"
            [pscustomobject]@{
                guid        = (New-GuidFromString "setup-apps:${distro}:${theme}")
                name        = $pname
                commandline = "wsl.exe -d $distro --cd ~"
                colorScheme = $theme
                historySize = $HistorySize
                font        = [pscustomobject]@{ face = $TerminalFont; size = 12 }
                icon        = 'https://assets.ubuntu.com/v1/49a1a858-favicon-32x32.png'
                hidden      = $false
            }
        }
    }
    $newGuids = $newProfiles.guid
    $keptProfiles = @($json.profiles.list | Where-Object { $_.guid -notin $newGuids })
    $json.profiles.list = @($keptProfiles + $newProfiles)

    $out = $json | ConvertTo-Json -Depth 32
    [IO.File]::WriteAllText($path, $out, [Text.UTF8Encoding]::new($false))
    Write-Host ("Wrote {0} schemes and {1} profiles to settings.json" -f $schemeObjs.Count, $newProfiles.Count) -ForegroundColor Green
}

# ---- winget apps -----------------------------------------------------------

function Test-AppInstalled {
    # Take the whole app object: winget's --id view misses apps installed outside
    # winget (e.g. Spotify, which installs per-user to %APPDATA% and shows up under
    # Add/Remove Programs but not as a winget package), causing a needless reinstall
    # that then fails because Spotify refuses to run elevated. Check several sources.
    param($App)
    # 1) winget's own view
    winget list --id $App.Id -e --accept-source-agreements 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) { return $true }
    # 2) Add/Remove Programs DisplayName match (catches non-winget installs)
    if ($App.Match) {
        if (Get-UninstallEntries | Where-Object { $_.DisplayName -match $App.Match } | Select-Object -First 1) { return $true }
    }
    # 3) App Paths registry (per-exe), e.g. Spotify.exe / chrome.exe
    if ($App.Exe) {
        foreach ($root in 'HKLM:', 'HKCU:') {
            if (Get-ItemProperty "$root\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\$($App.Exe)" -ErrorAction SilentlyContinue) { return $true }
        }
    }
    # 4) Appx (Store) package present
    if ($App.Appx -and (Get-AppxPackage -Name $App.Appx -ErrorAction SilentlyContinue)) { return $true }
    return $false
}

function Get-WingetVersion {
    param([string]$Id)
    $raw = winget list --id $Id -e --accept-source-agreements 2>$null
    if ($LASTEXITCODE -ne 0) { return $null }
    $line = $raw | Where-Object { $_ -match [regex]::Escape($Id) } | Select-Object -First 1
    if (-not $line) { return 'installed' }
    $after = $line.Substring($line.IndexOf($Id) + $Id.Length).Trim()
    $tok = ($after -split '\s+') | Where-Object { $_ } | Select-Object -First 1
    if ($tok) { return $tok } else { return 'installed' }
}

function Get-UninstallEntries {
    $paths = @(
        'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*'
        'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*'
        'HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*'
    )
    Get-ItemProperty -Path $paths -ErrorAction SilentlyContinue |
        Where-Object { $_.DisplayName } |
        Select-Object DisplayName, DisplayVersion, InstallLocation, DisplayIcon
}

function Resolve-AppLocation {
    param($App, [string]$Version, $Reg)

    $hits = $Reg | Where-Object { $_.DisplayName -match $App.Match }
    if ($hits) {
        $best = $hits | Where-Object { $_.DisplayVersion -eq $Version } | Select-Object -First 1
        if (-not $best) { $best = $hits | Select-Object -First 1 }
        if ($best.InstallLocation) { return $best.InstallLocation.TrimEnd('\') }
        if ($best.DisplayIcon) {
            $icon = ($best.DisplayIcon -split ',')[0].Trim('"')
            if ($icon) { return (Split-Path $icon -Parent) }
        }
    }
    if ($App.Exe) {
        foreach ($root in 'HKLM:', 'HKCU:') {
            $key = "$root\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\$($App.Exe)"
            $exe = (Get-ItemProperty $key -ErrorAction SilentlyContinue).'(default)'
            if ($exe) { return (Split-Path ($exe.Trim('"')) -Parent) }
        }
    }
    if ($App.Appx) {
        $pkg = Get-AppxPackage -Name $App.Appx -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($pkg) { return $pkg.InstallLocation }
    }
    return '(location not found)'
}

# =============================== Status mode ================================

if ($Status) {
    Write-Host "===== WSL =====" -ForegroundColor Cyan
    Get-WslReport

    Write-Host "`n===== Apps =====" -ForegroundColor Cyan
    $reg = Get-UninstallEntries
    $appCount = $apps.Count
    $currentIndex = 0
    $report = foreach ($app in $apps) {
        $currentIndex++
        Write-Host ("[{0}/{1}] Querying {2} ({3})..." -f $currentIndex, $appCount, $app.Name, $app.Id) -ForegroundColor DarkGray
        
        $version = Get-WingetVersion -Id $app.Id
        $installed = [bool]$version
        $location = '-'
        if ($installed) { $location = Resolve-AppLocation -App $app -Version $version -Reg $reg }
        [pscustomobject]@{
            App       = $app.Name
            Installed = if ($installed) { 'Yes' } else { 'No' }
            Version   = if ($installed) { $version } else { '-' }
            Location  = $location
        }
    }
    Write-Host ""
    $report | Format-Table App, Installed, Version, Location -AutoSize |
        Out-String -Width 500 | Write-Host
    return
}

# =============================== Install mode ===============================

# [1/6] WSL platform
$wslOk = Ensure-Wsl

if ($wslOk) {
    # [2/6] Distros
    Write-Host "`n=== [2/6] Ubuntu distributions ===" -ForegroundColor Cyan
    foreach ($distro in $Distros.Keys) { Ensure-Distro -Name $distro }

    # [3/6] GitHub folder link (follow the Windows symlink to its real target)
    Write-Host "`n=== [3/6] GitHub folder link ===" -ForegroundColor Cyan
    $ghItem = Get-Item $GitHubLink -Force -ErrorAction SilentlyContinue
    if ($ghItem) {
        $target = if ($ghItem.Target) { $ghItem.Target } else { $ghItem.FullName }
        # e.g. %USERPROFILE%\OneDrive\GitHub -> /mnt/c/Users/<user>/OneDrive/GitHub
        $wslGh = '/mnt/' + $target.Substring(0,1).ToLower() + ($target.Substring(2) -replace '\\','/')
        Write-Host "Windows '$GitHubLink' -> '$target' -> WSL '$wslGh'" -ForegroundColor DarkGray
        foreach ($distro in $Distros.Keys) {
            if ((Get-WslDistros) -contains $distro) { Link-GitHubInto -Name $distro -WslPath $wslGh }
        }
    } else {
        Write-Host "$GitHubLink not found -- skipping link step." -ForegroundColor Yellow
    }
} else {
    Write-Host "Skipping distro/link steps until WSL is installed." -ForegroundColor Yellow
}

# [4/6] Font
Install-NerdFont

# [5/6] Desktop apps (winget)
Write-Host "`n=== [5/6] Desktop apps (winget) ===" -ForegroundColor Cyan
if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
    Write-Host "winget not available -- skipping app installs." -ForegroundColor Red
} else {
    # Quiet winget's spinner/progress bar. install.sh captures this output to a file
    # (and pty runs make winget think it's interactive), so the animated / - \ | frames
    # each land on their own line -- ugly. 'disabled' keeps just the meaningful lines.
    # Best-effort: older winget without 'settings --set' just no-ops here.
    winget settings --set visual.progressBar disabled 2>$null | Out-Null

    # iTunes: the Microsoft Store build (AppleInc.iTunes) shadows the winget Win32
    # package (Apple.iTunes) -- winget never sees a match, reinstalls each run, and the
    # Win32 MSI then fails 1603. Remove the Store build (per-user Appx, no elevation
    # needed) so the Win32 build installs cleanly in the elevated batch below.
    $storeITunes = Get-AppxPackage -Name '*iTunes*' -ErrorAction SilentlyContinue
    if ($storeITunes) {
        Write-Host "Removing Microsoft Store iTunes ($($storeITunes.Name)) so the Win32 build can install..." -ForegroundColor Yellow
        try { $storeITunes | Remove-AppxPackage -ErrorAction Stop; Write-Host "  removed." -ForegroundColor DarkGray }
        catch { Write-Warning "  could not remove Store iTunes: $($_.Exception.Message)" }
    }

    $results = [ordered]@{}
    $appCount = $apps.Count
    $currentIndex = 0

    foreach ($app in $apps) {
        $currentIndex++
        # Elevation-only apps (e.g. iTunes' Win32 MSI) are installed in the single
        # elevated batch (setup-elevated.ps1), not here -- this pass is non-elevated
        # on purpose (Spotify refuses to install elevated).
        if ($app.Elevated) {
            Write-Host ("[{0}/{1}] {2} ({3}) -- deferred to the elevated batch." -f $currentIndex, $appCount, $app.Name, $app.Id) -ForegroundColor DarkGray
            $results[$app.Name] = 'Deferred (elevated)'
            continue
        }
        Write-Host ("[{0}/{1}] Checking {2} ({3})..." -f $currentIndex, $appCount, $app.Name, $app.Id) -ForegroundColor Cyan

        if (Test-AppInstalled -App $app) {
            Write-Host "$($app.Name) ($($app.Id)) already installed -- skipping." -ForegroundColor DarkGray
            $results[$app.Name] = 'Already installed'
            continue
        }
        Write-Host "Installing $($app.Name) ($($app.Id))..." -ForegroundColor Cyan
        winget install --id $app.Id -e --source winget `
            --accept-package-agreements --accept-source-agreements --disable-interactivity
        $code = $LASTEXITCODE
        if ($code -eq 0 -or $code -eq -1978335189) { $results[$app.Name] = 'Installed' }
        else { $results[$app.Name] = "FAILED (exit $code)" }
    }

    Write-Host "`n=================== APP SUMMARY ===================" -ForegroundColor Yellow
    foreach ($name in $results.Keys) {
        $appStatus = $results[$name]
        $color = if ($appStatus -like 'FAILED*') { 'Red' } else { 'Green' }
        Write-Host ("{0,-16} {1}" -f $name, $appStatus) -ForegroundColor $color
    }
    Write-Host "===================================================" -ForegroundColor Yellow
}

# [6/6] Windows Terminal configuration
Configure-Terminal

# === Elevated setup (one UAC prompt) =======================================
# Everything that needs admin -- the macOS-hotkeys logon task, the iTunes Win32
# MSI, the Wispr Flow MSI, and the PowerToys Copilot-key remap -- runs once here,
# in a single elevated child. This pass stays non-elevated (above) so Spotify can
# install; we batch the admin work into ONE Start-Process -Verb RunAs so the user
# approves a single UAC prompt instead of one per installer.
if (-not $SkipElevated) {
    Write-Host "`n=== Elevated setup (approve ONE UAC prompt) ===" -ForegroundColor Cyan
    # Resolve a LOCAL deployed path for the elevated child. When install_windows.sh
    # launches this script via WSL, $PSScriptRoot is a \\wsl.localhost\... UNC path
    # that an elevated process cannot read; and the child would inherit that UNC as
    # its CWD. Point -File at the local Desktop copy and pin -WorkingDirectory local.
    $elevDeployed = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Apps\scripts\setup-elevated.ps1'
    $elev = if ($PSScriptRoot -like '\\*' -and (Test-Path $elevDeployed)) { $elevDeployed }
            else { Join-Path $PSScriptRoot 'setup-elevated.ps1' }
    if (Test-Path $elev) {
        Write-Host "Finishing setup (hotkey task, iTunes, Wispr Flow, Copilot-key remap)..." -ForegroundColor Cyan
        try {
            Start-Process powershell -Verb RunAs -WorkingDirectory $env:SystemRoot -ArgumentList @(
                '-NoProfile','-ExecutionPolicy','Bypass','-File',"`"$elev`"") -Wait
            Write-Host "Elevated setup finished. Details: C:\Windows\Temp\setup-elevated.log" -ForegroundColor Green
        } catch {
            Write-Warning "Elevated setup was cancelled or failed: $($_.Exception.Message)"
            Write-Warning "Run it later from a native PowerShell (approve UAC):"
            Write-Warning "  powershell -ExecutionPolicy Bypass -File `"$elev`""
        }
    } else {
        Write-Warning "setup-elevated.ps1 not found next to setup-apps.ps1; skipping elevated setup."
    }
}
