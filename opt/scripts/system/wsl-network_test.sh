#!/usr/bin/env bash
# =============================================================================
# wsl-network_test.sh - test driver for the sshd / WSL-networking gff flags.
#
#   STATIC (always, incl. Linux CI) - the wiring is present in source.
#   LIVE (only when powershell.exe is resolvable - WSL dev) - parse
#     set-wsl-network.ps1 and drive its -ConfigOnly / -Status paths against
#     temp .wslconfig files (never the real one, never a real -Mode apply).
#     SKIPs (not fails) without PowerShell.
#
# Run standalone: bash opt/scripts/system/wsl-network_test.sh
# =============================================================================
set -u

SCRIPT_DIR=$(cd -- "$(dirname "$0")" && pwd -P)
REPO_ROOT=$(cd -- "${SCRIPT_DIR}/../../.." && pwd -P)
. "${REPO_ROOT}/ai/_test_helpers.sh"

INSTALL_SH="${REPO_ROOT}/install.sh"

echo "==== STATIC ===="
# F1: install.sh requires a working gff instead of fail-open.
# shellcheck disable=SC2016  # literal pattern, not a shell expansion
assert_grep "install.sh requires gff" 'gff_require "\$\{BASE_DIR\}" \|\| exit 1' "$INSTALL_SH"
assert_grep_negative "install.sh no longer fails open on a gff build error" \
  'gff build failed; all components will run' "$INSTALL_SH"

# F2: sshd is a gff opt-in flag; ~/.sshd.env is migrated once and no longer read.
assert_grep "install.sh gates sshd on the opt-in flag" 'gff_opt_in install\.network\.sshd' "$INSTALL_SH"
assert_grep "install.sh migrates ~/.sshd.env" '^sshd_migrate$' "$INSTALL_SH"
assert_grep_negative "sshd_run.sh no longer reads ~/.sshd.env" '\.sshd\.env' "${REPO_ROOT}/opt/scripts/network/sshd_run.sh"

# F6: set-wsl-network.ps1 applies the recorded mode on Windows.
SET_NET="${REPO_ROOT}/opt/Desktop/Apps/scripts/set-wsl-network.ps1"
if [ -f "$SET_NET" ]; then
  # Every line naming `wsl --shutdown` must carry it inside a string (a hint
  # printed to the operator), never as a command.
  _bad=$(grep -nE 'wsl(\.exe)?[[:space:]]+--shutdown' "$SET_NET" \
    | grep -vE "[\"'][^\"']*wsl(\.exe)? --shutdown" || true)
  assert_eq "$_bad" "" "set-wsl-network never runs wsl --shutdown"
  _bad=$(grep -nE 'New-VMSwitch' "$SET_NET" | grep -vE "[\"'][^\"']*New-VMSwitch" || true)
  assert_eq "$_bad" "" "set-wsl-network names New-VMSwitch only inside a string"
  # The one config write site comes after the timestamped backup.
  # shellcheck disable=SC2016  # literal PowerShell $bak
  _bak=$(grep -nE 'Copy-Item .*\$bak' "$SET_NET" | head -1 | cut -d: -f1)
  _writes=$(grep -nE 'WriteAll(Bytes|Text)|Set-Content|Out-File' "$SET_NET" | cut -d: -f1)
  _ok=yes
  [ -n "$_bak" ] && [ -n "$_writes" ] || _ok=no
  for _w in $_writes; do [ "${_bak:-0}" -lt "$_w" ] || _ok=no; done
  assert_eq "$_ok" "yes" "set-wsl-network backs .wslconfig up before every write"
else
  assert_file_exists "$SET_NET" "set-wsl-network.ps1 exists"
