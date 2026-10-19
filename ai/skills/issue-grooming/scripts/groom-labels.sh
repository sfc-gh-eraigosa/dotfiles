#!/usr/bin/env bash
# groom-labels.sh — make sure the five grooming priority labels exist (idempotent).
#
#   priority:1  most important — blocks daily work, or a correctness/safety bug in a daily tool
#   priority:2  in-flight objective with design/spec on main and a clear next build step
#   priority:3  valuable follow-up to a shipped feature; not blocking
#   priority:4  nice-to-have, speculative, or a large program not started
#   priority:5  least important — parked, likely obsolete or superseded; kept as reference
#
# Creating a label that already exists is a no-op (--force updates colour/description).
# Needs gh. Pass -R owner/name to target another repo.
set -euo pipefail

REPO=""
while [ $# -gt 0 ]; do
  case "$1" in
    -R|--repo) REPO="${2:-}"; shift ;;
    -h|--help) sed -n '2,11p' "$0"; exit 0 ;;
    *) echo "groom-labels: unknown arg '$1'" >&2; exit 1 ;;
  esac
  shift
done
command -v gh >/dev/null 2>&1 || { echo "groom-labels: gh is required" >&2; exit 1; }

GH=(gh)
[ -n "$REPO" ] && GH+=(-R "$REPO")

ensure() {
  # $1 name  $2 colour  $3 description
  "${GH[@]}" label create "$1" --color "$2" --description "$3" --force >/dev/null
  echo "ok  $1"
}

ensure "priority:1" "b60205" "Groomed P1 (most important): blocks daily work, or a correctness/safety bug in a daily-use tool"
ensure "priority:2" "d93f0b" "Groomed P2: in-flight objective with design/spec on main and a clear next build step"
ensure "priority:3" "fbca04" "Groomed P3: valuable follow-up to a shipped feature; not blocking"
ensure "priority:4" "0e8a16" "Groomed P4: nice-to-have, speculative, or a large program not started"
ensure "priority:5" "c5def5" "Groomed P5 (least important): parked, likely obsolete or superseded; kept as reference"
