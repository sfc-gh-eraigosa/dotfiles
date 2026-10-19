#!/usr/bin/env bash
# groom-snapshot.sh — capture everything a grooming pass needs to read, in one place.
#
# Writes into OUT_DIR (default: ./groom-<date>):
#   issues.json      every open issue: number,title,body,labels,createdAt,updatedAt,comments
#   issues.txt       the same, flattened for reading (one ISSUE block per issue)
#   labels.json      the repo's label set
#   merged-prs.json  the last N merged PRs with the issues they closed (closingIssuesReferences)
#   clusters.txt     a first-cut clustering by title prefix / shared label, to split the triage
#
# Pure read-only; needs gh + jq. Repo defaults to the cwd's GitHub remote; pass -R owner/name
# to override.
set -euo pipefail

OUT_DIR=""
REPO=""
PR_LIMIT=300
while [ $# -gt 0 ]; do
  case "$1" in
    -R|--repo) REPO="${2:-}"; shift ;;
    -o|--out) OUT_DIR="${2:-}"; shift ;;
    --pr-limit) PR_LIMIT="${2:-300}"; shift ;;
    -h|--help)
      sed -n '2,13p' "$0"; exit 0 ;;
    *) echo "groom-snapshot: unknown arg '$1'" >&2; exit 1 ;;
  esac
  shift
done

command -v gh >/dev/null 2>&1 || { echo "groom-snapshot: gh is required" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "groom-snapshot: jq is required" >&2; exit 1; }

[ -n "$OUT_DIR" ] || OUT_DIR="./groom-$(date +%Y-%m-%d)"
mkdir -p "$OUT_DIR"

GH=(gh)
[ -n "$REPO" ] && GH+=(-R "$REPO")

echo "==> open issues"
"${GH[@]}" issue list --state open --limit 500 \
  --json number,title,body,labels,createdAt,updatedAt,comments,url \
  > "$OUT_DIR/issues.json"

jq -r '.[] | "=====ISSUE #\(.number) (\(.createdAt[:10]), updated \(.updatedAt[:10])) labels=\([.labels[].name]|join(","))\nTITLE: \(.title)\nURL: \(.url)\nBODY:\n\(.body)\n\(if (.comments|length) > 0 then "COMMENTS (\(.comments|length)):\n" + ([.comments[] | "  - [\(.createdAt[:10])] \(.body | gsub("\n"; "\n    "))"] | join("\n")) else "" end)\n"' \
  "$OUT_DIR/issues.json" > "$OUT_DIR/issues.txt"

echo "==> labels"
"${GH[@]}" label list --limit 200 --json name,description,color > "$OUT_DIR/labels.json"

echo "==> merged PRs (last $PR_LIMIT) with closing references"
"${GH[@]}" pr list --state merged --limit "$PR_LIMIT" \
  --json number,title,mergedAt,closingIssuesReferences,url \
  > "$OUT_DIR/merged-prs.json"

echo "==> clusters (first cut: title prefix before ':' or '/', else the first shared label)"
jq -r '
  .[] |
  (.title
    | capture("^(?<p>[^:/]+)[:/]") .p // "misc"
    | ascii_downcase | gsub("^\\s+|\\s+$"; "")) as $prefix |
  ([.labels[].name] | map(select(test("^(bug|enhancement|duplicate|invalid|question|wontfix|help wanted|priority:.*)$") | not)) | .[0] // "") as $lbl |
  "\(if $lbl != "" then $lbl else $prefix end)\t#\(.number)\t\(.title)"
' "$OUT_DIR/issues.json" | sort > "$OUT_DIR/clusters.txt"

n_issues=$(jq length "$OUT_DIR/issues.json")
n_clusters=$(cut -f1 "$OUT_DIR/clusters.txt" | sort -u | wc -l | tr -d ' ')
echo "snapshot: $n_issues open issues in $n_clusters first-cut clusters -> $OUT_DIR"
echo "next: read $OUT_DIR/clusters.txt, regroup by hand, then triage each cluster (see SKILL.md §2)."
