#Requires -RunAsAdministrator
<#
.SYNOPSIS
  Set a classic UltraVNC password and switch VNC to password auth (disabling
  MS-Logon / Windows authentication). Companion to setup-sshd.ps1.

.DESCRIPTION
  On machines whose Windows sign-in is a passwordless Microsoft account
  (PIN / Windows Hello only), VNC's MS-Logon and RDP's NLA cannot work —
  there is no account password to authenticate against. This script puts
  VNC back on a classic, self-chosen VNC password using UltraVNC's own
  setpasswd.exe (which writes the correct ini encoding), then flips the
  auth mode in ultravnc.ini and restarts the service.

.PARAMETER Pw
  The VNC password to set (only the first 8 chars are significant, per the
  VNC protocol). If omitted, you are prompted securely.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File set-vnc-password.ps1
#>
param([string]$Pw)
$ErrorActionPreference = 'Stop'

$dir   = 'C:\Program Files\uvnc bvba\UltraVNC'
$ini   = Join-Path $dir 'ultravnc.ini'
$setpw = Join-Path $dir 'setpasswd.exe'
if (-not (Test-Path $setpw)) { throw "setpasswd.exe not found at $setpw (is UltraVNC installed?)" }
if (-not (Test-Path $ini))   { throw "ultravnc.ini not found at $ini" }

if (-not $Pw) {
    $sec  = Read-Host 'New VNC password (first 8 chars used)' -AsSecureString
    $cred = [System.Management.Automation.PSCredential]::new('vnc', $sec)
    $Pw   = $cred.GetNetworkCredential().Password
}
if ([string]::IsNullOrEmpty($Pw)) { throw 'Empty password; aborting.' }

Copy-Item $ini "$ini.bak" -Force

# 1. Set the VNC password via UltraVNC's own encoder (writes correct ini format)
& $setpw $Pw | Out-Null
if ($LASTEXITCODE -ne 0) { throw "setpasswd.exe failed (rc=$LASTEXITCODE)" }

# 2. Switch auth mode: classic VNC password ON, MS-Logon OFF; mirror the
#    freshly-written control password to the view-only slot (passwd2).
$c = Get-Content $ini -Raw
function Set-Key($t, $k, $v) {
    if ($t -match "(?m)^$k=") { return ($t -replace "(?m)^$k=.*", "$k=$v") }
    return ($t -replace '(?m)^\[ultravnc\]', "[ultravnc]`r`n$k=$v")
}
$newpw = ((Get-Content $ini | Select-String '^passwd=') -split '=', 2)[1]
$c = Set-Key $c 'MSLogonRequired' '0'
$c = Set-Key $c 'NewMSLogon'      '0'
$c = Set-Key $c 'AuthRequired'    '1'
if ($newpw) { $c = Set-Key $c 'passwd2' $newpw }
Set-Content $ini $c -Encoding Ascii

# 3. Restart the service to apply
Restart-Service uvnc_service -Force
Start-Sleep -Seconds 2
Write-Host "VNC password set; MS-Logon disabled; uvnc_service = $((Get-Service uvnc_service).Status)"
Write-Host "Connect any VNC viewer to <host>:5900 and enter this password. Rollback: restore $ini.bak"
