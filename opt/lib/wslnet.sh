# shellcheck shell=sh
# wslnet.sh — detect / recommend / ask for the install.windows.wsl-network gff
# flag (POSIX/dash-safe). Needs opt/lib/gff.sh (gff_cli, gff_user_set,
# gff_opt_in) sourced first. Read-only: nothing here changes Windows or
# .wslconfig; it only records the mode in gff.
# Test seams: WSLNET_PROC_VERSION, WSLNET_WSLCONFIG, WSLNET_USERS_DIR
# (/mnt/c/Users), WSLNET_NETSH (netsh.exe), WSLNET_PS (powershell.exe),
# WSLNET_K3S_BIN (/usr/local/bin/k3s), WSLNET_ANSWER, WSLNET_TTY (/dev/tty).
# Every powershell.exe/netsh.exe call gets </dev/null: under WSL interop they
# otherwise consume the parent's stdin.

wslnet_is_wsl() {
  grep -qi microsoft "${WSLNET_PROC_VERSION:-/proc/version}" 2>/dev/null
}

_wslnet_ps_ok() {
  command -v "${WSLNET_PS:-powershell.exe}" >/dev/null 2>&1
}

# _wslnet_ps COMMAND - run a PowerShell snippet, CRs stripped.
_wslnet_ps() {
  "${WSLNET_PS:-powershell.exe}" -NoProfile -Command "$1" </dev/null 2>/dev/null | tr -d '\r'
}

