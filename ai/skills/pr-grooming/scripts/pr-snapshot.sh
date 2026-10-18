#!/usr/bin/env bash
# pr-snapshot.sh — capture everything a PR grooming pass needs to read, in one place.
#
# Writes into OUT_DIR (default: ./pr-groom-<date>):
#   prs.json      every open PR: number,title,body,isDraft,author,labels,dates,base/head,
#                 mergeStateStatus, closingIssuesReferences, files, comments, CI rollup
#   prs.txt       the same, flattened for reading (one PR block per PR)
#   triage.tsv    one line per PR: number, category, age_days, draft, mergeState, ci, title
#   mbo-rows.txt  the docs/mbo/index.md rows that mention any open PR number or head slug
#
# Categories (first match wins; see SKILL.md §1):
#   dependabot   author is dependabot (or the PR carries the `dependencies` label)
#   bugfix       title starts with fix/bugfix, or carries the `bug` label
#   mbo-draft    draft whose files touch docs/mbo/, or whose body links docs/mbo/ artifacts
#   stale-draft  any other draft older than --stale-days (default 30)
#   other        everything else
# Pure read-only; needs gh + jq. Repo defaults to the cwd's GitHub remote; pass -R owner/name.
set -euo pipefail

OUT_DIR=""
REPO=""
STALE_DAYS=30
INDEX="docs/mbo/index.md"
while [ $# -gt 0 ]; do
  case "$1" in
    -R|--repo) REPO="${2:-}"; shift ;;
    -o|--out) OUT_DIR="${2:-}"; shift ;;
    --stale-days) STALE_DAYS="${2:-30}"; shift ;;
    --index) INDEX="${2:-}"; shift ;;
    -h|--help) sed -n '2,17p' "$0"; exit 0 ;;
    *) echo "pr-snapshot: unknown arg '$1'" >&2; exit 1 ;;
  esac
  shift
done
command -v gh >/dev/null 2>&1 || { echo "pr-snapshot: gh is required" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "pr-snapshot: jq is required" >&2; exit 1; }
[ -n "$OUT_DIR" ] || OUT_DIR="./pr-groom-$(date +%Y-%m-%d)"
mkdir -p "$OUT_DIR"
GH=(gh)
[ -n "$REPO" ] && GH+=(-R "$REPO")

echo "==> open PRs"
"${GH[@]}" pr list --state open --limit 100 \
  --json number,title,body,isDraft,author,labels,createdAt,updatedAt,baseRefName,headRefName,headRefOid,mergeStateStatus,mergeable,closingIssuesReferences,files,comments,statusCheckRollup,url \
  > "$OUT_DIR/prs.json"

# now() is UTC epoch seconds in jq; createdAt is ISO-8601 Z.
jq -r --argjson stale "$STALE_DAYS" '
  def ci:
    [.statusCheckRollup[]? | (.conclusion // .state // "PENDING") | ascii_upcase]
    | if length == 0 then "none"
      elif any(. == "FAILURE" or . == "ERROR") then "failing"
      elif any(. == "PENDING" or . == "IN_PROGRESS" or . == "QUEUED" or . == "EXPECTED") then "pending"
      else "passing" end;
  def age: ((now - (.createdAt | fromdateiso8601)) / 86400 | floor);
  def category:
    if (.author.login | test("dependabot")) or ([.labels[].name] | index("dependencies")) then "dependabot"
    elif (.title | test("^(fix|bugfix)\\b"; "i")) or ([.labels[].name] | index("bug")) then "bugfix"
    elif .isDraft and (([.files[].path] | any(startswith("docs/mbo/"))) or (.body | test("docs/mbo/"))) then "mbo-draft"
    elif .isDraft and (age >= $stale) then "stale-draft"
    else "other" end;
  .[] | [.number, category, age, (if .isDraft then "draft" else "ready" end), .mergeStateStatus, ci, ([.closingIssuesReferences[].number] | map("#\(.)") | join(" ")), .title] | @tsv
' "$OUT_DIR/prs.json" | sort -t$'\t' -k2,2 -k3,3nr > "$OUT_DIR/triage.tsv"

jq -r '.[] | "=====PR #\(.number) \(if .isDraft then "DRAFT" else "READY" end) (\(.createdAt[:10]), updated \(.updatedAt[:10])) by \(.author.login) labels=\([.labels[].name]|join(","))\nTITLE: \(.title)\nURL: \(.url)\nBRANCH: \(.headRefName) -> \(.baseRefName) @ \(.headRefOid[:8]) mergeState=\(.mergeStateStatus)\nCLOSES: \([.closingIssuesReferences[].number] | map("#\(.)") | join(" "))\nFILES (\(.files|length)): \([.files[].path] | .[:30] | join(", "))\nCHECKS: \([.statusCheckRollup[]? | "\(.name // .context)=\(.conclusion // .state)"] | join(", "))\nBODY:\n\(.body)\n\(if (.comments|length) > 0 then "COMMENTS (\(.comments|length)):\n" + ([.comments[] | "  - [\(.createdAt[:10])] \(.body | gsub("\n"; "\n    "))"] | join("\n")) else "" end)\n"' \
  "$OUT_DIR/prs.json" > "$OUT_DIR/prs.txt"

: > "$OUT_DIR/mbo-rows.txt"
if [ -f "$INDEX" ]; then
  echo "==> $INDEX rows that mention an open PR or its branch slug"
  jq -r '.[] | "#\(.number)", (.headRefName | split("/") | .[1] // empty | select(length > 4))' "$OUT_DIR/prs.json" | sort -u | while read -r key; do
    [ -n "$key" ] || continue
    # "#39" must not match "#394": require a non-digit (or end of line) after a number key
    grep -nE -- "$(printf '%s' "$key" | sed 's/[][\.*^$]/\\&/g')([^0-9]|$)" "$INDEX" | sed "s/^/[$key] /" | cut -c1-400 >> "$OUT_DIR/mbo-rows.txt" || true
  done
fi

n=$(jq length "$OUT_DIR/prs.json")
echo "snapshot: $n open PRs -> $OUT_DIR"
column -t -s$'\t' "$OUT_DIR/triage.tsv" 2>/dev/null | cut -c1-160 || cat "$OUT_DIR/triage.tsv"
echo "next: read prs.txt + mbo-rows.txt, decide a verdict per PR (SKILL.md §2), write verdicts.json."
