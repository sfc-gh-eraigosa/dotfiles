#!/usr/bin/env bash
# validate_hooks.sh [settings.json]
#
# D3 of the AI-config provisioning design: validate the hook wiring that is
# ACTUALLY configured in the live settings file — not the repo copy — and
# exercise it. This is the check that would have caught #111: a settings.json
# pointing at a moved/dead hook path passes a stat of the repo hook but is, in
# fact, not wired. Defaults to ~/.claude/settings.json.
#
# Checks:
#   1. every PreToolUse hook command resolves to an executable file
#   2. the statusLine command's script is readable
#   3. safety_guard, driven through its CONFIGURED command, BLOCKS a known-bad
#      command (exit 2) and ALLOWS a known-good one (exit 0)
#   4. privacy_guard, driven through its configured command, runs clean (exit 0)
#      — block-behaviour coverage lives in ai/hooks/privacy_guard_test.sh
#
# Exit 0 = all good; non-zero = a configured hook is missing/inert.
set -u

SETTINGS="${1:-$HOME/.claude/settings.json}"
FAILS=0
fail() { echo "FAIL: $1" >&2; FAILS=$((FAILS + 1)); }

if ! command -v jq > /dev/null 2>&1; then
    echo "SKIP: jq not installed — hook validation skipped"
    exit 0
fi
[ -f "$SETTINGS" ] || { echo "FAIL: settings file not found: $SETTINGS" >&2; exit 1; }

# Expand a leading ~ and any $HOME in a path string.
expand() { local s="$1"; s="${s/#\~/$HOME}"; s="${s//\$HOME/$HOME}"; printf '%s' "$s"; }

# rc of running a hook script at $1 with the JSON payload $2 on stdin.
hook_rc() { local path="$1" payload="$2"; printf '%s' "$payload" | bash "$path" > /dev/null 2>&1; echo $?; }

# Event-agnostic: gather hook commands across every hook event so this validates
# both Claude (.hooks.PreToolUse[]) and Gemini (.hooks.BeforeTool[]) settings.
# mapfile is bash-4 only; use a bash-3.2-safe read loop (macOS system bash is 3.2).
HOOK_CMDS=()
while IFS= read -r _hc; do HOOK_CMDS+=("$_hc"); done < <(jq -r '[.hooks[]?[]?.hooks[]?.command] | .[]' "$SETTINGS" 2>/dev/null)
STATUS_CMD="$(jq -r '.statusLine.command // empty' "$SETTINGS" 2>/dev/null)"

if [ "${#HOOK_CMDS[@]}" -eq 0 ]; then
    fail "no hook commands configured in $SETTINGS"
fi

for cmd in "${HOOK_CMDS[@]:-}"; do
    [ -n "$cmd" ] || continue
    path="$(expand "${cmd%% *}")"   # the hook command IS the script path
    if [ ! -x "$path" ]; then
        fail "configured hook command not executable: '$cmd' (resolved: $path)"
        continue
    fi
    case "$(basename "$path")" in
        safety_guard.sh)
            [ "$(hook_rc "$path" '{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}')" = "2" ] \
                || fail "safety_guard did NOT block a known-bad command via the configured path ($path) — fail-open"
            [ "$(hook_rc "$path" '{"tool_name":"Bash","tool_input":{"command":"ls -la"}}')" = "0" ] \
                || fail "safety_guard did NOT allow a known-good command via the configured path ($path)"
            ;;
        privacy_guard.sh)
            [ "$(hook_rc "$path" '{"tool_name":"Write","tool_input":{"file_path":"/tmp/_vh_clean.md","content":"hello world"}}')" = "0" ] \
                || fail "privacy_guard errored on clean input via the configured path ($path)"
            ;;
    esac
done

if [ -n "$STATUS_CMD" ]; then
    sl_path="$(expand "${STATUS_CMD##* }")"   # last token = script path
    [ -r "$sl_path" ] || fail "statusLine script not readable: '$STATUS_CMD' (resolved: $sl_path)"
fi

if [ "$FAILS" -gt 0 ]; then
    echo "FAIL: $FAILS configured-hook problem(s) in $SETTINGS" >&2
    exit 1
fi
echo "PASS: configured hooks resolve; safety_guard blocks via the configured command"