fi
assert_grep "backup name is .bak-<yyyyMMdd-HHmmss>" "\.bak-\\\$\\(Get-Date -Format 'yyyyMMdd-HHmmss'\\)" "$SET_NET"
assert_grep_negative "no Set-Content (it adds a BOM on PS 5.1)" 'Set-Content' "$SET_NET"
# shellcheck disable=SC2016  # literal PowerShell, not a shell expansion
assert_grep "bridged MAC hashes COMPUTERNAME" 'SHA256.*env:COMPUTERNAME|env:COMPUTERNAME.*SHA256' "$SET_NET"
assert_grep "bridged MAC is locally administered 02:" "'02:'" "$SET_NET"
assert_grep "apply needs admin" 'IsInRole\(\[Security\.Principal\.WindowsBuiltInRole\]::Administrator\)' "$SET_NET"
# shellcheck disable=SC2016  # literal PowerShell, not a shell expansion
assert_grep "nat reuses refresh-wsl-portproxy.ps1" "Join-Path \\\$PSScriptRoot 'refresh-wsl-portproxy\.ps1'" "$SET_NET"
assert_grep "mirrored uses the WSL VMCreatorId" '\{40E0AC32-46A5-438A-A0B2-2B479E8F2E90\}' "$SET_NET"
assert_grep "Get-VMSwitch is probed, guarded" 'Get-Command Get-VMSwitch' "$SET_NET"
# The refresh child's output must not become Invoke-Refresh's return value.
# shellcheck disable=SC2016  # literal PowerShell, not a shell expansion
assert_grep "refresh output goes to the host" '-File \$RefreshScript @Arguments \| Out-Host' "$SET_NET"
assert_grep "test seams need WSLNET_TEST=1" "env:WSLNET_TEST -eq '1'" "$SET_NET"
assert_grep "mirrored is refused while Windows OpenSSH is on" 'Test-WindowsSshdOn)) { Write-Host' "$SET_NET"
# shellcheck disable=SC2016  # literal PowerShell $r, not a shell expansion
assert_grep "NAT cleanup keeps forwards that do not target WSL" 'Test-WslTarget \$r\.ConnectAddr \$targets' "$SET_NET"

# F6 trigger: install.sh runs the deferred --wsl-network step, and it is NOT
# behind the desktop-deploy gate (the mode is recorded independently of it).
assert_grep "install.sh calls install_windows.sh --wsl-network" 'install_windows\.sh" "\$\{BASE_DIR\}" --wsl-network' "$INSTALL_SH"
_gate_end=$(awk '/gff_skip_msg install\.windows\.desktop-deploy; fi/ {n=NR} END {print n}' "$INSTALL_SH")
_call_line=$(grep -n -e '--wsl-network' "$INSTALL_SH" | head -1 | cut -d: -f1)
if [ -n "$_gate_end" ] && [ -n "$_call_line" ] && [ "$_call_line" -gt "$_gate_end" ]; then _r=outside; else _r="gate_end=$_gate_end call=$_call_line"; fi
assert_eq "$_r" "outside" "--wsl-network call is after (outside) the desktop-deploy gate"
assert_grep "install.sh gates the call on the recorded mode" 'GFF_INSTALL_WINDOWS_WSL_NETWORK:-off\}" != off' "$INSTALL_SH"

echo "==== STUBBED (install_windows.sh --wsl-network; stub powershell.exe, never the real one) ===="
IW="${REPO_ROOT}/opt/bin/install_windows.sh"
_ST=$(mktemp -d)
: > "$_ST/tty"   # stand-in terminal for the UAC gate (sr passes it as WSLNET_TTY)
trap 'rm -rf "$_ST"' EXIT
mkdir -p "$_ST/bin" "$_ST/nops" "$_ST/desktop" "$_ST/localappdata" "$_ST/home"
cat > "$_ST/bin/powershell.exe" <<'STUB'
#!/bin/sh
echo "$*" >> "$STUB_LOG"
case "$*" in
  *GetFolderPath*) printf '%s\r\n' "$STUB_DESKTOP" ;;
  *LOCALAPPDATA*) printf '%s\r\n' "$STUB_LOCALAPPDATA" ;;
  *RunAs*) echo "stub elevated"; [ -n "${STUB_ELEV_LOG:-}" ] && printf '%s\r\n' "$STUB_ELEV_LOG" >> "$STUB_LOCALAPPDATA/dotfiles/set-wsl-network.log"; exit "${STUB_ELEV_RC:-0}" ;;
  *-Status*) echo "stub status"; exit "${STUB_STATUS_RC:-0}" ;;
esac
exit 0
STUB
cat > "$_ST/bin/wslpath" <<'STUB'
#!/bin/sh
case "$1" in
  -u) printf '%s\n' "$2" ;;
  -w) printf 'W:%s\n' "$2" ;;
esac
STUB
cp "$_ST/bin/wslpath" "$_ST/nops/wslpath"
chmod +x "$_ST/bin/powershell.exe" "$_ST/bin/wslpath" "$_ST/nops/wslpath"

