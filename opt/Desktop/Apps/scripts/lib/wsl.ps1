# wsl.ps1 — the ONE WSL distro this machine runs.
#
# The giga workstation is a k3s node, and its node lives in this distro: the
# k3s-agent, the node-local ollama and sshd all run here, so ssh, fleet and
# every script below land in the same place. A second distro split them once
# (2026-10-11: sshd and claude-remote in 'Ubuntu', the k3s node in
# 'Ubuntu-22.04'), and remote work kept touching the wrong one.
#
# The name is not the same on every machine (giga runs 'Ubuntu-22.04', the
# laptop 'Ubuntu'), so it is resolved, first hit wins:
#
#   1. $env:DOTFILES_WSL_DISTRO   explicit override; install_windows.sh sets it
#                                 (via WSLENV) to the distro install.sh runs in
#   2. this file's UNC path       run straight from the repo inside WSL
#                                 (\\wsl.localhost\<distro>\... or \\wsl$\<distro>\...)
#   3. wsl-distro.txt beside it   written by install_windows.sh when it deploys
#                                 the Desktop copy, so hand runs and scheduled
#                                 tasks keep targeting the install.sh distro
#   4. 'Ubuntu-22.04'             fresh machine, before install.sh has ever run
#
# Usage:  . (Join-Path $PSScriptRoot 'lib\wsl.ps1')      # sets $WslDistro
function Resolve-WslDistro {
    $valid = '^[A-Za-z0-9._-]+$'
    if ($env:DOTFILES_WSL_DISTRO -match $valid) { return $env:DOTFILES_WSL_DISTRO }
    if ($PSScriptRoot -match '^\\\\wsl(?:\.localhost|\$)\\([^\\]+)\\') {
        $name = $Matches[1]   # copy first: the next -match replaces $Matches
        if ($name -match $valid) { return $name }
    }
    $marker = Join-Path $PSScriptRoot 'wsl-distro.txt'
    if (Test-Path -LiteralPath $marker) {
        $name = ([string](Get-Content -LiteralPath $marker -TotalCount 1)).Trim()
        if ($name -match $valid) { return $name }
    }
    return 'Ubuntu-22.04'
}
$WslDistro = Resolve-WslDistro
