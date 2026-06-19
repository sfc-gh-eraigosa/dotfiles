#!/usr/bin/env bash
# sync-mcp.sh — ensure the standalone MCP servers declared in ai/mcp.yaml are
# registered for Claude Code (`claude mcp add`) and Gemini CLI (`gemini mcp add`).
# Ensure-only (additive): never removes anything. Mirrors sync-plugins.sh.
# Safe to re-run.
#
# Distinct from sync-plugins.sh: that installs marketplace plugins / git extensions;
# this registers raw MCP servers (a different concern — see ai/mcp.yaml header).
#
# Registration ONLY: this writes the server *definition* (a config entry). It never
# runs the server, never opens a browser, and never performs the interactive Google
# login (`setup_auth`) that notebooklm-mcp needs at first use — that is a manual,
# user-run runtime step (docs/ai-mcp.md). Keeping install = "register" (and auth =
# a separate human step) is a hard security boundary: an unattended installer must
# never launch a credential-handling browser.
#
# Usage:
#   sync-mcp.sh            register per the manifest (user scope)
#   sync-mcp.sh --dry-run  print planned actions, change nothing (and previews even
#                          when the claude/gemini CLIs are absent)
set -u

# Resolve the real repo root even when invoked via the ~/opt symlink.
SCRIPT_PATH="$(readlink -f "$0")"
BASE_DIR="$(cd "$(dirname "$SCRIPT_PATH")/../../.." && pwd)"
# SYNC_MCP_MANIFEST overrides the manifest path (used by the test driver to feed
# hermetic fixtures); defaults to the repo's ai/mcp.yaml.
MANIFEST="${SYNC_MCP_MANIFEST:-${BASE_DIR}/ai/mcp.yaml}"

DRY_RUN=0
case "${1:-}" in
    --dry-run) DRY_RUN=1 ;;
    --help|-h)
        echo "Usage: sync-mcp [--dry-run]"
        echo "Register the MCP servers listed in ai/mcp.yaml at user scope (ensure-only)."
        exit 0 ;;
    "") ;;
    *) echo "sync-mcp: unknown argument '$1'" >&2; exit 2 ;;
esac

# Resolve a timeout binary (GNU 'timeout', or 'gtimeout' from coreutils on macOS).
# Empty when neither exists — calls then run unwrapped but still stdin-guarded.
TIMEOUT_BIN="$(command -v timeout || command -v gtimeout || true)"
# Resolve setsid (util-linux). The claude/gemini subcommands may open /dev/tty
# directly and render an interactive TUI when a controlling terminal is present —
# bypassing the </dev/null stdin guard below and wedging an unattended
# `./install.sh`. Run them under `setsid` so they get a new session with NO
# controlling terminal, can't open /dev/tty, and fall back to non-interactive
# mode. Empty on macOS (no native setsid); the per-call </dev/null + timeout
# guards still apply there.
SETSID_BIN="$(command -v setsid || true)"
# macOS ships no native setsid. Homebrew's keg-only util-linux provides a working
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
# can't wedge install.sh indefinitely. Override with SYNC_MCP_TIMEOUT for slow links.
CMD_TIMEOUT="${SYNC_MCP_TIMEOUT:-300}"
# A Node-based CLI may ignore SIGTERM, so a plain `timeout N` would send SIGTERM
# and then wait forever. `-k KILL_GRACE` escalates to SIGKILL KILL_GRACE seconds
# after the initial signal, guaranteeing the call actually terminates.
KILL_GRACE="${SYNC_MCP_KILL_GRACE:-15}"

# Guard prefix applied to every claude/gemini invocation (see SETSID_BIN/TIMEOUT_BIN
# above). Either tool may be absent (minimal containers / macOS lacks setsid); the
# prefix omits whatever is missing. </dev/null at each call site is the stdin guard.
GUARD=()
[ -n "$SETSID_BIN" ] && GUARD+=("$SETSID_BIN" -w)
[ -n "$TIMEOUT_BIN" ] && GUARD+=("$TIMEOUT_BIN" -k "$KILL_GRACE" "$CMD_TIMEOUT")

if ! command -v yq >/dev/null 2>&1; then
    echo "sync-mcp: 'yq' not found. Install it via opt/scripts/system/install_yq.sh" >&2
    exit 1
fi
if [ ! -f "$MANIFEST" ]; then
    echo "sync-mcp: manifest not found: $MANIFEST" >&2
    exit 1
fi

# Read one server field (returns empty string when absent/null).
server_field() {
    local name="$1" field="$2" val
    val="$(yq ".servers[] | select(.name == \"$name\") | .$field" "$MANIFEST")"
    [ "$val" = "null" ] && val=""
    printf '%s' "$val"
}

# Does this server have a (possibly empty) per-tool block?  `claude: {}` -> yes.
server_has_tool() {
    local name="$1" tool="$2" val
    val="$(yq ".servers[] | select(.name == \"$name\") | has(\"$tool\")" "$MANIFEST")"
    [ "$val" = "true" ]
}