# sr <mode-or-empty> <status-rc> <elev-rc> [desktop-dir]: run the step; sets OUT, RC, LOG.
sr() {
  : > "$_ST/log"
  OUT=$(env -u GFF_INSTALL_WINDOWS_WSL_NETWORK -u SSH_CONNECTION ${SR_SSH:+SSH_CONNECTION="$SR_SSH"} HOME="$_ST/home" PATH="$_ST/bin:$PATH" \
    STUB_LOG="$_ST/log" STUB_DESKTOP="${4:-$_ST/desktop}" STUB_LOCALAPPDATA="$_ST/localappdata" \
    STUB_ELEV_LOG="${STUB_ELEV_LOG:-}" STUB_STATUS_RC="$2" STUB_ELEV_RC="$3" GFF_INSTALL_WINDOWS_WSL_NETWORK="$1" \
    WSLNET_TTY="${SR_TTY:-$_ST/tty}" WSLNET_STATE_DIR="$_ST/state" \
    bash "$IW" "$REPO_ROOT" "${SR_FLAG:---wsl-network}" 2>&1 </dev/null)
  RC=$?
  LOG=$(cat "$_ST/log")
}
ncalls() { printf '%s' "$LOG" | grep -c . | tr -d ' '; }

sr "" 0 0
assert_eq "$RC/$(ncalls)" "0/0" "mode unset -> exit 0, no powershell call"
sr off 0 0
assert_eq "$RC/$(ncalls)" "0/0" "mode off -> exit 0, no powershell call"

# Desktop copy present: used in place, no LOCALAPPDATA copy.
mkdir -p "$_ST/desktop/Apps/scripts/lib"
cp "${REPO_ROOT}/opt/Desktop/Apps/scripts/set-wsl-network.ps1" "${REPO_ROOT}/opt/Desktop/Apps/scripts/refresh-wsl-portproxy.ps1" "$_ST/desktop/Apps/scripts/"
cp "${REPO_ROOT}/opt/Desktop/Apps/scripts/lib/wsl.ps1" "${REPO_ROOT}/opt/Desktop/Apps/scripts/lib/gff.ps1" "$_ST/desktop/Apps/scripts/lib/"
sr nat 0 0
assert_eq "$RC" "0" "nat, status 0 -> exit 0"
case "$OUT" in *"WSL network 'nat' already current"*) _r=ok ;; *) _r="missing: $OUT" ;; esac
assert_eq "$_r" "ok" "status 0 prints the already-current line"
assert_eq "$(printf '%s' "$LOG" | grep -c -e 'GetFolderPath' -e '-Status')" "2" "status 0: desktop lookup + one -Status call"
assert_eq "$(printf '%s' "$LOG" | grep -c -e RunAs)" "0" "status 0: never elevates"
case "$LOG" in *"-Status -Mode nat"*) _r=ok ;; *) _r="argv: $LOG" ;; esac
assert_eq "$_r" "ok" "the -Status call carries -Mode nat"
assert_eq "$([ -e "$_ST/localappdata/dotfiles" ] && echo copied || echo none)" "none" "Desktop copy present: nothing copied to LOCALAPPDATA"

# rc 3: exactly one elevated run, Mode passed, local path (no UNC), exit code surfaced.
sr nat 3 0
assert_eq "$RC" "0" "nat, status 3, elevation ok -> exit 0"
assert_eq "$(printf '%s' "$LOG" | grep -c -e '-Verb RunAs')" "1" "status 3: one -Verb RunAs call"
case "$LOG" in *"'-Mode','nat'"*) _r=ok ;; *) _r="argv: $LOG" ;; esac
assert_eq "$_r" "ok" "the elevated call passes -Mode nat"
case "$LOG" in *wsl.localhost*|*'\\wsl'*) _r="UNC path: $LOG" ;; *) _r=ok ;; esac
assert_eq "$_r" "ok" "the elevated call never uses a UNC path"
case "$OUT" in *"UAC"*) _r=ok ;; *) _r="no UAC notice: $OUT" ;; esac
assert_eq "$_r" "ok" "a UAC notice is printed before elevating"
sr nat 3 5
assert_eq "$RC" "0" "a failing elevated run does not kill the install (exit 0)"
case "$OUT" in *"exit"*"5"*|*"code 5"*) _r=ok ;; *) _r="rc not surfaced: $OUT" ;; esac
assert_eq "$_r" "ok" "the elevated child's exit code is surfaced"
sr nat 1 0
assert_eq "$(printf '%s' "$LOG" | grep -c -e RunAs)" "0" "status rc 1 (error): no elevation"
# rc 3 with no terminal (a batch fleet wave): never elevate, since nobody can answer UAC.
SR_TTY="$_ST/no-such-tty" sr nat 3 0
assert_eq "$RC" "0" "status 3, no terminal -> exit 0"
assert_eq "$(printf '%s' "$LOG" | grep -c -e RunAs)" "0" "status 3, no terminal: no elevation"
case "$OUT" in *"nobody at this machine's desktop can answer UAC"*) _r=ok ;; *) _r="no no-terminal warning: $OUT" ;; esac
assert_eq "$_r" "ok" "status 3, no terminal: says why and how to re-run"
# rc 3 over ssh (fleet's ssh -t: a terminal, but nobody at the Windows desktop).
SR_SSH="192.0.2.1 50000 192.0.2.2 22" sr nat 3 0
assert_eq "$(printf '%s' "$LOG" | grep -c -e RunAs)" "0" "status 3 over ssh: no elevation"

