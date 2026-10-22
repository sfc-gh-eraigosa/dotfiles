#!/usr/bin/env bash
# shellcheck disable=SC2016  # snippets are deliberately single-quoted for the child shell
# wslnet_test.sh — drives opt/lib/wslnet.sh under bash AND dash (sh).
# Scratch dir per case; stub gff / powershell.exe / netsh.exe / systemctl.
# Fixtures use placeholder names and RFC 5737-style addresses only.
set -u
here="$(cd -- "$(dirname "$0")" && pwd -P)"
pass=0; fail=0
ok()  { pass=$((pass+1)); echo "PASS: $1"; }
bad() { fail=$((fail+1)); echo "FAIL: $1"; }

# fixture: fresh scratch T with stubs on PATH-independent seams.
#   STUB_ROWS (netsh rows), STUB_TASK (yes|""), STUB_WIRED (adapter name|"")
#   gff: `list` prints a row for the wsl-network key with layer $GFF_LAYER.
fixture() {
  T="$(mktemp -d)"
  mkdir -p "${T}/bin" "${T}/users"
  cat > "${T}/bin/gff" <<STUB
#!/bin/sh
echo "args=\$*" >> "${T}/gff.log"
case "\$1" in
  list) echo "install.windows.wsl-network  choice  off  \${GFF_LAYER:-repo-live}" ;;
  set) exit \${GFF_SET_RC:-0} ;;
esac
exit 0
STUB
  cat > "${T}/bin/netsh.exe" <<'STUB'
#!/bin/sh
printf 'Listen on ipv4:             Connect to ipv4:\r\n\r\nAddress         Port        Address         Port\r\n--------------- ----------  --------------- ----------\r\n'
[ -n "${STUB_ROWS:-}" ] && printf '%s\r\n' "$STUB_ROWS"
exit 0
STUB
  cat > "${T}/bin/powershell.exe" <<'STUB'
#!/bin/sh
case "$*" in
  *USERPROFILE*) printf '%s\r\n' "${STUB_PROFILE:-}" ;;
  *RefreshWslPortProxy*) [ -n "${STUB_TASK:-}" ] && printf 'yes\r\n' ;;
  *Get-NetAdapter*) [ -n "${STUB_WIRED:-}" ] && printf '%s\r\n' "$STUB_WIRED" ;;
esac
exit 0
STUB
  cat > "${T}/bin/wslpath" <<'STUB'
#!/bin/sh
printf '%s\n' "$2"
STUB
  chmod +x "${T}/bin/"*
  printf 'Linux version 5.15-microsoft-standard-WSL2\n' > "${T}/proc_version"
  export GFF_BIN="${T}/bin/gff" GFF_REPO_DIR="$T"
  export WSLNET_PROC_VERSION="${T}/proc_version"
  export WSLNET_NETSH="${T}/bin/netsh.exe" WSLNET_PS="${T}/bin/powershell.exe"
  export WSLNET_WSLCONFIG="${T}/wslconfig" WSLNET_USERS_DIR="${T}/users"
  export WSLNET_K3S_BIN="${T}/nok3s" WSLNET_TTY="${T}/notty" WSLNET_STATE_DIR="${T}/state"
  unset WSLNET_ANSWER STUB_ROWS STUB_TASK STUB_WIRED STUB_PROFILE GFF_LAYER GFF_SET_RC
  unset GFF_INSTALL_NETWORK_SSHD GFF_INSTALL_WINDOWS_WSL_NETWORK
  # PATH: stubs first so systemctl probing is controlled too.
  PATH="${T}/bin:${ORIG_PATH}"; export PATH
}
ORIG_PATH="$PATH"

run_case() {  # SHELL DESC SNIPPET
  if "$1" -c ". '${here}/gff.sh'; . '${here}/wslnet.sh'; $3"; then ok "[$1] $2"; else bad "[$1] $2"; fi
}

