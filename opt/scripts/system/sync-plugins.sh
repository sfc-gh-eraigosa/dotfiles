#!/usr/bin/env bash
# sync-plugins.sh — ensure the AI-assistant plugins declared in ai/plugins.yaml
# are installed, enabled, and UPDATED to latest. Ensure-only (additive): never
# removes anything. Mirrors sync-skills.sh. Safe to re-run.
#
# Per host, gff flags slim it down (all on by default; features.yaml area `ai`):
#   ai.plugin.<name>=false      claude plugin disable (never uninstall); no
#                               install or update for it
#   ai.mcp.<server>=false       deny that one server (deniedMcpServers in
#                               ~/.claude/settings.json, matched on the command
#                               or URL in the installed plugin's .mcp.json)
#   ai.connectors.claude-ai=false  disableClaudeAiConnectors: true
# Setting a flag back on removes only what this script wrote (it keeps a record
# in $SYNC_PLUGINS_STATE_DIR/claude-mcp-flags.json).
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

# shellcheck source=../../lib/gff.sh
. "${BASE_DIR}/opt/lib/gff.sh"
# Run from install.sh the GFF_* flags are already exported; run by hand, read
# them from gff (fail-open: no gff means every flag is on).
# SYNC_PLUGINS_GFF_EXPORT=0 skips this (tests set GFF_* themselves).
if [ "${SYNC_PLUGINS_GFF_EXPORT:-1}" != "0" ] && ! env | grep -q '^GFF_' && command -v gff >/dev/null 2>&1; then
    set -a
    eval "$( (cd "$BASE_DIR" && gff export --shell) 2>/dev/null)"
    set +a
fi

CLAUDE_PLUGINS_DIR="${CLAUDE_PLUGINS_DIR:-$HOME/.claude/plugins}"
CLAUDE_SETTINGS_FILE="${CLAUDE_SETTINGS_FILE:-$HOME/.claude/settings.json}"
SYNC_PLUGINS_STATE_DIR="${SYNC_PLUGINS_STATE_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/dotfiles}"

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

# Retry budget for a git `index.lock` failure inside `claude plugin
# update`'s own internal clone/checkout — not a git call this script makes
# itself. On a real fleet-update run, four plugins in a row each failed on a
# FRESH clone dir with "Unable to create '.../.clone/.git/index.lock': File
# exists", each succeeding again a few plugins later — which reads like a
# short-lived internal lock. Live-verified follow-up (dotfiles#343) found
# it is NOT reliably transient: on one host, one specific plugin failed this
# way 100% of the time across several standalone retries up to 15s apart, on
# the latest published claude-code, with no stray process or lock file
# between attempts — consistent with a real ordering bug in the CLI's own
# checkout step, not a lock that will clear on its own. The bounded retry
# below is kept anyway because it costs almost nothing and still recovers a
# GENUINELY momentary lock; it is not a fix for the deterministic case, which
# needs an upstream claude-code fix.
PLUGIN_UPDATE_LOCK_RETRIES="${SYNC_PLUGINS_LOCK_RETRIES:-2}"
PLUGIN_UPDATE_LOCK_DELAY="${SYNC_PLUGINS_LOCK_RETRY_DELAY:-2}"