# A setup install.sh FOUND (wslnet_sync's marker) is reported by the automatic
# run, never changed; a hand-run --wsl-network or a different mode takes it over.
found() { mkdir -p "$_ST/state"; printf '%s\n' "$1" > "$_ST/state/wsl-network-detected"; }
marker() { cat "$_ST/state/wsl-network-detected" 2>/dev/null || echo none; }
found nat; SR_FLAG=--wsl-network-auto sr nat 3 0
assert_eq "$RC/$(printf '%s' "$LOG" | grep -c -e RunAs)" "0/0" "auto, found nat, status 3: no elevation"
case "$OUT" in *"leaves it as it is"*) _r=ok ;; *) _r="no hands-off notice: $OUT" ;; esac
assert_eq "$_r" "ok" "auto, found setup: says it is left alone and how to hand it over"
assert_eq "$(marker)" "nat" "auto, found setup: still marked as found"
SR_FLAG=--wsl-network-auto sr nat 0 0
assert_eq "$(marker)" "nat" "auto, found setup already current: still marked as found"
SR_FLAG=--wsl-network sr nat 3 0
assert_eq "$(printf '%s' "$LOG" | grep -c -e RunAs)" "1" "hand-run --wsl-network on a found setup: elevates"
assert_eq "$(marker)" "none" "hand-run apply succeeded: dotfiles manages it from now on"
found bridged; SR_FLAG=--wsl-network-auto sr nat 3 0
assert_eq "$(printf '%s' "$LOG" | grep -c -e RunAs)" "1" "auto, mode differs from the found one (operator chose): elevates"
assert_eq "$(marker)" "none" "auto, operator chose: the stale found marker is dropped"
found nat; SR_FLAG=--wsl-network sr nat 3 7
assert_eq "$(marker)" "nat" "hand-run apply failed: still marked as found"
rm -f "$_ST/state/wsl-network-detected"
found mirrored; SR_FLAG=--wsl-network-auto sr mirrored 4 0
case "$OUT" in *WARNING*) _r="warned: $OUT" ;; *"left as it is"*) _r=ok ;; *) _r="no notice: $OUT" ;; esac
assert_eq "$_r" "ok" "auto, found setup, status 4: an info line, no warning"
rm -f "$_ST/state/wsl-network-detected"
# rc 4: the mode cannot be applied as the machine is (e.g. mirrored + Windows sshd).
sr mirrored 4 0
assert_eq "$RC/$(printf '%s' "$LOG" | grep -c -e RunAs)" "0/0" "status 4: no elevation"
case "$OUT" in *"cannot be applied"*) _r=ok ;; *) _r="no rc-4 warning: $OUT" ;; esac
assert_eq "$_r" "ok" "status 4: says nothing changed"

# The elevated run's log lines are shown; lines from earlier runs are not.
mkdir -p "$_ST/localappdata/dotfiles"
printf '2026-01-01 00:00:00 stale-line-from-earlier-run\r\n' > "$_ST/localappdata/dotfiles/set-wsl-network.log"
STUB_ELEV_LOG="2026-01-02 00:00:00 restart WSL to apply: wsl --shutdown" sr mirrored 3 0
case "$OUT" in *"restart WSL to apply: wsl --shutdown"*) _r=ok ;; *) _r="restart line not shown: $OUT" ;; esac
assert_eq "$_r" "ok" "a restart line the elevated run logged is printed"
case "$OUT" in *stale-line-from-earlier-run*) _r="stale line printed" ;; *) _r=ok ;; esac
assert_eq "$_r" "ok" "log lines from earlier runs are not printed"
sr mirrored 3 0
case "$OUT" in *"restart WSL"*|*stale-line*) _r="unexpected log lines: $OUT" ;; *) _r=ok ;; esac
assert_eq "$_r" "ok" "no new log lines -> nothing printed from the log"
rm -f "$_ST/localappdata/dotfiles/set-wsl-network.log"
sr mirrored 3 0
assert_eq "$RC" "0" "missing log is tolerated"

