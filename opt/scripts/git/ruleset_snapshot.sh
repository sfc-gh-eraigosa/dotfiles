#!/usr/bin/env bash
# ruleset_snapshot.sh — export a repo's GitHub branch rulesets to
# .github/rulesets/<name>.json so settings changes are reviewable in PRs.
# Rulesets are repository SETTINGS (no file representation), so this snapshot
# is the audit trail: re-run after any ruleset change and commit the diff.
# The snapshot is documentation — GitHub does not read it back; the ruleset
# UI/API remains the enforcement source.
#
# Usage: ruleset_snapshot.sh [owner/repo] [output-dir]
#   Defaults: repo of the current directory's origin; <repo-root>/.github/rulesets
#
# Volatile fields (node_id, timestamps, links, current_user_can_bypass) are
# stripped and keys sorted so diffs stay meaningful.

set -eu

command -v gh >/dev/null 2>&1 || { echo "ruleset_snapshot: gh is required" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "ruleset_snapshot: jq is required" >&2; exit 1; }

REPO="${1:-}"
if [ -z "$REPO" ]; then
  REPO="$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null)" || {
    echo "ruleset_snapshot: not in a repo and no owner/repo argument given" >&2; exit 1; }
fi

OUT_DIR="${2:-}"
if [ -z "$OUT_DIR" ]; then
  ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || {
    echo "ruleset_snapshot: not in a git repo; pass an output dir" >&2; exit 1; }
  OUT_DIR="$ROOT/.github/rulesets"
fi
mkdir -p "$OUT_DIR"

ids="$(gh api "repos/$REPO/rulesets" --jq '.[].id')"
[ -n "$ids" ] || { echo "ruleset_snapshot: no rulesets on $REPO"; exit 0; }

for id in $ids; do
  raw="$(gh api "repos/$REPO/rulesets/$id")"
  name="$(printf '%s' "$raw" | jq -r '.name' | tr -d '\n' | tr -c 'A-Za-z0-9._-' '-' )"
  printf '%s' "$raw" | jq -S 'del(.node_id, .created_at, .updated_at, ._links, .current_user_can_bypass)' \
    > "$OUT_DIR/$name.json"
  echo "wrote $OUT_DIR/$name.json (ruleset $id)"
done