# update_claude_plugin <plugin> — converge to latest, retrying on the
# index.lock failure above (which may or may not clear — see the comment on
# the retry budget). Any other failure (including a lock still held after
# the retry budget) is reported immediately, exactly as before this existed.
update_claude_plugin() {
    local plugin="$1" out rc=0 attempt=1 lock=0
    if [ "$DRY_RUN" = "1" ]; then
        echo "DRY-RUN: claude plugin update $plugin"
        return 0
    fi
    while :; do
        echo "+ claude plugin update $plugin"
        rc=0
        out="$("${GUARD[@]+"${GUARD[@]}"}" claude plugin update "$plugin" </dev/null 2>&1)" || rc=$?
        [ -n "$out" ] && echo "$out"
        if [ "$rc" -eq 0 ]; then
            return 0
        fi
        if ! printf '%s' "$out" | grep -q 'index\.lock'; then
            break # a real failure — never retry, fall through to the warning
        fi
        lock=1
        if [ "$attempt" -ge "$PLUGIN_UPDATE_LOCK_RETRIES" ]; then
            break # retry budget spent — fall through to the warning
        fi
        attempt=$((attempt + 1))
        # stdout, deliberately: fleet counts every stderr line it cannot
        # classify as benign as a ⚠ on the host, so a retry that RECOVERS
        # would still score a warning in `fleet history`. Only the exhausted
        # budget below is a warning.
        echo "sync-plugins: git lock updating $plugin; retrying (attempt $attempt/$PLUGIN_UPDATE_LOCK_RETRIES)…"
        sleep "$PLUGIN_UPDATE_LOCK_DELAY"
    done
    if [ "$lock" -eq 1 ]; then
        # The lock is the symptom, not the cause: git 2.34.1 (Ubuntu 22.04's
        # frozen release) SEGFAULTS on claude's --filter=tree:0 sparse-cone
        # checkout, and a crashed git cannot remove its index.lock, so the
        # next checkout dies on "File exists". Proven on the Jetson with a
        # git exit-status shim; git 2.55 ran the same updates cleanly. A retry
        # can never fix it — every attempt is a fresh clone that crashes the
        # same way. Say which git this host runs, and what fixes it.
        echo "sync-plugins: WARNING — 'claude plugin update $plugin' still hit a git index.lock after $attempt attempt(s) (rc=$rc); on this host git is $(git --version 2>/dev/null | awk '{print $3}' || echo unknown). git 2.34.x crashes on claude's partial-clone checkout and leaves the lock behind (dotfiles#343); dotfiles installs a current git from the git-core PPA on Ubuntu (setup_git_apt_repo.sh) — re-run install.sh, then this update — continuing." >&2
    else
        echo "sync-plugins: WARNING — 'claude plugin update $plugin' failed (rc=$rc); continuing." >&2
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
    # Install + enable + update each enabled plugin that has a claude.plugin,
    # unless its ai.plugin.<name> flag is off on this host: then disable it.
    # A synced row (claude.ai account) is only ever enabled or disabled.
    while IFS='|' read -r name plugin synced; do
        { [ -z "$plugin" ] || [ "$plugin" = "null" ]; } && continue
        if ! gff_on "ai.plugin.$name"; then
            disable_claude_plugin "$plugin"
            continue
        fi
        if [ "$synced" = "true" ]; then
            enable_claude_plugin "$plugin"
            continue
        fi
        run claude plugin install "$plugin"
        enable_claude_plugin "$plugin"
        # Converge to latest (no-op when current; a restart picks up changes).
        update_claude_plugin "$plugin"
    done < <(yq '.plugins[] | select(.enabled == true) | select(.claude.plugin != null) | .name + "|" + .claude.plugin + "|" + ((.claude.synced // false) | tostring)' "$MANIFEST")
}

# disable_claude_plugin <plugin> — the ai.plugin.<name>=false path. Quiet when
# it is already disabled or not installed here (nothing to turn off).
disable_claude_plugin() {
    local plugin="$1" out rc=0
    if [ "$DRY_RUN" = "1" ]; then
        echo "DRY-RUN: claude plugin disable $plugin"
        return 0
    fi
    echo "+ claude plugin disable $plugin (gff: ai.plugin flag is false)"
    out="$("${GUARD[@]+"${GUARD[@]}"}" claude plugin disable "$plugin" </dev/null 2>&1)" || rc=$?
    if [ "$rc" -eq 0 ]; then
        [ -n "$out" ] && echo "$out"
    elif printf '%s' "$out" | grep -qiE "already disabled|not installed|not found"; then
        echo "  ($plugin already off)"
    else
        printf '%s\n' "$out" >&2
        echo "sync-plugins: WARNING — disable $plugin failed; continuing." >&2
    fi
}

