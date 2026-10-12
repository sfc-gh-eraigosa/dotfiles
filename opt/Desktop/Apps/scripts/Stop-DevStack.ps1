<#
.SYNOPSIS
  Stop the dev stack: Docker Desktop if it runs, then shut WSL down (the one
  distro, and with it the k3s-agent, ollama and sshd). Frees vmmem entirely.
#>
$env:WSL_UTF8 = '1'
function Say($m) { Write-Host "==> $m" -ForegroundColor Cyan }

$dockerProcs = 'Docker Desktop', 'com.docker.backend', 'com.docker.build', 'com.docker.dev-envs'
if (Get-Process $dockerProcs -ErrorAction SilentlyContinue) {
    Say 'Stopping Docker Desktop'
    Get-Process $dockerProcs -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
}
Say 'wsl --shutdown'
wsl.exe --shutdown
# wsl --shutdown returns before the VM is gone: wait up to 15 s for vmmem to exit.
for ($i = 0; $i -lt 15; $i++) {
    $vm = Get-Process vmmem, vmmemWSL -ErrorAction SilentlyContinue
    if (-not $vm) { break }
    Start-Sleep -Seconds 1
}
if ($vm) { Write-Warning ("vmmem still resident: {0:N0} MB" -f (($vm | Measure-Object WorkingSet64 -Sum).Sum / 1MB)) }
else { Write-Host 'Stack is down.' -ForegroundColor Green }
