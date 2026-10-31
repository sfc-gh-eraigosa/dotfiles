<#
.SYNOPSIS
  One-time (idempotent) setup so the wslgiga1 Windows host recovers by itself
  after sign-in and reapplies the GPU power cap at boot. Run elevated.
.DESCRIPTION
  Easiest: double-click install-wslgiga1.cmd (same folder). It asks for UAC, runs
  this script with -ExecutionPolicy Bypass and keeps the window open so you can
  read the Autologon steps. By hand, from an elevated PowerShell:
    powershell.exe -ExecutionPolicy Bypass -File .\install-wslgiga1.ps1 [-WhatIf]

  - Registers PlaygroundLogon: at logon of the operator, limited (not elevated),
    short delay, runs playground-logon.ps1.
  - Registers PlaygroundGpuCapApply: at startup, SYSTEM, highest, runs
    playground-gpu-cap.ps1 from a locked copy in %ProgramData%\playground.
  - Reads both tasks back, and only THEN unregisters the hand-made
    PlaygroundDevStackAtLogon and PlaygroundGpuPowerCap. If anything fails, it
    prints which of the four tasks exist, and the old ones are still in place.
  - Deploys the desktop app's exe to the operator's state dir, only if found.
  - Prints the Autologon steps and whether C: is BitLocker-protected.

  WHO IS THE OPERATOR. A task for SYSTEM sees SYSTEM's %LOCALAPPDATA%, and an
  elevated shell started under a different admin account has that admin's
  $env:USERNAME. So the operator is not taken from the environment: it is -User
  if given, else the account signed in at the console (Win32_ComputerSystem). If
  nobody is at the console, -User is required. Its profile folder comes from the
  ProfileList registry key by SID, and that absolute path is written into the cap
  task's arguments.

  THE LOCKED FOLDER. SYSTEM runs a copy of the cap script from
  %ProgramData%\playground, so that folder must not be writable by a normal user
  (who could otherwise get SYSTEM to run code). The installer refuses a reparse
  point at the folder or the file, makes Administrators the owner, rebuilds the
  ACL from empty (inheritance off, only SYSTEM and Administrators full, Users
  read), deletes the old script before copying, then reads the ACL back and fails
  if any other principal can write, delete or change permissions. The SHA-256 of
  the deployed script is printed. The same folder holds the cap task's logs\.

  -WhatIf prints every action, changes nothing, and does not need elevation.
  Autologon's password is never read, passed or stored by this script.
.PARAMETER User
  Operator account (DOMAIN\name or name). Default: the console user.
.PARAMETER AppExe
  The desktop app's exe to deploy. Default: wslgiga1-status*.exe next to this
  script, else in the operator's %LOCALAPPDATA%\playground.
.PARAMETER LogonDelaySeconds
  Delay after logon before PlaygroundLogon runs (default 30, so the profile and network settle).
.EXAMPLE
  .\install-wslgiga1.cmd
.EXAMPLE
  powershell.exe -ExecutionPolicy Bypass -File .\install-wslgiga1.ps1 -WhatIf
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
$SidSystem = New-Object Security.Principal.SecurityIdentifier('S-1-5-18')
$SidAdmins = New-Object Security.Principal.SecurityIdentifier('S-1-5-32-544')
$SidUsers = New-Object Security.Principal.SecurityIdentifier('S-1-5-32-545')

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
           ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin -and -not $WhatIfPreference) {
    throw 'Run this from an elevated PowerShell (or use -WhatIf for a dry run). install-wslgiga1.cmd elevates for you.'
}
if (-not $isAdmin) { Say 'Not elevated: -WhatIf only, nothing will be changed' }

function Test-Reparse([string]$path) {
    $i = Get-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
    return [bool]($i -and ($i.Attributes -band [IO.FileAttributes]::ReparsePoint))
}

