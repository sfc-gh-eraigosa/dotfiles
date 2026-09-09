# gff.ps1 — feature-flag gates for the Windows setup scripts.
# Mirrors opt/lib/gff.sh. Flags cross from WSL via WSLENV, built by
# opt/bin/install_windows.sh (export_gff_wslenv) before each powershell.exe
# invocation — with the /w flag: /u is Win32->WSL and was refuted empirically
# (2026-07-26), see the note at that function.
#
# Two gates, opposite directions — pick by the flag's boolDefault:
#   Test-GffOn     fail-OPEN  (boolDefault: true)  — ON unless exactly 'false'.
#   Test-GffOptIn  fail-CLOSED (boolDefault: false) — OFF unless exactly 'true'.
#
# Usage:  . (Join-Path $PSScriptRoot 'lib\gff.ps1')
#         if (Test-GffOn 'install.windows.wispr-flow') { ... }

# ToUpperInvariant, never ToUpper: ToUpper is culture-sensitive, and under a
# Turkish locale 'i' uppercases to 'İ' (U+0130) — the mangled name would then
# never match the GFF_* variable WSLENV actually exported, silently ignoring
# every flag containing an 'i' (i.e. all of install.*).
function Get-GffVarName([string]$Key) {
    return 'GFF_' + ($Key.ToUpperInvariant() -replace '[.-]', '_')
}

function Test-GffOn([string]$Key) {
    $val = [Environment]::GetEnvironmentVariable((Get-GffVarName $Key))
    return $val -ne 'false'   # fail-open: unset/anything-else => on
}

# FAIL-CLOSED mirror of opt/lib/gff.sh gff_opt_in, for opt-in
# (boolDefault: false) steps: ON only when the env var is exactly 'true'.
# No caller today — setup-security-audit.ps1 is deliberately ungated because a
# manual PowerShell run IS the explicit intent (the gate lives at the install.sh
# call site, opt/bin/install_windows.sh run_security_audit_setup). Kept here so
# the PS gate library stays a faithful mirror of the POSIX one.
function Test-GffOptIn([string]$Key) {
    return ([Environment]::GetEnvironmentVariable((Get-GffVarName $Key)) -eq 'true')
}
