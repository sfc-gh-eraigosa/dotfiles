# shellcheck shell=bash
# winpowershell.sh — locate powershell.exe from WSL. Sourced by
# opt/bin/install_windows.sh and sdk/gsl/scripts/install_nerd_font_linux.sh.
#
# Usage: ps_exe="$(find_powershell)" || <not found>
#   Prints the path of a usable powershell.exe and returns 0, or prints nothing
#   and returns 1.
#
# Checks PATH first, then the standard System32 location that wslpath derives
# from the Windows path. The fallback is needed because Windows exes are often
# not on the WSL PATH: appendWindowsPath=false in wsl.conf, or an ssh session
# (fleet runs) whose PATH never gets the /mnt/c dirs, even though powershell.exe
# is there and works. Off WSL there is no wslpath, so it returns 1.
#
# POSIX sh only (no `local`, no arrays): the helper may be sourced from any shell.
find_powershell() {
  _wps_exe="$(command -v powershell.exe 2>/dev/null || true)"
  if [ -z "$_wps_exe" ] && command -v wslpath >/dev/null 2>&1; then
    _wps_exe="$(wslpath -u 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe' 2>/dev/null || true)"
    if [ -z "$_wps_exe" ] || [ ! -x "$_wps_exe" ]; then _wps_exe=""; fi
  fi
  if [ -n "$_wps_exe" ]; then
    printf '%s\n' "$_wps_exe"
    unset _wps_exe
    return 0
  fi
  unset _wps_exe
  return 1
}