for sh_bin in bash sh; do
  # ---- detect ----
  fixture; printf '[wsl2]\nnetworkingMode=bridged\n' > "$WSLNET_WSLCONFIG"
  run_case "$sh_bin" "detect: [wsl2] bridged" '[ "$(wslnet_detect)" = bridged ]'; rm -rf "$T"

  fixture; printf '[wsl2]\nmemory=4GB\nnetworkingMode = mirrored\n' > "$WSLNET_WSLCONFIG"
  run_case "$sh_bin" "detect: [wsl2] mirrored" '[ "$(wslnet_detect)" = mirrored ]'; rm -rf "$T"

  fixture; printf '[experimental]\nnetworkingMode=mirrored\n' > "$WSLNET_WSLCONFIG"
  run_case "$sh_bin" "detect: key under [experimental] (older WSL)" '[ "$(wslnet_detect)" = mirrored ]'; rm -rf "$T"

  fixture; printf '[wsl2]\nnetworkingMode="Mirrored"  # set by hand\n' > "$WSLNET_WSLCONFIG"
  run_case "$sh_bin" "detect: quoted value + inline comment (as set-wsl-network.ps1 reads it)" '[ "$(wslnet_detect)" = mirrored ]'; rm -rf "$T"

  fixture; printf '\357\273\277[wsl2]\r\nnetworkingMode=bridged\r\n' > "$WSLNET_WSLCONFIG"
  run_case "$sh_bin" "detect: CRLF + BOM" '[ "$(wslnet_detect)" = bridged ]'; rm -rf "$T"

  fixture; printf '[wsl2]\nnetworkingMode=bridged\n' > "$WSLNET_WSLCONFIG"
  export STUB_ROWS='0.0.0.0         22          172.20.0.2      22'
  run_case "$sh_bin" "detect: bridged file wins over a portproxy rule" '[ "$(wslnet_detect)" = bridged ]'; rm -rf "$T"

  fixture; export STUB_ROWS='0.0.0.0         22          172.20.0.2      22'
  run_case "$sh_bin" "detect: no file + portproxy rule to :22 -> nat" '[ "$(wslnet_detect)" = nat ]'; rm -rf "$T"

  fixture; export STUB_ROWS='0.0.0.0         2222        172.20.0.2      22'
  run_case "$sh_bin" "detect: 0.0.0.0:2222 -> :22 (Windows sshd host) -> nat" '[ "$(wslnet_detect)" = nat ]'; rm -rf "$T"

  fixture; export STUB_ROWS='127.0.0.1       8022        10.0.0.5        22'
  run_case "$sh_bin" "detect: a forward not shaped like WSL's (127.0.0.1:8022) is not nat" '[ -z "$(wslnet_detect)" ]'; rm -rf "$T"

  fixture; printf '[ wsl2 ]\nnetworkingMode=bridged\n' > "$WSLNET_WSLCONFIG"
  run_case "$sh_bin" "detect: a [ wsl2 ] header (spaces) is [wsl2], as in Get-LineSections" '[ "$(wslnet_detect)" = bridged ]'; rm -rf "$T"

  fixture; printf '[wsl2]\nnetworkingMode=bridged\n[wsl2] # not a header\nnetworkingMode=mirrored\n' > "$WSLNET_WSLCONFIG"
  run_case "$sh_bin" "detect: [wsl2] # comment is not a header, as in Get-LineSections" '[ "$(wslnet_detect)" = mirrored ]'; rm -rf "$T"

  fixture; export STUB_ROWS='0.0.0.0         8080        172.20.0.2      8080'
  run_case "$sh_bin" "detect: portproxy for another port only -> nothing" '[ -z "$(wslnet_detect)" ]'; rm -rf "$T"

  fixture; printf '[wsl2]\nnetworkingMode=nat\n' > "$WSLNET_WSLCONFIG"
  run_case "$sh_bin" "detect: file networkingMode=nat, no rule/task -> nothing recorded" '[ -z "$(wslnet_detect)" ]'; rm -rf "$T"

  fixture; export STUB_ROWS='0.0.0.0         2222        172.20.0.2      22'
  run_case "$sh_bin" "detect: listen 2222 -> connect 22 -> nat" '[ "$(wslnet_detect)" = nat ]'; rm -rf "$T"

  fixture; export STUB_ROWS='0.0.0.0         22          172.20.0.2      8080'
  run_case "$sh_bin" "detect: listen 22 -> connect 8080 -> nothing" '[ -z "$(wslnet_detect)" ]'; rm -rf "$T"

  fixture; export STUB_TASK=1
  run_case "$sh_bin" "detect: RefreshWslPortProxy task -> nat" '[ "$(wslnet_detect)" = nat ]'; rm -rf "$T"

  fixture
  run_case "$sh_bin" "detect: nothing -> empty" '[ -z "$(wslnet_detect)" ]'; rm -rf "$T"

  # ---- wslconfig discovery ----
  fixture; unset WSLNET_WSLCONFIG
  mkdir -p "${T}/users/Test User"; : > "${T}/users/Test User/.wslconfig"
  run_case "$sh_bin" "wslconfig: no interop, one candidate in a dir with a space" '
    PATH="/usr/bin:/bin"; WSLNET_PS="'"$T"'/absent.exe"; export WSLNET_PS
    [ "$(wslnet_wslconfig)" = "'"$T"'/users/Test User/.wslconfig" ]'; rm -rf "$T"

  fixture; unset WSLNET_WSLCONFIG
  mkdir -p "${T}/users/A One" "${T}/users/B Two"
  : > "${T}/users/A One/.wslconfig"; : > "${T}/users/B Two/.wslconfig"
  run_case "$sh_bin" "wslconfig: no interop, two candidates -> empty" '
    WSLNET_PS="'"$T"'/absent.exe"; export WSLNET_PS; [ -z "$(wslnet_wslconfig)" ]'; rm -rf "$T"

  fixture; unset WSLNET_WSLCONFIG
  mkdir -p "${T}/users/Test User"; printf '[wsl2]\nnetworkingMode=bridged\n' > "${T}/users/Test User/.wslconfig"
  export STUB_PROFILE="${T}/users/Test User"
  run_case "$sh_bin" "wslconfig+detect: interop profile path with a space" '
    [ "$(wslnet_wslconfig)" = "'"$T"'/users/Test User/.wslconfig" ] && [ "$(wslnet_detect)" = bridged ]'; rm -rf "$T"

  # ---- recommend ----
  fixture; printf '#!/bin/sh\n' > "${T}/k3s"; chmod +x "${T}/k3s"; export WSLNET_K3S_BIN="${T}/k3s" STUB_WIRED="Ethernet"
  run_case "$sh_bin" "recommend: k3s + wired adapter -> bridged" '[ "$(wslnet_recommend)" = bridged ]'; rm -rf "$T"

  fixture; printf '#!/bin/sh\n' > "${T}/k3s"; chmod +x "${T}/k3s"; export WSLNET_K3S_BIN="${T}/k3s"
  run_case "$sh_bin" "recommend: k3s + Wi-Fi only -> nat" '[ "$(wslnet_recommend)" = nat ]'; rm -rf "$T"

  fixture; export STUB_WIRED="Ethernet"
  printf '#!/bin/sh\nexit 1\n' > "${T}/bin/systemctl"; chmod +x "${T}/bin/systemctl"
  run_case "$sh_bin" "recommend: no k3s -> nat" '[ "$(wslnet_recommend)" = nat ]'; rm -rf "$T"

  fixture; export STUB_WIRED="Ethernet"
  printf '#!/bin/sh\n[ "$2" = k3s-agent ]\n' > "${T}/bin/systemctl"; chmod +x "${T}/bin/systemctl"
  run_case "$sh_bin" "recommend: k3s-agent unit enabled + wired -> bridged" '[ "$(wslnet_recommend)" = bridged ]'; rm -rf "$T"

  # ---- ask ----
  fixture
  run_case "$sh_bin" "ask: WSLNET_ANSWER=nat" 'WSLNET_ANSWER=nat; export WSLNET_ANSWER; [ "$(wslnet_ask bridged)" = nat ]'; rm -rf "$T"
  fixture
  run_case "$sh_bin" "ask: WSLNET_ANSWER=skip" 'WSLNET_ANSWER=skip; export WSLNET_ANSWER; [ "$(wslnet_ask nat)" = skip ]'; rm -rf "$T"
  fixture
  run_case "$sh_bin" "ask: junk answer, no TTY -> skip" 'WSLNET_ANSWER=junk; export WSLNET_ANSWER; [ "$(wslnet_ask nat)" = skip ]'; rm -rf "$T"
  fixture
  run_case "$sh_bin" "ask: no answer, no TTY -> skip" '[ "$(wslnet_ask nat)" = skip ]'; rm -rf "$T"
  fixture; printf 'bogus\nmirrored\n' > "${T}/tty"; export WSLNET_TTY="${T}/tty"
  run_case "$sh_bin" "ask: TTY re-asks once on an invalid answer" '[ "$(wslnet_ask nat 2>/dev/null)" = mirrored ]'; rm -rf "$T"
  fixture; printf 'bogus\nbogus\n' > "${T}/tty"; export WSLNET_TTY="${T}/tty"
  run_case "$sh_bin" "ask: TTY invalid twice -> skip" '[ "$(wslnet_ask nat 2>/dev/null)" = skip ]'; rm -rf "$T"
  fixture; printf '\n' > "${T}/tty"; export WSLNET_TTY="${T}/tty"
  run_case "$sh_bin" "ask: TTY Enter accepts recommended; prompt marks it" '
    [ "$(wslnet_ask bridged 2>/dev/null)" = bridged ] && wslnet_ask bridged 2>&1 >/dev/null | grep -q "bridged (recommended)"'; rm -rf "$T"

  # ---- sync ----
  fixture; printf 'Linux version 5.15-generic\n' > "${T}/proc_version"; printf '[wsl2]\nnetworkingMode=bridged\n' > "$WSLNET_WSLCONFIG"
  run_case "$sh_bin" "sync: not WSL -> no gff calls" 'wslnet_sync; [ ! -f "'"$T"'/gff.log" ]'; rm -rf "$T"

  fixture; export GFF_LAYER=user-override; printf '[wsl2]\nnetworkingMode=bridged\n' > "$WSLNET_WSLCONFIG"
  run_case "$sh_bin" "sync: key user-set -> only list" '
    wslnet_sync; ! grep -q "args=set" "'"$T"'/gff.log" && grep -q "args=list" "'"$T"'/gff.log"'; rm -rf "$T"

  fixture; export GFF_INSTALL_NETWORK_SSHD=true; printf '[wsl2]\nnetworkingMode=bridged\n' > "$WSLNET_WSLCONFIG"
  run_case "$sh_bin" "sync: detected -> set + message" '
    out=$(wslnet_sync) && grep -q "args=set install.windows.wsl-network bridged" "'"$T"'/gff.log" &&
    [ "$out" = "Recorded existing WSL networking: install.windows.wsl-network=bridged (nothing changed; install.sh leaves a found setup as it is)." ] &&
    [ "$(wslnet_detected_mode)" = bridged ]'; rm -rf "$T"

  fixture; export GFF_INSTALL_NETWORK_SSHD=true; printf '[wsl2]\nnetworkingMode=mirrored\n' > "$WSLNET_WSLCONFIG"
  run_case "$sh_bin" "sync: export visible in caller shell (not a subshell)" '
    wslnet_sync >/dev/null; [ "${GFF_INSTALL_WINDOWS_WSL_NETWORK:-}" = mirrored ]'; rm -rf "$T"

  fixture; export GFF_INSTALL_NETWORK_SSHD=true; printf '[wsl2]\nnetworkingMode=mirrored\n' > "$WSLNET_WSLCONFIG"; export GFF_SET_RC=1
  run_case "$sh_bin" "sync: gff set fails -> warning, rc 0, no export" '
    out=$(wslnet_sync); rc=$?; [ "$rc" -eq 0 ] && [ -z "${GFF_INSTALL_WINDOWS_WSL_NETWORK:-}" ] &&
    case "$out" in WARNING:*) true ;; *) false ;; esac && [ -z "$(wslnet_detected_mode)" ]'; rm -rf "$T"

  fixture
  run_case "$sh_bin" "sync: nothing detected + sshd off -> no set, no prompt" '
    out=$(wslnet_sync); [ -z "$out" ] && ! grep -q "args=set" "'"$T"'/gff.log"'; rm -rf "$T"

  fixture; export GFF_INSTALL_NETWORK_SSHD=true WSLNET_ANSWER=nat
  run_case "$sh_bin" "sync: sshd on + answer nat -> set nat, exported" '
    wslnet_sync >/dev/null; grep -q "args=set install.windows.wsl-network nat" "'"$T"'/gff.log" &&
    [ "${GFF_INSTALL_WINDOWS_WSL_NETWORK:-}" = nat ]'; rm -rf "$T"

  fixture; export GFF_INSTALL_NETWORK_SSHD=true WSLNET_ANSWER=skip
  run_case "$sh_bin" "sync: answer skip -> no set + hint" '
    out=$(wslnet_sync); ! grep -q "args=set" "'"$T"'/gff.log" &&
    [ "$out" = "Choose later: gff set install.windows.wsl-network <off|nat|bridged|mirrored>" ]'; rm -rf "$T"

  # ---- sync: sshd gate, found-vs-chosen marker, remembered skip ----
  fixture; printf '[wsl2]\nnetworkingMode=bridged\n' > "$WSLNET_WSLCONFIG"
  printf '#!/bin/sh\ntouch "%s/ps.called"\n' "$T" > "${T}/bin/powershell.exe"
  run_case "$sh_bin" "sync: sshd off -> nothing recorded, Windows never probed" '
    out=$(wslnet_sync); [ -z "$out" ] && ! grep -q "args=set" "'"$T"'/gff.log" &&
    [ ! -e "'"$T"'/ps.called" ] && [ -z "$(wslnet_detected_mode)" ]'; rm -rf "$T"

  fixture; export GFF_INSTALL_NETWORK_SSHD=true WSLNET_ANSWER=nat
  mkdir -p "${T}/state"; printf 'bridged\n' > "${T}/state/wsl-network-detected"
  run_case "$sh_bin" "sync: a chosen mode clears the found marker" '
    wslnet_sync >/dev/null; [ -z "$(wslnet_detected_mode)" ]'; rm -rf "$T"

  fixture; export GFF_INSTALL_NETWORK_SSHD=true; printf 'skip\nnat\n' > "${T}/tty"; export WSLNET_TTY="${T}/tty"
  run_case "$sh_bin" "sync: an operator skip is remembered; the next run does not ask" '
    wslnet_sync >/dev/null 2>&1; [ -f "'"$T"'/state/wsl-network-skipped" ] &&
    out=$(wslnet_sync 2>&1); [ -z "$out" ] && ! grep -q "args=set" "'"$T"'/gff.log"'; rm -rf "$T"

  fixture; export GFF_INSTALL_NETWORK_SSHD=true; printf 'nat\n' > "${T}/tty"; export WSLNET_TTY="${T}/tty"
  mkdir -p "${T}/state"; printf 'skip\n' > "${T}/state/wsl-network-skipped"
  run_case "$sh_bin" "sync --ask: asks despite a remembered skip, then forgets it" '
    wslnet_sync --ask >/dev/null 2>&1; grep -q "args=set install.windows.wsl-network nat" "'"$T"'/gff.log" &&
    [ ! -f "'"$T"'/state/wsl-network-skipped" ]'; rm -rf "$T"

  fixture; export GFF_INSTALL_NETWORK_SSHD=true WSLNET_ANSWER=nat
  mkdir -p "${T}/state"; printf 'skip\n' > "${T}/state/wsl-network-skipped"
  run_case "$sh_bin" "sync: WSLNET_ANSWER still applies over a remembered skip" '
    wslnet_sync >/dev/null; grep -q "args=set install.windows.wsl-network nat" "'"$T"'/gff.log"'; rm -rf "$T"

  fixture; export GFF_INSTALL_NETWORK_SSHD=true WSLNET_ANSWER=skip; printf 'x\n' > "${T}/tty"; export WSLNET_TTY="${T}/tty"
  run_case "$sh_bin" "sync: a pre-answered skip (fleet) is not remembered" '
    wslnet_sync >/dev/null; [ ! -f "'"$T"'/state/wsl-network-skipped" ]'; rm -rf "$T"

  fixture; export GFF_INSTALL_NETWORK_SSHD=true; printf 'bogus\nbogus\n' > "${T}/tty"; export WSLNET_TTY="${T}/tty"
  run_case "$sh_bin" "sync: two invalid entries are not remembered as a skip" '
    wslnet_sync >/dev/null 2>&1; [ ! -f "'"$T"'/state/wsl-network-skipped" ]'; rm -rf "$T"

  fixture; export GFF_INSTALL_NETWORK_SSHD=true; printf '[wsl2]\nnetworkingMode=bridged\n' > "$WSLNET_WSLCONFIG"
  : > "${T}/state"   # a file where the state dir should be: the marker cannot be written
  run_case "$sh_bin" "sync: found marker unwritable -> nothing recorded, warning" '
    out=$(wslnet_sync); ! grep -q "args=set" "'"$T"'/gff.log" &&
    case "$out" in WARNING:*wsl-network-detected*) true ;; *) false ;; esac'; rm -rf "$T"
done

echo "----------------------------------------"
echo "wslnet_test: ${pass} passed, ${fail} failed"
[ "$fail" -eq 0 ]
