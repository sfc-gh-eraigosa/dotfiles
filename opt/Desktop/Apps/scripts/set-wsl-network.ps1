<#
.SYNOPSIS
  Apply the recorded WSL networking mode (gff install.windows.wsl-network)
  on the Windows side: .wslconfig, portproxy, firewall and scheduled task.

.DESCRIPTION
  Run by `install_windows.sh --wsl-network` at the end of install.sh. That
  step first runs -Status (no elevation) and only elevates for -Mode when
  -Status says something would change, so a current machine never sees UAC.

    off       hands off: changes nothing, cleans up nothing.
    nat       refresh-wsl-portproxy.ps1 -ListenPort <p> -RegisterTask (the
              forward, the LocalSubnet firewall rule, the at-logon task).
              <p> is 22, or 2222 when a Windows sshd service exists.
    bridged   an existing networkingMode=bridged is current and kept byte for
              byte (its vmSwitch/macAddress pin the k3s node's DHCP address).
              Otherwise, with an External Hyper-V switch named WSLBridge, it
              writes networkingMode/vmSwitch/dhcp/macAddress; without one it
              only prints the command to create it. It never creates a switch.
    mirrored  networkingMode=mirrored plus a Hyper-V firewall rule letting
              inbound TCP 22 reach WSL.
              Refused (exit 4) while the Windows OpenSSH service is running
              or enabled: mirrored WSL shares the host's ports, so both would
              claim 22 (nat avoids this by listening on 2222).

  Cleanup of the previous mode runs only when this run changed networkingMode
  in .wslconfig (a real switch the operator chose), never after a mode that
  detection merely recorded.

  .wslconfig edits touch only networkingMode/vmSwitch/macAddress/dhcp in
  [wsl2] (and drop a networkingMode left under [experimental]); every other
  byte, line ending and BOM is kept. A .bak-<yyyyMMdd-HHmmss> copy is written
  first; a file that is already current is not touched. It never runs the
  WSL shutdown itself: it prints the command instead.

.PARAMETER Mode
  off | nat | bridged | mirrored (default off).

.PARAMETER Status
  Read-only report. Exit 0 = current for -Mode, 3 = applying -Mode would
  change something, 4 = -Mode cannot be applied as the machine is (mirrored
  with Windows OpenSSH on). Needs no elevation; anything it can't read without admin
  is reported as "unknown (needs admin)" and treated as current.

.PARAMETER ConfigOnly
  Only the .wslconfig step (no portproxy/firewall/task). Needs no elevation.

.PARAMETER WslConfigPath
  The .wslconfig to read/edit (default %USERPROFILE%\.wslconfig).

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File set-wsl-network.ps1 -Status -Mode nat
  powershell -ExecutionPolicy Bypass -File set-wsl-network.ps1 -Mode bridged
#>
param(
    [ValidateSet('off','nat','bridged','mirrored')][string]$Mode = 'off',
    [switch]$Status,
    [switch]$ConfigOnly,
    [string]$WslConfigPath = "$env:USERPROFILE\.wslconfig"
)
$ErrorActionPreference = 'Stop'
$env:WSL_UTF8 = '1'

$SwitchName     = 'WSLBridge'
$HvRuleName     = 'WSL-SSH-Mirrored'
$WslVmCreatorId = '{40E0AC32-46A5-438A-A0B2-2B479E8F2E90}'
$TaskName       = 'RefreshWslPortProxy'
$NatPorts       = @(22, 2222)   # every listen port nat may have used
$RefreshScript  = Join-Path $PSScriptRoot 'refresh-wsl-portproxy.ps1'
$logDir         = Join-Path $env:LOCALAPPDATA 'dotfiles'
$logFile        = Join-Path $logDir 'set-wsl-network.log'

# Test seams (wsl-network_test.sh), honored only with WSLNET_TEST=1:
#   WSLNET_TEST_SWITCH=present|absent|unknown  the WSLBridge probe result; only
#                                              with -Status or -ConfigOnly
#   WSLNET_TEST_REFRESH=<path>                 a stand-in refresh script; only
#                                              for -Mode nat, which then skips
#                                              the admin check and all cleanup
#   WSLNET_TEST_WINSSHD=on|off                 the Windows OpenSSH probe result
$TestMode    = $env:WSLNET_TEST -eq '1'
$TestSwitch  = $null
$TestRefresh = $false
$TestWinSshd = $null
if ($TestMode) {
    if ($env:WSLNET_TEST_WINSSHD -match '^(on|off)$') { $TestWinSshd = $env:WSLNET_TEST_WINSSHD -eq 'on' }
    if (($Status -or $ConfigOnly) -and $env:WSLNET_TEST_SWITCH -match '^(present|absent|unknown)$') { $TestSwitch = $env:WSLNET_TEST_SWITCH }
    if ($env:WSLNET_TEST_REFRESH -and $Mode -eq 'nat' -and -not $Status) {
        $RefreshScript = $env:WSLNET_TEST_REFRESH; $TestRefresh = $true
    }
}

function Write-Log([string]$msg) {
    if (-not (Test-Path $logDir)) { New-Item -ItemType Directory -Path $logDir -Force | Out-Null }
    "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') $msg" | Add-Content -Path $logFile
    Write-Host $msg
}

function Test-Admin {
    ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

# ---------------------------------------------------------------------------
# .wslconfig model: a list of lines, each { T = text, E = its own line ending
# ('' for an unterminated last line) }, plus the BOM flag. Serializing an
# unedited model gives back the original bytes exactly.
# ---------------------------------------------------------------------------
function Read-WslConfig([string]$Path) {
    $cfg = [pscustomobject]@{
        Exists = $false; Bom = $false; Nl = "`r`n"
        Lines  = New-Object System.Collections.ArrayList
    }
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return $cfg }
    $cfg.Exists = $true
    $bytes = [IO.File]::ReadAllBytes($Path)
    $start = 0
    if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) {
        $cfg.Bom = $true; $start = 3
    }
    $text = (New-Object Text.UTF8Encoding($false)).GetString($bytes, $start, $bytes.Length - $start)
    $nlSeen = $false
    $i = 0
    while ($i -lt $text.Length) {
        $j = $text.IndexOf("`n", $i)
        if ($j -lt 0) { [void]$cfg.Lines.Add([pscustomobject]@{ T = $text.Substring($i); E = '' }); break }
        $t = $text.Substring($i, $j - $i); $e = "`n"
        if ($t.EndsWith("`r")) { $t = $t.Substring(0, $t.Length - 1); $e = "`r`n" }
        if (-not $nlSeen) { $cfg.Nl = $e; $nlSeen = $true }   # new lines follow the file's style
        [void]$cfg.Lines.Add([pscustomobject]@{ T = $t; E = $e })
        $i = $j + 1
    }
    return $cfg
}

