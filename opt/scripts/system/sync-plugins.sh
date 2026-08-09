#!/usr/bin/env bash
# sync-plugins.sh — ensure the AI-assistant plugins declared in ai/plugins.yaml
# are installed, enabled, and UPDATED to latest. Ensure-only (additive): never
# removes anything. Mirrors sync-skills.sh. Safe to re-run.
#
# Usage:
#   sync-plugins.sh            install + enable per the manifest
#   sync-plugins.sh --dry-run  print planned actions, change nothing (and
#                              previews even when the claude/agy CLIs are absent)
set -u

# Resolve the real repo root even when invoked via the ~/opt symlink.
# Portable replacement for `readlink -f "$0"` (GNU-only; absent on macOS/BSD):
# `cd ... && pwd -P` resolves symlinks to the physical script dir.
SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" >/dev/null 2>&1 && pwd -P)"
BASE_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd -P)"
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
# Resolve setsid (util-linux). The claude/agy plugin subcommands open
# /dev/tty directly and render an interactive TUI when a controlling terminal is
# present — bypassing the </dev/null stdin guard below and wedging an unattended
# `./install.sh` (the CLI ends up job-control-stopped on a SIGTTOU/SIGTTIN). Run
# them under `setsid` so they get a new session with NO controlling terminal,
# can't open /dev/tty, and fall back to non-interactive mode. Empty on macOS
# (no setsid); the per-call </dev/null + timeout guards still apply there.
SETSID_BIN="$(command -v setsid || true)"
# macOS ships no native setsid, so the detach guard above no-ops there — and a
# FRESH `claude plugin install` then opens /dev/tty, renders its TUI, and hangs
# until the timeout SIGKILLs it (≈CMD_TIMEOUT *per uninstalled plugin* on a clean
# machine — tens of minutes). Homebrew's keg-only util-linux provides a working
# setsid but does not symlink it onto PATH, so probe the keg explicitly. Install
# it via opt/profiles/Brewfile (`brew 'util-linux'`). No-op when util-linux is
# absent: the </dev/null + timeout guards remain the (slower) fallback.
if [ -z "$SETSID_BIN" ] && command -v brew >/dev/null 2>&1; then
    _ul_prefix="$(brew --prefix util-linux 2>/dev/null || true)"
    for _ul_cand in "$_ul_prefix/bin/setsid" "$_ul_prefix/sbin/setsid"; do
        if [ -n "$_ul_prefix" ] && [ -x "$_ul_cand" ]; then SETSID_BIN="$_ul_cand"; break; fi
    done
    unset _ul_prefix _ul_cand
fi
# Per-call ceiling so a hung network fetch or an unexpected interactive prompt
# can't wedge install.sh indefinitely (the orphaned-process failure mode this
# guards against). Override with SYNC_PLUGINS_TIMEOUT for slow links.
CMD_TIMEOUT="${SYNC_PLUGINS_TIMEOUT:-300}"
# Some CLIs ignore SIGTERM (the retired gemini CLI famously did), so a plain
# `timeout N` would send SIGTERM and then wait forever for a process that never
# dies — defeating the guard. `-k KILL_GRACE` escalates to SIGKILL KILL_GRACE
# seconds after the initial signal, guaranteeing the call actually terminates.
KILL_GRACE="${SYNC_PLUGINS_KILL_GRACE:-15}"

