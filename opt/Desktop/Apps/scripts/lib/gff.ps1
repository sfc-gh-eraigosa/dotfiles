# gff.ps1 — fail-open feature-flag gate for the Windows setup scripts.
# Mirrors opt/lib/gff.sh: a key is ON unless its mangled env var is exactly
# the literal string 'false'. Flags cross from WSL via WSLENV (/u), built by
# opt/bin/install_windows.sh before each powershell.exe invocation.
# Usage:  . (Join-Path $PSScriptRoot 'lib\gff.ps1')
#         if (Test-GffOn 'install.windows.wispr-flow') { ... }
function Test-GffOn([string]$Key) {
    $var = 'GFF_' + ($Key.ToUpper() -replace '[.-]', '_')
    $val = [Environment]::GetEnvironmentVariable($var)
    return $val -ne 'false'   # fail-open: unset/anything-else => on
}
