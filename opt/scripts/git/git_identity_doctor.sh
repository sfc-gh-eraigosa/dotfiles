#!/usr/bin/env bash
# git_identity_doctor.sh — verify the git identity config matches the
# authenticated GitHub account, catching mismatches BEFORE they fail at push
# time (e.g. GitHub's "block command line pushes that expose my email"
# protection, which rejects pushes whose commits carry a private account
# email — there is no API for the toggle itself, so we check the risk
# condition instead).
#
# Usage: git_identity_doctor.sh [repo-path ...]
#   Checks the global git config plus each given repo (default: the current
#   repo when run inside one). Requires gh for account checks; degrades to
#   local-only checks without it. The optional `user` token scope
#   (gh auth refresh -h github.com -s user) enables definitive
#   email-visibility verdicts.
#
# Exit codes: 0 all OK · 1 warnings · 2 failures (push likely rejected)

set -u

WARNINGS=0
FAILURES=0

ok()   { echo "OK:   $*"; }
info() { echo "INFO: $*"; }
warn() { echo "WARN: $*"; WARNINGS=$((WARNINGS + 1)); }
fail() { echo "FAIL: $*"; FAILURES=$((FAILURES + 1)); }

GH_READY=0
GH_ID=""
GH_LOGIN=""
GH_PUBLIC_EMAIL=""
if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
  GH_ID="$(gh api user --jq '.id' 2>/dev/null)"
  GH_LOGIN="$(gh api user --jq '.login' 2>/dev/null)"
  if [ -n "$GH_ID" ] && [ -n "$GH_LOGIN" ]; then
    GH_READY=1
    GH_PUBLIC_EMAIL="$(gh api "users/${GH_LOGIN}" --jq '.email // ""' 2>/dev/null)"
  fi
fi
if [ "$GH_READY" = "1" ]; then
  info "GitHub account: ${GH_LOGIN} (id ${GH_ID})"
else
  info "gh not installed or not authenticated — account checks skipped"
fi

# email_visibility <email> -> prints public|private|absent|unknown
# "private" here means: on the account but not public (visibility private or
# null) — exactly the set the push-privacy block rejects.
email_visibility() {
  v="$(gh api user/emails \
    --jq "map(select(.email == \"$1\")) | if length == 0 then \"absent\" else (.[0].visibility // \"private\") end" \
    2>/dev/null)" || { echo "unknown"; return; }
  [ -z "$v" ] && v="unknown"
  echo "$v"
}

check_email() {
  scope="$1" email="$2"
  if [ -z "$email" ]; then
    warn "$scope: user.email is not set"
    return
  fi
  if [ "$GH_READY" != "1" ]; then
    info "$scope: user.email=$email (unverified — gh unavailable)"
    return
  fi
  case "$email" in
    "${GH_ID}+${GH_LOGIN}@users.noreply.github.com")
      ok "$scope: user.email is the account's ID-based noreply (rename-proof, never blocked)"
      return ;;
    "${GH_LOGIN}@users.noreply.github.com")
      warn "$scope: legacy username-only noreply — only pre-2017 accounts keep attribution with this form; use ${GH_ID}+${GH_LOGIN}@users.noreply.github.com"
      return ;;
    *@users.noreply.github.com)
      warn "$scope: user.email=$email is a noreply for a DIFFERENT account/username than ${GH_LOGIN} (stale after a rename?)"
      return ;;
  esac
  if [ -n "$GH_PUBLIC_EMAIL" ] && [ "$email" = "$GH_PUBLIC_EMAIL" ]; then
    ok "$scope: user.email matches the account's public email"
    return
  fi
  case "$(email_visibility "$email")" in
    public)
      ok "$scope: user.email is a public email on the account" ;;
    private)
      fail "$scope: user.email=$email is a PRIVATE email on ${GH_LOGIN} — pushes are rejected if 'Block command line pushes that expose my email' is enabled. Use ${GH_ID}+${GH_LOGIN}@users.noreply.github.com or make the email public." ;;
    absent)
      warn "$scope: user.email=$email is not associated with ${GH_LOGIN} — commits will not be attributed to the account" ;;
    *)
      warn "$scope: user.email=$email — cannot verify visibility (token lacks the 'user' scope; run: gh auth refresh -h github.com -s user). If the email-privacy push block is enabled and this email is private, pushes will be rejected." ;;
  esac
}

GLOBAL_EMAIL="$(git config --global user.email 2>/dev/null || true)"
GLOBAL_NAME="$(git config --global user.name 2>/dev/null || true)"
[ -z "$GLOBAL_NAME" ] && warn "global: user.name is not set"
check_email "global" "$GLOBAL_EMAIL"

if [ "$#" -gt 0 ]; then
  for repo in "$@"; do
    if git -C "$repo" rev-parse --git-dir >/dev/null 2>&1; then
      check_email "repo:$repo" "$(git -C "$repo" config user.email 2>/dev/null || true)"
    else
      warn "repo:$repo is not a git repository"
    fi
  done
elif git rev-parse --git-dir >/dev/null 2>&1; then
  check_email "repo:$(pwd)" "$(git config user.email 2>/dev/null || true)"
fi

if ! git config --get-regexp '^credential\.' 2>/dev/null | grep -q "gh auth git-credential"; then
  info "gh is not the git credential helper for github.com (auth may be managed elsewhere)"
fi

[ "$FAILURES" -gt 0 ] && exit 2
[ "$WARNINGS" -gt 0 ] && exit 1
exit 0