# Guard prefix applied to every claude/agy plugin invocation:
#   setsid -w  → new session, no controlling terminal (see SETSID_BIN above);
#                -w makes setsid wait and return the child's real exit code so
#                the timeout rc detection below still works.
#   timeout -k → bound runtime and SIGKILL a SIGTERM-ignoring CLI after the grace.
# Either tool may be absent (minimal containers / macOS lacks setsid); the prefix
# omits whatever is missing. </dev/null at each call site stays as the stdin guard.
GUARD=()
[ -n "$SETSID_BIN" ] && GUARD+=("$SETSID_BIN" -w)
[ -n "$TIMEOUT_BIN" ] && GUARD+=("$TIMEOUT_BIN" -k "$KILL_GRACE" "$CMD_TIMEOUT")

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
    "${GUARD[@]+"${GUARD[@]}"}" "$@" </dev/null || rc=$?
    # 124 = timed out (SIGTERM); 137 = had to SIGKILL after -k grace (a
    # SIGTERM-ignoring CLI makes this the common timeout outcome, not a crash).
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
    out="$("${GUARD[@]+"${GUARD[@]}"}" claude plugin enable "$plugin" </dev/null 2>&1)" || rc=$?
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
    # Refresh every marketplace catalog from its source BEFORE installing, so
    # fresh installs resolve against current manifests. Without an explicit
    # update pass, plugins stay PINNED at their first-installed version:
    # `claude plugin install` is a no-op on an existing install, and the
    # per-marketplace autoUpdate setting is unset by default.
    run claude plugin marketplace update
    # Install + enable + update each enabled plugin that has a claude.plugin.
    while IFS= read -r plugin; do
        { [ -z "$plugin" ] || [ "$plugin" = "null" ]; } && continue
        run claude plugin install "$plugin"
        enable_claude_plugin "$plugin"
        # Converge to latest (no-op when current; a restart picks up changes).
        run claude plugin update "$plugin"
    done < <(yq '.plugins[] | select(.enabled == true) | select(.claude.plugin != null) | .claude.plugin' "$MANIFEST")
}

# Names of currently-installed Antigravity plugins, one per line. The empty
# state prints "No imported plugins." (must not be mistaken for a plugin
# named "No"); the populated row format is unpinned, so emit the first TWO
# fields of every non-indented row — that covers both "name (version)" and a
# glyph-prefixed "<glyph> name (version)" layout (the retired gemini CLI used
# the latter). Stray version/glyph tokens are harmless: callers match with
# grep -qxF against a repo basename. Empty on any error.
agy_installed_names() {
    agy plugin list 2>&1 | awk '!/^[[:space:]]/ && !/^No imported plugins/ && NF>=1 {print $1; if (NF>=2) print $2}'
}

# Install one Antigravity plugin, stdin/timeout-guarded. Treat "already
# installed" as a quiet skip (mirrors how enable_claude_plugin treats
# "already enabled"). This is the safety net for sources whose plugin name
# differs from the repo basename, which the name-based pre-skip in
# sync_antigravity cannot match.
install_agy_plugin() {
    local source="$1" out rc=0
    echo "+ agy plugin install $source"
    out="$("${GUARD[@]+"${GUARD[@]}"}" agy plugin install "$source" </dev/null 2>&1)" || rc=$?
    if [ "$rc" -eq 0 ]; then
        [ -n "$out" ] && echo "$out"
    elif [ "$rc" -eq 124 ] || [ "$rc" -eq 137 ]; then
        echo "sync-plugins: WARNING — agy install $source timed out after ${CMD_TIMEOUT}s; continuing." >&2
    elif printf '%s' "$out" | grep -qi "already installed"; then
        echo "  ($source already installed)"
    else
        printf '%s\n' "$out" >&2
        echo "sync-plugins: WARNING — agy install $source failed (rc=$rc); continuing." >&2
    fi
}

sync_antigravity() {
    if [ "$DRY_RUN" = "0" ] && ! command -v agy >/dev/null 2>&1; then
        echo "sync-plugins: 'agy' CLI not on PATH; skipping Antigravity plugins."
        return 0
    fi
    # Snapshot installed plugins so re-runs skip them instead of erroring.
    # Plugins are named after their repo basename, so match on that. The set
    # also grows as we install, so an in-manifest duplicate (the code-review
    # source is shared by two plugins) is only installed once.
    local seen=""
    [ "$DRY_RUN" = "0" ] && seen="$(agy_installed_names)"
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
            install_agy_plugin "$source"
            continue
        fi
        # Dry-run path: keep printing the planned action via run().
        run agy plugin install "$source"
    done < <(yq '.plugins[] | select(.enabled == true) | select(.antigravity.source != null) | .antigravity.source' "$MANIFEST")
    [ "$any" = "0" ] && echo "sync-plugins: no Antigravity plugin sources in manifest (nothing to do)."
}

echo "Syncing AI plugins from ${MANIFEST}$([ "$DRY_RUN" = "1" ] && echo ' (dry-run)')..."
sync_claude
sync_antigravity
echo "sync-plugins: done."
