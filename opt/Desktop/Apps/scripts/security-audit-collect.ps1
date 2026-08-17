<#
.SYNOPSIS
  Read-only weekly Windows security-audit collector (v2). Snapshots ~40 anomaly
  lenses across persistence, defense-evasion, network, process, account and
  posture surfaces, and writes stable, diff-friendly text that the Claude
  "weekly-security-audit" task analyzes UNATTENDED (no computer-use, no admin,
  no approvals). Detection only - it never changes configuration.

.DESCRIPTION
  Every lens runs through Add-Section and resolves to exactly one visible state,
  so the analysis run can never mistake a broken/blocked lens for a clean host:
    <data>                     the lens ran and found items
    (none)                     the lens ran and found nothing
    !! COLLECTION ERROR: ..    the lens itself threw (report, do NOT read as clean)
    ## ADMIN-REQUIRED (<Type>) the data needs elevation we do not have (honest gap)
  Admin-gap markers are byte-stable (exception TYPE only, never the localized
  .Message) so they diff cleanly week to week.

  High-churn surfaces are split into a [STABLE] block (version-normalized, the
  strict-diff surface) and a [VOLATILE] block (ephemeral data shown for triage
  but never alarmed on by absence/change alone).

.PARAMETER BaseDir
  Output root (default %USERPROFILE%\Claude\SecurityAudit). Tests pass a temp dir.
.PARAMETER Days
  Event-log look-back window in days (default 7).
.PARAMETER StdOut
  Emit the report to stdout and skip ALL file writes (for tests / ad-hoc runs).

.PARAMETER OncePerDay
  Exit immediately (rc 0) when a report for TODAY already exists. This is what
  makes hourly polling cheap: the scheduled task fires every hour in the evening
  window so a machine that was asleep still gets a run, but only the FIRST fire
  of the day does real work. Measured on the reference host: a full collection is
  ~28s wall / ~19s CPU, this gate is ~4ms - so 7 of the 8 daily fires cost
  essentially nothing.

.PARAMETER LowPriority
  Drop the process to BelowNormal priority so a background collection never
  competes with foreground work. The scheduled task passes this; manual runs
  should not (you want your ad-hoc run to finish promptly).
#>
param(
    [string]$BaseDir = (Join-Path $env:USERPROFILE 'Claude\SecurityAudit'),
    [int]$Days = 7,
    [switch]$StdOut,
    [switch]$OncePerDay,
    [switch]$LowPriority,
    # Test-only: append one deliberately-throwing lens so the driver can prove the
    # throw -> '!! COLLECTION ERROR' contract, RC still 0, later sections render.
    [switch]$FaultInject
)

# 'Stop' so a failing cmdlet raises a catchable terminating error that
# Add-Section turns into a visible marker. Under 'Continue' the error goes to a
# stderr nobody reads and the section is written EMPTY - indistinguishable from
# "nothing suspicious found".
$ErrorActionPreference = 'Stop'
$COLLECTOR_VERSION = '3'
$SELF_PATH = $MyInvocation.MyCommand.Path
$WINDOW    = (Get-Date).AddDays(-[Math]::Abs($Days))

# --- cheap once-per-day gate (must run BEFORE any lens) ---------------------
# The evening schedule fires hourly so an off/asleep machine still gets a daily
# collection, but re-collecting every hour would burn ~19s CPU x 8. Gate on the
# canonical report's write date: that file IS the artifact the analysis reads, so
# "written today" is the true 'we already have today's data' signal. A run that
# died before writing it leaves the gate open, so the next hour retries - the
# self-healing behavior we want.
if ($OncePerDay) {
    $__canonical = Join-Path $BaseDir 'latest-audit.txt'
    if (Test-Path -LiteralPath $__canonical) {
        try {
            if ((Get-Item -LiteralPath $__canonical).LastWriteTime.Date -eq (Get-Date).Date) {
                Write-Host "SKIP: today's report already collected ($__canonical). -OncePerDay gate."
                exit 0
            }
        } catch { }   # unreadable stat => fall through and collect
    }
}

# Background collection must never compete with foreground work.
if ($LowPriority) {
    try { [Diagnostics.Process]::GetCurrentProcess().PriorityClass = 'BelowNormal' } catch { }
}

# ---- helpers ---------------------------------------------------------------
$script:sections = @()

# Add-Section - run one lens; record its outcome unambiguously (see .DESCRIPTION).
function Add-Section {
    param([Parameter(Mandatory)][string]$Title,[Parameter(Mandatory)][scriptblock]$Body)
    $script:sections += "`r`n=== $Title ==="
    try {
        $out = (& $Body | Out-String -Width 400).TrimEnd()
        if ([string]::IsNullOrWhiteSpace($out)) { $out = '(none)' }
        $script:sections += $out
    } catch {
        $script:sections += "!! COLLECTION ERROR: $($_.Exception.Message)"
    }
}

# Fixed-text admin-gap marker (byte-stable: type name only, no localized message).
function Deny-Marker([string]$what,[System.Exception]$e) {
    "## ADMIN-REQUIRED ($($e.GetType().Name)): $what"
}

# Collapse version-ish path/name churn so updaters don't diff every week:
#   \app-1.2.3\  1.2.3.4  \127.0.6533.100\  ->  X   (single canonical token)
# IPv4 literals are PRESERVED: a hard-coded C2/exfil IP in a service or task
# command must still diff, so a dotted run that is a valid 4-octet IPv4 (every
# octet 0-255) is left intact; version strings (e.g. 148.0.7778.1018, octet
# >255) are still collapsed.
function Norm-Ver([string]$s) {
    if ($null -eq $s) { return '' }
    $ipv4 = '^(25[0-5]|2[0-4][0-9]|1?[0-9]?[0-9])(\.(25[0-5]|2[0-4][0-9]|1?[0-9]?[0-9])){3}$'
    $s = [regex]::Replace($s, '\d+(\.\d+){2,}', { param($m) if ($m.Value -match $ipv4) { $m.Value } else { 'X.X' } })
    $s = $s -replace '(?<=[\\_\-])\d{2,}(\.\d+)*(?=[\\_\-]|$)', 'X'  # \12345\ build dirs
    return $s
}

# Cached Authenticode status by path (sig checks are ~ms each; dedupe them).
$script:SIGCACHE = @{}
function Sig-Status([string]$path) {
    if (-not $path) { return 'NoPath' }
    if ($script:SIGCACHE.ContainsKey($path)) { return $script:SIGCACHE[$path] }
    $st = 'Unchecked'
    try { if (Test-Path -LiteralPath $path) { $st = (Get-AuthenticodeSignature -LiteralPath $path).Status.ToString() } else { $st = 'PathMissing' } } catch { $st = 'SigError' }
    $script:SIGCACHE[$path] = $st; return $st
}