function ConvertTo-WslConfigBytes($cfg) {
    $sb = New-Object Text.StringBuilder
    foreach ($l in $cfg.Lines) { [void]$sb.Append($l.T).Append($l.E) }
    $body = (New-Object Text.UTF8Encoding($false)).GetBytes($sb.ToString())
    # the leading comma keeps an empty array an array (PowerShell unrolls it to $null)
    if ($cfg.Bom) { return , [byte[]](@(0xEF, 0xBB, 0xBF) + $body) }
    return , [byte[]]$body
}

function Copy-WslConfig($cfg) {
    $c = [pscustomobject]@{ Exists = $cfg.Exists; Bom = $cfg.Bom; Nl = $cfg.Nl; Lines = New-Object System.Collections.ArrayList }
    foreach ($l in $cfg.Lines) { [void]$c.Lines.Add([pscustomobject]@{ T = $l.T; E = $l.E }) }
    return $c
}

# Section name (lowercase, '' before any header) of every line.
function Get-LineSections($cfg) {
    $sec = ''
    $out = @()
    foreach ($l in $cfg.Lines) {
        if ($l.T -match '^\s*\[\s*([^\]]+?)\s*\]\s*$') { $sec = $Matches[1].ToLower() }
        $out += $sec
    }
    return , $out
}

# Indices of `key=...` lines in a section.
function Find-KeyLines($cfg, [string]$Section, [string]$Key) {
    $secs = Get-LineSections $cfg
    $hits = @()
    for ($i = 0; $i -lt $cfg.Lines.Count; $i++) {
        if ($secs[$i] -eq $Section -and $cfg.Lines[$i].T -match "^\s*(?i:$Key)\s*=") { $hits += $i }
    }
    return , $hits
}

