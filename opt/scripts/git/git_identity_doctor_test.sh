#!/usr/bin/env bash
# Test driver for git_identity_doctor.sh. Uses a fake `gh` shim on PATH and an
# isolated HOME so no network, auth, or real git config is touched. CI-safe.

set -u
DOCTOR="$(cd -- "$(dirname "$0")" && pwd -P)/git_identity_doctor.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# --- fake gh shim: behavior driven by GH_FAKE_MODE ---------------------------
# Modes: noauth | noscope | ok  (account: id 42, login testuser, public email
# pub@x.com in mode ok; priv@x.com is on the account but private)
mkdir -p "$TMP/bin"
cat > "$TMP/bin/gh" <<'SHIM'
#!/usr/bin/env bash
mode="${GH_FAKE_MODE:-ok}"
cmd="${1:-}"; shift || true
[ "$mode" = "noauth" ] && exit 1
if [ "$cmd" = "auth" ]; then exit 0; fi
if [ "$cmd" = "api" ]; then
  path="${1:-}"; shift || true
  jq_expr=""
  while [ $# -gt 0 ]; do
    if [ "$1" = "--jq" ]; then jq_expr="${2:-}"; shift; fi
    shift || true
  done
  case "$path" in
    user)
      case "$jq_expr" in
        *id*) echo 42 ;;
        *login*) echo testuser ;;
      esac ;;
    users/testuser)
      echo "pub@x.com" ;;
    user/emails)
      if [ "$mode" = "noscope" ]; then exit 1; fi
      case "$jq_expr" in
        *priv@x.com*) echo "private" ;;
        *pub@x.com*)  echo "public" ;;
        *)            echo "absent" ;;
      esac ;;
  esac
  exit 0
fi
exit 0
SHIM
chmod +x "$TMP/bin/gh"

# Isolated HOME so --global config is ours; keep real PATH for git/jq.
export HOME="$TMP/home"
mkdir -p "$HOME"
export PATH="$TMP/bin:$PATH"
git config --global user.name "Test User"

REPO="$TMP/repo"
git init -q "$REPO"

FAILS=0
run_case() {
  desc="$1" mode="$2" email="$3" want_rc="$4" want_grep="$5"
  git config --global user.email "$email"
  git -C "$REPO" config user.email "$email"
  out="$(cd "$REPO" && GH_FAKE_MODE="$mode" bash "$DOCTOR" 2>&1)"
  rc=$?
  if [ "$rc" -ne "$want_rc" ]; then
    echo "FAIL: $desc — want exit $want_rc, got $rc"; echo "$out" | head -5
    FAILS=$((FAILS + 1)); return
  fi
  if ! printf '%s' "$out" | grep -q "$want_grep"; then
    echo "FAIL: $desc — output missing '$want_grep'"; echo "$out" | head -5
    FAILS=$((FAILS + 1)); return
  fi
  echo "PASS: $desc"
}

run_case "gh unauthenticated degrades to info, exit 0" \
  noauth "whatever@x.com" 0 "account checks skipped"

run_case "ID-based noreply passes" \
  ok "42+testuser@users.noreply.github.com" 0 "ID-based noreply"

run_case "legacy username-only noreply warns" \
  ok "testuser@users.noreply.github.com" 1 "legacy username-only"

run_case "noreply of a different account warns" \
  ok "99+other@users.noreply.github.com" 1 "DIFFERENT account"

run_case "public email passes" \
  ok "pub@x.com" 0 "public email"

run_case "private email FAILS with push-block explanation" \
  ok "priv@x.com" 2 "PRIVATE email"

run_case "unassociated email warns about attribution" \
  ok "stranger@nowhere.com" 1 "not associated"

run_case "missing user scope warns with refresh hint" \
  noscope "priv@x.com" 1 "gh auth refresh"

# Empty email: unset both scopes then expect a warning.
git config --global --unset user.email
git -C "$REPO" config --unset user.email
out="$(cd "$REPO" && GH_FAKE_MODE=ok bash "$DOCTOR" 2>&1)"; rc=$?
if [ "$rc" -eq 1 ] && printf '%s' "$out" | grep -q "user.email is not set"; then
  echo "PASS: unset user.email warns"
else
  echo "FAIL: unset user.email — exit $rc"; echo "$out" | head -5; FAILS=$((FAILS + 1))
fi

if [ "$FAILS" -gt 0 ]; then
  echo "$FAILS case(s) failed"
  exit 1
fi
echo "All git_identity_doctor cases passed"