# Pull the first quoted-or-bare .exe/.sys/.dll out of a service/command string.
# The final fallback allows SPACES before the extension, so an UNQUOTED service
# path with spaces (itself a red flag) still yields an image to sig-check rather
# than $null - stopping just before a trailing " -flag" / " /flag" argument.
function Image-Of([string]$cmd) {
    if (-not $cmd) { return $null }
    if ($cmd -match '^"([^"]+)"') { return $Matches[1] }
    if ($cmd -match '^\s*([A-Za-z]:\\[^\s,]+?\.(?:exe|sys|dll|com|bat|cmd|ps1|scr))') { return $Matches[1] }
    if ($cmd -match '([A-Za-z]:\\[^\s,]+?\.(?:exe|sys|dll|com|bat|cmd|ps1|scr))') { return $Matches[1] }
    if ($cmd -match '^\s*([A-Za-z]:\\.+?\.(?:exe|sys|dll|com|bat|cmd|ps1|scr))(?:\s+[-/]|$)') { return $Matches[1] }
    return $null
}

# User-writable / suspicious image locations (case-insensitive regex fragments).
# Broad on purpose: the whole per-user profile is writable (AppData\Local +
# LocalLow, Documents, Desktop, %LOCALAPPDATA%\Programs - where a large share of
# real per-user malware and infostealers land), so a substring match on
# \users\<name>\ catches it; Program Files / WindowsApps sit outside the profile
# and are excluded. (Alarm severity is still gated on signature elsewhere, so
# broad coverage here does not by itself create noise.)
$USERWRITABLE = 'appdata\\roaming','appdata\\local','appdata\\locallow',
                '\\temp\\','\\downloads\\','\\users\\public\\','\\programdata\\',
                '\\\$recycle','\\windows\\temp\\',
                '\\users\\[^\\]+\\(documents|desktop|downloads|music|pictures|videos|favorites)\\'
function Is-UserWritable([string]$p) {
    if (-not $p) { return $false }
    $l = $p.ToLower()
    foreach ($u in $USERWRITABLE) { if ($l -match $u) { return $true } }
    return $false
}

# Probe-first event read: returns @{state='ok'|'denied'|'empty'|'error'; events=@(); err=} .
# Going straight to -FilterHashtable on a DENIED log throws a generic
# "No events were found" (NOT UnauthorizedAccessException) - so probe the log
# with -MaxEvents 1 first to tell "denied" from "readable but quiet".
# "No matching events" is a TERMINATING error with no distinct exception type;
# key on the locale-INDEPENDENT FullyQualifiedErrorId, not the localized message
# (the English 'No events were found' string breaks on non-English Windows and
# would misreport a clean, readable log as a broken lens).
function Test-NoEvents($errRec) {
    return ($errRec.FullyQualifiedErrorId -match 'NoMatchingEventsFound')
}
function Read-Events([string]$log,[int[]]$ids,[int]$max=200) {
    try { Get-WinEvent -LogName $log -MaxEvents 1 -ErrorAction Stop | Out-Null }
    catch [System.UnauthorizedAccessException] { return @{ state='denied'; events=@(); err=$_.Exception } }
    catch {
        if (Test-NoEvents $_) { return @{ state='empty'; events=@() } }
        return @{ state='error'; events=@(); err=$_.Exception }
    }
    $filter = @{ LogName=$log; StartTime=$WINDOW }
    if ($ids -and $ids.Count) { $filter['Id'] = $ids }
    try {
        $ev = @(Get-WinEvent -FilterHashtable $filter -MaxEvents $max -ErrorAction Stop)
        return @{ state='ok'; events=$ev }
    } catch {
        if (Test-NoEvents $_) { return @{ state='empty'; events=@() } }
        return @{ state='error'; events=@(); err=$_.Exception }
    }
}