function Get-KeyValue($cfg, [string]$Section, [string]$Key) {
    $hits = Find-KeyLines $cfg $Section $Key
    if ($hits.Count -eq 0) { return $null }
    $v = ($cfg.Lines[$hits[-1]].T -replace '^[^=]*=', '') -replace '\s+[#;].*$', ''
    return $v.Trim().Trim('"').Trim()
}

function Remove-Key($cfg, [string]$Section, [string]$Key) {
    $hits = Find-KeyLines $cfg $Section $Key
    for ($k = $hits.Count - 1; $k -ge 0; $k--) {
        $i = $hits[$k]
        # Removing an unterminated last line: the line before it becomes last
        # and keeps the "no trailing newline" shape.
        if ($cfg.Lines[$i].E -eq '' -and $i -gt 0) { $cfg.Lines[$i - 1].E = '' }
        $cfg.Lines.RemoveAt($i)
    }
}

function Add-LineAfter($cfg, [int]$Index, [string]$Text) {
    $new = [pscustomobject]@{ T = $Text; E = $cfg.Nl }
    if ($Index -ge 0 -and $cfg.Lines[$Index].E -eq '') { $cfg.Lines[$Index].E = $cfg.Nl; $new.E = '' }
    $cfg.Lines.Insert($Index + 1, $new)
}

# Set key=value in [wsl2]: rewrite the first existing line (dropping
# duplicates), else add it after the section's last non-blank line, else
# append a [wsl2] section.
function Set-Wsl2Key($cfg, [string]$Key, [string]$Value) {
    $hits = Find-KeyLines $cfg 'wsl2' $Key
    if ($hits.Count -gt 0) {
        if ((Get-KeyValue $cfg 'wsl2' $Key) -ceq $Value -and $hits.Count -eq 1) { return }
        for ($k = $hits.Count - 1; $k -ge 1; $k--) {
            $i = $hits[$k]
            # As in Remove-Key: dropping an unterminated last line leaves the
            # line before it as the unterminated last line.
            if ($cfg.Lines[$i].E -eq '' -and $i -gt 0) { $cfg.Lines[$i - 1].E = '' }
            $cfg.Lines.RemoveAt($i)
        }
        $cfg.Lines[$hits[0]].T = "$Key=$Value"
        return
    }
    $secs = Get-LineSections $cfg
    $at = -1
    for ($i = 0; $i -lt $cfg.Lines.Count; $i++) {
        if ($secs[$i] -ne 'wsl2') { if ($at -ge 0) { break } else { continue } }
        if ($at -lt 0 -or $cfg.Lines[$i].T.Trim() -ne '') { $at = $i }
    }
    if ($at -lt 0) {
        Add-LineAfter $cfg ($cfg.Lines.Count - 1) '[wsl2]'
        $at = $cfg.Lines.Count - 1
    }
    Add-LineAfter $cfg $at "$Key=$Value"
}

# The mode WSL runs with this file: [wsl2] wins, then a legacy
# [experimental] networkingMode, else WSL's default (nat).
function Get-EffectiveMode($cfg) {
    $m = Get-KeyValue $cfg 'wsl2' 'networkingMode'
    if (-not $m) { $m = Get-KeyValue $cfg 'experimental' 'networkingMode' }
    if (-not $m) { return 'nat' }
    return $m.ToLower()
}

# 02: + the first 5 bytes of SHA-256(COMPUTERNAME): stable per machine and
# locally administered, so the bridged distro keeps its DHCP lease.
function Get-BridgeMac {
    $hash = [Security.Cryptography.SHA256]::Create().ComputeHash([Text.Encoding]::UTF8.GetBytes($env:COMPUTERNAME))
    return '02:' + (($hash[0..4] | ForEach-Object { $_.ToString('x2') }) -join ':')
}

# present | absent | unknown (could not read: needs admin / Hyper-V Administrators)
function Get-BridgeSwitchState {
    if ($TestSwitch) { return $TestSwitch }
    if (-not (Get-Command Get-VMSwitch -ErrorAction SilentlyContinue)) { return 'absent' }   # no Hyper-V module
    try {
        $s = @(Get-VMSwitch -Name $SwitchName -ErrorAction Stop | Where-Object { "$($_.SwitchType)" -eq 'External' })
        if ($s.Count -gt 0) { return 'present' } else { return 'absent' }
    } catch {
        # Only a definite "no such switch" is absent; anything else (no
        # permission when not elevated) is unknown, so -Status asks for the
        # elevated run instead of guessing.
        if ("$($_.CategoryInfo.Category)" -eq 'ObjectNotFound' -or $_.Exception.Message -match 'unable to find') { return 'absent' }
        return 'unknown'
    }
}

