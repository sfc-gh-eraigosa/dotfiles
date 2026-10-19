#!/usr/bin/env bash
# groom-report.sh — render a verdicts file as the grooming summary (markdown).
#
# Usage: groom-report.sh VERDICTS.json [--repo-url https://github.com/owner/name]
# Prints a counts line, then one table per verdict (addressed, obsolete, partially, open),
# sorted by priority. Paste it into the final report / the docs/mbo/index.md audit row / the PR.
set -euo pipefail
VERDICTS="${1:-}"
URL=""
[ $# -gt 0 ] && shift
while [ $# -gt 0 ]; do
  case "$1" in
    --repo-url) URL="${2:-}"; shift ;;
    *) echo "groom-report: unknown arg '$1'" >&2; exit 1 ;;
  esac
  shift
done
[ -n "$VERDICTS" ] && [ -f "$VERDICTS" ] || { echo "groom-report: verdicts file required" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "groom-report: jq is required" >&2; exit 1; }
[ -n "$URL" ] || URL=$(gh repo view --json url --jq .url 2>/dev/null || echo "")

jq -r --arg url "$URL" '
  def link: if $url != "" then "[#\(.number)](\($url)/issues/\(.number))" else "#\(.number)" end;
  def cell: tostring | gsub("\n"; " ") | gsub("\\|"; "\\|");
  . as $in |
  "**\($in | length) issues groomed** — " +
  ([$in | group_by(.verdict)[] | "\(.[0].verdict): \(length)"] | join(" · ")) + "\n" +
  ( ["addressed", "obsolete", "partially", "open"]
    | map(. as $v | [ $in[] | select(.verdict == $v) ]
      | if length == 0 then "" else
        "\n### \($v) (\(length))\n\n| Issue | P | Title | Action | Evidence |\n| :-- | :-: | :-- | :-- | :-- |\n" +
        ( sort_by(.priority, .number)
          | map("| \(link) | \(.priority) | \(.title | cell) | \(.suggested_action) | \(.evidence | cell) |")
          | join("\n") )
      end)
    | map(select(. != ""))
    | join("\n") )
' "$VERDICTS"
