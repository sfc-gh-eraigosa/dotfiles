# Windows Setup: Consolidated Elevation + App-Install Fixes — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `install.sh`'s Windows customization perform all elevation-requiring work via a single UAC prompt, fix iTunes (swap the Store build for the Win32 build), skip already-installed Spotify, and move the Wispr Flow MSI install into a shared function the unified app step triggers (with `install-wisprflow.ps1` retaining customizations).

**Architecture:** `setup-apps.ps1` stays the non-elevated orchestrator (it installs user-scope winget apps, some of which — Spotify — *refuse* to run elevated). After the non-elevated pass it fires **one** `Start-Process -Verb RunAs` (with a local working directory and a local deployed script path — the proven fix from `setup-autostart.ps1`) at a new `setup-elevated.ps1`, which performs **all** admin work in a single elevated child: register the hotkey task, swap iTunes, install the Wispr MSI, and run `suppress-copilot-key.ps1`. The Wispr MSI download/install logic is factored into `wispr-install-core.ps1`, dot-sourced by both `setup-elevated.ps1` and `install-wisprflow.ps1`.

**Tech Stack:** Windows PowerShell 5.1 (.ps1, run via WSL interop from `install_windows.sh`), winget, msiexec, AutoHotkey v2, PowerToys Keyboard Manager.

---

## Hard prerequisite (blocks ALL verification)

WSL→Windows interop must be working (`powershell.exe -Command "echo ok"` succeeds). If the `WSLInterop` binfmt handler is unregistered, restore it first:

```
sudo bash -c 'echo ":WSLInterop:M::MZ::/init:PF" > /proc/sys/fs/binfmt_misc/register'
```

or `wsl --shutdown` (from Windows) and reopen. Every "Verify" step below requires interop **and** a human to approve the single UAC prompt.

## Key unverified assumption (validate in Task 0)

`Start-Process -Verb RunAs -WorkingDirectory <local> -File <local .ps1> -Wait`, fired from a WSL-spawned `powershell.exe`, produces a working elevated child (local CWD avoids the `\\wsl.localhost` inheritance that kills admin processes). This is strongly indicated by the `setup-autostart.ps1` fix but never cleanly confirmed end-to-end. Task 0 confirms it before we build on it.

## File structure

- **Create** `opt/Desktop/Apps/scripts/wispr-install-core.ps1` — dot-sourceable functions: `Get-WisprInstalled`, `Resolve-WisprLatestVersion`, `Save-WisprMsi`, `Install-WisprMsi`. No top-level side effects.
- **Create** `opt/Desktop/Apps/scripts/setup-elevated.ps1` — the single elevated orchestrator. Assumes it is already admin; performs task registration, iTunes swap, Wispr MSI install, KBM remap. Writes a result log to `C:\Windows\Temp\setup-elevated.log`.
- **Modify** `opt/Desktop/Apps/scripts/install-wisprflow.ps1` — dot-source the core; keep `-Status`/`-Uninstall`/customizations/manual-guidance.
- **Modify** `opt/Desktop/Apps/scripts/setup-apps.ps1` — improve install detection (registry/App-Paths/Appx, not just `winget list`); after the non-elevated pass, fire the single elevated batch; add `-SkipElevated` switch.
- **Modify** `opt/Desktop/Apps/scripts/setup-autostart.ps1` — no behavior change required (already fixed); `setup-elevated.ps1` calls it while already admin so it skips self-elevation.
- **Modify** `opt/bin/install_windows.sh` — the `[y]` branch runs `setup-apps.ps1` (which now drives the single elevation) and drops the separate `setup-autostart.ps1` invocation; update the banner.
- **Modify** `opt/Desktop/Apps/scripts/WISPR-FLOW.md` — document the new single-elevation flow and that the MSI install now runs in the batch.

---

## Task 0: Confirm WSL-driven single elevation works

**Files:** none (diagnostic only).

- [ ] **Step 1: Write a local elevated probe**

Create `/mnt/c/Users/<user>/OneDrive/Desktop/Apps/scripts/_elev_probe.ps1`:

```powershell
$out = 'C:\Windows\Temp\elev_probe_out.txt'
"ran_at=$(Get-Date -Format o)" | Set-Content $out
"CWD=$((Get-Location).Path)"   | Add-Content $out
$admin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
"IsAdmin=$admin" | Add-Content $out
```