function Get-NewVMSwitchHint {
    $adapter = '<wired adapter>'
    try {
        $wired = @(Get-NetAdapter -Physical -ErrorAction Stop |
            Where-Object { $_.Status -eq 'Up' -and $_.PhysicalMediaType -notmatch '802\.11|Wireless' })
        if ($wired.Count -eq 1) { $adapter = $wired[0].Name }
    } catch { }
    return "New-VMSwitch -Name $SwitchName -NetAdapterName '$adapter' -AllowManagementOS `$true"
}

# The .wslconfig edit for -Mode, computed but not written.
function Get-ConfigPlan($cfg) {
    $plan = [pscustomobject]@{
        Before = (Get-EffectiveMode $cfg); After = $null; Changed = $false
        Bytes = $null; NoSwitch = $false; SwitchState = $null
    }
    $new = Copy-WslConfig $cfg
    switch ($Mode) {
        'nat' {
            Remove-Key $new 'experimental' 'networkingMode'
            if ($plan.Before -ne 'nat') {
                foreach ($k in 'networkingMode', 'vmSwitch', 'macAddress') { Remove-Key $new 'wsl2' $k }
            }
        }
        'mirrored' {
            Remove-Key $new 'experimental' 'networkingMode'
            Set-Wsl2Key $new 'networkingMode' 'mirrored'
        }
        'bridged' {
            if ($plan.Before -eq 'bridged') {
                # Current: keep vmSwitch/macAddress/dhcp exactly; only move a
                # legacy [experimental] networkingMode into [wsl2].
                Remove-Key $new 'experimental' 'networkingMode'
                if ((Get-KeyValue $new 'wsl2' 'networkingMode') -ne 'bridged') { Set-Wsl2Key $new 'networkingMode' 'bridged' }
            } else {
                $plan.SwitchState = Get-BridgeSwitchState
                if ($plan.SwitchState -ne 'present') {
                    $plan.NoSwitch = $true; $plan.After = $plan.Before
                    return $plan
                }
                Remove-Key $new 'experimental' 'networkingMode'
                Set-Wsl2Key $new 'networkingMode' 'bridged'
                Set-Wsl2Key $new 'vmSwitch' $SwitchName
                Set-Wsl2Key $new 'dhcp' 'true'
                Set-Wsl2Key $new 'macAddress' (Get-BridgeMac)
            }
        }
    }
    $plan.After = Get-EffectiveMode $new
    $old = ConvertTo-WslConfigBytes $cfg
    $bytes = ConvertTo-WslConfigBytes $new
    if (-not $cfg.Exists -and $new.Lines.Count -eq 0) { return $plan }   # nothing to create
    $plan.Changed = (-not $cfg.Exists) -or
        ([Convert]::ToBase64String($old) -ne [Convert]::ToBase64String($bytes))
    if ($plan.Changed) { $plan.Bytes = $bytes }
    return $plan
}

function Write-WslConfig([string]$Path, [byte[]]$Bytes) {
    if (Test-Path -LiteralPath $Path -PathType Leaf) {
        $bak = "$Path.bak-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
        $n = 1
        while (Test-Path -LiteralPath $bak) { $bak = "$Path.bak-$(Get-Date -Format 'yyyyMMdd-HHmmss')-$n"; $n++ }
        Copy-Item -LiteralPath $Path -Destination $bak
        Write-Log "backed up $Path -> $bak"
    }
    [IO.File]::WriteAllBytes($Path, $Bytes)
}

# ---------------------------------------------------------------------------
# Windows-side state
# ---------------------------------------------------------------------------
function Get-NatPort { if (Get-Service sshd -ErrorAction SilentlyContinue) { 2222 } else { 22 } }

# Windows OpenSSH listens on the host's port 22. Mirrored WSL shares the host's
# ports, so a WSL sshd and the Windows one would both claim 22: mirrored is
# refused while the service runs or is set to start (nat moves to 2222).
function Test-WindowsSshdOn {
    if ($null -ne $TestWinSshd) { return $TestWinSshd }
    $s = Get-Service sshd -ErrorAction SilentlyContinue
    return [bool]($s -and ($s.Status -eq 'Running' -or "$($s.StartType)" -ne 'Disabled'))
}