# --- who is the operator, and where is their state dir ----------------------
if (-not $User) {
    # Load the CIM module with -WhatIf off: under -WhatIf its alias setup would print noise.
    $savedWhatIf = $WhatIfPreference
    try { $WhatIfPreference = $false; Import-Module CimCmdlets -ErrorAction SilentlyContinue }
    finally { $WhatIfPreference = $savedWhatIf }
    try { $User = [string](Get-CimInstance Win32_ComputerSystem).UserName } catch { $User = '' }
    if (-not $User) { throw 'Nobody is signed in at the console, so the operator cannot be told from the elevated account. Pass -User DOMAIN\name.' }
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

# --- the locked folder (see .DESCRIPTION) -----------------------------------
$sysDir = Join-Path $env:ProgramData 'playground'
$capCopy = Join-Path $sysDir 'playground-gpu-cap.ps1'
$capSrc = Join-Path $scripts 'playground-gpu-cap.ps1'

function New-LockedAcl {
    $acl = New-Object Security.AccessControl.DirectorySecurity
    $acl.SetOwner($SidAdmins)
    $acl.SetAccessRuleProtection($true, $false)
    $inh = [Security.AccessControl.InheritanceFlags]'ContainerInherit,ObjectInherit'
    $none = [Security.AccessControl.PropagationFlags]::None
    $allow = [Security.AccessControl.AccessControlType]::Allow
    $FR = [Security.AccessControl.FileSystemRights]
    $acl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($SidSystem, $FR::FullControl, $inh, $none, $allow)))
    $acl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($SidAdmins, $FR::FullControl, $inh, $none, $allow)))
    $acl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($SidUsers, $FR::ReadAndExecute, $inh, $none, $allow)))
    return $acl
}

# Assert-LockedAcl: throw if any principal but SYSTEM/Administrators holds a right that lets it change the folder, its files, or their ACL/owner.
function Assert-LockedAcl([string]$path) {
    $acl = Get-Acl -LiteralPath $path
    $owner = $acl.GetOwner([Security.Principal.SecurityIdentifier])
    if ($owner -ne $SidAdmins -and $owner -ne $SidSystem) { throw "$path is owned by $owner, not Administrators" }
    $bad = [Security.AccessControl.FileSystemRights]'WriteData,AppendData,WriteExtendedAttributes,WriteAttributes,Delete,DeleteSubdirectoriesAndFiles,ChangePermissions,TakeOwnership'
    foreach ($r in $acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])) {
        if ($r.AccessControlType -ne 'Allow') { continue }
        if ($r.IdentityReference -eq $SidSystem -or $r.IdentityReference -eq $SidAdmins) { continue }
        if ($r.FileSystemRights -band $bad) { throw "$path lets $($r.IdentityReference) write ($($r.FileSystemRights)); refusing" }
    }
}

# Lock-Folder: create/repair <dir> so only SYSTEM and Administrators can write.
function Lock-Folder([string]$dir) {
    if (Test-Path -LiteralPath $dir) {
        if (Test-Reparse $dir) { throw "$dir is a reparse point (junction or symlink); refusing. Remove it and re-run." }
    } else { $null = New-Item -ItemType Directory -Path $dir }
    # Take ownership of everything below, then drop all explicit ACEs and inherit from the rebuilt parent.
    & icacls.exe $dir /setowner '*S-1-5-32-544' /T /C /L | Out-Null
    Set-Acl -LiteralPath $dir -AclObject (New-LockedAcl)
    & icacls.exe $dir /reset /T /C /L | Out-Null
    Assert-LockedAcl $dir
}

# --- state: plan the mutating steps; every one goes through ShouldProcess ----
function Show-Tasks {
    foreach ($t in @($LogonTask, $CapTask) + $OldTasks) {
        $e = if (Get-ScheduledTask -TaskName $t -ErrorAction SilentlyContinue) { 'EXISTS' } else { 'absent' }
        Write-Host ("    {0,-28} {1}" -f $t, $e)
    }
}

