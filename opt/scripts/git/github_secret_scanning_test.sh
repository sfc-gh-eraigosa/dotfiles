#!/usr/bin/env bash
# Test driver for github_secret_scanning.sh — hermetic: `gh` is a stub on PATH
# whose repo state lives in a file, so no network and no real repo is touched.
#
# exit 0 = all pass, 1 = a failure.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/github_secret_scanning.sh"
PASS=0
FAIL=0
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$TMP/bin"
export STUB_STATE="$TMP/state"     # three lines: scanning, push-protection, non-provider
export STUB_LOG="$TMP/gh.log"      # every gh invocation, one per line
export STUB_AUTH_RC=0              # gh auth status exit code
export STUB_PATCH_RC=0             # PATCH exit code (1 => "HTTP 422")
export STUB_PRIVATE=false
export STUB_NONPROVIDER_STICKS=1  # 0 => GitHub accepts the PATCH but the setting stays disabled

cat > "$TMP/bin/gh" <<'SHIM'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$STUB_LOG"
case "$1 ${2:-}" in
  "auth status") exit "$STUB_AUTH_RC" ;;
  "repo view")   printf '{"nameWithOwner":"acme/dots"}\n'; exit 0 ;;
esac
[ "$1" = api ] || { echo "stub gh: unexpected $*" >&2; exit 99; }
shift
if [ "$1" = "-X" ] && [ "$2" = PATCH ]; then
  cat > "$STUB_STATE.patch-body"
  if [ "$STUB_PATCH_RC" != 0 ]; then
    echo 'gh: Advanced Security must be enabled for this repository to use secret scanning. (HTTP 422)' >&2
    exit 1
  fi
  if [ "${STUB_NONPROVIDER_STICKS:-1}" = 1 ]; then printf 'enabled\nenabled\nenabled\n' > "$STUB_STATE"
  else printf 'enabled\nenabled\ndisabled\n' > "$STUB_STATE"; fi
  exit 0
fi
# GET repos/<owner>/<name>
{ read -r s1; read -r s2; read -r s3; } < "$STUB_STATE"
printf '{"visibility":"%s","private":%s,"security_and_analysis":{"secret_scanning":{"status":"%s"},"secret_scanning_push_protection":{"status":"%s"},"secret_scanning_non_provider_patterns":{"status":"%s"}}}\n' \
  "$([ "$STUB_PRIVATE" = true ] && echo private || echo public)" "$STUB_PRIVATE" "$s1" "$s2" "$s3"
SHIM
chmod +x "$TMP/bin/gh"
export PATH="$TMP/bin:$PATH"

set_state() { printf '%s\n%s\n%s\n' "$1" "$2" "$3" > "$STUB_STATE"; : > "$STUB_LOG"; rm -f "$STUB_STATE.patch-body"; }
patches() { grep -c -- '-X PATCH' "$STUB_LOG" 2>/dev/null || true; }

# usage: expect <rc> <label> -- <args...>   (captures stdout+stderr in $OUT)
OUT=""
expect() {
    local want="$1" label="$2"; shift 2; [ "$1" = "--" ] && shift
    local rc
    OUT="$("$SCRIPT" "$@" 2>&1)"; rc=$?
    if [ "$rc" -eq "$want" ]; then echo "PASS: $label"; PASS=$((PASS+1))
    else echo "FAIL: $label (want exit $want, got $rc) :: $OUT"; FAIL=$((FAIL+1)); fi
}
check_out() { # usage: check_out <label> <grep -E pattern>
    if printf '%s' "$OUT" | grep -Eq -- "$2"; then echo "PASS: $1"; PASS=$((PASS+1))
    else echo "FAIL: $1 :: output was: $OUT"; FAIL=$((FAIL+1)); fi
}
check_eq() { # usage: check_eq <label> <want> <got>
    if [ "$2" = "$3" ]; then echo "PASS: $1"; PASS=$((PASS+1))
    else echo "FAIL: $1 (want '$2', got '$3')"; FAIL=$((FAIL+1)); fi
}