$MirroredSshdHint = "the Windows OpenSSH service (sshd) is running or enabled, and mirrored networking would put WSL's sshd on the same port 22. Choose nat (it listens on 2222), or stop and disable it: Stop-Service sshd; Set-Service sshd -StartupType Disabled"

# v4tov4 rows: { ListenAddr, ListenPort, ConnectAddr, ConnectPort }
function Get-PortProxyRules {
    $rows = @()
    foreach ($line in (netsh interface portproxy show v4tov4)) {
        if ($line -match '^\s*(\S+)\s+(\d+)\s+(\S+)\s+(\d+)\s*$') {
            $rows += [pscustomobject]@{ ListenAddr = $Matches[1]; ListenPort = [int]$Matches[2]
                                        ConnectAddr = $Matches[3]; ConnectPort = [int]$Matches[4] }
        }
    }
    return , $rows
}

function Get-WslRule($rules, [int]$Port) {
    @($rules | Where-Object { $_.ListenAddr -eq '0.0.0.0' -and $_.ListenPort -eq $Port -and $_.ConnectPort -eq 22 }) | Select-Object -First 1
}

# present | absent | unknown — runs a Get-* by name; "not found" is absent,
# any other failure (access denied when not elevated) is unknown.
function Get-ObjectState([scriptblock]$Get) {
    try { $null = & $Get; return 'present' }
    catch {
        if ("$($_.CategoryInfo.Category)" -eq 'ObjectNotFound') { return 'absent' }
        return 'unknown'
    }
}

function Format-State([string]$s) { if ($s -eq 'unknown') { 'unknown (needs admin)' } else { $s } }

function Get-WslIp {
    try {
        . (Join-Path $PSScriptRoot 'lib\wsl.ps1')
        $ip = ((& wsl.exe -d $WslDistro -- hostname -I) -split '\s+')[0]
        if ($ip -match '^\d+\.\d+\.\d+\.\d+$') { return $ip }
    } catch { }
    return $null
}

function Invoke-Refresh([string[]]$Arguments) {
    if (-not (Test-Path -LiteralPath $RefreshScript)) {
        Write-Log "ERROR: $RefreshScript not found"; return $false
    }
    # Out-Host: the child's output goes to the operator, not into this
    # function's return value (an array of strings + a bool is always truthy).
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $RefreshScript @Arguments | Out-Host
    return ($LASTEXITCODE -eq 0)
}