# Splat a server's args list into the global ARGV array (bash 3.2 safe — no mapfile).
ARGV=()
load_server_args() {
    local name="$1" a
    ARGV=()
    while IFS= read -r a; do
        [ -z "$a" ] && continue
        ARGV+=("$a")
    done < <(yq ".servers[] | select(.name == \"$name\") | .args[]" "$MANIFEST")
}

# Register one server for Claude Code at USER scope. `--scope user` is load-bearing:
# the default scope is `local`/project, which would bind the server to install.sh's
# working directory and make it invisible everywhere else. `claude mcp add` only
# writes config (it does not spawn the server), so this never launches a browser.
# `claude mcp add` is rc=0 even on a duplicate ("already exists"), so tolerate that
# string as a quiet skip (mirrors how enable_claude_plugin tolerates "already enabled").
add_claude_server() {
    local name="$1"; shift
    if [ "$DRY_RUN" = "1" ]; then
        echo "DRY-RUN: claude mcp add --scope user $name -- $*"
        return 0
    fi
    echo "+ claude mcp add --scope user $name -- $*"
    local out rc=0
    out="$("${GUARD[@]+"${GUARD[@]}"}" claude mcp add --scope user "$name" -- "$@" </dev/null 2>&1)" || rc=$?
    if printf '%s' "$out" | grep -qi "already exists"; then
        echo "  ($name already registered for Claude)"
    elif [ "$rc" -eq 0 ]; then
        [ -n "$out" ] && echo "$out"
    elif [ "$rc" -eq 124 ] || [ "$rc" -eq 137 ]; then
        echo "sync-mcp: WARNING — claude mcp add $name timed out after ${CMD_TIMEOUT}s; continuing." >&2
    else
        printf '%s\n' "$out" >&2
        echo "sync-mcp: WARNING — claude mcp add $name failed (rc=$rc); continuing." >&2
    fi
}

# Register one server for Gemini CLI at USER scope. Gemini's add is idempotent
# (rc=0 "already configured ... updated"); tolerate that wording as a quiet skip.
add_gemini_server() {
    local name="$1" transport="$2"; shift 2
    if [ "$DRY_RUN" = "1" ]; then
        echo "DRY-RUN: gemini mcp add --scope user --transport $transport $name $*"
        return 0
    fi
    echo "+ gemini mcp add --scope user --transport $transport $name $*"
    local out rc=0
    out="$("${GUARD[@]+"${GUARD[@]}"}" gemini mcp add --scope user --transport "$transport" "$name" "$@" </dev/null 2>&1)" || rc=$?
    if printf '%s' "$out" | grep -qiE "already configured|already exists"; then
        echo "  ($name already registered for Gemini)"
    elif [ "$rc" -eq 0 ]; then
        [ -n "$out" ] && echo "$out"
    elif [ "$rc" -eq 124 ] || [ "$rc" -eq 137 ]; then
        echo "sync-mcp: WARNING — gemini mcp add $name timed out after ${CMD_TIMEOUT}s; continuing." >&2
    else
        printf '%s\n' "$out" >&2
        echo "sync-mcp: WARNING — gemini mcp add $name failed (rc=$rc); continuing." >&2
    fi
}

claude_available=0
gemini_available=0
if [ "$DRY_RUN" = "1" ]; then
    claude_available=1; gemini_available=1   # dry-run previews regardless of CLIs
else
    command -v claude >/dev/null 2>&1 && claude_available=1
    command -v gemini >/dev/null 2>&1 && gemini_available=1
    [ "$claude_available" = "0" ] && echo "sync-mcp: 'claude' CLI not on PATH; skipping Claude MCP servers."
    [ "$gemini_available" = "0" ] && echo "sync-mcp: 'gemini' CLI not on PATH; skipping Gemini MCP servers."
fi

echo "Syncing MCP servers from ${MANIFEST}$([ "$DRY_RUN" = "1" ] && echo ' (dry-run)')..."

any=0
while IFS= read -r name; do
    { [ -z "$name" ] || [ "$name" = "null" ]; } && continue
    any=1
    transport="$(server_field "$name" transport)"
    [ -z "$transport" ] && transport="stdio"
    # stdio only. http/SSE would expose an unauthenticated endpoint that can drive a
    # browser holding the user's Google session — refuse it rather than register it.
    if [ "$transport" != "stdio" ]; then
        echo "sync-mcp: WARNING — server '$name' has unsupported transport '$transport' (stdio only); skipping." >&2
        continue
    fi
    command_bin="$(server_field "$name" command)"
    if [ -z "$command_bin" ]; then
        echo "sync-mcp: WARNING — server '$name' has no 'command'; skipping." >&2
        continue
    fi
    load_server_args "$name"

    if [ "$claude_available" = "1" ] && server_has_tool "$name" claude; then
        add_claude_server "$name" "$command_bin" "${ARGV[@]+"${ARGV[@]}"}"
    fi
    if [ "$gemini_available" = "1" ] && server_has_tool "$name" gemini; then
        add_gemini_server "$name" "$transport" "$command_bin" "${ARGV[@]+"${ARGV[@]}"}"
    fi
done < <(yq '.servers[] | select(.enabled == true) | .name' "$MANIFEST")

[ "$any" = "0" ] && echo "sync-mcp: no enabled MCP servers in manifest (nothing to do)."
echo "sync-mcp: done."