try {
    # 1. locked folder + script copy
    if ($PSCmdlet.ShouldProcess($sysDir, 'Create or repair the locked folder (owner Administrators, ACL rebuilt: SYSTEM/Administrators full, Users read) and copy playground-gpu-cap.ps1')) {
        Lock-Folder $sysDir
        Lock-Folder (Join-Path $sysDir 'logs')
        if (Test-Reparse $capCopy) { throw "$capCopy is a reparse point; refusing." }
        if (Test-Path -LiteralPath $capCopy) { Remove-Item -LiteralPath $capCopy -Force }   # no write through a planted hardlink
        Copy-Item -LiteralPath $capSrc -Destination $capCopy
        Assert-LockedAcl $capCopy
        Note ("deployed {0} sha256 {1}" -f $capCopy, (Get-FileHash -LiteralPath $capCopy -Algorithm SHA256).Hash)
    }

    # 2. the operator's state dir (may not exist yet)
    if ($PSCmdlet.ShouldProcess($stateDir, 'Create folder')) {
        $null = New-Item -ItemType Directory -Force -Path $stateDir
    }

    # 3. PlaygroundLogon
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
    }

    # 4. PlaygroundGpuCapApply
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
    }

    # 5. read both back; only then retire the old tasks
    if ($PSCmdlet.ShouldProcess("$LogonTask, $CapTask", 'Verify by reading both tasks back')) {
        $l = Get-ScheduledTask -TaskName $LogonTask -ErrorAction Stop
        $c = Get-ScheduledTask -TaskName $CapTask -ErrorAction Stop
        if ($l.Principal.RunLevel -ne 'Limited') { throw "$LogonTask is not Limited (got $($l.Principal.RunLevel))" }
        if ($l.Triggers[0].Delay -ne ('PT{0}S' -f $LogonDelaySeconds)) { throw "$LogonTask delay is '$($l.Triggers[0].Delay)'" }
        if ($c.Principal.UserId -notmatch 'SYSTEM' -or $c.Principal.RunLevel -ne 'Highest') { throw "$CapTask is not SYSTEM/Highest" }
        if ($c.Actions[0].Arguments -notmatch [regex]::Escape($stateDir)) { throw "$CapTask does not carry the operator state dir" }
        Note "verified $LogonTask and $CapTask"
    }
    foreach ($t in $OldTasks) {
        if (Get-ScheduledTask -TaskName $t -ErrorAction SilentlyContinue) {
            if ($PSCmdlet.ShouldProcess($t, 'Unregister old scheduled task (after the new ones verified)')) {
                Unregister-ScheduledTask -TaskName $t -Confirm:$false
                Note "removed $t"
            }
        } else { Note "$t not present" }
    }
} catch {
    Write-Host "INSTALL FAILED: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host 'Tasks now (old tasks are removed only after the new ones verify):'
    Show-Tasks
    Write-Host 'Fix the cause and re-run; the installer is idempotent.'
    throw
}

# --- the desktop app, only if we can find it ---------------------------------
$appDest = Join-Path $stateDir 'wslgiga1-status.exe'
if (-not $AppExe) {
    foreach ($dir in $scripts, $stateDir) {
        $hit = Get-ChildItem -LiteralPath $dir -Filter 'wslgiga1-status*.exe' -File -ErrorAction SilentlyContinue |
            Sort-Object LastWriteTime -Descending | Select-Object -First 1
        if ($hit) { $AppExe = $hit.FullName; break }
    }
}
if ($AppExe -and (Test-Path -LiteralPath $AppExe -PathType Leaf)) {
    if ($AppExe -ieq $appDest) { Note "app already in place: $appDest" }
    elseif ($PSCmdlet.ShouldProcess($appDest, "Copy desktop app from $AppExe")) {
        try { Copy-Item -LiteralPath $AppExe -Destination $appDest -Force; Note "app deployed to $appDest" }
        catch { Write-Warning "Could not copy the app (is it running?): $($_.Exception.Message)" }
    }
} else {
    Write-Warning "Desktop app not found. Put the packaged exe at $appDest (or beside this script as wslgiga1-status*.exe, or pass -AppExe) and re-run; PlaygroundLogon skips the app until then."
}

# --- BitLocker ---------------------------------------------------------------
Say 'BitLocker on C:'
$bde = (& manage-bde.exe -status C: 2>&1) -join "`n"
if ($bde -match 'Protection Status:\s*(.+)') {
    $ps = $Matches[1].Trim()
    if ($ps -match 'On') { Note 'C: is BitLocker-protected (Protection On).' }
    else { Write-Warning "C: is NOT BitLocker-protected ($ps). Autologon stores the password on this disk; anyone with the disk can read it." }
} else {
    Write-Warning 'Could not read BitLocker status (manage-bde needs elevation). Check it from an elevated shell: manage-bde -status C:'
}

# --- Autologon: a manual step ------------------------------------------------
Say 'Autologon (manual: you type the password; this script never sees it)'
Note '1. Download Autologon from Microsoft Sysinternals (learn.microsoft.com/sysinternals/downloads/autologon).'
Note "2. Run Autologon64.exe as administrator. Username: $User. Enter the password yourself."
Note '3. Click Enable. It stores the password as an LSA secret, not plain text.'
Note '4. Reboot once and confirm the desktop appears, then PlaygroundLogon runs about 30 s later.'
Note '   To undo: run Autologon again and click Disable.'
$global:LASTEXITCODE = 0   # do not leak manage-bde's code
Say 'Done'