function Ensure-MirroredRule {
    if (-not (Get-Command New-NetFirewallHyperVRule -ErrorAction SilentlyContinue)) {
        Write-Warning "Hyper-V firewall cmdlets are not on this Windows build; allow inbound TCP 22 to WSL yourself"
        return
    }
    if (Get-NetFirewallHyperVRule -Name $HvRuleName -ErrorAction SilentlyContinue) {
        Write-Host "Hyper-V firewall rule '$HvRuleName' already present"
        return
    }
    New-NetFirewallHyperVRule -Name $HvRuleName -DisplayName 'WSL sshd (mirrored)' -Direction Inbound `
        -VMCreatorId $WslVmCreatorId -Protocol TCP -LocalPorts 22 -Action Allow | Out-Null
    Write-Log "Hyper-V firewall rule '$HvRuleName' created (inbound TCP 22 to WSL)"
}

function Remove-MirroredRule {
    if (-not (Get-Command Get-NetFirewallHyperVRule -ErrorAction SilentlyContinue)) { return }
    if (Get-NetFirewallHyperVRule -Name $HvRuleName -ErrorAction SilentlyContinue) {
        Remove-NetFirewallHyperVRule -Name $HvRuleName
        Write-Log "removed Hyper-V firewall rule '$HvRuleName'"
    }
}

# Test-SameSubnet A B LEN - A and B share the first LEN bits (IPv4).
function Test-SameSubnet([string]$A, [string]$B, [int]$Len) {
    try {
        $x = [BitConverter]::ToUInt32(([Net.IPAddress]::Parse($A)).GetAddressBytes()[3..0], 0)
        $y = [BitConverter]::ToUInt32(([Net.IPAddress]::Parse($B)).GetAddressBytes()[3..0], 0)
    } catch { return $false }
    if ($Len -le 0) { return $true }
    $mask = [uint32]([math]::Pow(2, 32) - [math]::Pow(2, 32 - [math]::Min($Len, 32)))
    return (($x -band $mask) -eq ($y -band $mask))
}

# A forward is WSL's when it targets the distro's current address or the WSL
# virtual switch's subnet (the address moves across WSL restarts). Any other
# 0.0.0.0:22/2222 forward belongs to someone else and is left alone.
function Test-WslTarget([string]$Addr) {
    $ip = Get-WslIp
    if ($ip -and $Addr -eq $ip) { return $true }
    foreach ($a in @(Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
                     Where-Object { $_.InterfaceAlias -like 'vEthernet (WSL*' })) {
        if (Test-SameSubnet $Addr $a.IPAddress $a.PrefixLength) { return $true }
    }
    return $false
}

function Remove-NatLeftovers {
    if (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) {
        if (-not (Invoke-Refresh @('-UnregisterTask'))) { Write-Warning "could not unregister '$TaskName'" }
    }
    $rules = Get-PortProxyRules
    foreach ($p in $NatPorts) {
        $r = Get-WslRule $rules $p
        if ($r -and -not (Test-WslTarget $r.ConnectAddr)) {
            # Its firewall rule opens the port for that forward too: keep both.
            Write-Log "left portproxy 0.0.0.0:$p -> $($r.ConnectAddr):22 alone (not a WSL address)"
            continue
        }
        if ($r) {
            netsh interface portproxy delete v4tov4 listenport=$p listenaddress=0.0.0.0 | Out-Null
            Write-Log "removed portproxy 0.0.0.0:$p -> WSL:22"
        }
        if (Get-NetFirewallRule -Name "WSL-SSH-$p" -ErrorAction SilentlyContinue) {
            Remove-NetFirewallRule -Name "WSL-SSH-$p"
            Write-Log "removed firewall rule 'WSL-SSH-$p'"
        }
    }
}

function Show-Config($cfg) {
    if (-not $cfg.Exists) { Write-Host ".wslconfig: $WslConfigPath (missing)"; return }
    Write-Host ".wslconfig: $WslConfigPath"
    $any = $false
    foreach ($k in 'networkingMode', 'vmSwitch', 'macAddress', 'dhcp') {
        $v = Get-KeyValue $cfg 'wsl2' $k
        if ($null -ne $v) { Write-Host "  [wsl2] $k=$v"; $any = $true }
    }
    $x = Get-KeyValue $cfg 'experimental' 'networkingMode'
    if ($null -ne $x) { Write-Host "  [experimental] networkingMode=$x"; $any = $true }
    if (-not $any) { Write-Host '  (no networking keys: WSL default nat)' }
}

# ---------------------------------------------------------------------------
# -Status
# ---------------------------------------------------------------------------
if ($Status) {
    $cfg = Read-WslConfig $WslConfigPath
    Write-Host "mode: $Mode (requested)"
    Show-Config $cfg
    if ($Mode -eq 'off') { Write-Host 'status: current (off is hands-off)'; exit 0 }
    if ($Mode -eq 'mirrored' -and (Test-WindowsSshdOn)) { Write-Host "status: cannot apply mirrored: $MirroredSshdHint"; exit 4 }

    $why = @()
    $plan = Get-ConfigPlan $cfg
    if ($plan.Changed) { $why += ".wslconfig ($($plan.Before) -> $($plan.After))" }
    if ($plan.NoSwitch) {
        if ($plan.SwitchState -eq 'unknown') {
            # Can't see it unelevated: let the elevated apply decide (it writes
            # the config when the switch exists, else prints this hint).
            Write-Host "switch: $SwitchName unknown (needs admin); the elevated apply will check it. If missing, create it with:"
            $why += "switch $SwitchName unknown (needs admin)"
        } else {
            Write-Host "switch: no External Hyper-V switch '$SwitchName'; create it (elevated), then re-run install.sh:"
        }
        Write-Host "  $(Get-NewVMSwitchHint)"
    }

    if (-not $ConfigOnly) {
        $rules = Get-PortProxyRules
        $natRows = @($NatPorts | ForEach-Object { Get-WslRule $rules $_ } | Where-Object { $_ })
        if ($natRows.Count -eq 0) { Write-Host 'portproxy: none to WSL' }
        foreach ($r in $natRows) { Write-Host "portproxy: $($r.ListenAddr):$($r.ListenPort) -> $($r.ConnectAddr):$($r.ConnectPort)" }
        $task = Get-ObjectState { Get-ScheduledTask -TaskName $TaskName -ErrorAction Stop }
        Write-Host "task: $TaskName $(Format-State $task)"
        $fw = @{}
        foreach ($p in $NatPorts) {
            $fw[$p] = Get-ObjectState { Get-NetFirewallRule -Name "WSL-SSH-$p" -ErrorAction Stop }
            Write-Host "firewall: WSL-SSH-$p $(Format-State $fw[$p])"
        }
        $hv = 'unsupported'
        if (Get-Command Get-NetFirewallHyperVRule -ErrorAction SilentlyContinue) {
            $hv = Get-ObjectState { Get-NetFirewallHyperVRule -Name $HvRuleName -ErrorAction Stop }
        }
        Write-Host "firewall: $HvRuleName (Hyper-V) $(Format-State $hv)"

        switch ($Mode) {
            'nat' {
                $p = Get-NatPort
                $rule = Get-WslRule $rules $p
                if (-not $rule) { $why += "portproxy :$p" }
                else {
                    $ip = Get-WslIp
                    if ($ip -and $rule.ConnectAddr -ne $ip) { $why += "portproxy :$p stale ($($rule.ConnectAddr) -> $ip)" }
                }
                if ($fw[$p] -eq 'absent') { $why += "firewall WSL-SSH-$p" }
                if ($task -eq 'absent') { $why += "task $TaskName" }
            }
            'mirrored' { if ($hv -eq 'absent') { $why += "firewall $HvRuleName" } }
        }
    }

    if ($why.Count -gt 0) { Write-Host "status: would change: $($why -join ', ')"; exit 3 }
    if ($plan.NoSwitch) { Write-Host "status: nothing to apply for '$Mode' until the switch exists" }
    else { Write-Host "status: current for '$Mode'" }
    exit 0
}

# ---------------------------------------------------------------------------
# -Mode (apply)
# ---------------------------------------------------------------------------
if ($Mode -eq 'off') { Write-Host 'wsl-network: off - hands off, nothing changed'; exit 0 }
if ($Mode -eq 'mirrored' -and (Test-WindowsSshdOn)) { Write-Log "ERROR: not applying mirrored: $MirroredSshdHint"; exit 4 }
if ($TestRefresh) {
    Write-Host "test: refresh script $RefreshScript (admin check and cleanup skipped)"
} elseif (-not $ConfigOnly -and -not (Test-Admin)) {
    Write-Host "ERROR: set-wsl-network.ps1 -Mode $Mode needs an elevated PowerShell (Run as administrator)."
    Write-Host '       -Status and -ConfigOnly run without elevation.'
    exit 1
}

$cfg  = Read-WslConfig $WslConfigPath
$plan = Get-ConfigPlan $cfg
if ($plan.NoSwitch) {
    Write-Host "bridged needs an External Hyper-V switch named '$SwitchName'; none found, so nothing was changed."
    Write-Host 'Create it from an elevated PowerShell, then re-run install.sh:'
    Write-Host "  $(Get-NewVMSwitchHint)"
    exit 0
}
if ($plan.Changed) {
    Write-WslConfig $WslConfigPath $plan.Bytes
    Write-Log ".wslconfig networking set for '$Mode' (was $($plan.Before), now $($plan.After))"
    Write-Log 'restart WSL to apply: wsl --shutdown'
} else {
    Write-Host ".wslconfig already current for '$Mode'"
}
if ($ConfigOnly) { exit 0 }

$ok = $true
switch ($Mode) {
    'nat'      { $ok = Invoke-Refresh @('-ListenPort', "$(Get-NatPort)", '-RegisterTask') }
    'mirrored' { Ensure-MirroredRule }
}

# Clean up the previous mode only when this run switched networkingMode.
if ($plan.Before -ne $plan.After -and -not $TestRefresh) {
    switch ($Mode) {
        'nat'      { Remove-MirroredRule }
        'bridged'  { Remove-NatLeftovers }
        'mirrored' { Remove-NatLeftovers }
    }
}
if (-not $ok) { Write-Log "ERROR: applying '$Mode' failed"; exit 1 }
exit 0