- [ ] **Step 2: Fire it the way setup-apps will (WSL-spawned, local CWD + local -File), approve the one UAC prompt**

```bash
PS=powershell.exe
"$PS" -NoProfile -Command "Remove-Item 'C:\Windows\Temp\elev_probe_out.txt' -EA SilentlyContinue; Start-Process powershell -Verb RunAs -WorkingDirectory 'C:\\Windows' -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-File','C:\\Users\\<user>\\OneDrive\\Desktop\\Apps\\scripts\\_elev_probe.ps1' -Wait"
```

- [ ] **Step 3: Verify**

```bash
powershell.exe -NoProfile -Command "Get-Content 'C:\Windows\Temp\elev_probe_out.txt'"
```
Expected: file exists, `IsAdmin=True`, `CWD=C:\Windows`. **If the file is absent after approving UAC, STOP** — the consolidated-elevation approach is not viable from WSL; fall back to "instruct the user to run `setup-elevated.ps1` from a native elevated PowerShell" and revise Tasks 4–5 accordingly.

- [ ] **Step 4: Clean up**

```bash
rm -f /mnt/c/Users/<user>/OneDrive/Desktop/Apps/scripts/_elev_probe.ps1
powershell.exe -NoProfile -Command "Remove-Item 'C:\Windows\Temp\elev_probe_out.txt' -EA SilentlyContinue"
```

---

## Task 1: Robust "is it installed?" detection (fixes Spotify re-install)

**Files:**
- Modify: `opt/Desktop/Apps/scripts/setup-apps.ps1` (the `Test-AppInstalled` function, ~268–272)

- [ ] **Step 1: Replace `Test-AppInstalled` with a multi-source check**

The current check only trusts `winget list --id`, which misses apps installed outside winget (Spotify installs per-user to `%APPDATA%\Spotify`). Use the same metadata the `$apps` entries already carry (`Match`, `Exe`, `Appx`) that `Resolve-AppLocation` relies on.

```powershell
function Test-AppInstalled {
    param($App)   # now takes the whole app object, not just an Id
    # 1) winget's own view
    winget list --id $App.Id -e --accept-source-agreements 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) { return $true }
    # 2) Add/Remove Programs DisplayName match (catches non-winget installs, e.g. Spotify)
    if ($App.Match) {
        $hit = Get-UninstallEntries | Where-Object { $_.DisplayName -match $App.Match } | Select-Object -First 1
        if ($hit) { return $true }
    }
    # 3) App Paths registry (per-exe), e.g. iTunes.exe / Spotify.exe
    if ($App.Exe) {
        foreach ($root in 'HKLM:', 'HKCU:') {
            $key = "$root\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\$($App.Exe)"
            if (Get-ItemProperty $key -ErrorAction SilentlyContinue) { return $true }
        }
    }
    # 4) Appx (Store) package present
    if ($App.Appx -and (Get-AppxPackage -Name $App.Appx -ErrorAction SilentlyContinue)) { return $true }
    return $false
}
```

- [ ] **Step 2: Update the two call sites to pass the app object**

In the install loop (~398) and anywhere else calling `Test-AppInstalled -Id $app.Id`, change to `Test-AppInstalled -App $app`.

- [ ] **Step 3: Verify (needs interop)**

```bash
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "C:\Users\<user>\OneDrive\Desktop\Apps\scripts\setup-apps.ps1" -Status
```
Expected: Spotify shows `Installed = Yes` (so the install pass will skip it). Confirm the `$apps` Spotify entry has a `Match` (e.g. `'Spotify'`) and/or `Exe` (`'Spotify.exe'`); add them if missing (Task 1a).

- [ ] **Step 3a (if needed): add detection metadata to the Spotify app entry**

Locate the `$apps` array (near the top of `setup-apps.ps1`) and ensure the Spotify entry includes `Match = 'Spotify'; Exe = 'Spotify.exe'`. Show the edited entry in the commit.

- [ ] **Step 4: Commit**

```bash
git add opt/Desktop/Apps/scripts/setup-apps.ps1
git commit -m "fix(setup-apps): detect non-winget installs (Spotify) via registry/App-Paths/Appx"
```