# An unknown mode is rejected before it reaches a -Command string.
sr "x';calc" 3 0
assert_eq "$RC/$(ncalls)" "0/0" "unknown mode -> warn, exit 0, no powershell call"

# Desktop copy absent: the script dir lands under %LOCALAPPDATA%\dotfiles\wsl-network\.
mkdir -p "$_ST/desktop2"
sr nat 3 0 "$_ST/desktop2"
_dst="$_ST/localappdata/dotfiles/wsl-network"
_r=ok
for _f in set-wsl-network.ps1 refresh-wsl-portproxy.ps1 lib/wsl.ps1 lib/gff.ps1; do [ -f "$_dst/$_f" ] || _r="missing $_f"; done
assert_eq "$_r" "ok" "no Desktop copy: scripts copied to LOCALAPPDATA/dotfiles/wsl-network (+lib)"
case "$LOG" in *"W:${_dst}/set-wsl-network.ps1"*) _r=ok ;; *) _r="argv: $LOG" ;; esac
assert_eq "$_r" "ok" "the -File path is the LOCALAPPDATA copy"

# No powershell.exe at all: quiet exit 0.
OUT=$(env HOME="$_ST/home" PATH="$_ST/nops:/usr/bin:/bin" GFF_INSTALL_WINDOWS_WSL_NETWORK=nat \
  bash "$IW" "$REPO_ROOT" --wsl-network 2>&1 </dev/null); RC=$?
assert_eq "$RC" "0" "no powershell.exe -> exit 0"

echo "==== LIVE ===="
PS_EXE=$(command -v powershell.exe 2>/dev/null || true)
if [ -z "$PS_EXE" ] || ! command -v wslpath >/dev/null 2>&1 || [ ! -f "$SET_NET" ]; then
  echo "SKIP: powershell.exe/wslpath (or set-wsl-network.ps1) not available"
  _test_report
  exit $?
fi

# shellcheck disable=SC2016  # $env:TEMP is PowerShell syntax
WIN_TEMP=$(wslpath -u "$("$PS_EXE" -NoProfile -Command '$env:TEMP' </dev/null | tr -d '\r')")
TMP_NET=$(mktemp -d "${WIN_TEMP}/wsl-network-test.XXXXXX")
trap 'rm -rf "$TMP_NET"' EXIT
SET_NET_W=$(wslpath -w "$SET_NET")

# Parser: the script must be syntactically clean.
_perr=$("$PS_EXE" -NoProfile -NonInteractive -Command \
  "\$t=\$null; \$e=\$null; [void][System.Management.Automation.Language.Parser]::ParseFile('${SET_NET_W}', [ref]\$t, [ref]\$e); \$e.Count" \
  </dev/null 2>&1 | tr -d '\r')
assert_eq "$_perr" "0" "set-wsl-network.ps1 parses with no errors"

OUT=""
RC=0
# run <case-dir> <args...>: runs the script against <case-dir>/.wslconfig;
# sets OUT (stdout+stderr, \r stripped) and RC.
run() {
  _cfg_w="$(wslpath -w "$1")\\.wslconfig"; shift
  "$PS_EXE" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$SET_NET_W" \
    "$@" -WslConfigPath "$_cfg_w" </dev/null >"$TMP_NET/out" 2>&1
  RC=$?
  OUT=$(tr -d '\r' <"$TMP_NET/out")
}
# fixture <name> <printf-format>: a fresh case dir holding .wslconfig.
fixture() {
  _d="$TMP_NET/$1"; mkdir -p "$_d"
  # shellcheck disable=SC2059  # the format IS the fixture (exact bytes)
  printf "$2" > "$_d/.wslconfig"
  echo "$_d"
}
# same <file> <printf-format> <label>: byte-for-byte compare.
same() {
  # shellcheck disable=SC2059
  if printf "$2" | cmp -s - "$1"; then _r=same; else _r="differs: $(od -c "$1" | head -5 | tr '\n' ' ')"; fi
  assert_eq "$_r" "same" "$3"
}
nbak() { find "$1" -maxdepth 1 -name '.wslconfig.bak-*' | wc -l | tr -d ' '; }