# ---- ensure (default mode) ------------------------------------------------------
set_state disabled disabled disabled
expect 0 "ensure: enables all three when disabled" -- --repo acme/dots
check_eq "ensure: exactly one PATCH" 1 "$(patches)"
check_out "ensure: PATCH body carries secret_scanning" 'secret_scanning' 
grep -q '"secret_scanning_push_protection"' "$STUB_STATE.patch-body" && { echo "PASS: ensure: body enables push protection"; PASS=$((PASS+1)); } || { echo "FAIL: ensure: body lacks push protection"; FAIL=$((FAIL+1)); }
grep -q '"secret_scanning_non_provider_patterns"' "$STUB_STATE.patch-body" && { echo "PASS: ensure: body enables non-provider patterns"; PASS=$((PASS+1)); } || { echo "FAIL: ensure: body lacks non-provider patterns"; FAIL=$((FAIL+1)); }
check_out "ensure: reports what it enabled" 'enabled'

set_state enabled enabled enabled
expect 0 "ensure: already enabled is a no-op" -- --repo acme/dots
check_eq "ensure: no PATCH when already enabled" 0 "$(patches)"
check_out "ensure: says already enabled" 'already'

set_state enabled disabled enabled
expect 0 "ensure: enables only the missing one" -- --repo acme/dots
check_eq "ensure: one PATCH for a partial state" 1 "$(patches)"

set_state disabled disabled disabled
expect 0 "ensure: repo defaults to the current checkout (gh repo view)" --
grep -q 'repos/acme/dots' "$STUB_LOG" && { echo "PASS: ensure: resolved acme/dots from gh repo view"; PASS=$((PASS+1)); } || { echo "FAIL: ensure: did not resolve repo"; FAIL=$((FAIL+1)); }

# ---- check mode --------------------------------------------------------------------
set_state disabled enabled disabled
expect 1 "check: exit 1 when anything is disabled" -- --check --repo acme/dots
check_out "check: names the disabled settings" 'secret_scanning.*disabled'
check_out "check: names non-provider patterns as disabled" 'non_provider_patterns.*disabled'
check_eq "check: never PATCHes" 0 "$(patches)"

set_state enabled enabled enabled
expect 0 "check: exit 0 when all enabled" -- --check --repo acme/dots

# Non-provider patterns are best-effort: GitHub accepts the PATCH (HTTP 200) yet
# leaves the setting disabled on repos without GitHub Secret Protection. That
# must be a WARN, not a permanently red check — the two core settings are the gate.
set_state disabled disabled disabled
STUB_NONPROVIDER_STICKS=0 expect 0 "ensure: non-provider patterns not honoured => WARN, exit 0" -- --repo acme/dots
check_out "ensure: warns that non-provider patterns did not stick" 'WARN.*non_provider_patterns'
set_state enabled enabled disabled
expect 0 "check: core settings on, non-provider off => exit 0 with WARN" -- --check --repo acme/dots
check_out "check: WARN names non-provider patterns" 'WARN.*non_provider_patterns'

# ---- degraded environments -----------------------------------------------------------
set_state disabled disabled disabled
STUB_AUTH_RC=1 expect 0 "ensure: not authenticated => SKIP, exit 0 (install must not break offline)" -- --repo acme/dots
check_out "ensure: says SKIP when unauthenticated" 'SKIP'
STUB_AUTH_RC=1 expect 2 "check: not authenticated => exit 2 (cannot determine)" -- --check --repo acme/dots

# a PATH with jq but no gh (CI runners have gh in /usr/bin, so /usr/bin alone is not "gh missing")
mkdir -p "$TMP/nogh"; for t in bash jq sed env; do ln -s "$(command -v "$t")" "$TMP/nogh/$t"; done
PATH="$TMP/nogh" expect 0 "ensure: gh missing => SKIP, exit 0" -- --repo acme/dots
PATH="$TMP/nogh" expect 2 "check: gh missing => exit 2" -- --check --repo acme/dots

# ---- private repo without Advanced Security -------------------------------------------
set_state disabled disabled disabled
STUB_PRIVATE=true STUB_PATCH_RC=1 expect 1 "ensure: private repo PATCH refused => exit 1" -- --repo acme/dots
check_out "ensure: explains the paid requirement for private repos" 'Secret Protection|Advanced Security'

echo "----"
echo "github_secret_scanning_test: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
