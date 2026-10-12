# wsl.ps1 — the ONE WSL distro this machine runs.
#
# The giga workstation is a k3s node, and its node lives in this distro: the
# k3s-agent, the node-local ollama and sshd all run here, so ssh, fleet and
# every script below land in the same place. A second distro split them once
# (2026-10-11: sshd and claude-remote in 'Ubuntu', the k3s node in
# 'Ubuntu-22.04'), and remote work kept touching the wrong one.
#
# Usage:  . (Join-Path $PSScriptRoot 'lib\wsl.ps1')      # sets $WslDistro
$WslDistro = 'Ubuntu-22.04'