---

## Task 2: iTunes — swap the Store build for the Win32 build

**Decision:** The `$apps` list installs `Apple.iTunes` (Win32). The machine has `AppleInc.iTunes` (Store), which never matches that id, so winget reinstalls every run and the Win32 MSI then collides (`1603`). Uninstall the Store build (no elevation needed — Appx is per-user) so the Win32 install (run elevated in Task 4) succeeds; detection from Task 1 then recognizes it.

**Files:**
- Modify: `opt/Desktop/Apps/scripts/setup-apps.ps1` (non-elevated pre-step before the winget loop)

- [ ] **Step 1: Add a one-time Store-iTunes removal before the install loop**

Just before the `foreach ($app in $apps)` install loop (~394):

```powershell
# iTunes: the Microsoft Store build (AppleInc.iTunes) shadows the winget Win32
# package (Apple.iTunes) — winget never sees a match, reinstalls each run, and the
# Win32 MSI then fails 1603. Remove the Store build (per-user Appx, no elevation)
# so the Win32 build installs cleanly in the elevated batch.
$storeITunes = Get-AppxPackage -Name '*iTunes*' -ErrorAction SilentlyContinue
if ($storeITunes) {
    Write-Host "Removing Microsoft Store iTunes ($($storeITunes.Name)) so the Win32 build can install..." -ForegroundColor Yellow
    try { $storeITunes | Remove-AppxPackage -ErrorAction Stop; Write-Host "  removed." }
    catch { Write-Warning "  could not remove Store iTunes: $($_.Exception.Message)" }
}
```

- [ ] **Step 2: Verify (needs interop)**

```bash
powershell.exe -NoProfile -Command "Get-AppxPackage *iTunes* | Select-Object Name"
```
Expected after a run: no `AppleInc.iTunes` Appx; `winget list --id Apple.iTunes -e` later shows the Win32 build.

- [ ] **Step 3: Commit**

```bash
git add opt/Desktop/Apps/scripts/setup-apps.ps1
git commit -m "fix(setup-apps): remove Store iTunes so the Win32 winget package installs"
```

---

## Task 3: Extract the shared Wispr install core

**Files:**
- Create: `opt/Desktop/Apps/scripts/wispr-install-core.ps1`
- Modify: `opt/Desktop/Apps/scripts/install-wisprflow.ps1`

- [ ] **Step 1: Create `wispr-install-core.ps1` (functions only, no side effects)**

Move the install primitives out of `install-wisprflow.ps1` verbatim (names prefixed `Wispr` to avoid collisions when dot-sourced):

