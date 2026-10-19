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
export WSLENV="${WSLENV:+${WSLENV}:}WSLNET_TEST:WSLNET_TEST_SWITCH:WSLNET_TEST_REFRESH"
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
unset WSLNET_TEST

# Full -Status (no -ConfigOnly) is read-only and reports the Windows side.
D=$(fixture status-full '[wsl2]\nmemory=1GB\n')
run "$D" -Status -Mode nat
case "$RC" in 0|3) _r=ok ;; *) _r="rc=$RC: $OUT" ;; esac
assert_eq "$_r" "ok" "full -Status nat exits 0 or 3"
case "$OUT" in *portproxy*) _r=ok ;; *) _r="missing: $OUT" ;; esac
assert_eq "$_r" "ok" "full -Status reports the portproxy"

_test_report