# ---- header ----------------------------------------------------------------
# Every header statement is guarded: an unhandled throw here would abort BEFORE
# any report is written (no in-band signal, only a stale timestamp) - worse than
# a COLLECTION ERROR. So nothing in the header may be allowed to terminate.
$isAdmin = try { ([Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator) } catch { $false }
$os = try { Get-CimInstance Win32_OperatingSystem } catch { $null }
# UBR (the .NNN revision) is not in Win32_OperatingSystem.Version (that ends at
# the build number); read it from the registry so BUILD shows build.revision,
# not the build number twice.
$ubr = try { (Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion' -ErrorAction Stop).UBR } catch { $null }
$sections += "AUDIT TIMESTAMP: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss zzz')"
$sections += "COLLECTOR: v$COLLECTOR_VERSION  WINDOW: ${Days}d  ELEVATED: $isAdmin"
$sections += "COMPUTER: $env:COMPUTERNAME  USER: $env:USERNAME"
if ($os) { $sections += "OS: $($os.Caption)  BUILD: $($os.BuildNumber)$(if ($ubr) { ".$ubr" })" }

# ============================================================================
# A. PERSISTENCE - registry autostart extensibility points (ASEPs)
# ============================================================================
Add-Section 'A1. RUN / RUNONCE KEYS (version-normalized)' {
    $paths = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Run',
             'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce',
             'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run',
             'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run',
             'HKCU:\Software\Microsoft\Windows\CurrentVersion\RunOnce'
    foreach ($p in $paths) {
        if (-not (Test-Path -LiteralPath $p)) { "$p :: <absent>"; continue }
        $k = Get-Item -LiteralPath $p
        $names = @($k.GetValueNames() | Sort-Object)
        if (-not $names.Count) { "$p :: <empty>"; continue }
        foreach ($n in $names) { '{0} :: {1} = {2}' -f ($p -replace '^HK.*?:','HK'), $n, (Norm-Ver ([string]$k.GetValue($n,$null,'DoNotExpandEnvironmentNames'))) }
    }
}

Add-Section 'A2. HIGH-SIGNAL ASEPs (Policies\Run, RunOnceEx - should be empty)' {
    $paths = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\Explorer\Run',
             'HKCU:\Software\Microsoft\Windows\CurrentVersion\Policies\Explorer\Run',
             'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnceEx',
             'HKCU:\Software\Microsoft\Windows\CurrentVersion\RunOnceEx'
    foreach ($p in $paths) {
        if (-not (Test-Path -LiteralPath $p)) { "$p :: <absent>"; continue }
        $k = Get-Item -LiteralPath $p
        $names = @($k.GetValueNames() | Sort-Object)
        $kids  = @(Get-ChildItem -LiteralPath $p -Recurse -ErrorAction SilentlyContinue)
        if (-not $names.Count -and -not $kids.Count) { "$p :: <empty>"; continue }
        foreach ($n in $names) { '{0} :: {1} = {2}' -f $p,$n,$k.GetValue($n,$null,'DoNotExpandEnvironmentNames') }
        foreach ($c in ($kids | Sort-Object PSChildName)) { foreach ($n in @($c.GetValueNames()|Sort-Object)) { '{0}\{1} :: {2} = {3}' -f $p,$c.PSChildName,$n,$c.GetValue($n) } }
    }
}

Add-Section 'A3. WINLOGON Shell/Userinit/Notify (HKLM + HKCU)' {
    foreach ($p in 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon','HKCU:\Software\Microsoft\Windows NT\CurrentVersion\Winlogon') {
        if (-not (Test-Path -LiteralPath $p)) { "$p :: <absent>"; continue }
        $k = Get-Item -LiteralPath $p
        foreach ($v in 'Shell','Userinit','Taskman','VmApplet','System','GinaDLL','AppSetup') {
            '{0} :: {1} = [{2}]' -f ($p -replace '^HK.*?:','HK'),$v,$k.GetValue($v,'<unset>','DoNotExpandEnvironmentNames')
        }
    }
}

Add-Section 'A4. IFEO debuggers + AppInit_DLLs' {
    # Reset a SCRIPT-scoped flag each run: the ForEach-Object body runs in a child
    # scope, so a plain $found there never reaches this block - the read and the
    # write must use the SAME script scope, or the "<none set>" footer prints even
    # when a real Debugger/VerifierDlls hijack was found two lines above.
    $script:ifeoFound = $false
    foreach ($b in 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options','HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows NT\CurrentVersion\Image File Execution Options') {
        if (-not (Test-Path -LiteralPath $b)) { continue }
        # Per-subkey try/catch: some IFEO subkeys are ACL'd to block non-admin
        # reads (e.g. the Store/WindowsApps entries). One denied subkey must not
        # fail the whole lens - skip it, keep scanning the rest.
        Get-ChildItem -LiteralPath $b -ErrorAction SilentlyContinue | Sort-Object PSChildName | ForEach-Object {
            try {
                $d=$_.GetValue('Debugger'); $v=$_.GetValue('VerifierDlls')
                if ($d -or $v) { $script:ifeoFound=$true; '{0} :: Debugger=[{1}] VerifierDlls=[{2}]' -f $_.PSChildName,$d,($v -join ';') }
            } catch { }
        }
    }
    $a = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Windows' -ErrorAction SilentlyContinue
    "AppInit_DLLs=[$($a.AppInit_DLLs)] LoadAppInit_DLLs=$($a.LoadAppInit_DLLs)"
    if (-not $script:ifeoFound) { "IFEO debuggers: <none set>" }
}

Add-Section 'A6. LOGON-SCRIPT / .NET-PROFILER / WIN.INI ASEPs (HKCU)' {
    # High-value, non-admin-writable persistence Autoruns covers but naive audits
    # miss. All of these should be empty/unset on a clean host.
    $e = Get-ItemProperty 'HKCU:\Environment' -ErrorAction SilentlyContinue
    "UserInitMprLogonScript = [$($e.UserInitMprLogonScript)]"                 # T1037.001 logon script
    "COR_ENABLE_PROFILING   = [$($e.COR_ENABLE_PROFILING)]  COR_PROFILER=[$($e.COR_PROFILER)]  COR_PROFILER_PATH=[$($e.COR_PROFILER_PATH)]"  # T1574.012 profiler DLL
    $w = Get-ItemProperty 'HKCU:\Software\Microsoft\Windows NT\CurrentVersion\Windows' -ErrorAction SilentlyContinue
    "win.ini Load=[$($w.Load)]  Run=[$($w.Run)]"                              # T1546 win.ini-mapped ASEP
    # PowerShell profile scripts run on every shell launch (non-admin persistence).
    foreach ($pf in $PROFILE.CurrentUserAllHosts,$PROFILE.CurrentUserCurrentHost) {
        if ($pf -and (Test-Path -LiteralPath $pf)) { "PS profile PRESENT: $pf" }
    }
    # Active Setup per-user StubPath (T1547.014).
    $asRoot = 'HKCU:\Software\Microsoft\Active Setup\Installed Components'
    if (Test-Path -LiteralPath $asRoot) {
        Get-ChildItem -LiteralPath $asRoot -ErrorAction SilentlyContinue | ForEach-Object {
            $sp = $_.GetValue('StubPath'); if ($sp) { "ActiveSetup StubPath: $($_.PSChildName) = $(Norm-Ver $sp)" }
        }
    }
    "(HKCU Environment / win.ini / PS-profile / Active-Setup scan complete)"
}

Add-Section 'A7. UAC-BYPASS / FILE-HANDLER HIJACKS (HKCU Software\Classes)' {
    # Per-user shell\open\command hijacks of protected ProgIDs are the classic
    # fileless UAC-bypass primitives (fodhelper=ms-settings, eventvwr=mscfile) and
    # default-handler hijacks. On a clean host every one of these is <unset>.
    $any = $false
    foreach ($progid in 'ms-settings','mscfile','exefile','Folder','.bat','.cmd','.exe','.ps1') {
        $cmdKey = "HKCU:\Software\Classes\$progid\shell\open\command"
        if (Test-Path -LiteralPath $cmdKey) {
            $val = (Get-Item -LiteralPath $cmdKey).GetValue($null)
            if ($val) { $any=$true; "[HANDLER-HIJACK] $progid\shell\open\command = $val" }
        }
        $delegKey = "HKCU:\Software\Classes\$progid\shell\open\command"  # DelegateExecute variant
        $de = if (Test-Path -LiteralPath $delegKey) { (Get-Item -LiteralPath $delegKey).GetValue('DelegateExecute') } else { $null }
        if ($de -ne $null -and $de -eq '') { $any=$true; "[UAC-BYPASS-SHAPE] $progid has empty DelegateExecute (fodhelper/eventvwr pattern)" }
    }
    if (-not $any) { '(no per-user handler hijacks - clean)' }
}

Add-Section 'A5. COM hijack - HKCU CLSID InprocServer32 (user-writable / scriptlet only)' {
    $root = 'HKCU:\Software\Classes\CLSID'
    if (-not (Test-Path -LiteralPath $root)) { "$root :: <absent>"; return }
    $flagged = 0; $total = 0
    Get-ChildItem -LiteralPath $root -ErrorAction SilentlyContinue | ForEach-Object {
        foreach ($srv in 'InprocServer32','LocalServer32') {
            $sp = Join-Path $_.PSPath $srv
            if (Test-Path -LiteralPath $sp) {
                $total++
                $val = (Get-Item -LiteralPath $sp).GetValue($null)
                if ($val -and ((Is-UserWritable $val) -or $val -match 'scrobj\.dll|\.sct|ScriptletURL')) {
                    $flagged++; '{0} :: {1} = {2}' -f $_.PSChildName,$srv,(Norm-Ver $val)
                }
            }
        }
    }
    "(scanned $total HKCU CLSID servers; $flagged flagged user-writable/scriptlet)"
}

# ============================================================================
# B. PERSISTENCE - execution objects
# ============================================================================
Add-Section 'B1. NON-MICROSOFT AUTO-START SERVICES (signature + path hygiene)' {
    # Anchor the \Windows\ exclusion to the REAL system root - an unanchored
    # substring lets a service imaged under a user-created ...\Windows\ decoy
    # folder in the profile evade the lens. \Windows\Temp\ stays visible.
    $winRoot = [regex]::Escape($env:SystemRoot) + '\\'         # the true Windows dir
    $winTemp = [regex]::Escape($env:SystemRoot) + '\\Temp\\'
    Get-CimInstance Win32_Service | Where-Object {
        $_.StartMode -eq 'Auto' -and ($_.PathName -notmatch $winRoot -or $_.PathName -match $winTemp)
    } | Sort-Object Name | ForEach-Object {
        $img = Image-Of $_.PathName
        $sig = Sig-Status $img
        $flag = ''
        if ($sig -notin 'Valid','NoPath') { $flag += ' [SIG:' + $sig + ']' }
        # Only ALARM on a user-writable path when the image is NOT validly signed:
        # a Valid-signed binary in ProgramData (e.g. Defender's own MDCoreSvc /
        # WinDefend under ProgramData\...\Platform) is benign - flagging it is noise.
        if ((Is-UserWritable $img) -and $sig -ne 'Valid') { $flag += ' [USER-WRITABLE-PATH]' }
        '{0,-34} sig={1,-12} {2}{3}' -f $_.Name, $sig, (Norm-Ver $img), $flag
    }
}

Add-Section 'B2. NON-MICROSOFT SCHEDULED TASKS (enabled; hidden/suspicious flagged)' {
    # '^\\Microsoft\\' (WITH the trailing folder separator) - '^\\Microsoft'
    # without it also excludes an attacker-created \MicrosoftUpdater\ folder,
    # since a non-admin can register tasks in new root folders. The real
    # Microsoft subtree is \Microsoft\Windows\... and keeps the boundary.
    Get-ScheduledTask | Where-Object { $_.TaskPath -notmatch '^\\Microsoft\\' -and $_.State -ne 'Disabled' } |
      Sort-Object TaskPath,TaskName | ForEach-Object {
        $hidden = if ($_.Settings.Hidden) { ' [HIDDEN]' } else { '' }
        $susp = ''; $shown = ''
        foreach ($act in $_.Actions) {
            $exe  = [string]$act.Execute
            $full = ($exe + ' ' + [string]$act.Arguments).Trim()
            # User-writable check is on the EXECUTABLE only - an incidental log-file
            # or data-path argument under ProgramData (e.g. Firefox --MOZ_LOG) is not
            # a suspicious action. Script-host + download/proxy-exec LOLBins match on
            # the full command.
            if ((Is-UserWritable $exe) -or $full -match 'powershell|pwsh|wscript|cscript|mshta|rundll32|regsvr32|regasm|regsvcs|installutil|msbuild|certutil|bitsadmin|msiexec|curl\.exe|\.js\b|\.vbs\b|\.bat\b|\.cmd\b|\.ps1\b|\.scr\b') {
                $susp=' [SUSPICIOUS-ACTION]'; $shown = "  <= " + (Norm-Ver $full); break
            }
        }
        '{0}{1}{2}{3}' -f (Norm-Ver ($_.TaskPath + $_.TaskName)), $hidden, $susp, $shown
    }
}

Add-Section 'B3. STARTUP FOLDER ITEMS + SHELL-FOLDER REDIRECT' {
    foreach ($d in "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup","$env:ProgramData\Microsoft\Windows\Start Menu\Programs\Startup") {
        $tag = if ($d -match 'ProgramData') { 'ALL' } else { 'USER' }
        if (-not (Test-Path -LiteralPath $d)) { "[$tag] <absent>"; continue }
        $items = @(Get-ChildItem -LiteralPath $d -ErrorAction SilentlyContinue | Where-Object { $_.Name -ne 'desktop.ini' })
        if (-not $items.Count) { "[$tag] <empty>"; continue }
        $items | Sort-Object Name | ForEach-Object { "[$tag] $($_.Name)" }
    }
    # Shell-folder REDIRECT: repointing the Startup shell folder to an attacker
    # directory evades the enumeration above and is non-admin (T1547.001).
    $usf = Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\User Shell Folders' -ErrorAction SilentlyContinue
    $startup = $usf.Startup
    if ($startup -and $startup -notmatch 'Microsoft\\Windows\\Start Menu\\Programs\\Startup') {
        "[REDIRECT] User Shell Folders Startup = $startup"
    } else {
        "[REDIRECT] Startup shell folder = default"
    }
}

Add-Section 'B4. WMI EVENT-SUBSCRIPTION PERSISTENCE' {
    $f = @(Get-CimInstance -Namespace root\subscription -Class __EventFilter -ErrorAction SilentlyContinue)
    $c = @(Get-CimInstance -Namespace root\subscription -Class __EventConsumer -ErrorAction SilentlyContinue)
    $b = @(Get-CimInstance -Namespace root\subscription -Class __FilterToConsumerBinding -ErrorAction SilentlyContinue)
    $f | Sort-Object Name | ForEach-Object { "FILTER   $($_.Name) :: $($_.Query)" }
    $c | Sort-Object Name | ForEach-Object { "CONSUMER $($_.__CLASS)/$($_.Name) :: $($_.CommandLineTemplate)$($_.ScriptText)$($_.ExecutablePath)" }
    "(filters=$($f.Count) consumers=$($c.Count) bindings=$($b.Count))"
}

# ============================================================================
# C. DEFENSE EVASION - Microsoft Defender health
# ============================================================================
Add-Section 'C1. DEFENDER CORE STATUS' {
    $s = Get-MpComputerStatus
    "RealTimeProtection = $($s.RealTimeProtectionEnabled)"
    "TamperProtection   = $($s.IsTamperProtected)"
    "BehaviorMonitor    = $($s.BehaviorMonitorEnabled)"
    "AntivirusEnabled   = $($s.AntivirusEnabled)"
    "NIS/Network        = $($s.NISEnabled)"
    "SignatureAge(days) = $($s.AntivirusSignatureAge)"
    "LastQuickScanEnd   = $($s.QuickScanEndTime)"
    $p = Get-MpPreference
    "CloudMAPS(0off)    = $($p.MAPSReporting)"
    "SubmitSamples      = $($p.SubmitSamplesConsent)"
    "DisableRealtime    = $($p.DisableRealtimeMonitoring)"
    "ASRrules(count)    = $(($p.AttackSurfaceReductionRules_Ids | Measure-Object).Count)"
}

Add-Section 'C2. DEFENDER EXCLUSIONS (contents admin-only on this host)' {
    $p = Get-MpPreference
    $hide = $p.HideExclusionsFromLocalUsers
    foreach ($pair in @(@('Path',$p.ExclusionPath),@('Process',$p.ExclusionProcess),@('Extension',$p.ExclusionExtension))) {
        $vals = @($pair[1])
        if ($vals -match 'must be an administrator' -or $hide) {
            "Exclusion$($pair[0]) = HIDDEN-BY-POLICY (contents UNKNOWN without elevation)"
        } elseif (-not $vals.Count -or ($vals.Count -eq 1 -and -not $vals[0])) {
            "Exclusion$($pair[0]) = (none)"
        } else {
            "Exclusion$($pair[0]) = " + ($vals -join ' | ')
        }
    }
    "HideExclusionsFromLocalUsers = $hide  (attacker-added exclusions detectable non-admin only via C4 5007 events)"
}

Add-Section 'C3. DEFENDER DETECTION EVENTS (1116/1117)' {
    $r = Read-Events 'Microsoft-Windows-Windows Defender/Operational' @(1116,1117) 50
    switch ($r.state) {
        'denied' { Deny-Marker 'Defender operational log' $r.err }
        'empty'  { '(none in window)' }
        'error'  { throw $r.err }
        default  { if (-not $r.events.Count) { '(none in window)' } else { $r.events | ForEach-Object { '{0:yyyy-MM-dd HH:mm}  {1}' -f $_.TimeCreated, (($_.Message -split "`n")[0].Trim()) } } }
    }
}

Add-Section 'C4. DEFENDER CONFIG-CHANGE EVENTS (5001 RTP-off / 5007 security-relevant only)' {
    $r = Read-Events 'Microsoft-Windows-Windows Defender/Operational' @(5001,5007,5010,5012) 400
    switch ($r.state) {
        'denied' { Deny-Marker 'Defender operational log' $r.err; return }
        'error'  { throw $r.err }
    }
    if ($r.state -eq 'empty' -or -not $r.events.Count) { '(none in window)'; return }
    # 5007 ("Configuration has changed") fires on every signature/telemetry update -
    # dozens per day of pure churn. Surface ONLY the security-relevant ones (the
    # message echoes the changed value path): exclusions, real-time/behavior/tamper
    # toggles, engine disables. Everything else collapses to a single count line so
    # a genuine "someone disabled protection" event is not buried under update noise.
    $relevant = 'Exclusions\\|DisableRealtimeMonitoring|DisableBehaviorMonitoring|DisableOnAccessProtection|DisableAntiSpyware|DisableAntiVirus|TamperProtection|DisableIOAVProtection|PUAProtection|MAPSReporting|SubmitSamplesConsent'
    $churn = 0
    foreach ($e in $r.events) {
        $m = ($e.Message -replace "`r?`n",' ') -replace '\s{2,}',' '
        if ($e.Id -eq 5001 -or $m -match $relevant) {
            $tag = if ($m -match 'Exclusions\\') { ' [EXCLUSION-CHANGE]' } elseif ($e.Id -eq 5001) { ' [REALTIME-DISABLED]' } else { ' [SECURITY-SETTING]' }
            '{0:yyyy-MM-dd HH:mm}  id={1}{2}  {3}' -f $e.TimeCreated,$e.Id,$tag,$m.Substring(0,[Math]::Min(160,$m.Length))
        } else { $churn++ }
    }
    "(+ $churn generic 'configuration changed' events suppressed as signature/telemetry churn)"
}

# ============================================================================
# D. DEFENSE EVASION - event-log integrity
# ============================================================================
Add-Section 'D1. EVENT-LOG CLEARED (System 104; Security 1102 = admin-gap)' {
    $r = Read-Events 'System' @(104) 20
    if ($r.state -eq 'ok' -and $r.events.Count) { $r.events | ForEach-Object { '{0:yyyy-MM-dd HH:mm}  System log cleared' -f $_.TimeCreated } }
    elseif ($r.state -eq 'ok' -or $r.state -eq 'empty') { 'System-104: (no clears in window)' }
    else { 'System-104: ' + (Deny-Marker 'System log' $r.err) }
    $sec = Read-Events 'Security' @(1102) 5
    if ($sec.state -eq 'denied') { 'Security-1102: ' + (Deny-Marker 'Security log (1102 clear-audit)' $sec.err) }
    elseif ($sec.state -eq 'ok' -and $sec.events.Count) { $sec.events | ForEach-Object { '{0:yyyy-MM-dd HH:mm}  Security log cleared' -f $_.TimeCreated } }
    else { 'Security-1102: (none in window)' }
}

Add-Section 'D2. EVENT CHANNEL HEALTH (fixed list; <absent> is affirmative)' {
    $channels = 'System','Application','Microsoft-Windows-Windows Defender/Operational',
                'Microsoft-Windows-PowerShell/Operational','Microsoft-Windows-TaskScheduler/Operational',
                'Microsoft-Windows-CodeIntegrity/Operational'
    foreach ($ch in $channels) {
        $l = Get-WinEvent -ListLog $ch -ErrorAction SilentlyContinue
        if (-not $l) { "{0,-52} <absent/unreadable>" -f $ch }
        else { "{0,-52} enabled={1} records={2}" -f $ch, $l.IsEnabled, $l.RecordCount }
    }
}

# ============================================================================
# E. NETWORK - exposure
# ============================================================================
Add-Section 'E1. FIREWALL PROFILES' {
    Get-NetFirewallProfile | Sort-Object Name | ForEach-Object {
        $flag = if (-not $_.Enabled) { ' [DISABLED]' } else { '' }
        '{0,-8} Enabled={1} DefaultInbound={2} DefaultOutbound={3}{4}' -f $_.Name,$_.Enabled,$_.DefaultInboundAction,$_.DefaultOutboundAction,$flag
    }
}

Add-Section 'E2. TCP LISTENERS (well-known strict; dynamic/loopback volatile)' {
    # Non-admin cannot read protected system-process image paths, so a signature
    # is only meaningful for user-owned listeners (shown when present). Ports
    # >=49152 are the dynamic/ephemeral RPC range - they rotate every boot, so
    # they are fenced as VOLATILE and keyed by process, never by port number.
    $conns = Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue
    $rows = foreach ($c in $conns) {
        $loop = $c.LocalAddress -in '127.0.0.1','::1'
        $pr = Get-Process -Id $c.OwningProcess -ErrorAction SilentlyContinue
        $img = $pr.Path
        $sig = Sig-Status $img
        [pscustomobject]@{ Loop=$loop; Dyn=([int]$c.LocalPort -ge 49152); Port=[int]$c.LocalPort; Proc=$pr.ProcessName; Sig=$sig; UW=(Is-UserWritable $img); HasImg=[bool]$img }
    }
    function _sigtag($r) { $t=''; if($r.HasImg -and $r.Sig -notin 'Valid'){$t+=' [SIG:'+$r.Sig+']'}; if($r.UW){$t+=' [USER-WRITABLE]'}; $t }
    "[STABLE-EXPOSED] non-loopback, well-known ports <49152 (strict diff)"
    $exp = @($rows | Where-Object { -not $_.Loop -and -not $_.Dyn } | Sort-Object Port,Proc -Unique)
    if ($exp.Count) { $exp | ForEach-Object { '  {0,-6} <- {1}{2}' -f $_.Port,$_.Proc,(_sigtag $_) } } else { '  (none)' }
    "[VOLATILE-DYNAMIC] non-loopback dynamic ports >=49152 (by process; ports rotate each boot)"
    $dyn = @($rows | Where-Object { -not $_.Loop -and $_.Dyn } | Sort-Object Proc -Unique)
    if ($dyn.Count) { $dyn | ForEach-Object { '  {0}{1}' -f $_.Proc,(_sigtag $_) } } else { '  (none)' }
    "[VOLATILE-LOOPBACK] dev ports (by process; triage only)"
    $lo = @($rows | Where-Object { $_.Loop } | Sort-Object Proc -Unique)
    if ($lo.Count) { $lo | ForEach-Object { '  {0}{1}' -f $_.Proc,(_sigtag $_) } } else { '  (none)' }
}

Add-Section 'E3. SMB SHARES (non-default)' {
    Get-SmbShare -ErrorAction SilentlyContinue | Where-Object { $_.Name -notmatch '^(ADMIN\$|[A-Z]\$|IPC\$|print\$)$' } |
      Sort-Object Name | ForEach-Object { '{0,-20} -> {1}' -f $_.Name, $_.Path }
}

Add-Section 'E4. PORTPROXY FORWARDING (pivot surface)' {
    $out = netsh interface portproxy show all 2>$null
    $rows = @($out | Where-Object { $_ -match '^\s*\d' -or $_ -match '^\s*\d+\.\d+\.\d+\.\d+' })
    if ($rows.Count) { $rows | ForEach-Object { '  ' + ($_ -replace '\s+',' ').Trim() } } else { '(no portproxy rules)' }
}

Add-Section 'E5. REMOTE-ACCESS SERVICES (state)' {
    $names = 'TermService','UmRdpService','SessionEnv','sshd','TeamViewer','AnyDesk','tvnserver','winvnc','RustDesk'
    $any = $false
    foreach ($n in $names) { $s = Get-Service -Name $n -ErrorAction SilentlyContinue; if ($s) { $any=$true; $flag = if ($s.Status -eq 'Running') { ' [RUNNING]' } else { '' }; '{0,-14} {1} / {2}{3}' -f $s.Name,$s.Status,$s.StartType,$flag } }
    if (-not $any) { '(no remote-access services present)' }
}

# ============================================================================
# F. NETWORK - anomaly / C2
# ============================================================================
Add-Section 'F1. OUTBOUND TO PUBLIC IPs (process strict; endpoints volatile)' {
    $conns = Get-NetTCPConnection -State Established -ErrorAction SilentlyContinue |
      Where-Object { $_.RemoteAddress -notmatch '^(10\.|172\.(1[6-9]|2[0-9]|3[01])\.|192\.168\.|127\.|169\.254\.|::1|fe80|0\.0\.0\.0|224\.)' }
    $rows = foreach ($c in $conns) {
        $pr = Get-Process -Id $c.OwningProcess -ErrorAction SilentlyContinue
        [pscustomobject]@{ Proc=$pr.ProcessName; Sig=(Sig-Status $pr.Path); UW=(Is-UserWritable $pr.Path); Remote=$c.RemoteAddress; Port=[int]$c.RemotePort }
    }
    "[STABLE] processes making public connections (unsigned/user-writable = alarm)"
    # sig is NoPath for protected system processes (can't read their image non-admin);
    # only surface a sig tag when it is a real, non-Valid status on a readable image.
    $byproc = @($rows | Sort-Object Proc -Unique)
    if ($byproc.Count) { $byproc | ForEach-Object { $f=''; if($_.Sig -notin 'Valid','NoPath'){$f+=' [SIG:'+$_.Sig+']'}; if($_.UW){$f+=' [USER-WRITABLE]'}; '  {0}{1}' -f $_.Proc,$f } } else { '  (none)' }
    "[VOLATILE] distinct endpoints (triage only - CDN rotation churns weekly)"
    $eps = @($rows | Sort-Object Remote,Port -Unique | Select-Object -First 40)
    if ($eps.Count) { $eps | ForEach-Object { '  {0}:{1} <- {2}' -f $_.Remote,$_.Port,$_.Proc } } else { '  (none)' }
}

Add-Section 'F2. HOSTS FILE (non-default entries)' {
    $hf = "$env:windir\System32\drivers\etc\hosts"
    if (-not (Test-Path $hf)) { '(hosts file absent)'; return }
    $lines = @(Get-Content $hf | ForEach-Object { $_.Trim() } | Where-Object { $_ -and $_ -notmatch '^#' })
    if (-not $lines.Count) { '(no active host entries - default)' } else { $lines | Sort-Object | ForEach-Object { "  $_" } }
}

Add-Section 'F3. DNS SERVERS (physical strict; virtual/VPN volatile)' {
    $addrs = Get-DnsClientServerAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue | Where-Object { $_.ServerAddresses.Count }
    "[STABLE] physical adapters"
    $phys = @($addrs | Where-Object { $_.InterfaceAlias -notmatch 'vEthernet|WSL|VPN|Tailscale|TAP|VirtualBox|Hyper-V|Loopback|Bluetooth' } | Sort-Object InterfaceAlias)
    if ($phys.Count) { $phys | ForEach-Object { '  {0,-24} {1}' -f $_.InterfaceAlias, ($_.ServerAddresses -join ', ') } } else { '  (none)' }
    "[VOLATILE] virtual/VPN adapters"
    $virt = @($addrs | Where-Object { $_.InterfaceAlias -match 'vEthernet|WSL|VPN|Tailscale|TAP|VirtualBox|Hyper-V' } | Sort-Object InterfaceAlias)
    if ($virt.Count) { $virt | ForEach-Object { '  {0,-24} {1}' -f $_.InterfaceAlias, ($_.ServerAddresses -join ', ') } } else { '  (none)' }
}

Add-Section 'F4. PROXY CONFIGURATION (HKCU Internet Settings)' {
    $k = Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' -ErrorAction SilentlyContinue
    "ProxyEnable  = $($k.ProxyEnable)"
    "ProxyServer  = $($k.ProxyServer)"
    "AutoConfigURL= $($k.AutoConfigURL)"
}

# ============================================================================
# G. PROCESS & BINARY ANOMALY
# ============================================================================
Add-Section 'G1. PROCESSES FROM USER-WRITABLE PATHS (unsigned strict; signed counted)' {
    # Is-UserWritable is broad (the whole profile), so lots of legit Electron/dev
    # apps live under AppData - listing them all is noise. The THREAT is an
    # UNSIGNED/invalid image in a user-writable path, so those are listed in full
    # (strict-diff surface); validly-signed user-writable processes collapse to a
    # count line (informational).
    $procs = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue
    $byId = @{}; foreach ($p in $procs) { $byId[[int]$p.ProcessId] = $p }
    $hits = foreach ($p in $procs) {
        if (Is-UserWritable $p.ExecutablePath) {
            $par = $byId[[int]$p.ParentProcessId]
            [pscustomobject]@{ Name=$p.Name; Img=(Norm-Ver $p.ExecutablePath); Sig=(Sig-Status $p.ExecutablePath); Parent=($par.Name) }
        }
    }
    $u = @($hits | Sort-Object Img -Unique)
    $unsigned = @($u | Where-Object { $_.Sig -ne 'Valid' })
    $signed   = @($u | Where-Object { $_.Sig -eq 'Valid' })
    if ($unsigned.Count) { $unsigned | ForEach-Object { '[UNSIGNED] {0,-22} {1}  parent={2}  [SIG:{3}]' -f $_.Name,$_.Img,$_.Parent,$_.Sig } }
    else { '(no unsigned/invalid processes from user-writable paths)' }
    "(+ $($signed.Count) validly-signed user-writable processes - benign app baseline, not listed)"
}

Add-Section 'G2. SYSTEM-PROCESS MASQUERADE (critical names off System32)' {
    $crit = 'svchost.exe','lsass.exe','services.exe','csrss.exe','winlogon.exe','smss.exe','wininit.exe','spoolsv.exe','taskhostw.exe','explorer.exe'
    # Anchor the legit-root regex to the REAL SystemRoot, not an unanchored
    # substring - otherwise a decoy path containing '\Windows\System32\' anywhere
    # (a user-created folder in the profile) would be treated as legitimate.
    $sr = [regex]::Escape($env:SystemRoot)
    $legit = "^$sr\\(System32|SysWOW64|WinSxS)\\|^$sr\\explorer\.exe$"
    $procs = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | Where-Object { $_.Name -in $crit -and $_.ExecutablePath }
    $bad = @($procs | Where-Object { $_.ExecutablePath -notmatch $legit } | Sort-Object ExecutablePath -Unique)
    if (-not $bad.Count) { '(none - all critical processes run from the real System32/SysWOW64)' }
    else { $bad | ForEach-Object { '[MASQUERADE] {0} @ {1}' -f $_.Name,$_.ExecutablePath } }
}

# ============================================================================
# H. ACCOUNTS & PRIVILEGE
# ============================================================================
Add-Section 'H1. LOCAL USERS (enabled/pw flags)' {
    Get-LocalUser | Sort-Object Name | ForEach-Object {
        $flag = ''
        if ($_.Enabled -and -not $_.PasswordRequired) { $flag += ' [PW-NOT-REQUIRED]' }
        '{0,-20} Enabled={1,-5} PwReq={2,-5} PwExpires={3}{4}' -f $_.Name,$_.Enabled,$_.PasswordRequired,($_.PasswordExpires),$flag
    }
}

Add-Section 'H2. PRIVILEGED GROUP MEMBERSHIP' {
    # Get-LocalGroupMember THROWS (past -ErrorAction) if a group holds an
    # unresolvable member SID (deleted account) - which would blank this CRITICAL
    # lens. Per-group try/catch with a `net localgroup` fallback so one orphaned
    # SID degrades to a partial result instead of failing the whole section.
    foreach ($g in 'Administrators','Remote Desktop Users','Remote Management Users','Backup Operators') {
        # Skip groups that don't exist on this edition (Home lacks some) so the
        # fallback is only reached for a real orphaned-SID failure.
        if (-not (Get-LocalGroup -Name $g -ErrorAction SilentlyContinue)) { '{0,-24} (group absent on this edition)' -f $g; continue }
        try {
            $m = @(Get-LocalGroupMember -Group $g -ErrorAction Stop)
            if ($m.Count) { $m | Sort-Object Name | ForEach-Object { '{0,-24} {1} [{2}]' -f $g,$_.Name,$_.ObjectClass } }
            else { '{0,-24} (empty)' -f $g }
        } catch {
            "{0,-24} [Get-LocalGroupMember failed ({1}); net-localgroup fallback:]" -f $g,$_.Exception.GetType().Name
            # `net` writes to stderr on error; under Stop that becomes a
            # NativeCommandError, so drop the preference locally for the call.
            $eap = $ErrorActionPreference; $ErrorActionPreference = 'SilentlyContinue'
            $raw = & net localgroup "$g" 2>&1
            $ErrorActionPreference = $eap
            $inList = $false
            foreach ($line in @($raw)) {
                $t = [string]$line
                if ($t -match '^-{4,}') { $inList = $true; continue }
                if ($t -match 'completed successfully') { $inList = $false; continue }
                if ($inList -and $t.Trim()) { '{0,-24} {1}' -f $g, $t.Trim() }
            }
        }
    }
}

Add-Section 'H3. BUILT-IN ACCOUNT STATE + AUTOLOGON' {
    Get-LocalUser | Where-Object { $_.Name -in 'Administrator','Guest','DefaultAccount','WDAGUtilityAccount' } |
      Sort-Object Name | ForEach-Object {
        $flag = if ($_.Enabled -and $_.Name -in 'Administrator','Guest') { ' [ENABLED-REVIEW]' } else { '' }
        '{0,-20} Enabled={1}{2}' -f $_.Name,$_.Enabled,$flag
      }
    $lg = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon' -ErrorAction SilentlyContinue
    $ap = if ([bool]$lg.DefaultPassword) { ' [DEFAULTPASSWORD-STORED]' } else { '' }
    "AutoAdminLogon=$($lg.AutoAdminLogon) DefaultUserName=$($lg.DefaultUserName)$ap"
}

Add-Section 'H4. HIDDEN USERS (Winlogon SpecialAccounts UserList)' {
    $sa = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon\SpecialAccounts\UserList'
    if (-not (Test-Path $sa)) { '(no SpecialAccounts hidden list - default)'; return }
    $names = @((Get-Item $sa).GetValueNames())
    if (-not $names.Count) { '(list present but empty)' } else { $names | Sort-Object | ForEach-Object { $v=(Get-Item $sa).GetValue($_); "[HIDDEN-LOGIN-SCREEN] $_ = $v" } }
}

# ============================================================================
# I. PATCH & INTEGRITY POSTURE
# ============================================================================
Add-Section 'I1. PATCH STALENESS + OS SERVICING' {
    $hf = Get-HotFix | Where-Object { $_.InstalledOn } | Sort-Object InstalledOn -Descending | Select-Object -First 1
    if ($hf) { "LastHotfix = $($hf.HotFixID) on $($hf.InstalledOn.ToString('yyyy-MM-dd'))  ($([int]((Get-Date)-$hf.InstalledOn).TotalDays)d ago)" }
    else { "LastHotfix = (none reported by Get-HotFix)" }
    if ($os) { "OS build   = $($os.BuildNumber)  installed $($os.InstallDate.ToString('yyyy-MM-dd'))" }
    $upd = @('wuauserv','UsoSvc','WaaSMedicSvc') | ForEach-Object { $s=Get-Service -Name $_ -EA SilentlyContinue; if($s){ '{0}={1}/{2}' -f $s.Name,$s.Status,$s.StartType } }
    "WU services: " + ($upd -join '  ')
    $wu = Get-ItemProperty 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate\AU' -ErrorAction SilentlyContinue
    $wusvr = Get-ItemProperty 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate' -ErrorAction SilentlyContinue
    if ($wu.NoAutoUpdate -eq 1) { "[WU-POLICY] NoAutoUpdate=1 (auto-update disabled by policy)" }
    if ($wusvr.WUServer) { "[WSUS-REDIRECT] WUServer=$($wusvr.WUServer)" }
}

Add-Section 'I2. UAC / SMARTSCREEN / SIGNING POSTURE' {
    $sys = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System' -ErrorAction SilentlyContinue
    $lua = if ($sys.EnableLUA -ne 1) { ' [UAC-DISABLED]' } else { '' }
    "EnableLUA=$($sys.EnableLUA) ConsentPromptBehaviorAdmin=$($sys.ConsentPromptBehaviorAdmin)$lua"
    $exp = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer' -ErrorAction SilentlyContinue
    "SmartScreen(Explorer)=$($exp.SmartScreenEnabled)"
    try { "SecureBoot=" + (Confirm-SecureBootUEFI) } catch { "SecureBoot: " + (Deny-Marker 'Confirm-SecureBootUEFI' $_.Exception) }
}

Add-Section 'I3. CODE-INTEGRITY EVENTS (unsigned/blocked images)' {
    $r = Read-Events 'Microsoft-Windows-CodeIntegrity/Operational' @(3033,3034,3004,3010,3077) 30
    switch ($r.state) {
        'denied' { Deny-Marker 'CodeIntegrity operational log' $r.err }
        'empty'  { '(none in window)' }
        'error'  { throw $r.err }
        default  { if (-not $r.events.Count) { '(none in window)' } else { $r.events | ForEach-Object { '{0:yyyy-MM-dd HH:mm}  id={1}  {2}' -f $_.TimeCreated,$_.Id,(($_.Message -split "`n")[0].Trim()) } } }
    }
}

# ============================================================================
# J. EVENT-LOG SIGNALS (non-admin, windowed)
# ============================================================================
Add-Section 'J1. SERVICE INSTALLED (System 7045)' {
    $r = Read-Events 'System' @(7045) 50
    if ($r.state -in 'empty') { '(none in window)'; return }
    if ($r.state -ne 'ok') { if($r.err){throw $r.err} else {'(none in window)'}; return }
    if (-not $r.events.Count) { '(none in window)'; return }
    $r.events | ForEach-Object {
        $x=[xml]$_.ToXml(); $d=@{}; $x.Event.EventData.Data | ForEach-Object { $d[$_.Name]=$_.'#text' }
        $img = Norm-Ver $d['ImagePath']
        $f = if (Is-UserWritable $d['ImagePath']) { ' [USER-WRITABLE]' } else { '' }
        '{0:yyyy-MM-dd}  svc={1,-24} start={2} img={3}{4}' -f $_.TimeCreated,$d['ServiceName'],$d['StartType'],$img,$f
    }
}

Add-Section 'J2. SERVICE START-TYPE CHANGED (System 7040; BITS/updater churn tagged)' {
    $r = Read-Events 'System' @(7040) 50
    if ($r.state -ne 'ok' -or -not $r.events.Count) { '(none in window)'; return }
    $r.events | ForEach-Object {
        $m = ($_.Message -replace "`r?`n",' ') -replace '\s{2,}',' '
        $churn = if ($m -match '\b(Background Intelligent Transfer Service|Windows Update|wuauserv|BITS|Update Orchestrator|Windows Modules Installer|TrustedInstaller|WaaSMedic|Delivery Optimization|Windows Search)\b') { ' [CHURN]' } else { '' }
        '{0:yyyy-MM-dd}{1}  {2}' -f $_.TimeCreated,$churn,$m.Substring(0,[Math]::Min(120,$m.Length))
    }
}

Add-Section 'J3. POWERSHELL SUSPICIOUS SCRIPT-BLOCKS (4104; self-excluded)' {
    # 4104 is HIGH VOLUME when script-block logging is on (this host: ~400/wk), and
    # Level=3 (Warning) here fires for benign warning-level blocks - NOT an AMSI
    # "malicious" signal - so we do not use it (it would report ~50 benign hits/run,
    # including this collector's own blocks). Instead: a conservative regex built
    # from CONCATENATED fragments so the collector's own source can never self-match,
    # AND an explicit exclusion of the collector's script path. Reported as a count
    # baseline + timestamps; keyword scans are noise-prone by nature, so the analysis
    # treats a small nonzero count as "review if unexpected", not an automatic alarm.
    $rx = (('Download'+'String'),('Invoke'+'-Expression'),('FromBase64'+'String'),
           ('-nop'+' -w hidden'),('-e'+'ncodedcommand'),('Reflection.'+'Assembly'),
           ('Net.'+'WebClient'),('IEX'+'\(')) -join '|'
    $all = @()
    try { $all = @(Get-WinEvent -FilterHashtable @{LogName='Microsoft-Windows-PowerShell/Operational';Id=4104;StartTime=$WINDOW} -MaxEvents 1000 -ErrorAction Stop) }
    catch { if ($_.Exception.Message -match 'No events') { '(no 4104 script-block events in window - script-block logging may be off)'; return } else { throw } }
    $selfRx = if ($SELF_PATH) { [regex]::Escape((Split-Path -Leaf $SELF_PATH)) } else { '\x00NOMATCH\x00' }
    $susp = @($all | Where-Object { $_.Message -match $rx -and $_.Message -notmatch $selfRx })
    "total 4104 in window = $($all.Count)   keyword-suspicious (self-excluded) = $($susp.Count)"
    if ($susp.Count) { $susp | Select-Object -First 8 | ForEach-Object { '  [REVIEW] {0:yyyy-MM-dd HH:mm}  script block matched suspicious keyword' -f $_.TimeCreated } }
    else { '  (no suspicious-keyword script blocks - clean)' }
}

Add-Section 'K. SECURITY-LOG VISIBILITY GAP (consolidated)' {
    $probe = Read-Events 'Security' @() 1
    if ($probe.state -eq 'denied') {
        Deny-Marker 'Security event log' $probe.err
        "  -> NOT audited non-admin: 4720 new-user, 4722/4724 enable/pwreset, 4728/4732 group-add, 4624/4625 logons, 4672 priv-assign, 1102 clear."
        "  -> Enable by adding this account to 'Event Log Readers', or run the collector elevated, to light these up."
    } elseif ($probe.state -eq 'ok') {
        '(Security log READABLE - account is elevated or in Event Log Readers; account-change events available)'
    } else {
        '(Security log state: ' + $probe.state + ')'
    }
}

# Test-only fault-injection lens: proves the throw -> '!! COLLECTION ERROR'
# contract (marker appears, RC stays 0, later sections still render). Placed last
# so a following section (the assemble step) still renders after it. Never set in
# production - the scheduled task invokes the collector with no extra args.
if ($FaultInject) {
    Add-Section 'ZZ. FAULT-INJECT (test only - expected to error)' { throw 'injected fault (test)' }
    Add-Section 'ZZ2. POST-FAULT SENTINEL (must still render)' { 'sentinel-ok' }
}

# ---- assemble + write ------------------------------------------------------
$report = $sections -join "`r`n"

if ($StdOut) { $report; return }

$taskDir = Join-Path $env:USERPROFILE 'Claude\Scheduled\weekly-security-audit'
$histDir = Join-Path $BaseDir 'history'
New-Item -ItemType Directory -Force -Path $BaseDir, $histDir | Out-Null

# Atomic canonical write: render to a temp file in the same dir, then Move-Item
# over the canonical name. A transient AV/sync lock can then fail the temp write
# without leaving latest-audit.txt half-written (the analysis reads a whole file).
$canonical = Join-Path $BaseDir 'latest-audit.txt'
$tmp = Join-Path $BaseDir (".latest-audit.$PID.tmp")
$report | Set-Content -Path $tmp -Encoding UTF8
Move-Item -LiteralPath $tmp -Destination $canonical -Force
try { if (Test-Path $taskDir) { $report | Set-Content -Path (Join-Path $taskDir 'latest-audit.txt') -Encoding UTF8 } } catch { Write-Warning "task-folder copy: $($_.Exception.Message)" }
try {
    $report | Set-Content -Path (Join-Path $histDir ("audit-{0}.txt" -f (Get-Date -Format 'yyyy-MM-dd'))) -Encoding UTF8
    Get-ChildItem $histDir -Filter 'audit-*.txt' | Sort-Object Name -Descending | Select-Object -Skip 20 | Remove-Item -Force
} catch { Write-Warning "history: $($_.Exception.Message)" }
try {
    foreach ($gd in @((Join-Path $env:USERPROFILE 'My Drive'),'G:\My Drive')) {
        if (Test-Path $gd) { Copy-Item $canonical (Join-Path $gd 'security-audit-latest.txt') -Force; break }
    }
} catch { Write-Warning "drive copy: $($_.Exception.Message)" }