# plugin_mcp_file <plugin id> <name> <synced> — the .mcp.json of the plugin as
# installed on this host: an install's installPath (installed_plugins.json),
# or the account's synced copy (synced/<account>/<name>[~gN]/). Empty if none.
plugin_mcp_file() {
    local f p
    if [ "$3" = "true" ]; then
        for f in "$CLAUDE_PLUGINS_DIR"/synced/*/"$2"/.mcp.json "$CLAUDE_PLUGINS_DIR"/synced/*/"$2"~*/.mcp.json; do
            [ -f "$f" ] && { printf '%s\n' "$f"; return 0; }
        done
        return 0
    fi
    p="$(jq -r --arg id "$1" '.plugins[$id][0].installPath // empty' "$CLAUDE_PLUGINS_DIR/installed_plugins.json" 2>/dev/null)"
    [ -n "$p" ] && [ -f "$p/.mcp.json" ] && printf '%s\n' "$p/.mcp.json"
    return 0
}

# deny_entry <.mcp.json> <server> — the deniedMcpServers entry that matches
# that server: its exact command line, or its URL. Empty if it has neither.
deny_entry() {
    jq -c --arg s "$2" '(.mcpServers // .)[$s] // empty
        | if .command then {serverCommand: ([.command] + (.args // []))}
          elif (.url // "") != "" then {serverUrl: .url}
          else empty end' "$1" 2>/dev/null
}

# sync_mcp_flags — apply the ai.mcp.<server> and ai.connectors.claude-ai flags
# to ~/.claude/settings.json. Entries this script added earlier (the state
# file) are replaced by the current set, so a flag turned back on is undone;
# anything else in the file is left alone.
sync_mcp_flags() {
    local want="[]" off=false name plugin synced servers s f e
    gff_on ai.connectors.claude-ai || off=true
    if ! command -v jq >/dev/null 2>&1; then
        if [ "$off" = "true" ] || env | grep -q '^GFF_AI_MCP_.*=false$'; then
            echo "sync-plugins: WARNING — jq is not on PATH; the ai.mcp/ai.connectors flags were not applied." >&2
        fi
        return 0
    fi
    while IFS='|' read -r name plugin synced servers; do
        gff_on "ai.plugin.$name" || continue # the plugin is off: none of its servers load
        for s in $servers; do
            gff_on "ai.mcp.$s" && continue
            e=""
            f="$(plugin_mcp_file "$plugin" "$name" "$synced")"
            [ -n "$f" ] && e="$(deny_entry "$f" "$s")"
            if [ -z "$e" ]; then
                echo "sync-plugins: WARNING — ai.mcp.$s is false, but $plugin has no '$s' server installed here; not denied." >&2
                continue
            fi
            [ "$DRY_RUN" = "1" ] && echo "DRY-RUN: deny MCP server $s $e"
            want="$(jq -c --argjson e "$e" 'if any(.[]; . == $e) then . else . + [$e] end' <<<"$want")"
        done
    done < <(yq '.plugins[] | select(.enabled == true) | select(.claude.plugin != null) | select(.mcp != null) | .name + "|" + .claude.plugin + "|" + ((.claude.synced // false) | tostring) + "|" + (.mcp | join(" "))' "$MANIFEST")
    if [ "$DRY_RUN" = "1" ]; then
        [ "$off" = "true" ] && echo "DRY-RUN: disableClaudeAiConnectors: true in $CLAUDE_SETTINGS_FILE"
        return 0
    fi
    local state="$SYNC_PLUGINS_STATE_DIR/claude-mcp-flags.json" prev cur new newstate
    prev='{"denied":[],"connectors":false}'
    [ -f "$state" ] && prev="$(jq -c '{denied: (.denied // []), connectors: (.connectors // false)}' "$state" 2>/dev/null || echo "$prev")"
    # Nothing asked for and nothing of ours to undo: leave the file untouched.
    if [ "$want" = "[]" ] && [ "$off" = "false" ] && [ "$prev" = '{"denied":[],"connectors":false}' ]; then
        return 0
    fi
    cur='{}'
    [ -s "$CLAUDE_SETTINGS_FILE" ] && cur="$(cat "$CLAUDE_SETTINGS_FILE")"
    new="$(jq --argjson prev "$prev" --argjson want "$want" --argjson off "$off" '
        ((.deniedMcpServers // []) | map(. as $x | select(any($prev.denied[]; . == $x) | not))) as $kept
        | ($kept + ($want | map(. as $x | select(any($kept[]; . == $x) | not)))) as $denied
        | (if ($denied | length) > 0 then .deniedMcpServers = $denied else del(.deniedMcpServers) end)
        | if $off then .disableClaudeAiConnectors = true
          elif $prev.connectors then del(.disableClaudeAiConnectors)
          else . end' <<<"$cur")" || {
        echo "sync-plugins: WARNING — $CLAUDE_SETTINGS_FILE is not valid JSON; the ai.mcp/ai.connectors flags were not applied." >&2
        return 0
    }
    newstate="$(jq -c -n --argjson prev "$prev" --argjson want "$want" --argjson off "$off" --argjson cur "$cur" '
        ((($cur.deniedMcpServers // []) | map(. as $x | select(any($prev.denied[]; . == $x) | not)))) as $kept
        | {denied: ($want | map(. as $x | select(any($kept[]; . == $x) | not))),
           connectors: ($off and ($prev.connectors or ($cur.disableClaudeAiConnectors != true)))}')"
    if [ "$(jq -S . <<<"$cur")" != "$(jq -S . <<<"$new")" ]; then
        mkdir -p "$(dirname "$CLAUDE_SETTINGS_FILE")"
        printf '%s\n' "$new" > "$CLAUDE_SETTINGS_FILE"
        echo "sync-plugins: updated MCP flags in $CLAUDE_SETTINGS_FILE ($(jq 'length' <<<"$want") server(s) denied, claude.ai connectors $([ "$off" = "true" ] && echo off || echo on)); restart Claude Code sessions to apply."
    fi
    mkdir -p "$SYNC_PLUGINS_STATE_DIR"
    printf '%s\n' "$newstate" > "$state"
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

# Runtime-prerequisite check for the plugins whose MCP servers launch via
# `uvx` (Astral's `uv` tool-runner — the npx equivalent for a PyPI CLI, a
# single static binary with no Python prerequisite of its own). Neither uvx
# nor uv is installed by this repo, so on a host without them these
# plugins' servers fail to connect at every future session start
# (dotfiles#312): a confusing "Executable not found in $PATH: uvx" surfaces
# later, inside an assistant session, naming a binary the user never chose
# to depend on. Scope is the AWS trio only — the only enabled rows that
# need it today; extend this list if a future plugin adds the same
# dependency. Read-only and runs in both dry-run and real mode.
check_uvx_prereq() {
    command -v uvx >/dev/null 2>&1 && return 0
    local need
    need="$(yq '.plugins[] | select(.enabled == true) | select(.name == "deploy-on-aws" or .name == "aws-serverless" or .name == "aws-core") | .name' "$MANIFEST" 2>/dev/null | while IFS= read -r n; do gff_on "ai.plugin.$n" && printf '%s ' "$n"; done)"
    need="${need% }"
    [ -z "$need" ] && return 0
    echo "sync-plugins: WARNING — uvx is not on PATH; the MCP server(s) for: $need will fail to connect. Install uv (https://docs.astral.sh/uv/getting-started/installation/), or set those plugins' 'enabled: false' in ai/plugins.yaml if they aren't needed on this host." >&2
}

echo "Syncing AI plugins from ${MANIFEST}$([ "$DRY_RUN" = "1" ] && echo ' (dry-run)')..."
sync_claude
sync_mcp_flags
sync_antigravity
check_uvx_prereq
echo "sync-plugins: done."
