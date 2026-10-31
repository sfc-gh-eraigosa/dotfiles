<#
.SYNOPSIS
  One-time (idempotent) setup so the wslgiga1 Windows host recovers by itself
  after sign-in and reapplies the GPU power cap at boot. Run elevated.
.DESCRIPTION
  - Unregisters the hand-made tasks PlaygroundDevStackAtLogon and PlaygroundGpuPowerCap.
  - Registers PlaygroundLogon: at logon of the operator, limited (not elevated),
    short delay, runs playground-logon.ps1.
  - Registers PlaygroundGpuCapApply: at startup, SYSTEM, highest, runs
    playground-gpu-cap.ps1 from a SYSTEM-owned copy in %ProgramData%\playground
    (SYSTEM never runs a script a normal user can edit).
  - Deploys the desktop app's exe to the operator's state dir, only if found.
  - Prints the Autologon steps and whether C: is BitLocker-protected.

  WHO IS THE OPERATOR. A task for SYSTEM sees SYSTEM's %LOCALAPPDATA%, and an
  elevated shell started under a different admin account has that admin's
  $env:USERNAME. So the operator is not taken from the environment: it is -User
  if given, else the account signed in at the console (Win32_ComputerSystem), else
  the current user. Its profile folder comes from the ProfileList registry key by
  SID, and that absolute path is written into the cap task's arguments.

  -WhatIf prints every action, changes nothing, and does not need elevation.
  Autologon's password is never read, passed or stored by this script.
.PARAMETER User
  Operator account (DOMAIN\name or name). Default: the console user.
.PARAMETER AppExe
  The desktop app's exe to deploy. Default: wslgiga1-status*.exe next to this script, if any.
.PARAMETER LogonDelaySeconds
  Delay after logon before PlaygroundLogon runs (default 30, so the profile and network settle).
.EXAMPLE
  .\install-wslgiga1.ps1 -WhatIf
.EXAMPLE
  .\install-wslgiga1.ps1
#>
[CmdletBinding(SupportsShouldProcess)]
param(
    [string]$User,
    [string]$AppExe,
    [ValidateRange(0, 600)][int]$LogonDelaySeconds = 30
)
$ErrorActionPreference = 'Stop'
function Say($m) { Write-Host "==> $m" -ForegroundColor Cyan }
function Note($m) { Write-Host "    $m" }

$LogonTask = 'PlaygroundLogon'
$CapTask = 'PlaygroundGpuCapApply'
$OldTasks = 'PlaygroundDevStackAtLogon', 'PlaygroundGpuPowerCap'
$PsExe = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
           ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin -and -not $WhatIfPreference) {
    throw 'Run this from an elevated PowerShell (or use -WhatIf for a dry run).'
}
if (-not $isAdmin) { Say 'Not elevated: -WhatIf only, nothing will be changed' }

# --- who is the operator, and where is their state dir ----------------------
if (-not $User) {
    # Load the CIM module with -WhatIf off: under -WhatIf its alias setup would print noise.
    $savedWhatIf = $WhatIfPreference; $WhatIfPreference = $false
    Import-Module CimCmdlets -ErrorAction SilentlyContinue
    $WhatIfPreference = $savedWhatIf
    try { $User = [string](Get-CimInstance Win32_ComputerSystem).UserName } catch { $User = '' }
    if (-not $User) { $User = [Security.Principal.WindowsIdentity]::GetCurrent().Name }
}
$acct = New-Object Security.Principal.NTAccount($User)
$sid = $acct.Translate([Security.Principal.SecurityIdentifier]).Value
$profileKey = "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\$sid"
$profileDir = [string](Get-ItemProperty -LiteralPath $profileKey -ErrorAction SilentlyContinue).ProfileImagePath
if (-not $profileDir) { throw "No profile folder found for $User ($sid); has that account signed in once?" }
$profileDir = [Environment]::ExpandEnvironmentVariables($profileDir)
$stateDir = Join-Path $profileDir 'AppData\Local\playground'
Say "Operator $User; state dir $stateDir"

$scripts = $PSScriptRoot
if ($scripts -match '^\\\\wsl') {
    $m = 'Run the copy deployed to the Windows Desktop (Apps\scripts), not one on a \\wsl path: the tasks must work before WSL is up.'
    if ($WhatIfPreference) { Write-Warning $m } else { throw $m }
}
foreach ($f in 'playground-logon.ps1', 'playground-gpu-cap.ps1', 'Start-DevStack.ps1', 'lib\wsl.ps1') {
    if (-not (Test-Path -LiteralPath (Join-Path $scripts $f))) { throw "Missing $f next to this script" }
}

# --- 1. remove the hand-made tasks ------------------------------------------
foreach ($t in $OldTasks) {
    if (Get-ScheduledTask -TaskName $t -ErrorAction SilentlyContinue) {
        if ($PSCmdlet.ShouldProcess($t, 'Unregister scheduled task')) {
            Unregister-ScheduledTask -TaskName $t -Confirm:$false
            Note "removed $t"
        }
    } else { Note "$t not present" }
}

