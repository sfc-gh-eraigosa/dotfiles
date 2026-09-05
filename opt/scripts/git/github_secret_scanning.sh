#!/usr/bin/env bash
# github_secret_scanning.sh — make sure GitHub's server-side secret backstop is
# on for a repository: secret scanning (alerts + partner revocation, full
# history), push protection (rejects a push carrying a known secret shape),
# and non-provider patterns (private keys, connection strings).
#
# This is the layer our local privacy hooks cannot be: it runs on GitHub's side
# of the wire, so it covers history and any machine where install.sh never ran.
# It knows nothing about identity (usernames, hosts, emails) — that stays ours.
#
# Usage: github_secret_scanning.sh [--check|--status] [--repo owner/name]
#   (default)  ensure — enable whatever is disabled (idempotent, one PATCH)
#   --check    report; exit 1 if anything is disabled, 2 if it cannot be read
#   --status   report only, exit 0
#   --repo     target (default: the repo of the current checkout via gh)
#
# Cost: free on public repos. A PRIVATE repo needs GitHub Secret Protection
# (paid, per active committer); the PATCH is refused with HTTP 422 there and
# this script says so instead of guessing.
#
# Exit codes — ensure: 0 enabled/already/SKIP (no gh or not authenticated:
# install.sh must not break offline) · 1 could not enable
#             check:  0 all enabled · 1 something disabled · 2 cannot determine
set -u

MODE=ensure
REPO=""
while [ $# -gt 0 ]; do
    case "$1" in
        --check)  MODE=check ;;
        --status) MODE=status ;;
        --repo)   shift; REPO="${1:-}" ;;
        -h|--help) sed -n '2,24p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *) echo "unknown argument: $1" >&2; exit 2 ;;
    esac
    shift
done

SETTINGS="secret_scanning secret_scanning_push_protection secret_scanning_non_provider_patterns"

skip() { # $1 = reason. ensure => 0 (offline install is fine), check => 2
    if [ "$MODE" = check ]; then echo "FAIL: cannot read secret-scanning settings: $1" >&2; exit 2; fi
    echo "SKIP: github_secret_scanning — $1"; exit 0
}

command -v gh >/dev/null 2>&1 || skip "gh is not installed"
command -v jq >/dev/null 2>&1 || skip "jq is not installed"
gh auth status >/dev/null 2>&1 || skip "gh is not authenticated (gh auth login)"

if [ -z "$REPO" ]; then
    REPO="$(gh repo view --json nameWithOwner 2>/dev/null | jq -r '.nameWithOwner // empty')"
    [ -n "$REPO" ] || skip "not inside a GitHub repository checkout (pass --repo owner/name)"
fi

INFO="$(gh api "repos/$REPO" 2>/dev/null)" || skip "could not read repos/$REPO"
PRIVATE="$(printf '%s' "$INFO" | jq -r '.private // false')"

# security_and_analysis is only returned to repo admins; absent => cannot judge.
if [ "$(printf '%s' "$INFO" | jq -r '.security_and_analysis // empty | type')" != "object" ]; then
    skip "security_and_analysis not visible for $REPO (admin permission required)"
fi

DISABLED=""
for s in $SETTINGS; do
    st="$(printf '%s' "$INFO" | jq -r ".security_and_analysis.${s}.status // \"disabled\"")"
    echo "$REPO: $s = $st"
    [ "$st" = enabled ] || DISABLED="$DISABLED $s"
done

case "$MODE" in
    status) exit 0 ;;
    check)
        if [ -n "$DISABLED" ]; then
            echo "FAIL: disabled on $REPO:$DISABLED (run: make secret-scanning)" >&2
            exit 1
        fi
        echo "OK: secret scanning, push protection and non-provider patterns are enabled on $REPO"
        exit 0 ;;
esac

# ensure
if [ -z "$DISABLED" ]; then
    echo "OK: already enabled on $REPO — nothing to do"
    exit 0
fi

BODY='{"security_and_analysis":{'
first=1
for s in $DISABLED; do
    [ "$first" = 1 ] || BODY="$BODY,"
    BODY="$BODY\"$s\":{\"status\":\"enabled\"}"
    first=0
done
BODY="$BODY}}"

echo "Enabling on $REPO:$DISABLED"
if ! ERR="$(printf '%s' "$BODY" | gh api -X PATCH "repos/$REPO" --input - 2>&1 >/dev/null)"; then
    echo "FAIL: GitHub refused the change for $REPO: $ERR" >&2
    if [ "$PRIVATE" = true ]; then
        echo "  $REPO is private. Secret scanning on private repos needs GitHub Secret Protection" >&2
        echo "  (formerly Advanced Security; paid per active committer). Enable billing for it or" >&2
        echo "  keep relying on the local privacy hooks (make hook-test) for this repo." >&2
    fi
    exit 1
fi
echo "OK: enabled on $REPO:$DISABLED"
