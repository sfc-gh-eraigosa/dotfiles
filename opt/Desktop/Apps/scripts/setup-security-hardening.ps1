<#
.SYNOPSIS
  Opt-in Windows security hardening: closes the weekly security audit's two
  visibility gaps and turns on a conservative set of Defender ASR rules in
  AUDIT MODE ONLY. Self-elevating (one UAC prompt), idempotent, and precisely
  reversible with -Uninstall.
  Opt-in: gated at the install.sh call site by gff flag
  install.windows.security-hardening (boolDefault: false, fail-closed).

.DESCRIPTION
  Follow-up to the security-audit objective (PR #225). That shipped DETECTION;
  this makes the detection better and trims two attack surfaces. Three actions,
  each individually idempotent, read-back verified, and reported by -Status:

    1. Add the invoking user to the local 'Event Log Readers' group.
       The audit collector runs NON-elevated, so its Security-log lenses report
       '## ADMIN-REQUIRED' every week (1102 log-clear, 4720 new-user, 4624/4625
       logons, 4728/4732 group-add). Membership lights them up with no collector
       change. The group is resolved by well-known SID S-1-5-32-573 because the
       NAME is localized on non-English Windows and the SID is not.

    2. Enable the Microsoft-Windows-TaskScheduler/Operational event channel.
       It is disabled by default, so task-CREATION forensics simply do not exist;
       the audit can only see current task state, never the moment one appeared.

    3. Configure five Defender ASR rules in AuditMode (NEVER Block).
       This is a developer machine (Docker/WSL/AutoHotkey/games), so enforcement
       is a separate, later decision taken only after the weekly audit shows a
       clean audit-mode window. Audit-mode hits surface as events 1122/1125/
       1132/1134. Uses Add-MpPreference (additive) so a rule the user already
       configured is never clobbered - it is skipped and reported instead.

  DETECTION-FRIENDLY BY DESIGN: this script does not touch the Claude scheduled
  task or anything under %USERPROFILE%\Claude\. The weekly audit notices the
  resulting posture change on its own (section K's admin-gap disappearing, D2
  showing the channel enabled, C1 showing ASRrules(count) = 5) and folds it into
  its baseline with user confirmation.

  REVERSIBILITY: a state file (%ProgramData%\dotfiles\security-hardening.state.json)
  records which changes THIS script actually made, so -Uninstall never disables a
  channel that was already on, never removes a pre-existing group membership, and
  never removes an ASR rule the user has since promoted to Block.

.PARAMETER Status
  Report the current state of all three actions and exit. READ-ONLY and runs
  WITHOUT elevation (it never self-elevates).

.PARAMETER Uninstall
  Revert exactly the changes recorded in the state file, then remove the file.
  Self-elevates. With no state file it reports that and changes nothing.

.PARAMETER TargetUserSid
  INTERNAL. The SID of the user to add to Event Log Readers, captured by the
  non-elevated parent and passed across the UAC boundary. Necessary because
  Start-Process -Verb RunAs can be satisfied by a DIFFERENT admin account, so an
  elevated child that trusted $env:USERNAME could add the wrong principal.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File setup-security-hardening.ps1
  powershell -ExecutionPolicy Bypass -File setup-security-hardening.ps1 -Status
  powershell -ExecutionPolicy Bypass -File setup-security-hardening.ps1 -Uninstall
#>
param(
    [switch]$Status,
    [switch]$Uninstall,
    [string]$TargetUserSid
)
$ErrorActionPreference = 'Stop'

# --- constants --------------------------------------------------------------

# ASR rule GUIDs transcribed from the official Microsoft Learn reference:
# https://learn.microsoft.com/defender-endpoint/attack-surface-reduction-rules-overview#asr-rules
# The rule NAME travels with the GUID so a reviewer can re-verify each pair
# against that table without trusting this file. AUDIT MODE ONLY (see .DESCRIPTION).
# Deliberately excluded on a dev machine: PSExec/WMI (Docker + WSL tooling use
# WMI), untrusted-USB, and the prevalence/age rule (hostile to locally-built
# binaries). Add those later from the weekly audit's evidence, not preemptively.
$ASR_RULES = [ordered]@{
    '56a863a9-875e-4185-98a7-b882c64b5ce5' = 'Block abuse of exploited vulnerable signed drivers'
    '9e6c4e1f-7d60-472f-ba1a-a39ef669e4b2' = 'Block credential stealing from the Windows LSASS'
    'd4f940ab-401b-4efc-aadc-ad5f3c50688a' = 'Block all Office applications from creating child processes'
    '3b576869-a4ec-4529-8536-b80a7769e899' = 'Block Office applications from creating executable content'
    '5beb7efe-fd9a-4556-801d-275e5ffc04cc' = 'Block execution of potentially obfuscated scripts'
}

$ELR_SID    = 'S-1-5-32-573'                                  # BUILTIN\Event Log Readers
$TS_CHANNEL = 'Microsoft-Windows-TaskScheduler/Operational'
$STATE_DIR  = Join-Path $env:ProgramData 'dotfiles'
$STATE_FILE = Join-Path $STATE_DIR 'security-hardening.state.json'
$LOG_FILE   = Join-Path $STATE_DIR 'security-hardening.log'

$script:failures = 0

# --- helpers ----------------------------------------------------------------

# Say - console AND log. The elevated child owns a window that closes the moment
# it exits, so without a log its output would be invisible; the parent prints
# this file after -Wait. Logging is best-effort: never let it break an action.
function Say([string]$msg) {
    Write-Host $msg
    try {
        if (Test-Path $STATE_DIR) { Add-Content -LiteralPath $LOG_FILE -Value $msg -Encoding UTF8 }
    } catch { }
}
function Warn([string]$msg) { Say "WARNING: $msg"; $script:failures++ }

function Test-IsAdmin {
    try {
        return ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
               ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    } catch { return $false }
}

# Normalize the ASR action to a stable label. Get-MpPreference returns an enum
# that stringifies differently across platform versions ('2' vs 'AuditMode'),
# and the docs define 0/Disabled, 1/Enabled(Block), 2/AuditMode, 5/NotConfigured,
# 6/Warn - so map every spelling onto one word.
function Format-AsrAction($raw) {
    switch ("$raw") {
        '0'             { 'Disabled' }
        'Disabled'      { 'Disabled' }
        '1'             { 'Block' }
        'Enabled'       { 'Block' }
        'Block'         { 'Block' }
        '2'             { 'AuditMode' }
        'Audit'         { 'AuditMode' }
        'AuditMode'     { 'AuditMode' }
        '5'             { 'NotConfigured' }
        'NotConfigured' { 'NotConfigured' }
        '6'             { 'Warn' }
        'Warn'          { 'Warn' }
        default         { "Unknown($raw)" }
    }
}

# Current ASR configuration as guid -> normalized action.
function Get-AsrMap {
    $map = @{}
    try {
        $p    = Get-MpPreference
        $ids  = @($p.AttackSurfaceReductionRules_Ids)
        $acts = @($p.AttackSurfaceReductionRules_Actions)
        for ($i = 0; $i -lt $ids.Count; $i++) {
            if ($ids[$i]) { $map[([string]$ids[$i]).ToLowerInvariant()] = (Format-AsrAction $acts[$i]) }
        }
    } catch { }
    return $map
}

function Get-ChannelEnabled {
    try { return [bool](Get-WinEvent -ListLog $TS_CHANNEL -ErrorAction Stop).IsEnabled } catch { return $null }
}

# Set the channel's enabled state via wevtutil. NOTE: wevtutil writes to stderr
# on failure, which under $ErrorActionPreference='Stop' becomes a terminating
# NativeCommandError - the exact trap that bit `net localgroup` in the audit
# collector. Drop the preference locally, then check $LASTEXITCODE and verify.
function Set-ChannelEnabled([bool]$enabled) {
    $flag = if ($enabled) { '/e:true' } else { '/e:false' }
    $eap = $ErrorActionPreference
    $ErrorActionPreference = 'SilentlyContinue'
    $out = & wevtutil.exe sl "$TS_CHANNEL" $flag 2>&1
    $rc  = $LASTEXITCODE
    $ErrorActionPreference = $eap
    if ($rc -ne 0) { Warn "wevtutil sl $TS_CHANNEL $flag exited $rc : $(($out | Out-String).Trim())" ; return $false }
    return $true
}

function Test-ElrMember([string]$sid) {
    try {
        $members = @(Get-LocalGroupMember -SID $ELR_SID -ErrorAction Stop)
        foreach ($m in $members) { if ("$($m.SID)" -eq $sid) { return $true } }
        return $false
    } catch {
        # An unresolvable member SID makes Get-LocalGroupMember throw outright.
        # Treat as "unknown" rather than "not a member" so we never blindly re-add.
        return $null
    }
}

function Read-State {
    if (-not (Test-Path -LiteralPath $STATE_FILE)) { return $null }
    try { return Get-Content -LiteralPath $STATE_FILE -Raw | ConvertFrom-Json } catch { return $null }
}

# --- -Status: read-only, never elevates -------------------------------------

if ($Status) {
    $me = [Security.Principal.WindowsIdentity]::GetCurrent()
    Write-Host "Elevated : $(Test-IsAdmin)"
    Write-Host "User     : $($me.Name)  [$($me.User.Value)]"
    Write-Host ""

    $grp = Get-LocalGroup -SID $ELR_SID -ErrorAction SilentlyContinue
    $grpName = if ($grp) { $grp.Name } else { 'Event Log Readers (unresolved)' }
    $isMember = Test-ElrMember $me.User.Value
    $memberTxt = if ($null -eq $isMember) { 'UNKNOWN (group unreadable)' } elseif ($isMember) { 'MEMBER' } else { 'not a member' }
    Write-Host "[1] $grpName : $memberTxt"
    if ($isMember) { Write-Host "      (applies to the access token at LOGON - effective after the next sign-in)" }

    $chan = Get-ChannelEnabled
    $chanTxt = if ($null -eq $chan) { 'channel not found' } elseif ($chan) { 'ENABLED' } else { 'disabled' }
    Write-Host "[2] $TS_CHANNEL : $chanTxt"

    $map = Get-AsrMap
    $inAudit = @($ASR_RULES.Keys | Where-Object { $map[$_.ToLowerInvariant()] -eq 'AuditMode' })
    Write-Host "[3] Defender ASR (audit mode) : $($inAudit.Count)/$($ASR_RULES.Count) in AuditMode"
    foreach ($guid in $ASR_RULES.Keys) {
        $cur = $map[$guid.ToLowerInvariant()]
        if (-not $cur) { $cur = 'not configured' }
        Write-Host ("      {0}  {1,-13} {2}" -f $guid, $cur, $ASR_RULES[$guid])
    }

    Write-Host ""
    $st = Read-State
    if ($st) {
        Write-Host "State    : $STATE_FILE (applied $($st.appliedUtc) UTC)"
        Write-Host "           tracked -> group=$($st.eventLogReadersAdded) channel=$($st.taskSchedulerChannelEnabled) asr=$(@($st.asrRulesAdded).Count) rule(s)"
    } else {
        Write-Host "State    : none at $STATE_FILE (nothing tracked as applied by this script)"
    }
    return
}

# --- self-elevate (install + uninstall only) --------------------------------

if (-not (Test-IsAdmin)) {
    # Capture the REAL invoking user before crossing the UAC boundary (see
    # .PARAMETER TargetUserSid), and resolve a LOCAL script path: when launched
    # from WSL interop, $PSCommandPath is a \\wsl.localhost\... UNC share that an
    # elevated process runs in a different security context and cannot read - the
    # failure mode that makes UAC look like it "did nothing" (see the same guard
    # in setup-autostart.ps1). Pin -WorkingDirectory locally for the same reason.
    $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    $selfDeployed = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Apps\scripts\setup-security-hardening.ps1'
    $selfLocal = if ($PSCommandPath -like '\\*' -and (Test-Path $selfDeployed)) { $selfDeployed } else { $PSCommandPath }

    $argList = @('-NoProfile','-ExecutionPolicy','Bypass','-File',"`"$selfLocal`"",'-TargetUserSid',$sid)
    if ($Uninstall) { $argList += '-Uninstall' }

    Write-Host "Elevating (one UAC prompt)..."
    try {
        $proc = Start-Process powershell -Verb RunAs -Wait -PassThru `
                    -WorkingDirectory $env:SystemRoot -ArgumentList $argList
    } catch {
        Write-Host "FAILED: elevation was declined or could not start ($($_.Exception.Message))"
        exit 1
    }
    # The elevated window is gone; surface what it did.
    if (Test-Path -LiteralPath $LOG_FILE) {
        Write-Host "--- elevated run output ($LOG_FILE) ---"
        Get-Content -LiteralPath $LOG_FILE -Tail 60 | ForEach-Object { Write-Host $_ }
    }
    exit $proc.ExitCode
}

# --- elevated from here -----------------------------------------------------

New-Item -ItemType Directory -Force -Path $STATE_DIR | Out-Null
"" | Out-File -LiteralPath $LOG_FILE -Encoding UTF8    # fresh log per run
Say "=== setup-security-hardening $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') ==="

if (-not $TargetUserSid) {
    # Run directly from an elevated shell: the current user IS the target.
    $TargetUserSid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
}
Say "Target user SID: $TargetUserSid"

if ($Uninstall) {
    $st = Read-State
    if (-not $st) {
        Say "No state file at $STATE_FILE - this script has no recorded changes to revert."
        Say "Nothing changed. (Refusing to guess: an untracked revert could disable a channel you enabled yourself.)"
        exit 0
    }

    # 1. group membership - only if WE added it, and for the user we ACTUALLY
    #    added. The state file is authoritative here, not the invoking account:
    #    an uninstall run by a different admin must still revert the original
    #    principal, never strip the wrong one.
    $revertSid = if ($st.targetUserSid) { [string]$st.targetUserSid } else { $TargetUserSid }
    if ($revertSid -ne $TargetUserSid) { Say "[1] reverting the recorded install user ($revertSid), not the invoking one" }
    if ($st.eventLogReadersAdded) {
        $isMember = Test-ElrMember $revertSid
        if ($isMember) {
            try {
                Remove-LocalGroupMember -SID $ELR_SID -Member ([System.Security.Principal.SecurityIdentifier]$revertSid) -ErrorAction Stop
                Say "[1] removed $revertSid from Event Log Readers (takes effect at next logon)"
            } catch { Warn "[1] could not remove group membership: $($_.Exception.Message)" }
        } else { Say "[1] $revertSid is not a member - nothing to remove" }
    } else { Say "[1] not added by this script - left alone" }

    # 2. channel - only if WE enabled it
    if ($st.taskSchedulerChannelEnabled) {
        if (Set-ChannelEnabled $false) {
            if ((Get-ChannelEnabled) -eq $false) { Say "[2] disabled $TS_CHANNEL" }
            else { Warn "[2] channel still reports enabled after disable" }
        }
    } else { Say "[2] not enabled by this script - left alone" }

    # 3. ASR - only OUR guids, and never one the user promoted to Block
    $map = Get-AsrMap
    $removed = 0; $keptPromoted = 0
    foreach ($guid in @($st.asrRulesAdded)) {
        if (-not $guid) { continue }
        $cur = $map[([string]$guid).ToLowerInvariant()]
        if (-not $cur) { Say "[3] $guid already absent"; continue }
        if ($cur -ne 'AuditMode') {
            Say "[3] $guid is now '$cur' (promoted since install) - KEPT, remove manually if unintended"
            $keptPromoted++
            continue
        }
        try {
            Remove-MpPreference -AttackSurfaceReductionRules_Ids $guid `
                                -AttackSurfaceReductionRules_Actions AuditMode -ErrorAction Stop
            $removed++
        } catch { Warn "[3] could not remove $guid : $($_.Exception.Message)" }
    }
    Say "[3] removed $removed ASR rule(s); kept $keptPromoted promoted rule(s)"

    Remove-Item -LiteralPath $STATE_FILE -Force -ErrorAction SilentlyContinue
    Say "Removed state file. Uninstall complete."
    Say "NOTE: the security-audit collector keeps running - this only reverts hardening."
    exit ([int]($script:failures -gt 0))
}

# --- install ----------------------------------------------------------------

$state = [ordered]@{
    version                     = 1
    appliedUtc                  = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    targetUserSid               = $TargetUserSid
    eventLogReadersAdded        = $false
    taskSchedulerChannelEnabled = $false
    asrRulesAdded               = @()
}

# --- 1. Event Log Readers ---------------------------------------------------
$grp = Get-LocalGroup -SID $ELR_SID -ErrorAction SilentlyContinue
if (-not $grp) {
    Warn "[1] group SID $ELR_SID (Event Log Readers) not found on this system - skipped"
} else {
    $isMember = Test-ElrMember $TargetUserSid
    if ($null -eq $isMember) {
        Warn "[1] could not read '$($grp.Name)' membership - skipped rather than risk a duplicate add"
    } elseif ($isMember) {
        Say "[1] already a member of '$($grp.Name)' - no change"
    } else {
        try {
            Add-LocalGroupMember -SID $ELR_SID -Member ([System.Security.Principal.SecurityIdentifier]$TargetUserSid) -ErrorAction Stop
            if (Test-ElrMember $TargetUserSid) {
                $state.eventLogReadersAdded = $true
                Say "[1] added the user to '$($grp.Name)'"
            } else {
                Warn "[1] add reported success but membership did not read back"
            }
        } catch { Warn "[1] could not add to '$($grp.Name)': $($_.Exception.Message)" }
    }
}

# --- 2. TaskScheduler/Operational channel -----------------------------------
$chan = Get-ChannelEnabled
if ($null -eq $chan) {
    Warn "[2] channel $TS_CHANNEL not found - skipped"
} elseif ($chan) {
    Say "[2] $TS_CHANNEL already enabled - no change"
} else {
    if (Set-ChannelEnabled $true) {
        if ((Get-ChannelEnabled) -eq $true) {
            $state.taskSchedulerChannelEnabled = $true
            Say "[2] enabled $TS_CHANNEL"
        } else {
            Warn "[2] wevtutil succeeded but the channel still reads disabled"
        }
    }
}

# --- 3. Defender ASR rules in AUDIT MODE ------------------------------------
$map = Get-AsrMap
$toAdd = @()
foreach ($guid in $ASR_RULES.Keys) {
    $cur = $map[$guid.ToLowerInvariant()]
    if ($cur) {
        # Present already: never clobber a mode the user chose (R3).
        Say "[3] $guid already configured as '$cur' - left unchanged ($($ASR_RULES[$guid]))"
    } else {
        $toAdd += $guid
    }
}
if (-not $toAdd.Count) {
    Say "[3] all $($ASR_RULES.Count) rules already configured - no change"
} else {
    try {
        # Add-MpPreference (NOT Set-) so existing rules outside our list survive.
        Add-MpPreference -AttackSurfaceReductionRules_Ids $toAdd `
                         -AttackSurfaceReductionRules_Actions (@('AuditMode') * $toAdd.Count) -ErrorAction Stop
    } catch { Warn "[3] Add-MpPreference failed: $($_.Exception.Message)" }

    # Read back: Microsoft documents that protected writes can "appear to succeed
    # but are actually blocked", so never trust the call - verify each rule.
    $after = Get-AsrMap
    foreach ($guid in $toAdd) {
        $cur = $after[$guid.ToLowerInvariant()]
        if ($cur -eq 'AuditMode') {
            $state.asrRulesAdded += $guid
            Say "[3] AuditMode: $guid  $($ASR_RULES[$guid])"
        } else {
            Warn "[3] $guid did not take effect (reads '$(if ($cur) { $cur } else { 'absent' })')"
        }
    }
}

# --- persist state + closing guidance ---------------------------------------
try {
    ($state | ConvertTo-Json -Depth 4) | Set-Content -LiteralPath $STATE_FILE -Encoding UTF8
    Say "State written: $STATE_FILE"
} catch { Warn "could not write the state file: $($_.Exception.Message)" }

Say ""
Say "Done ($script:failures warning(s))."
if ($state.eventLogReadersAdded) {
    Say "IMPORTANT: group membership is applied to your access token at LOGON."
    Say "           Sign out and back in (or reboot) before the audit's Security-log"
    Say "           lenses (section K) start reporting data instead of ADMIN-REQUIRED."
}
Say "ASR rules are in AUDIT MODE ONLY - nothing is blocked. Review the weekly audit,"
Say "then promote to Block deliberately (see docs/security-hardening.md)."
Say "Check state anytime: setup-security-hardening.ps1 -Status"

exit ([int]($script:failures -gt 0))
