<#
.SYNOPSIS
  Start the dev stack: boot the one WSL distro (its systemd brings up the
  k3s-agent, ollama and sshd) and open claude-remote in Windows Terminal.
.EXAMPLE
  .\Start-DevStack.ps1                  # the distro + claude-remote
  .\Start-DevStack.ps1 -NoClaudeRemote  # just the distro
  .\Start-DevStack.ps1 -Docker          # also Docker Desktop (k3s does not need it)
#>
param([switch]$NoClaudeRemote, [switch]$Docker)
$env:WSL_UTF8 = '1'
. (Join-Path $PSScriptRoot 'lib\wsl.ps1')
function Say($m) { Write-Host "==> $m" -ForegroundColor Cyan }

Say "Booting $WslDistro"
wsl.exe -d $WslDistro -- true
foreach ($unit in 'k3s-agent', 'ollama', 'ssh') {
    $state = (wsl.exe -d $WslDistro -- systemctl is-active $unit 2>$null) -join ''
    Write-Host ("    {0,-10} {1}" -f $unit, $state)
}
if ($Docker) {
    if (Get-Process 'Docker Desktop' -ErrorAction SilentlyContinue) { Say 'Docker Desktop already running' }
    else { Say 'Starting Docker Desktop'; Start-Process 'C:\Program Files\Docker\Docker\Docker Desktop.exe' }
}
if (-not $NoClaudeRemote) {
    Say "Opening claude-remote in $WslDistro"
    Start-Process wt.exe -ArgumentList "wsl.exe -d $WslDistro -- bash -lc `"exec ~/.local/bin/claude-rc-boot gigabyte`""
}
Say 'Done'
