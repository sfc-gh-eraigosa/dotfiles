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
#   --check    report; exit 1 if a core setting (scanning, push protection) is
#              disabled, 2 if it cannot be read. Non-provider patterns are
#              best-effort: WARN only (see below)
#   --status   report only, exit 0
#   --repo     target (default: the repo of the current checkout via gh)
#
# Cost: free on public repos. A PRIVATE repo needs GitHub Secret Protection
# (paid, per active committer); the PATCH is refused with HTTP 422 there and
# this script says so instead of guessing. Non-provider patterns (generic
# secrets) turned out to need Secret Protection even on a public user-owned
# repo: GitHub answers 200 and leaves it disabled — observed 2026-09-05 — so it
# is requested, re-read, and warned about rather than failing the check.
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

# Non-provider patterns (generic secrets: private keys, connection strings) are
# BEST-EFFORT: GitHub answers the PATCH with 200 yet leaves the setting disabled
# on repositories without GitHub Secret Protection. So it is asked for, reported,
# and warned about — but only the two core settings gate --check.
CORE="secret_scanning secret_scanning_push_protection"   # everything else in SETTINGS is best-effort
read_states() { # fills DISABLED (core) and SOFT (best-effort) from a repo JSON in $1
    DISABLED=""; SOFT=""
    local s st
    for s in $SETTINGS; do
        st="$(printf '%s' "$1" | jq -r ".security_and_analysis.${s}.status // \"disabled\"")"
        echo "$REPO: $s = $st"
        [ "$st" = enabled ] && continue
        case " $CORE " in *" $s "*) DISABLED="$DISABLED $s" ;; *) SOFT="$SOFT $s" ;; esac
    done
}
soft_warn() {
    [ -n "$SOFT" ] || return 0
    echo "WARN: not honoured on $REPO:$SOFT — GitHub accepts the request but keeps it disabled without GitHub Secret Protection (best-effort; the local privacy hooks cover generic secret shapes via gitleaks)." >&2
}
read_states "$INFO"

case "$MODE" in
    status) soft_warn; exit 0 ;;
    check)
        if [ -n "$DISABLED" ]; then
            echo "FAIL: disabled on $REPO:$DISABLED (run: make secret-scanning)" >&2
            exit 1
        fi
        soft_warn
        echo "OK: secret scanning and push protection are enabled on $REPO"
        exit 0 ;;
esac

# ensure — ask for everything that is off, core and best-effort alike
WANT="$DISABLED$SOFT"
if [ -z "$WANT" ]; then
    echo "OK: already enabled on $REPO — nothing to do"
    exit 0
fi

BODY='{"security_and_analysis":{'
first=1
for s in $WANT; do
    [ "$first" = 1 ] || BODY="$BODY,"
    BODY="$BODY\"$s\":{\"status\":\"enabled\"}"
    first=0
done
BODY="$BODY}}"

echo "Enabling on $REPO:$WANT"
if ! ERR="$(printf '%s' "$BODY" | gh api -X PATCH "repos/$REPO" --input - 2>&1 >/dev/null)"; then
    echo "FAIL: GitHub refused the change for $REPO: $ERR" >&2
    if [ "$PRIVATE" = true ]; then
        echo "  $REPO is private. Secret scanning on private repos needs GitHub Secret Protection" >&2
        echo "  (formerly Advanced Security; paid per active committer). Enable billing for it or" >&2
        echo "  keep relying on the local privacy hooks (make hook-test) for this repo." >&2
    fi
    exit 1
fi
# Re-read: what actually stuck? Core settings must; best-effort ones may not.
AFTER="$(gh api "repos/$REPO" 2>/dev/null)" || { echo "OK: change accepted for $REPO:$WANT (could not re-read to confirm)"; exit 0; }
read_states "$AFTER" >/dev/null
if [ -n "$DISABLED" ]; then
    echo "FAIL: GitHub accepted the change but these are still disabled on $REPO:$DISABLED" >&2
    exit 1
fi
soft_warn
echo "OK: enabled on $REPO:$WANT"
