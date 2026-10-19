#!/usr/bin/env bash
# shellcheck disable=SC2016  # snippets are deliberately single-quoted for the child shell
# sshdflags_test.sh — drives opt/lib/sshdflags.sh under bash AND dash (sh).
# Scratch HOME per case; a stub gff records argv and answers `list`.
set -u
here="$(cd -- "$(dirname "$0")" && pwd -P)"
pass=0; fail=0
ok()  { pass=$((pass+1)); echo "PASS: $1"; }
bad() { fail=$((fail+1)); echo "FAIL: $1"; }

fixture() {  # $1=file content ("" = no file) $2=layer for the key $3=set rc
  T="$(mktemp -d)"
  export SSHD_ENV_FILE="${T}/.sshd.env"
  [ -n "$1" ] && printf '%s\n' "$1" > "$SSHD_ENV_FILE"
  mkdir -p "${T}/bin"
  cat > "${T}/bin/gff" <<STUB
#!/bin/sh
echo "args=\$*" >> "${T}/gff.log"
case "\$1" in
  list) echo "install.network.sshd  bool  false  $2" ;;
  set) exit $3 ;;
esac
exit 0
STUB
  chmod +x "${T}/bin/gff"
  export GFF_BIN="${T}/bin/gff" GFF_REPO_DIR="$T"
  unset GFF_INSTALL_NETWORK_SSHD
}

# run_case SHELL DESC SNIPPET — snippet runs after sourcing both libs.
run_case() {
  if "$1" -c ". '${here}/gff.sh'; . '${here}/sshdflags.sh'; $3"; then ok "[$1] $2"; else bad "[$1] $2"; fi
}

for sh_bin in bash sh; do
  fixture "SSHD_LOGIN=true" default 0
  run_case "$sh_bin" "SSHD_LOGIN=true, unset key -> set true, file gone, message" '
    out=$(sshd_migrate) &&
    [ ! -f "$SSHD_ENV_FILE" ] &&
    grep -q "args=set install.network.sshd true" "'"$T"'/gff.log" &&
    [ "$out" = "Migrated ~/.sshd.env to gff: install.network.sshd=true (file removed)." ]'
  rm -rf "$T"

  fixture "SSHD_LOGIN=true" default 0
  run_case "$sh_bin" "GFF_INSTALL_NETWORK_SSHD exported in caller shell" '
    sshd_migrate >/dev/null; [ "${GFF_INSTALL_NETWORK_SSHD:-}" = "true" ]'
  rm -rf "$T"

  fixture "SSHD_LOGIN=true" user-override 0
  run_case "$sh_bin" "already user-override -> no set, stale file removed" '
    out=$(sshd_migrate) && [ ! -f "$SSHD_ENV_FILE" ] &&
    ! grep -q "args=set" "'"$T"'/gff.log" &&
    [ "$out" = "Removed stale ~/.sshd.env (install.network.sshd is already set on this machine)." ]'
  rm -rf "$T"

  fixture "FOO=bar" default 0
  run_case "$sh_bin" "no SSHD_LOGIN=true -> no set, file removed" '
    out=$(sshd_migrate) && [ ! -f "$SSHD_ENV_FILE" ] &&
    ! grep -q "args=set" "'"$T"'/gff.log" &&
    [ "$out" = "Removed ~/.sshd.env (it did not enable sshd)." ]'
  rm -rf "$T"

  fixture "SSHD_LOGIN=true" default 1
  run_case "$sh_bin" "set fails -> file kept, warning, rc 0" '
    out=$(sshd_migrate); rc=$?; [ "$rc" -eq 0 ] && [ -f "$SSHD_ENV_FILE" ] &&
    [ "$out" = "WARNING: could not record install.network.sshd in gff; kept ~/.sshd.env (retrying next run)." ]'
  rm -rf "$T"

  fixture "" default 0
  run_case "$sh_bin" "no file -> silent, no gff calls" '
    out=$(sshd_migrate); [ -z "$out" ] && [ ! -f "'"$T"'/gff.log" ]'
  rm -rf "$T"

  fixture "SSHD_LOGIN=true" default 0
  run_case "$sh_bin" "second run is a no-op" '
    sshd_migrate >/dev/null; : > "'"$T"'/gff.log"
    out=$(sshd_migrate); [ -z "$out" ] && [ ! -s "'"$T"'/gff.log" ]'
  rm -rf "$T"
done

echo "----------------------------------------"
echo "sshdflags_test: ${pass} passed, ${fail} failed"
[ "$fail" -eq 0 ]