# wslnet_wslconfig - echo the .wslconfig path (may not exist yet) or nothing.
wslnet_wslconfig() {
  if [ -n "${WSLNET_WSLCONFIG:-}" ]; then
    echo "${WSLNET_WSLCONFIG}"
    return 0
  fi
  if _wslnet_ps_ok; then
    # shellcheck disable=SC2016 # $env:USERPROFILE is PowerShell, not shell
    _wc_prof="$(_wslnet_ps '$env:USERPROFILE')"
    if [ -n "${_wc_prof}" ] && command -v wslpath >/dev/null 2>&1; then
      _wc_unix="$(wslpath -u "${_wc_prof}" 2>/dev/null)"
      [ -n "${_wc_unix}" ] && echo "${_wc_unix}/.wslconfig"
    fi
    return 0
  fi
  # No interop: only an unambiguous single candidate is trusted.
  _wc_n=0
  _wc_hit=""
  for _wc_f in "${WSLNET_USERS_DIR:-/mnt/c/Users}"/*/.wslconfig; do
    [ -f "${_wc_f}" ] || continue
    _wc_n=$((_wc_n + 1))
    _wc_hit="${_wc_f}"
  done
  [ "${_wc_n}" -eq 1 ] && echo "${_wc_hit}"
  return 0
}

# _wslnet_file_mode FILE - echo the networkingMode value (lowercase) found in
# [wsl2] (preferred) or [experimental]. CRLF and a UTF-8 BOM are tolerated.
_wslnet_file_mode() {
  _fm_bom="$(printf '\357\273\277')"
  tr -d '\r' < "$1" | sed "1s/^${_fm_bom}//" | awk '
    function trim(s) { gsub(/^[ \t]+|[ \t]+$/, "", s); return s }
    /^[ \t]*\[/ { sec = tolower(trim($0)); next }
    /^[ \t]*[#;]/ { next }
    {
      i = index($0, "=")
      if (i == 0) next
      if (tolower(trim(substr($0, 1, i - 1))) != "networkingmode") next
      v = tolower(trim(substr($0, i + 1)))
      if (sec == "[wsl2]") w = v
      else if (sec == "[experimental]") e = v
    }
    END { if (w != "") print w; else if (e != "") print e }'
}

# wslnet_detect - echo bridged|mirrored|nat, or nothing when no setup is found.
wslnet_detect() {
  _wd_cfg="$(wslnet_wslconfig)"
  if [ -n "${_wd_cfg}" ] && [ -f "${_wd_cfg}" ]; then
    case "$(_wslnet_file_mode "${_wd_cfg}")" in
      bridged)  echo bridged; return 0 ;;
      mirrored) echo mirrored; return 0 ;;
      nat)      echo nat; return 0 ;;
    esac
  fi
  if command -v "${WSLNET_NETSH:-netsh.exe}" >/dev/null 2>&1; then
    if "${WSLNET_NETSH:-netsh.exe}" interface portproxy show v4tov4 </dev/null 2>/dev/null \
        | tr -d '\r' | awk '$2 == "22" && $1 ~ /^[0-9]+\./ { f = 1 } END { exit !f }'; then
      echo nat
      return 0
    fi
  fi
  if _wslnet_ps_ok; then
    if [ -n "$(_wslnet_ps "if (Get-ScheduledTask -TaskName RefreshWslPortProxy -ErrorAction SilentlyContinue) {'yes'}")" ]; then
      echo nat
      return 0
    fi
  fi
  return 0
}

# wslnet_recommend - bridged when k3s is present and a wired adapter is up
# (a NAT address changes on every WSL restart, which breaks cluster nodes).
wslnet_recommend() {
  _wr_k3s=""
  if [ -x "${WSLNET_K3S_BIN:-/usr/local/bin/k3s}" ]; then
    _wr_k3s=1
  elif command -v systemctl >/dev/null 2>&1; then
    systemctl is-enabled k3s >/dev/null 2>&1 && _wr_k3s=1
    systemctl is-enabled k3s-agent >/dev/null 2>&1 && _wr_k3s=1
  fi
  if [ -n "${_wr_k3s}" ] && _wslnet_ps_ok; then
    _wr_wired="$(_wslnet_ps "Get-NetAdapter -Physical | Where-Object { \$_.Status -eq 'Up' -and \$_.PhysicalMediaType -notmatch '802\\.11|Wireless' } | ForEach-Object { \$_.Name }")"
    if [ -n "${_wr_wired}" ]; then
      echo bridged
      return 0
    fi
  fi
  echo nat
}

_wslnet_valid() {
  case "$1" in
    skip|off|nat|bridged|mirrored) return 0 ;;
  esac
  return 1
}

# wslnet_ask REC - echo off|nat|bridged|mirrored|skip. Prompt goes to stderr.
wslnet_ask() {
  _wa_rec="${1:-nat}"
  if _wslnet_valid "${WSLNET_ANSWER:-}"; then
    echo "${WSLNET_ANSWER}"
    return 0
  fi
  _wa_tty="${WSLNET_TTY:-/dev/tty}"
  if ! ( : < "${_wa_tty}" ) 2>/dev/null; then
    echo skip
    return 0
  fi
  exec 3< "${_wa_tty}"  # one open, so a file/pipe stand-in is consumed line by line
  _wa_try=0
  while [ "${_wa_try}" -lt 2 ]; do
    _wa_try=$((_wa_try + 1))
    {
      echo "How should this Windows host expose its WSL sshd to the LAN?"
      for _wa_m in off nat bridged mirrored; do
        if [ "${_wa_m}" = "${_wa_rec}" ]; then
          echo "  ${_wa_m} (recommended)"
        else
          echo "  ${_wa_m}"
        fi
      done
      printf 'Mode [%s, or skip]: ' "${_wa_rec}"
    } >&2
    read -r _wa_ans <&3 || _wa_ans=""
    [ -z "${_wa_ans}" ] && _wa_ans="${_wa_rec}"
    if _wslnet_valid "${_wa_ans}"; then
      exec 3<&-
      echo "${_wa_ans}"
      return 0
    fi
    echo "Not a valid mode: ${_wa_ans}" >&2
  done
  exec 3<&-
  echo skip
}

# _wslnet_record MODE - set the flag; export for the rest of this install run.
_wslnet_record() {
  if gff_cli set install.windows.wsl-network "$1" >/dev/null 2>&1; then
    GFF_INSTALL_WINDOWS_WSL_NETWORK="$1"
    export GFF_INSTALL_WINDOWS_WSL_NETWORK
    return 0
  fi
  echo "WARNING: could not record install.windows.wsl-network in gff (retrying next run)."
  return 1
}

# wslnet_sync - always returns 0. Records an existing setup silently; asks only
# when sshd is being enabled and no mode is known.
wslnet_sync() {
  wslnet_is_wsl || return 0
  gff_user_set install.windows.wsl-network && return 0
  _ws_mode="$(wslnet_detect)"
  if [ -n "${_ws_mode}" ]; then
    _wslnet_record "${_ws_mode}" \
      && echo "Recorded existing WSL networking: install.windows.wsl-network=${_ws_mode} (nothing changed)."
    return 0
  fi
  gff_opt_in install.network.sshd || return 0
  _ws_ans="$(wslnet_ask "$(wslnet_recommend)")"
  if [ "${_ws_ans}" = "skip" ]; then
    echo "Choose later: gff set install.windows.wsl-network <off|nat|bridged|mirrored>"
    return 0
  fi
  _wslnet_record "${_ws_ans}" \
    && echo "Recorded WSL networking: install.windows.wsl-network=${_ws_ans}."
  return 0
}