# nat removes the networking key, keeps memory, backs up first.
D=$(fixture nat-from-mirrored '[wsl2]\nmemory=192GB\nnetworkingMode=mirrored\n')
run "$D" -Mode nat -ConfigOnly
assert_eq "$RC" "0" "nat -ConfigOnly exits 0"
same "$D/.wslconfig" '[wsl2]\nmemory=192GB\n' "nat drops networkingMode, keeps memory"
assert_eq "$(nbak "$D")" "1" "nat wrote a .bak- first"
same "$(find "$D" -name '.wslconfig.bak-*' | head -1)" '[wsl2]\nmemory=192GB\nnetworkingMode=mirrored\n' "backup holds the original bytes"
case "$OUT" in *"wsl --shutdown"*) _r=hint ;; *) _r=none ;; esac
assert_eq "$_r" "hint" "a changed file prints the restart hint"

# An existing bridged config is current: byte-identical, no backup.
BR='# k3s node\n[wsl2]\nmemory=64GB\nnetworkingMode=bridged\nvmSwitch=Other\nmacAddress=02:00:00:00:00:01\ndhcp=true\n'
D=$(fixture bridged-current "$BR")
run "$D" -Mode bridged -ConfigOnly
assert_eq "$RC" "0" "bridged on a bridged file exits 0"
same "$D/.wslconfig" "$BR" "existing bridged config is byte-identical"
assert_eq "$(nbak "$D")" "0" "existing bridged config: no backup"
run "$D" -Status -Mode bridged -ConfigOnly
assert_eq "$RC" "0" "-Status bridged on a bridged file -> 0"

# mirrored on an empty file writes the [wsl2] section (CRLF default).
D=$(fixture mirrored-empty '')
run "$D" -Mode mirrored -ConfigOnly
same "$D/.wslconfig" '[wsl2]\r\nnetworkingMode=mirrored\r\n' "mirrored on an empty file adds [wsl2] networkingMode=mirrored"

# A networkingMode under [experimental] is removed for nat ...
D=$(fixture exp-nat '[wsl2]\nmemory=8GB\n\n[experimental]\nnetworkingMode=mirrored\nautoMemoryReclaim=gradual\n')
run "$D" -Mode nat -ConfigOnly
same "$D/.wslconfig" '[wsl2]\nmemory=8GB\n\n[experimental]\nautoMemoryReclaim=gradual\n' "nat removes networkingMode from [experimental]"
# ... and moved into [wsl2] for its own mode.
D=$(fixture exp-mirrored '[wsl2]\nmemory=8GB\n[experimental]\nnetworkingMode=mirrored\n')
run "$D" -Status -Mode mirrored -ConfigOnly
assert_eq "$RC" "3" "-Status sees an [experimental] networkingMode as needing a move"
run "$D" -Mode mirrored -ConfigOnly
same "$D/.wslconfig" '[wsl2]\nmemory=8GB\nnetworkingMode=mirrored\n[experimental]\n' "mirrored moves [experimental] networkingMode into [wsl2]"

# CRLF stays CRLF; a UTF-8 BOM stays, and none is added.
D=$(fixture crlf '[wsl2]\r\nmemory=192GB\r\nnetworkingMode=mirrored\r\n')
run "$D" -Mode nat -ConfigOnly
same "$D/.wslconfig" '[wsl2]\r\nmemory=192GB\r\n' "CRLF input keeps CRLF"
D=$(fixture bom '\357\273\277[wsl2]\r\nswap=0\r\n')
run "$D" -Mode mirrored -ConfigOnly
same "$D/.wslconfig" '\357\273\277[wsl2]\r\nswap=0\r\nnetworkingMode=mirrored\r\n' "a UTF-8 BOM is preserved"
D=$(fixture nobom '# note\n[wsl2]\nswap=0\n')
run "$D" -Mode mirrored -ConfigOnly
same "$D/.wslconfig" '# note\n[wsl2]\nswap=0\nnetworkingMode=mirrored\n' "no BOM is added; comments kept"

