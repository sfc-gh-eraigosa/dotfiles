#!/usr/bin/env bash
# =============================================================================
# wsl-distro_test.sh - test driver for "the one WSL distro" resolution in
# opt/Desktop/Apps/scripts/lib/wsl.ps1 and the install_windows.sh side that
# feeds it (DOTFILES_WSL_DISTRO via WSLENV + the deployed wsl-distro.txt).
#
#   STATIC (always, incl. Linux CI) - the resolution order and the installer
#     hand-off are present in source.
#   LIVE (only when powershell.exe is resolvable - WSL dev) - dot-source a copy
#     of wsl.ps1 from a Windows temp dir and from the repo's UNC path, and
#     assert each tier wins in order. SKIPs (not fails) without PowerShell.
#
# Run standalone: bash opt/scripts/system/wsl-distro_test.sh
# =============================================================================
set -u

SCRIPT_DIR=$(cd -- "$(dirname "$0")" && pwd -P)
REPO_ROOT=$(cd -- "${SCRIPT_DIR}/../../.." && pwd -P)
. "${REPO_ROOT}/ai/_test_helpers.sh"

WSL_PS1="${REPO_ROOT}/opt/Desktop/Apps/scripts/lib/wsl.ps1"
INSTALL_WIN="${REPO_ROOT}/opt/bin/install_windows.sh"

echo "==== STATIC ===="
assert_grep "wsl.ps1 reads DOTFILES_WSL_DISTRO"   'env:DOTFILES_WSL_DISTRO'           "$WSL_PS1"
assert_grep "wsl.ps1 reads its UNC distro"         'wsl\(\?:'                          "$WSL_PS1"
assert_grep "wsl.ps1 reads wsl-distro.txt"         "'wsl-distro\.txt'"                 "$WSL_PS1"
assert_grep "wsl.ps1 keeps the fresh-box fallback" "return 'Ubuntu-22\.04'"            "$WSL_PS1"
assert_grep_negative "no bare hardcoded assignment" "^\\\$WslDistro = '"               "$WSL_PS1"
assert_grep "installer forwards DOTFILES_WSL_DISTRO" 'DOTFILES_WSL_DISTRO/w'           "$INSTALL_WIN"
assert_grep "installer writes the deploy marker"   'Apps/scripts/lib/wsl-distro\.txt'  "$INSTALL_WIN"
# shellcheck disable=SC2016  # literal PowerShell $fwName, not a shell expansion
assert_grep "portproxy refresh opens its firewall port" 'New-NetFirewallRule -Name \$fwName' \
  "${REPO_ROOT}/opt/Desktop/Apps/scripts/refresh-wsl-portproxy.ps1"

echo "==== LIVE ===="
PS_EXE=$(command -v powershell.exe 2>/dev/null || true)
if [ -z "$PS_EXE" ] || ! command -v wslpath >/dev/null 2>&1; then
  echo "SKIP: powershell.exe/wslpath not available"
  _test_report
fi

# resolve <dir-holding-wsl.ps1, Windows form> [env assignments...]
# Prints the $WslDistro that dot-sourcing wsl.ps1 from that dir yields.
resolve() {
  _dir=$1; shift
  env -u DOTFILES_WSL_DISTRO "$@" "$PS_EXE" -NoProfile -NonInteractive -ExecutionPolicy Bypass \
    -Command ". '${_dir}\\wsl.ps1'; \$WslDistro" </dev/null 2>/dev/null | tr -d '\r'
}

# shellcheck disable=SC2016  # $env:TEMP is PowerShell syntax
WIN_TEMP=$(wslpath -u "$("$PS_EXE" -NoProfile -Command '$env:TEMP' </dev/null | tr -d '\r')")
TMP_LIB=$(mktemp -d "${WIN_TEMP}/wsl-distro-test.XXXXXX")
trap 'rm -rf "$TMP_LIB"' EXIT
cp "$WSL_PS1" "$TMP_LIB/"
TMP_LIB_W=$(wslpath -w "$TMP_LIB")

assert_eq "$(resolve "$TMP_LIB_W")" "Ubuntu-22.04" "no override, no marker -> fallback"

printf 'Marker-Distro\r\n' > "$TMP_LIB/wsl-distro.txt"
assert_eq "$(resolve "$TMP_LIB_W")" "Marker-Distro" "deploy marker wins over fallback"

assert_eq "$(resolve "$TMP_LIB_W" DOTFILES_WSL_DISTRO=Env-Distro WSLENV=DOTFILES_WSL_DISTRO/w)" \
  "Env-Distro" "DOTFILES_WSL_DISTRO wins over marker"

printf 'bad name; rm -rf\r\n' > "$TMP_LIB/wsl-distro.txt"
assert_eq "$(resolve "$TMP_LIB_W")" "Ubuntu-22.04" "invalid marker is ignored"

if [ -n "${WSL_DISTRO_NAME:-}" ]; then
  REPO_LIB_W=$(wslpath -w "$(dirname "$WSL_PS1")")
  assert_eq "$(resolve "$REPO_LIB_W")" "$WSL_DISTRO_NAME" "UNC path -> the distro it lives in"
else
  echo "SKIP: UNC tier (WSL_DISTRO_NAME unset)"
fi

_test_report
