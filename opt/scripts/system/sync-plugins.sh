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

# Resolve a timeout binary (GNU 'timeout', or 'gtimeout' from coreutils on macOS).
# Empty when neither exists — calls then run unwrapped but still stdin-guarded.
TIMEOUT_BIN="$(command -v timeout || command -v gtimeout || true)"
# Per-call ceiling so a hung network fetch or an unexpected interactive prompt
# can't wedge install.sh indefinitely (the orphaned-process failure mode this
# guards against). Override with SYNC_PLUGINS_TIMEOUT for slow links.
CMD_TIMEOUT="${SYNC_PLUGINS_TIMEOUT:-300}"
# The gemini CLI (a Node process) ignores SIGTERM, so a plain `timeout N` would
# send SIGTERM and then wait forever for a process that never dies — defeating
# the guard. `-k KILL_GRACE` escalates to SIGKILL KILL_GRACE seconds after the
# initial signal, guaranteeing the call actually terminates.
KILL_GRACE="${SYNC_PLUGINS_KILL_GRACE:-15}"

# Run a plugin command non-interactively. stdin comes from /dev/null so any
# unexpected prompt (e.g. a credential or overwrite question) gets EOF and fails
# fast instead of blocking forever; when a timeout binary is available the call
# also runs under CMD_TIMEOUT. All failures are non-fatal (ensure-only).
run() {
    if [ "$DRY_RUN" = "1" ]; then
        echo "DRY-RUN: $*"
        return 0
    fi
    echo "+ $*"
    local rc=0
    if [ -n "$TIMEOUT_BIN" ]; then
        "$TIMEOUT_BIN" -k "$KILL_GRACE" "$CMD_TIMEOUT" "$@" </dev/null || rc=$?
    else
        "$@" </dev/null || rc=$?
    fi
    # 124 = timed out (SIGTERM); 137 = had to SIGKILL after -k grace (the gemini
    # CLI ignores SIGTERM, so this is the common timeout outcome, not a crash).
    if [ "$rc" -eq 124 ] || [ "$rc" -eq 137 ]; then
        echo "sync-plugins: WARNING — '$*' timed out after ${CMD_TIMEOUT}s; continuing." >&2
    elif [ "$rc" -ne 0 ]; then
        echo "sync-plugins: WARNING — '$*' failed (rc=$rc); continuing." >&2
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

# `claude plugin enable` exits non-zero when the plugin is already enabled, which
# is the normal steady state on a configured host (and on every install.sh re-run).
# Treat "already enabled" as success so re-runs stay quiet — only a genuine failure
# surfaces a warning. Honors --dry-run so the planned action is still printed.
enable_claude_plugin() {
    local plugin="$1" out
    if [ "$DRY_RUN" = "1" ]; then
        echo "DRY-RUN: claude plugin enable $plugin"
        return 0
    fi
    echo "+ claude plugin enable $plugin"
    local rc=0
    if [ -n "$TIMEOUT_BIN" ]; then
        out="$("$TIMEOUT_BIN" -k "$KILL_GRACE" "$CMD_TIMEOUT" claude plugin enable "$plugin" </dev/null 2>&1)" || rc=$?
    else
        out="$(claude plugin enable "$plugin" </dev/null 2>&1)" || rc=$?
    fi
    if [ "$rc" -eq 0 ]; then
        [ -n "$out" ] && echo "$out"
    elif printf '%s' "$out" | grep -qi "already enabled"; then
        echo "  ($plugin already enabled)"
    else
        printf '%s\n' "$out" >&2
        echo "sync-plugins: WARNING — enable $plugin failed; continuing." >&2
    fi
}

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
        enable_claude_plugin "$plugin"
    done < <(yq '.plugins[] | select(.enabled == true) | select(.claude.plugin != null) | .claude.plugin' "$MANIFEST")
}

# Names of currently-installed Gemini extensions, one per line. The list output
# is "<glyph> <name> (<version>)" for each extension followed by indented detail
# lines, so the name is field 2 of every non-indented row. `gemini extensions
# list` prints to stderr, so capture 2>&1. Empty on any error.
gemini_installed_names() {
    gemini extensions list 2>&1 | awk '!/^[[:space:]]/ && NF>=2 {print $2}'
}

# Install one Gemini extension, stdin/timeout-guarded. `gemini extensions install`
# is not idempotent — it exits non-zero with "already installed" when the
# extension is present — so treat that as a quiet skip (mirrors how
# enable_claude_plugin treats "already enabled"). This is the safety net for
# sources whose extension name differs from the repo basename (e.g. the
# gemini-agent-creator repo installs as "agent-creator"), which the name-based
# pre-skip in sync_gemini cannot match.
install_gemini_extension() {
    local source="$1" out rc=0
    echo "+ gemini extensions install $source --consent --skip-settings"
    if [ -n "$TIMEOUT_BIN" ]; then
        out="$("$TIMEOUT_BIN" -k "$KILL_GRACE" "$CMD_TIMEOUT" gemini extensions install "$source" --consent --skip-settings </dev/null 2>&1)" || rc=$?
    else
        out="$(gemini extensions install "$source" --consent --skip-settings </dev/null 2>&1)" || rc=$?
    fi
    if [ "$rc" -eq 0 ]; then
        [ -n "$out" ] && echo "$out"
    elif [ "$rc" -eq 124 ] || [ "$rc" -eq 137 ]; then
        echo "sync-plugins: WARNING — gemini install $source timed out after ${CMD_TIMEOUT}s; continuing." >&2
    elif printf '%s' "$out" | grep -qi "already installed"; then
        echo "  ($source already installed)"
    else
        printf '%s\n' "$out" >&2
        echo "sync-plugins: WARNING — gemini install $source failed (rc=$rc); continuing." >&2
    fi
}

sync_gemini() {
    if [ "$DRY_RUN" = "0" ] && ! command -v gemini >/dev/null 2>&1; then
        echo "sync-plugins: 'gemini' CLI not on PATH; skipping Gemini extensions."
        return 0
    fi
    # Snapshot installed extensions so re-runs skip them instead of erroring with
    # "already installed, please uninstall first" (which, unguarded, could hang).
    # `gemini extensions install` names an extension after its repo basename, so
    # match on that. The set also grows as we install, so an in-manifest duplicate
    # (the code-review source is shared by two plugins) is only installed once.
    local seen=""
    [ "$DRY_RUN" = "0" ] && seen="$(gemini_installed_names)"
    local any=0
    while IFS= read -r source; do
        { [ -z "$source" ] || [ "$source" = "null" ]; } && continue
        any=1
        if [ "$DRY_RUN" = "0" ]; then
            local name="${source##*/}"
            if printf '%s\n' "$seen" | grep -qxF -- "$name"; then
                echo "  ($name already installed)"
                continue
            fi
            seen="${seen}
${name}"
            install_gemini_extension "$source"
            continue
        fi
        # Dry-run path: keep printing the planned action via run().
        run gemini extensions install "$source" --consent --skip-settings
    done < <(yq '.plugins[] | select(.enabled == true) | select(.gemini.source != null) | .gemini.source' "$MANIFEST")
    [ "$any" = "0" ] && echo "sync-plugins: no Gemini extension sources in manifest (nothing to do)."
}

echo "Syncing AI plugins from ${MANIFEST}$([ "$DRY_RUN" = "1" ] && echo ' (dry-run)')..."
sync_claude
sync_gemini
echo "sync-plugins: done."