# --- 2. SYSTEM-owned copy of the cap script ---------------------------------
$sysDir = Join-Path $env:ProgramData 'playground'
$capCopy = Join-Path $sysDir 'playground-gpu-cap.ps1'
if ($PSCmdlet.ShouldProcess($sysDir, 'Create folder (SYSTEM and Administrators full, Users read) and copy playground-gpu-cap.ps1')) {
    $null = New-Item -ItemType Directory -Force -Path $sysDir
    # *S-1-5-18 SYSTEM, *S-1-5-32-544 Administrators, *S-1-5-32-545 Users; no inheritance from ProgramData.
    & icacls.exe $sysDir /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-32-545:(OI)(CI)RX' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "icacls failed on $sysDir (exit $LASTEXITCODE)" }
    Copy-Item -LiteralPath (Join-Path $scripts 'playground-gpu-cap.ps1') -Destination $capCopy -Force
    Note "copied to $capCopy"
}

# --- 3. the state dir (the operator's; may not exist yet) -------------------
if ($PSCmdlet.ShouldProcess($stateDir, 'Create folder')) {
    $null = New-Item -ItemType Directory -Force -Path $stateDir
}

# --- 4. PlaygroundLogon ------------------------------------------------------
$logonArgs = '-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "{0}"' -f (Join-Path $scripts 'playground-logon.ps1')
if ($PSCmdlet.ShouldProcess($LogonTask, "Register: at logon of $User (+${LogonDelaySeconds}s), limited, $PsExe $logonArgs")) {
    $action = New-ScheduledTaskAction -Execute $PsExe -Argument $logonArgs
    $trigger = New-ScheduledTaskTrigger -AtLogOn -User $User
    $trigger.Delay = 'PT{0}S' -f $LogonDelaySeconds
    $principal = New-ScheduledTaskPrincipal -UserId $User -LogonType Interactive -RunLevel Limited
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
        -ExecutionTimeLimit (New-TimeSpan -Minutes 30) -MultipleInstances IgnoreNew -StartWhenAvailable
    Register-ScheduledTask -TaskName $LogonTask -Action $action -Trigger $trigger -Principal $principal `
        -Settings $settings -Description 'wslgiga1: recover WSL after sign-in unless a person paused the node, then start the status app.' -Force | Out-Null
    Note "registered $LogonTask"
}

# --- 5. PlaygroundGpuCapApply ------------------------------------------------
$capArgs = '-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "{0}" -StateDir "{1}"' -f $capCopy, $stateDir
if ($PSCmdlet.ShouldProcess($CapTask, "Register: at startup, SYSTEM, highest, $PsExe $capArgs")) {
    $action = New-ScheduledTaskAction -Execute $PsExe -Argument $capArgs
    $trigger = New-ScheduledTaskTrigger -AtStartup
    $principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
    # The driver can be late at boot: retry a few times if the script exits non-zero.
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
        -ExecutionTimeLimit (New-TimeSpan -Minutes 5) -StartWhenAvailable `
        -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
    Register-ScheduledTask -TaskName $CapTask -Action $action -Trigger $trigger -Principal $principal `
        -Settings $settings -Description 'wslgiga1: reapply the GPU power cap (gpu-cap-w, default 280 W) at boot.' -Force | Out-Null
    Note "registered $CapTask"
}

# --- 6. the desktop app, only if we can find it ------------------------------
if (-not $AppExe) {
    $hit = Get-ChildItem -LiteralPath $scripts -Filter 'wslgiga1-status*.exe' -File -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if ($hit) { $AppExe = $hit.FullName }
}
if ($AppExe -and (Test-Path -LiteralPath $AppExe -PathType Leaf)) {
    $dest = Join-Path $stateDir 'wslgiga1-status.exe'
    if ($PSCmdlet.ShouldProcess($dest, "Copy desktop app from $AppExe")) {
        try { Copy-Item -LiteralPath $AppExe -Destination $dest -Force; Note "app deployed to $dest" }
        catch { Write-Warning "Could not copy the app (is it running?): $($_.Exception.Message)" }
    }
} else {
    Say 'Desktop app not found: not deployed (pass -AppExe, or put wslgiga1-status*.exe next to this script and re-run)'
}

# --- 7. BitLocker ------------------------------------------------------------
Say 'BitLocker on C:'
$bde = (& manage-bde.exe -status C: 2>&1) -join "`n"
if ($bde -match 'Protection Status:\s*(.+)') {
    $ps = $Matches[1].Trim()
    if ($ps -match 'On') { Note 'C: is BitLocker-protected (Protection On).' }
    else { Write-Warning "C: is NOT BitLocker-protected ($ps). Autologon stores the password on this disk; anyone with the disk can read it." }
} else {
    Write-Warning 'Could not read BitLocker status (manage-bde needs elevation). Check it from an elevated shell: manage-bde -status C:'
}

# --- 8. Autologon: a manual step ---------------------------------------------
Say 'Autologon (manual: you type the password; this script never sees it)'
Note '1. Download Autologon from Microsoft Sysinternals (learn.microsoft.com/sysinternals/downloads/autologon).'
Note "2. Run Autologon64.exe as administrator. Username: $User. Enter the password yourself."
Note '3. Click Enable. It stores the password as an LSA secret, not plain text.'
Note '4. Reboot once and confirm the desktop appears, then PlaygroundLogon runs about 30 s later.'
Note '   To undo: run Autologon again and click Disable.'
$global:LASTEXITCODE = 0   # do not leak manage-bde's code
Say 'Done'
