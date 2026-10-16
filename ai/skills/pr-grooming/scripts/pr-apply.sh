#!/usr/bin/env bash
# pr-apply.sh — apply a reviewed PR verdicts file to GitHub: comments, closes, ready, labels.
#
# Usage: pr-apply.sh VERDICTS.json [--dry-run] [--only 12,34] [-R owner/name]
#
# VERDICTS.json is an array (see references/verdict-schema.md); the fields used here:
#   number          PR number
#   verdict         stale | duplicate | refresh | rebase-only | ready | leave
#   suggested_action
#       close-stale        comment + close (verdict must be stale or duplicate; needs close_comment)
#       comment            post `comment` only (a review-refresh note, a decision request)
#       ready              `gh pr ready` (+ comment if given)        — verdict refresh
#       ready-for-merge    add the ready-for-merge label (+ comment) — verdict ready; refuses drafts,
#                          failing CI, and non-passing merge state unless ci_verified is true
#       none               nothing (rebases are done by pr-rebase.sh, not here)
#   close_comment   required for close-stale; must say why it is stale and name the successor
#   comment         optional for ready / ready-for-merge / comment
#   ci_verified     true when the caller checked the rebased head's CI themselves
# Every action is echoed; with --dry-run nothing is sent. Actions append to <dir>/applied.log.
# Branches are never deleted: a closed PR keeps its branch so the work can be recovered.
set -euo pipefail

VERDICTS=""; DRY=0; ONLY=""; REPO=""
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY=1 ;;
    --only) ONLY="${2:-}"; shift ;;
    -R|--repo) REPO="${2:-}"; shift ;;
    -h|--help) sed -n '2,19p' "$0"; exit 0 ;;
    -*) echo "pr-apply: unknown arg '$1'" >&2; exit 1 ;;
    *) VERDICTS="$1" ;;
  esac
  shift
done
[ -n "$VERDICTS" ] && [ -f "$VERDICTS" ] || { echo "pr-apply: verdicts file required" >&2; exit 1; }
command -v gh >/dev/null 2>&1 || { echo "pr-apply: gh is required" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "pr-apply: jq is required" >&2; exit 1; }
jq -e 'type == "array"' "$VERDICTS" >/dev/null || { echo "pr-apply: $VERDICTS is not a JSON array" >&2; exit 1; }

GH=(gh)
[ -n "$REPO" ] && GH+=(-R "$REPO")
LOG="$(dirname "$VERDICTS")/applied.log"
TODAY=$(date +%Y-%m-%d)

gh_retry() {
  local attempt=1
  while :; do
    if "$@"; then return 0; fi
    [ "$attempt" -ge 3 ] && return 1
    attempt=$((attempt + 1)); sleep $((attempt * 5))
  done
}
run() {
  # echo the action; execute unless dry-run; retry transient gh failures three times.
  echo "  > $*"
  [ "$DRY" -eq 0 ] || return 0
  local attempt=1
  while :; do
    if "$@"; then return 0; fi
    if [ "$attempt" -ge 3 ]; then echo "  !! failed after $attempt attempts: $*" >&2; return 1; fi
    attempt=$((attempt + 1)); sleep $((attempt * 5))
  done
}
in_only() {
  [ -z "$ONLY" ] && return 0
  case ",$ONLY," in *",$1,"*) return 0 ;; esac
  return 1
}
ci_state() {
  gh_retry "${GH[@]}" pr view "$1" --json isDraft,mergeStateStatus,statusCheckRollup --jq '
    [.statusCheckRollup[]? | (.conclusion // .state // "PENDING") | ascii_upcase] as $c |
    "\(.isDraft)\t\(.mergeStateStatus)\t\(if ($c | any(. == "FAILURE" or . == "ERROR")) then "failing" elif ($c | any(. == "PENDING" or . == "IN_PROGRESS" or . == "QUEUED" or . == "EXPECTED")) then "pending" elif ($c|length) == 0 then "none" else "passing" end)"'
}

count=$(jq length "$VERDICTS")
i=0
while [ "$i" -lt "$count" ]; do
  row=$(jq -c ".[$i]" "$VERDICTS"); i=$((i + 1))
  n=$(printf '%s' "$row" | jq -r '.number')
  in_only "$n" || continue
  verdict=$(printf '%s' "$row" | jq -r '.verdict')
  action=$(printf '%s' "$row" | jq -r '.suggested_action // "none"')
  comment=$(printf '%s' "$row" | jq -r '.comment // empty')
  echo "#$n  verdict=$verdict action=$action"
  case "$action" in
    none) ;;
    comment)
      [ -n "$comment" ] || { echo "  !! comment action without comment text; skipping #$n" >&2; continue; }
      run "${GH[@]}" pr comment "$n" --body "$(printf 'PR grooming %s\n\n%s' "$TODAY" "$comment")"
      ;;
    close-stale)
      case "$verdict" in stale|duplicate) ;; *) echo "  !! refusing to close #$n: verdict '$verdict' is not stale/duplicate" >&2; continue ;; esac
      cc=$(printf '%s' "$row" | jq -r '.close_comment // empty')
      [ -n "$cc" ] || { echo "  !! close-stale needs close_comment; skipping #$n" >&2; continue; }
      run "${GH[@]}" pr close "$n" --comment "$(printf 'PR grooming %s — closing as %s. The branch is kept; reopen if this is still wanted.\n\n%s' "$TODAY" "$verdict" "$cc")"
      ;;
    ready)
      [ "$verdict" = "refresh" ] || { echo "  !! ready is for verdict refresh (got '$verdict'); skipping #$n" >&2; continue; }
      run "${GH[@]}" pr ready "$n"
      [ -z "$comment" ] || run "${GH[@]}" pr comment "$n" --body "$(printf 'PR grooming %s — refreshed, ready for review.\n\n%s' "$TODAY" "$comment")"
      ;;
    ready-for-merge)
      [ "$verdict" = "ready" ] || { echo "  !! ready-for-merge is for verdict ready (got '$verdict'); skipping #$n" >&2; continue; }
      verified=$(printf '%s' "$row" | jq -r '.ci_verified // false')
      st=$(ci_state "$n"); draft=${st%%$'\t'*}; rest=${st#*$'\t'}; ms=${rest%%$'\t'*}; ci=${rest#*$'\t'}
      if [ "$draft" = "true" ]; then echo "  !! #$n is a draft; not labelling" >&2; continue; fi
      if [ "$ci" != "passing" ] && [ "$verified" != "true" ]; then echo "  !! #$n CI is '$ci' (mergeState $ms); not labelling — set ci_verified after checking" >&2; continue; fi
      case "$ms" in CLEAN|BLOCKED|UNSTABLE|BEHIND) ;; *) echo "  !! #$n mergeState is $ms; rebase first" >&2; continue ;; esac
      run "${GH[@]}" pr edit "$n" --add-label ready-for-merge
      [ -z "$comment" ] || run "${GH[@]}" pr comment "$n" --body "$(printf 'PR grooming %s — queued for merge.\n\n%s' "$TODAY" "$comment")"
      ;;
    *) echo "  !! unknown action '$action' for #$n" >&2; continue ;;
  esac
  [ "$DRY" -eq 0 ] && printf '%s\t#%s\t%s\t%s\n' "$TODAY" "$n" "$verdict" "$action" >> "$LOG"
done
[ "$DRY" -eq 1 ] && echo "dry-run: nothing was sent"
exit 0
