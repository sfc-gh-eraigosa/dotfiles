#!/usr/bin/env bash
# winsetup_test.sh — drives opt/lib/winsetup.sh under bash AND dash (sh).
# Mirrors the opt/lib/gff_test.sh assert style. Every case runs in a scratch
# HOME-like tmpdir; a stub gff records its argv so delegation is observable.
set -u
here="$(cd -- "$(dirname "$0")" && pwd -P)"
pass=0; fail=0
ok()  { pass=$((pass+1)); echo "PASS: $1"; }
bad() { fail=$((fail+1)); echo "FAIL: $1"; }

fixture() {  # $1=with_gff(yes|no) -> sets WS_TMP, exports the override vars
  WS_TMP="$(mktemp -d)"
  export WINSETUP_CHOICE_FILE="${WS_TMP}/cache/win-setup-choice"
  export WINSETUP_SENTINEL="${WS_TMP}/config/.skip_windows_setup"
  if [ "$1" = "yes" ]; then
    mkdir -p "${WS_TMP}/bin"
    cat > "${WS_TMP}/bin/gff" <<'STUB'
#!/bin/sh
echo "$@" >> "${GFF_STUB_LOG}"
exit 0
STUB
    chmod +x "${WS_TMP}/bin/gff"
    export GFF_STUB_LOG="${WS_TMP}/gff-calls.log"
    export WINSETUP_GFF="${WS_TMP}/bin/gff"
  else
    export WINSETUP_GFF="${WS_TMP}/no-such-gff"
  fi
}

run_case() {  # $1=shell $2=desc $3=snippet ; snippet sources both libs first
  _sh="$1"; _desc="$2"; _snip="$3"
  if "$_sh" -c ". '${here}/gff.sh'; . '${here}/winsetup.sh'; ${_snip}"; then
    ok "[$_sh] $_desc"
  else
    bad "[$_sh] $_desc"
  fi
}

for sh_bin in bash sh; do
  # 1. choice round-trip: save then take echoes it back and consumes the file
  fixture no
  run_case "$sh_bin" "choice round-trip y" '
    winsetup_save_choice y &&
    [ "$(winsetup_take_choice)" = "y" ] &&
    [ ! -f "$WINSETUP_CHOICE_FILE" ]'
  rm -rf "$WS_TMP"

  # 2. take with no file -> "none"
  fixture no
  run_case "$sh_bin" "take_choice absent -> none" '
    [ "$(winsetup_take_choice)" = "none" ]'
  rm -rf "$WS_TMP"

  # 3. skip_state: sentinel present, no gff -> skipped (0), sentinel kept
  fixture no
  run_case "$sh_bin" "sentinel honored without gff" '
    mkdir -p "$(dirname "$WINSETUP_SENTINEL")" && : > "$WINSETUP_SENTINEL" &&
    winsetup_skip_state && [ -f "$WINSETUP_SENTINEL" ]'
  rm -rf "$WS_TMP"

  # 4. skip_state: sentinel + working gff -> migrated (gff set called, file gone), still 0
  fixture yes
  run_case "$sh_bin" "sentinel migrates to gff override" '
    mkdir -p "$(dirname "$WINSETUP_SENTINEL")" && : > "$WINSETUP_SENTINEL" &&
    winsetup_skip_state >/dev/null &&
    [ ! -f "$WINSETUP_SENTINEL" ] &&
    grep -q "set install.windows.desktop-deploy false" "$GFF_STUB_LOG"'
  rm -rf "$WS_TMP"

  # 5. skip_state: no sentinel, env override false -> skipped (via gff_on)
  fixture no
  run_case "$sh_bin" "env override false -> skipped" '
    GFF_INSTALL_WINDOWS_DESKTOP_DEPLOY=false winsetup_skip_state'
  rm -rf "$WS_TMP"

  # 6. skip_state: nothing set -> NOT skipped (rc 1)
  fixture no
  run_case "$sh_bin" "clean state -> not skipped" '
    ! winsetup_skip_state'
  rm -rf "$WS_TMP"

  # 7. record_skip with gff -> gff set called, no sentinel written
  fixture yes
  run_case "$sh_bin" "record_skip delegates to gff" '
    winsetup_record_skip >/dev/null &&
    grep -q "set install.windows.desktop-deploy false" "$GFF_STUB_LOG" &&
    [ ! -f "$WINSETUP_SENTINEL" ]'
  rm -rf "$WS_TMP"

  # 8. record_skip without gff -> sentinel fallback written
  fixture no
  run_case "$sh_bin" "record_skip sentinel fallback" '
    winsetup_record_skip >/dev/null && [ -f "$WINSETUP_SENTINEL" ]'
  rm -rf "$WS_TMP"

  # 9. ask with no controlling tty -> __notty__ (setsid detaches; skip if absent)
  if command -v setsid >/dev/null 2>&1; then
    fixture no
    if [ "$(setsid "$sh_bin" -c ". '${here}/gff.sh'; . '${here}/winsetup.sh'; winsetup_ask" </dev/null)" = "__notty__" ]; then
      ok "[$sh_bin] ask without tty -> __notty__"
    else
      bad "[$sh_bin] ask without tty -> __notty__"
    fi
    rm -rf "$WS_TMP"
  else
    echo "SKIP: setsid unavailable — no-tty case deferred to the human matrix"
  fi
done

echo "----------------------------------------"
echo "winsetup_test: ${pass} passed, ${fail} failed"
[ "$fail" -eq 0 ]
