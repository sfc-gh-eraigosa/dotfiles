#!/usr/bin/env bash
# sync-plugins.sh — ensure the AI-assistant plugins declared in ai/plugins.yaml
# are installed and enabled. Ensure-only (additive): never removes anything.
# Mirrors sync-skills.sh. Safe to re-run.
#
# Usage:
#   sync-plugins.sh            install + enable per the manifest
#   sync-plugins.sh --dry-run  print planned actions, change nothing (and
#                              previews even when the claude/gemini CLIs are absent)
set -u

# Resolve the real repo root even when invoked via the ~/opt symlink.
SCRIPT_PATH="$(readlink -f "$0")"
BASE_DIR="$(cd "$(dirname "$SCRIPT_PATH")/../../.." && pwd)"
MANIFEST="${BASE_DIR}/ai/plugins.yaml"

DRY_RUN=0
case "${1:-}" in
    --dry-run) DRY_RUN=1 ;;
    --help|-h)
        echo "Usage: sync-plugins [--dry-run]"
        echo "Install + enable the AI plugins listed in ai/plugins.yaml (ensure-only)."
        exit 0 ;;
    "") ;;
    *) echo "sync-plugins: unknown argument '$1'" >&2; exit 2 ;;
esac

run() {
    if [ "$DRY_RUN" = "1" ]; then
        echo "DRY-RUN: $*"
    else
        echo "+ $*"
        "$@" || echo "sync-plugins: WARNING — '$*' failed; continuing." >&2
    fi
}

if ! command -v yq >/dev/null 2>&1; then
    echo "sync-plugins: 'yq' not found. Install it via opt/scripts/system/install_yq.sh" >&2
    exit 1
fi
if [ ! -f "$MANIFEST" ]; then
    echo "sync-plugins: manifest not found: $MANIFEST" >&2
    exit 1
fi

sync_claude() {
    if [ "$DRY_RUN" = "0" ] && ! command -v claude >/dev/null 2>&1; then
        echo "sync-plugins: 'claude' CLI not on PATH; skipping Claude plugins."
        return 0
    fi
    # Marketplaces (idempotent add).
    while IFS= read -r src; do
        { [ -z "$src" ] || [ "$src" = "null" ]; } && continue
        run claude plugin marketplace add "$src"
    done < <(yq '.marketplaces[] | select(.claude != null) | .claude' "$MANIFEST")
    # Install + enable each enabled plugin that has a claude.plugin.
    while IFS= read -r plugin; do
        { [ -z "$plugin" ] || [ "$plugin" = "null" ]; } && continue
        run claude plugin install "$plugin"
        run claude plugin enable "$plugin"
    done < <(yq '.plugins[] | select(.enabled == true) | select(.claude.plugin != null) | .claude.plugin' "$MANIFEST")
}

sync_gemini() {
    if [ "$DRY_RUN" = "0" ] && ! command -v gemini >/dev/null 2>&1; then
        echo "sync-plugins: 'gemini' CLI not on PATH; skipping Gemini extensions."
        return 0
    fi
    local any=0
    while IFS= read -r source; do
        { [ -z "$source" ] || [ "$source" = "null" ]; } && continue
        any=1
        run gemini extensions install "$source"
    done < <(yq '.plugins[] | select(.enabled == true) | select(.gemini.source != null) | .gemini.source' "$MANIFEST")
    [ "$any" = "0" ] && echo "sync-plugins: no Gemini extension sources in manifest (nothing to do)."
}

echo "Syncing AI plugins from ${MANIFEST}$([ "$DRY_RUN" = "1" ] && echo ' (dry-run)')..."
sync_claude
sync_gemini
echo "sync-plugins: done."
