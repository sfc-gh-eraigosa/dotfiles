<#
.SYNOPSIS
    Restore the normal Windows AC system-sleep timeout after a keep-awake session.
    Run by the "Claude - Revert keep-awake" scheduled task, or manually.

.NOTES
    Without -Quiet, shows a Yes/No dialog and only reverts on confirmation.
#>
[CmdletBinding()]
param(
    # AC sleep timeout to restore, in minutes.
    [int]$Minutes = 5,

    # Also restore the battery (DC) timeout to the same value.
    [switch]$IncludeBattery,

    # Revert immediately without prompting.
    [switch]$Quiet
)

$ErrorActionPreference = 'Stop'

function Restore-Sleep {
    powercfg /change standby-timeout-ac $Minutes
    if ($IncludeBattery) { powercfg /change standby-timeout-dc $Minutes }
}

if ($Quiet) {
    Restore-Sleep
    Write-Host "AC system sleep restored to $Minutes minutes."
    return
}

Add-Type -AssemblyName System.Windows.Forms
$msg = "Keep-awake is still active (system sleep disabled on AC).`n`nRestore normal sleep ($Minutes min on AC) now?"
$result = [System.Windows.Forms.MessageBox]::Show(
    $msg,
    'Claude reminder: revert keep-awake',
    [System.Windows.Forms.MessageBoxButtons]::YesNo,
    [System.Windows.Forms.MessageBoxIcon]::Question)

if ($result -eq [System.Windows.Forms.DialogResult]::Yes) {
    Restore-Sleep
    [System.Windows.Forms.MessageBox]::Show(
        "Done. AC system sleep restored to $Minutes minutes.",
        'Claude reminder',
        [System.Windows.Forms.MessageBoxButtons]::OK,
        [System.Windows.Forms.MessageBoxIcon]::Information) | Out-Null
}
