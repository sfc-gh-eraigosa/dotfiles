#!/usr/bin/env bash
# pr-rebase.sh — rebase a PR's head branch onto its base in a throwaway worktree and push it.
#
# Usage: pr-rebase.sh PR_NUMBER [--work-dir DIR] [--dry-run] [-R owner/name]
#
# Steps: fetch base + head, `git worktree add` under --work-dir (default: a temp dir), rebase
# head onto origin/<base>, push --force-with-lease, remove the worktree. On a conflict it leaves
# the worktree in place, prints its path and the conflicting files, and exits 2 so the caller
# can resolve by hand (then `git rebase --continue` + `git push --force-with-lease` there).
# Never touches the caller's checkout. Refuses PRs whose head lives in a fork.
set -euo pipefail

PR=""
WORK=""
DRY=0
REPO=""
while [ $# -gt 0 ]; do
  case "$1" in
    --work-dir) WORK="${2:-}"; shift ;;
    --dry-run) DRY=1 ;;
    -R|--repo) REPO="${2:-}"; shift ;;
    -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
    -*) echo "pr-rebase: unknown arg '$1'" >&2; exit 1 ;;
    *) PR="$1" ;;
  esac
  shift
done
[ -n "$PR" ] || { echo "pr-rebase: PR number required" >&2; exit 1; }
command -v gh >/dev/null 2>&1 || { echo "pr-rebase: gh is required" >&2; exit 1; }
GH=(gh)
[ -n "$REPO" ] && GH+=(-R "$REPO")

info=$("${GH[@]}" pr view "$PR" --json headRefName,baseRefName,isCrossRepository,headRefOid)
head=$(printf '%s' "$info" | jq -r .headRefName)
base=$(printf '%s' "$info" | jq -r .baseRefName)
cross=$(printf '%s' "$info" | jq -r .isCrossRepository)
[ "$cross" = "false" ] || { echo "pr-rebase: #$PR head is in a fork; rebase it from that clone" >&2; exit 1; }

git fetch -q origin "$base" "$head"
behind=$(git rev-list --count "origin/$head..origin/$base")
echo "#$PR  $head -> $base  (behind by $behind commits)"
[ "$behind" -gt 0 ] || { echo "  = already up to date; nothing to do"; exit 0; }
[ "$DRY" -eq 0 ] || { echo "  > would rebase origin/$head onto origin/$base and push --force-with-lease"; exit 0; }

[ -n "$WORK" ] || WORK=$(mktemp -d)
wt="$WORK/pr-$PR"
rm -rf "$wt"
git worktree add -q --detach "$wt" "origin/$head"
if git -C "$wt" rebase -q "origin/$base"; then
  git -C "$wt" push -q --force-with-lease=refs/heads/"$head":"origin/$head" origin "HEAD:refs/heads/$head"
  echo "  > rebased and pushed: $(git -C "$wt" rev-parse --short HEAD)"
  git worktree remove --force "$wt"
  exit 0
fi
echo "  !! rebase stopped on conflicts in #$PR; worktree left at: $wt" >&2
git -C "$wt" diff --name-only --diff-filter=U | sed 's/^/     /' >&2
echo "  resolve there, then: git -C <wt> rebase --continue && git -C <wt> push --force-with-lease origin HEAD:$head; finally git worktree remove <wt>" >&2
exit 2