```powershell
# wispr-install-core.ps1 — dot-sourceable Wispr Flow MSI primitives. No top-level
# side effects: safe to dot-source from install-wisprflow.ps1 and setup-elevated.ps1.
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$script:WisprAppName   = 'Wispr Flow'
$script:WisprResolve   = 'https://dl.wisprflow.ai/windows/latest'
$script:WisprMsiUrlFmt = 'https://dl.wisprflow.com/wispr-flow/win32/x64/Wispr%20Flow-v{0}.msi'
$script:WisprPinned    = '1.5.530'

function Get-WisprInstalled {
    $hives = @(
        'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*',
        'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*',
        'HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*'
    )
    Get-ItemProperty $hives -ErrorAction SilentlyContinue |
        Where-Object { $_.DisplayName -match 'Wispr\s*Flow' } | Select-Object -First 1
}

function Resolve-WisprLatestVersion {
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
    if ((Test-Path $Dest) -and ((Get-Item $Dest).Length -gt 50MB)) { Write-Host "Using cached installer: $Dest"; return }
    Write-Host "Downloading from $Url ..."
    Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing
}

function Test-WisprAdmin {
    (New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())).IsInRole(
        [Security.Principal.WindowsBuiltinRole]::Administrator)
}

function Install-WisprMsi {
    # Returns $true if installed (or already present), $false on a handled failure.
    param([switch]$Latest, [string]$Version = $script:WisprPinned, [switch]$Force)
    $existing = Get-WisprInstalled
    if ($existing -and -not $Force) {
        Write-Host "$script:WisprAppName already installed (version $($existing.DisplayVersion)); skipping."
        return $true
    }
    $ver = $Version
    if ($Latest) { $r = Resolve-WisprLatestVersion; if ($r) { $ver = $r } }
    $msiUrl = [string]::Format($script:WisprMsiUrlFmt, $ver)
    $tmp    = Join-Path $env:TEMP "WisprFlow-v$ver.msi"
    Write-Host "Preparing $script:WisprAppName v$ver ... $msiUrl"
    try { Save-WisprMsi -Url $msiUrl -Dest $tmp }
    catch {
        if ($Latest) { throw }
        Write-Warning "Pinned download failed ($($_.Exception.Message)); resolving latest and retrying."
        $r = Resolve-WisprLatestVersion
        if (-not $r) { throw "Could not download v$ver and could not resolve latest." }
        $ver = $r; $msiUrl = [string]::Format($script:WisprMsiUrlFmt, $ver); $tmp = Join-Path $env:TEMP "WisprFlow-v$ver.msi"
        Save-WisprMsi -Url $msiUrl -Dest $tmp
    }
    $sizeMB = [math]::Round((Get-Item $tmp).Length / 1MB, 1)
    if ($sizeMB -lt 50) { throw "MSI is only ${sizeMB} MB - looks like an error page, not the installer." }
    Write-Host "Installing $script:WisprAppName v$ver (silent) ..."
    $verb = if (Test-WisprAdmin) { @{} } else { @{ Verb = 'RunAs' } }
    $p = Start-Process msiexec.exe -ArgumentList @('/i', "`"$tmp`"", '/quiet', '/norestart') -PassThru -Wait @verb
    switch ($p.ExitCode) {
        { $_ -in 0, 3010, 1638 } { Write-Host "$script:WisprAppName install ok (exit $($p.ExitCode))."; if ($_ -in 0,3010) { Remove-Item $tmp -EA SilentlyContinue }; return $true }
        default { Write-Warning "msiexec install failed (exit $($p.ExitCode)). Cached installer kept at $tmp"; return $false }
    }
}
```

- [ ] **Step 2: Refactor `install-wisprflow.ps1` to dot-source the core**

Replace its inline `Get-Installed`/`Resolve-LatestVersion`/`Save-Msi`/install block with:

```powershell
. (Join-Path $PSScriptRoot 'wispr-install-core.ps1')
# ... param block / Status / Uninstall unchanged (Status/Uninstall may call Get-WisprInstalled) ...
# Install path:
$ok = Install-WisprMsi -Latest:$Latest -Version $Version -Force:$Force
if (-not $ok) {
    Write-Host "Install did not complete. Re-run this script and approve UAC, or run msiexec from an admin shell."
    exit 1
}
```
Keep the existing PowerToys (`suppress-copilot-key.ps1`) block and the "One-time manual setup" banner exactly as-is after the install call.

- [ ] **Step 3: Verify standalone path still works (needs interop)**

```bash
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "C:\Users\<user>\OneDrive\Desktop\Apps\scripts\install-wisprflow.ps1" -Status
```
Expected: prints install status without error (proves the dot-source + functions load).

- [ ] **Step 4: Commit**

```bash
git add opt/Desktop/Apps/scripts/wispr-install-core.ps1 opt/Desktop/Apps/scripts/install-wisprflow.ps1
git commit -m "refactor(wispr): extract shared MSI install core; install-wisprflow dot-sources it"
```

---

## Task 4: The single elevated orchestrator `setup-elevated.ps1`

**Files:**
- Create: `opt/Desktop/Apps/scripts/setup-elevated.ps1`

- [ ] **Step 1: Create `setup-elevated.ps1`**

Runs as admin (launched once by `setup-apps.ps1`). Does every admin task, logging each so failures are visible.

```powershell
# setup-elevated.ps1 — ALL elevation-requiring setup, run once under a single UAC
# prompt (launched by setup-apps.ps1 via Start-Process -Verb RunAs). Designed to be
# already-admin: it self-elevates only if run standalone non-elevated.
$ErrorActionPreference = 'Continue'   # one failing step must not abort the rest
$log = 'C:\Windows\Temp\setup-elevated.log'
"=== setup-elevated $(Get-Date -Format o) ===" | Set-Content $log

