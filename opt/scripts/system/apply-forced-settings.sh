#!/usr/bin/env bash
# apply-forced-settings.sh <host_settings.json> <forced.json>
#
# Deep-merge the repo's "forced" (immutable) settings subset over a host-owned
# settings file, in place. Forced fields win; every undeclared host field is
# preserved. This is the reconcile mechanism for the AI-config provisioning
# design (docs/designs/2026-06-02-ai-config-home-provisioning.md §7 / D2) — it
# keeps security wiring (hooks, statusLine, deny/ask) current on every install
# without clobbering host customizations (enabledPlugins, theme, apiKeyHelper…).
#
# Uses jq's recursive merge `*`: objects merge, arrays/scalars are replaced by
# the right operand (the forced file). Fails loud (non-zero) and leaves the host
# file untouched if either input is missing or not valid JSON.
set -euo pipefail

host="${1:-}"
forced="${2:-}"

die() { echo "apply-forced-settings: $*" >&2; exit 1; }

[ -n "$host" ] && [ -n "$forced" ] || die "usage: apply-forced-settings.sh <host.json> <forced.json>"
command -v jq >/dev/null 2>&1 || die "jq is required but not installed"
[ -f "$host" ]   || die "host settings file not found: $host"
[ -f "$forced" ] || die "forced settings file not found: $forced"
jq -e . "$host"   >/dev/null 2>&1 || die "host settings is not valid JSON: $host"
jq -e . "$forced" >/dev/null 2>&1 || die "forced settings is not valid JSON: $forced"

# Merge into a temp file first; only replace the host file once the merge
# succeeds, so a jq error never leaves a half-written/clobbered settings file.
tmp="$(mktemp "${TMPDIR:-/tmp}/forced-settings.XXXXXX")"
trap 'rm -f "$tmp"' EXIT
# Drop top-level doc keys (leading underscore, e.g. "_comment") from the forced
# document before merging — they are self-documentation, not live config, and
# must not pollute the host settings file.
if jq -s '(.[1] | with_entries(select(.key | startswith("_") | not))) as $forced | .[0] * $forced' \
        "$host" "$forced" > "$tmp" && [ -s "$tmp" ]; then
    cat "$tmp" > "$host"
else
    die "merge failed; host file left unchanged: $host"
fi