# -Status exit codes and read-only-ness.
D=$(fixture status-current '[wsl2]\nmemory=1GB\n')
run "$D" -Status -Mode nat -ConfigOnly
assert_eq "$RC" "0" "-Status nat on a nat file -> 0"
D=$(fixture status-change '[wsl2]\nnetworkingMode=mirrored\n')
run "$D" -Status -Mode nat -ConfigOnly
assert_eq "$RC" "3" "-Status nat on a mirrored file -> 3"
same "$D/.wslconfig" '[wsl2]\nnetworkingMode=mirrored\n' "-Status changes nothing"
assert_eq "$(nbak "$D")" "0" "-Status writes no backup"

# off is hands-off.
run "$D" -Mode off -ConfigOnly
assert_eq "$RC" "0" "off exits 0"
same "$D/.wslconfig" '[wsl2]\nnetworkingMode=mirrored\n' "off changes nothing"

# bridged without a WSLBridge switch: print the command, change nothing.
if "$PS_EXE" -NoProfile -NonInteractive -Command \
    "if ((Get-Command Get-VMSwitch -ErrorAction SilentlyContinue) -and (Get-VMSwitch -Name WSLBridge -ErrorAction SilentlyContinue)) { exit 1 }" \
    </dev/null >/dev/null 2>&1; then
  D=$(fixture bridged-noswitch '[wsl2]\nmemory=1GB\n')
  run "$D" -Mode bridged -ConfigOnly
  same "$D/.wslconfig" '[wsl2]\nmemory=1GB\n' "bridged without a switch changes nothing"
  assert_eq "$(nbak "$D")" "0" "bridged without a switch: no backup"
  case "$OUT" in *"New-VMSwitch -Name WSLBridge"*) _r=hint ;; *) _r="no hint: $OUT" ;; esac
  assert_eq "$_r" "hint" "bridged without a switch prints the New-VMSwitch command"
else
  echo "SKIP: a WSLBridge switch exists on this host"
fi

# No .wslconfig: nat creates nothing; mirrored creates it.
D="$TMP_NET/missing"; mkdir -p "$D"
run "$D" -Mode nat -ConfigOnly
assert_eq "$([ -e "$D/.wslconfig" ] && echo yes || echo no)" "no" "nat with no .wslconfig creates none"
run "$D" -Mode mirrored -ConfigOnly
same "$D/.wslconfig" '[wsl2]\r\nnetworkingMode=mirrored\r\n' "mirrored with no .wslconfig creates one"

# An empty .wslconfig is already nat: no error, no write.
D=$(fixture nat-empty '')
run "$D" -Status -Mode nat -ConfigOnly
assert_eq "$RC" "0" "-Status nat on an empty file -> 0"
run "$D" -Mode nat -ConfigOnly
assert_eq "$RC:$(nbak "$D"):$(wc -c <"$D/.wslconfig" | tr -d ' ')" "0:0:0" "nat on an empty file: exit 0, no backup, still empty"

# Test seams (honored only with WSLNET_TEST=1) reach Windows via WSLENV.
export WSLENV="${WSLENV:+${WSLENV}:}WSLNET_TEST:WSLNET_TEST_SWITCH:WSLNET_TEST_REFRESH:WSLNET_TEST_WINSSHD"
export WSLNET_TEST=1

# bridged when the switch can't be read unelevated: the elevated run decides.
D=$(fixture bridged-unknown '[wsl2]\nmemory=1GB\n')
WSLNET_TEST_SWITCH=unknown run "$D" -Status -Mode bridged -ConfigOnly
assert_eq "$RC" "3" "-Status bridged, switch unknown, file not bridged -> 3"
WSLNET_TEST_SWITCH=absent run "$D" -Status -Mode bridged -ConfigOnly
assert_eq "$RC" "0" "-Status bridged, switch confirmed absent -> 0"
WSLNET_TEST_SWITCH=unknown run "$(fixture bridged-unknown-cur "$BR")" -Status -Mode bridged -ConfigOnly
assert_eq "$RC" "0" "-Status bridged on a bridged file stays 0 even when the switch is unknown"

# A fresh bridged setup with the switch present writes the four keys.
D=$(fixture bridged-fresh '[wsl2]\r\nmemory=1GB\r\n')
WSLNET_TEST_SWITCH=present run "$D" -Mode bridged -ConfigOnly
_mac=$(sed -n 's/^macAddress=\([^[:cntrl:]]*\).*/\1/p' "$D/.wslconfig")
case "$_mac" in 02:[0-9a-f][0-9a-f]:[0-9a-f][0-9a-f]:[0-9a-f][0-9a-f]:[0-9a-f][0-9a-f]:[0-9a-f][0-9a-f]) _r=ok ;; *) _r="bad mac '$_mac'" ;; esac
assert_eq "$_r" "ok" "fresh bridged MAC is 02: + 5 lowercase hex bytes"
same "$D/.wslconfig" "[wsl2]\r\nmemory=1GB\r\nnetworkingMode=bridged\r\nvmSwitch=WSLBridge\r\ndhcp=true\r\nmacAddress=${_mac}\r\n" \
  "fresh bridged writes networkingMode/vmSwitch/dhcp/macAddress, CRLF kept"
