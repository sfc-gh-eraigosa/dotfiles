#!/usr/bin/env bash
# groom-apply.sh — apply a reviewed verdicts file to GitHub: labels, closes, body refreshes.
#
# Usage: groom-apply.sh VERDICTS.json [--dry-run] [--only 12,34] [-R owner/name]
#
# VERDICTS.json is an array of objects (see references/verdict-schema.md); the fields used here:
#   number            issue number
#   verdict           addressed | partially | open | obsolete
#   priority          1..5  -> the issue gets exactly one priority:N label (others removed)
#   suggested_action  close-with-evidence | relabel-only | edit-body | edit-body-and-relabel | none
#                     (none = verified, nothing to change; the label is still reconciled)
#   close_comment     posted before closing (close-with-evidence only)
#   new_body          the refreshed body (edit-body*). The CURRENT body is folded beneath it in a
#                     <details> block automatically, so nothing the author wrote is lost.
#
# Close reason: addressed -> completed; obsolete -> "not planned". Any other verdict with a
# close action is refused (a partial/open issue must not be closed by a grooming pass).
# Every action is echoed; with --dry-run nothing is sent. Applied actions are appended to
# <VERDICTS dir>/applied.log so a re-run can be audited.
set -euo pipefail

VERDICTS=""
DRY=0
ONLY=""
REPO=""
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY=1 ;;
    --only) ONLY="${2:-}"; shift ;;
    -R|--repo) REPO="${2:-}"; shift ;;
    -h|--help) sed -n '2,18p' "$0"; exit 0 ;;
    -*) echo "groom-apply: unknown arg '$1'" >&2; exit 1 ;;
    *) VERDICTS="$1" ;;
  esac
  shift
done
[ -n "$VERDICTS" ] && [ -f "$VERDICTS" ] || { echo "groom-apply: verdicts file required" >&2; exit 1; }
command -v gh >/dev/null 2>&1 || { echo "groom-apply: gh is required" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "groom-apply: jq is required" >&2; exit 1; }
jq -e 'type == "array"' "$VERDICTS" >/dev/null || { echo "groom-apply: $VERDICTS is not a JSON array" >&2; exit 1; }

GH=(gh)
[ -n "$REPO" ] && GH+=(-R "$REPO")
LOG="$(dirname "$VERDICTS")/applied.log"
TODAY=$(date +%Y-%m-%d)

gh_retry() {
  # run a read/write gh call with the same three-attempt retry as run(), silently
  local attempt=1
  while :; do
    if "$@"; then return 0; fi
    [ "$attempt" -ge 3 ] && return 1
    attempt=$((attempt + 1))
    sleep $((attempt * 5))
  done
}

run() {
  # echo the action; execute unless dry-run. gh calls hit transient network errors
  # (TLS handshake timeouts) on long passes, so retry three times before giving up.
  echo "  > $*"
  [ "$DRY" -eq 0 ] || return 0
  local attempt=1
  while :; do
    if "$@"; then return 0; fi
    if [ "$attempt" -ge 3 ]; then
      echo "  !! failed after $attempt attempts: $*" >&2
      return 1
    fi
    attempt=$((attempt + 1))
    sleep $((attempt * 5))
  done
}

in_only() {
  [ -z "$ONLY" ] && return 0
  case ",$ONLY," in *",$1,"*) return 0 ;; esac
  return 1
}

count=$(jq length "$VERDICTS")
i=0
while [ "$i" -lt "$count" ]; do
  row=$(jq -c ".[$i]" "$VERDICTS")
  i=$((i + 1))
  n=$(printf '%s' "$row" | jq -r '.number')
  in_only "$n" || continue
  verdict=$(printf '%s' "$row" | jq -r '.verdict')
  prio=$(printf '%s' "$row" | jq -r '.priority // empty')
  action=$(printf '%s' "$row" | jq -r '.suggested_action // "relabel-only"')
  echo "#$n  verdict=$verdict priority=${prio:-?} action=$action"

  # --- priority label: exactly one priority:N -------------------------------------------
  if [ -n "$prio" ]; then
    case "$prio" in
      1|2|3|4|5) ;;
      *) echo "  !! priority must be 1..5 (got '$prio'); skipping #$n" >&2; continue ;;
    esac
    current=$(gh_retry "${GH[@]}" issue view "$n" --json labels --jq '[.labels[].name | select(startswith("priority:"))] | join(",")')
    remove=""
    oldifs=$IFS; IFS=','
    for l in $current; do
      [ -n "$l" ] && [ "$l" != "priority:$prio" ] && remove="${remove:+$remove,}$l"
    done
    IFS=$oldifs
    if [ -n "$remove" ]; then
      run "${GH[@]}" issue edit "$n" --remove-label "$remove" --add-label "priority:$prio"
    elif [ "$current" = "priority:$prio" ]; then
      echo "  = already priority:$prio"
    else
      run "${GH[@]}" issue edit "$n" --add-label "priority:$prio"
    fi
  fi

  # --- body refresh --------------------------------------------------------------------
  case "$action" in
    edit-body|edit-body-and-relabel)
      new_body=$(printf '%s' "$row" | jq -r '.new_body // empty')
      if [ -z "$new_body" ]; then
        echo "  !! $action without new_body; skipping body edit for #$n" >&2
      else
        old_body=$(gh_retry "${GH[@]}" issue view "$n" --json body --jq '.body')
        body=$(printf '%s\n\n<details>\n<summary>Original body (before grooming %s)</summary>\n\n%s\n\n</details>\n' "$new_body" "$TODAY" "$old_body")
        if [ "$DRY" -eq 1 ]; then
          echo "  > gh issue edit $n --body-file <refreshed body, $(printf '%s' "$body" | wc -c | tr -d ' ') bytes>"
        else
          tmp=$(mktemp)
          printf '%s' "$body" > "$tmp"
          gh_retry "${GH[@]}" issue edit "$n" --body-file "$tmp" >/dev/null
          rm -f "$tmp"
          echo "  > body refreshed (original folded)"
        fi
      fi
      ;;
  esac

  # --- close ---------------------------------------------------------------------------
  case "$action" in
    close-with-evidence)
      case "$verdict" in
        addressed) reason="completed" ;;
        obsolete) reason="not planned" ;;
        *) echo "  !! refusing to close #$n: verdict '$verdict' is not addressed/obsolete" >&2; continue ;;
      esac
      comment=$(printf '%s' "$row" | jq -r '.close_comment // empty')
      [ -n "$comment" ] || { echo "  !! close-with-evidence needs close_comment; skipping close of #$n" >&2; continue; }
      run "${GH[@]}" issue close "$n" --reason "$reason" --comment "$(printf 'Grooming %s — closing as %s.\n\n%s' "$TODAY" "$reason" "$comment")"
      ;;
  esac

  [ "$DRY" -eq 0 ] && printf '%s\t#%s\t%s\tpriority:%s\t%s\n' "$TODAY" "$n" "$verdict" "${prio:-}" "$action" >> "$LOG"
done
[ "$DRY" -eq 1 ] && echo "dry-run: nothing was sent"
exit 0