# Self-elevate if launched standalone without admin. Use a LOCAL deployed copy +
# local CWD so the elevated child does not inherit an inaccessible \\wsl.localhost.
$admin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $admin) {
    $selfDeployed = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Apps\scripts\setup-elevated.ps1'
    $self = if ($PSCommandPath -like '\\*' -and (Test-Path $selfDeployed)) { $selfDeployed } else { $PSCommandPath }
    Start-Process powershell -Verb RunAs -WorkingDirectory $env:SystemRoot -ArgumentList @(
        '-NoProfile','-ExecutionPolicy','Bypass','-File',"`"$self`"") -Wait
    return
}

$dir = $PSScriptRoot   # local deployed scripts dir (this file is admin-launched locally)

function Log($m) { Write-Host $m; "$m" | Add-Content $log }

# 1) macOS-style hotkeys logon task (setup-autostart.ps1 skips its own self-elevate
#    because we are already admin).
try { Log '== hotkey task =='; & (Join-Path $dir 'setup-autostart.ps1') }
catch { Log "  task registration failed: $($_.Exception.Message)" }

# 2) iTunes Win32 (Store build was removed non-elevated in setup-apps).
try {
    Log '== iTunes (Win32) =='
    winget install --id Apple.iTunes -e --source winget --accept-package-agreements --accept-source-agreements --disable-interactivity
    Log "  winget exit $LASTEXITCODE"
} catch { Log "  iTunes install failed: $($_.Exception.Message)" }

# 3) Wispr Flow MSI (shared core; already admin so no nested UAC).
try { Log '== Wispr Flow =='; . (Join-Path $dir 'wispr-install-core.ps1'); $null = Install-WisprMsi }
catch { Log "  Wispr install failed: $($_.Exception.Message)" }

# 4) PowerToys Copilot-key remap (best-effort; warns if PowerToys absent).
try { Log '== suppress-copilot-key =='; & (Join-Path $dir 'suppress-copilot-key.ps1') }
catch { Log "  suppress-copilot-key failed: $($_.Exception.Message)" }

Log "=== done $(Get-Date -Format o) ==="
```

- [ ] **Step 2: Verify standalone-elevated path (needs interop + UAC)**

From a native **elevated** PowerShell:
```powershell
powershell -ExecutionPolicy Bypass -File "C:\Users\<user>\OneDrive\Desktop\Apps\scripts\setup-elevated.ps1"
```
Expected console + `C:\Windows\Temp\setup-elevated.log`: task registered, iTunes winget exit 0, Wispr install ok, suppress-copilot ran. Then confirm: `Get-ScheduledTask 'macOS Hotkeys'` = Running; `install-wisprflow.ps1 -Status` = installed.

- [ ] **Step 3: Commit**

```bash
git add opt/Desktop/Apps/scripts/setup-elevated.ps1
git commit -m "feat(setup): single elevated orchestrator (task + iTunes + Wispr + KBM)"
```

---

## Task 5: `setup-apps.ps1` fires the one elevation; `install_windows.sh` simplified

**Files:**
- Modify: `opt/Desktop/Apps/scripts/setup-apps.ps1` (after the winget loop / before/after `[6/6]`)
- Modify: `opt/bin/install_windows.sh` (the `[y]` branch)

- [ ] **Step 1: Add `-SkipElevated` switch to `setup-apps.ps1`**

In its `param(...)` block add `[switch]$SkipElevated` (so `-Status` and CI can opt out).

- [ ] **Step 2: After the non-elevated app pass, fire the single elevated batch**

Append after `Configure-Terminal` (end of install mode):

```powershell
if (-not $SkipElevated) {
    Write-Host "`n=== [6/6] Elevated setup (one UAC prompt) ===" -ForegroundColor Cyan
    # Resolve a LOCAL deployed path for the elevated child (this script may be running
    # from \\wsl.localhost when launched by install_windows.sh). Pin a local CWD so the
    # admin child does not inherit the inaccessible WSL share.
    $elevDeployed = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Apps\scripts\setup-elevated.ps1'
    $elev = if ($PSScriptRoot -like '\\*' -and (Test-Path $elevDeployed)) { $elevDeployed } else { (Join-Path $PSScriptRoot 'setup-elevated.ps1') }
    if (Test-Path $elev) {
        Write-Host "Approve the single UAC prompt to finish (hotkey task, iTunes, Wispr, Copilot-key remap)..."
        try {
            Start-Process powershell -Verb RunAs -WorkingDirectory $env:SystemRoot -ArgumentList @(
                '-NoProfile','-ExecutionPolicy','Bypass','-File',"`"$elev`"") -Wait
            Write-Host "Elevated setup finished (see C:\Windows\Temp\setup-elevated.log)."
        } catch {
            Write-Warning "Elevated setup was cancelled/failed: $($_.Exception.Message)"
            Write-Warning "Run it later from a native PowerShell:  $elev"
        }
    } else { Write-Warning "setup-elevated.ps1 not found next to setup-apps.ps1; skipping elevated setup." }
}
```

- [ ] **Step 3: Simplify `install_windows.sh` `[y]` branch**

Remove the separate "Registering macOS-style hotkeys" `setup-autostart.ps1` invocation (lines ~115–118) and the standalone Wispr-instructions echo (~96–113) — both are now handled by the single elevated batch `setup-apps.ps1` triggers. Keep the marker write. The `[y]` branch becomes:

```bash
        echo "Starting Windows customization... (this may take a few minutes)"
        "$ps_exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "${BASE_DIR}/opt/Desktop/Apps/scripts/setup-apps.ps1" </dev/null > /tmp/setup_apps.log 2>&1
        cat /tmp/setup_apps.log
        echo ""
        echo "Wispr Flow one-time manual setup (sign-in, mic, shortcuts off Win) is documented in:"
        echo "    $(wslpath -w "${win_desktop}/Apps/scripts/WISPR-FLOW.md" 2>/dev/null)"
        mkdir -p "$(dirname "$WIN_SETUP_MARKER")"
        : > "$WIN_SETUP_MARKER"
```

(Note the added `</dev/null` on the setup-apps call — same Windows-exe stdin-drain hygiene as PR #69.)

- [ ] **Step 4: Verify end-to-end (needs interop + one UAC approval)**

From WSL: `./install.sh` (answer `y`, approve the one UAC prompt). Then:
```bash
powershell.exe -NoProfile -Command "Get-ScheduledTask 'macOS Hotkeys' | Select State; (Get-Content 'C:\Windows\Temp\setup-elevated.log')"
```
Expected: exactly one UAC prompt during the run; task Running; Wispr installed; Store iTunes gone + Win32 present; Spotify skipped (not re-attempted).

- [ ] **Step 5: Commit**

```bash
git add opt/Desktop/Apps/scripts/setup-apps.ps1 opt/bin/install_windows.sh
git commit -m "feat(install): drive all elevation through one setup-apps -> setup-elevated UAC prompt"
```

---

## Task 6: Docs

**Files:**
- Modify: `opt/Desktop/Apps/scripts/WISPR-FLOW.md`

- [ ] **Step 1: Update the install section**

Document that `install.sh`/`setup-apps.ps1` now installs Wispr (via `wispr-install-core.ps1`) inside the single elevated batch, that there is one UAC prompt, and that `install-wisprflow.ps1` remains the standalone installer + customization manager. Keep the one-time manual steps (sign-in, mic, shortcuts off Win, start-at-login).

- [ ] **Step 2: Commit**

```bash
git add opt/Desktop/Apps/scripts/WISPR-FLOW.md
git commit -m "docs(wispr): document single-UAC batch install + shared core"
```

---

## Self-review notes

- **Spec coverage:** iTunes swap → Task 2 (+1 for detection). Spotify skip → Task 1. Wispr via shared fn → Tasks 3–5. One UAC → Tasks 4–5 (gated by Task 0). Elevation auto-kick-in → Tasks 0/4/5.
- **Interaction flagged:** "setup-apps installs Wispr" + "one UAC" reconciled — setup-apps *orchestrates*, the elevated batch *executes* (Wispr install logic shared via core).
- **Risk:** Task 0 is a go/no-go. If WSL-driven `Start-Process -Verb RunAs` can't produce a working elevated child even with local CWD/path, Tasks 4–5 change to "instruct a native elevated run" (the reliable path proven this session).
- **Out of scope:** Spotify exit-29-when-elevated is avoided by *skipping* it (Task 1), not by fixing Spotify's elevation behavior.