assert_eq "$(nbak "$D")" "1" "fresh bridged backs up first"

# A failing refresh must fail the nat apply (and its output must show).
STUB="$TMP_NET/stub-refresh.ps1"
D=$(fixture nat-refresh '[wsl2]\nmemory=1GB\n')
printf "Write-Host 'stub-refresh: ERROR could not refresh'\r\nexit 1\r\n" > "$STUB"
WSLNET_TEST_REFRESH=$(wslpath -w "$STUB") run "$D" -Mode nat
assert_eq "$RC" "1" "nat apply exits 1 when refresh-wsl-portproxy fails"
case "$OUT" in *"stub-refresh: ERROR"*) _r=shown ;; *) _r="hidden: $OUT" ;; esac
assert_eq "$_r" "shown" "the refresh output reaches the operator"
printf "Write-Host 'stub-refresh: ok'\r\nexit 0\r\n" > "$STUB"
WSLNET_TEST_REFRESH=$(wslpath -w "$STUB") run "$D" -Mode nat
assert_eq "$RC" "0" "nat apply exits 0 when refresh-wsl-portproxy succeeds"

# mirrored while Windows OpenSSH is on: both would claim port 22, so refuse (exit 4).
D=$(fixture mirrored-winsshd '[wsl2]\nmemory=1GB\n')
WSLNET_TEST_WINSSHD=on run "$D" -Status -Mode mirrored -ConfigOnly
assert_eq "$RC" "4" "-Status mirrored with Windows sshd on exits 4"
case "$OUT" in *"cannot apply mirrored"*"Choose nat"*) _r=ok ;; *) _r="no reason: $OUT" ;; esac
assert_eq "$_r" "ok" "-Status mirrored with Windows sshd on says why and what to do"
WSLNET_TEST_WINSSHD=on run "$D" -Mode mirrored -ConfigOnly
assert_eq "$RC" "4" "apply mirrored with Windows sshd on exits 4"
same "$D/.wslconfig" '[wsl2]\nmemory=1GB\n' "apply mirrored refused: .wslconfig untouched"
WSLNET_TEST_WINSSHD=off run "$D" -Status -Mode mirrored -ConfigOnly
assert_eq "$RC" "3" "-Status mirrored with Windows sshd off: would change (exit 3)"
unset WSLNET_TEST

# NAT cleanup only removes forwards that target WSL: Test-SameSubnet, run alone.
_sub=$("$PS_EXE" -NoProfile -NonInteractive -Command \
  "\$t=\$null; \$e=\$null; \$a=[System.Management.Automation.Language.Parser]::ParseFile('${SET_NET_W}', [ref]\$t, [ref]\$e); \$f=\$a.Find({ param(\$n) \$n -is [System.Management.Automation.Language.FunctionDefinitionAst] -and \$n.Name -eq 'Test-SameSubnet' }, \$true); Invoke-Expression \$f.Extent.Text; '{0},{1},{2}' -f (Test-SameSubnet '172.20.1.5' '172.20.0.1' 20), (Test-SameSubnet '192.168.0.10' '172.20.0.1' 20), (Test-SameSubnet 'junk' '172.20.0.1' 20)" \
  </dev/null 2>&1 | tr -d '\r')
assert_eq "$_sub" "True,False,False" "Test-SameSubnet: same /20 yes, LAN host no, junk no"

# Full -Status (no -ConfigOnly) is read-only and reports the Windows side.
D=$(fixture status-full '[wsl2]\nmemory=1GB\n')
run "$D" -Status -Mode nat
case "$RC" in 0|3) _r=ok ;; *) _r="rc=$RC: $OUT" ;; esac
assert_eq "$_r" "ok" "full -Status nat exits 0 or 3"
case "$OUT" in *portproxy*) _r=ok ;; *) _r="missing: $OUT" ;; esac
assert_eq "$_r" "ok" "full -Status reports the portproxy"

_test_report
