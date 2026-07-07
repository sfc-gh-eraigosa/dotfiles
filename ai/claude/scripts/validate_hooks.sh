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

# Event-agnostic: gather hook commands across every hook event. Two layouts:
#   Claude settings.json:      {hooks: {Event: [{matcher, hooks: [{command}]}]}}
#   Antigravity hooks.json:    {name: {Event: [{matcher, hooks: [{command}]}]}}
# mapfile is bash-4 only; use a bash-3.2-safe read loop (macOS system bash is 3.2).
HOOK_CMDS=()
if jq -e '.hooks' "$SETTINGS" > /dev/null 2>&1; then
    while IFS= read -r _hc; do HOOK_CMDS+=("$_hc"); done < <(jq -r '[.hooks[]?[]?.hooks[]?.command] | .[]' "$SETTINGS" 2>/dev/null)
else
    while IFS= read -r _hc; do HOOK_CMDS+=("$_hc"); done < <(jq -r '[.[]?[]?[]?.hooks[]?.command] | .[]' "$SETTINGS" 2>/dev/null)
fi
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
        antigravity_adapter.sh)
            # Antigravity wiring: "adapter.sh <guard.sh> [guard2.sh ...]".
            # Bare guard names resolve relative to the adapter's directory
            # (mirroring the adapter's own resolution). Validate every guard
            # resolves, then drive the FULL configured command with
            # agy-dialect payloads and check the JSON verdict on stdout.
            read -r -a _tokens <<< "$cmd"
            if [ "${#_tokens[@]}" -lt 2 ]; then
                fail "adapter configured with NO guard argument ('$cmd') — it would answer ask/allow for everything"
                continue
            fi
            guards_ok=1
            for _g in "${_tokens[@]:1}"; do
                _gpath="$(expand "$_g")"
                case "$_gpath" in
                    */*) ;;
                    *) _gpath="$(dirname "$path")/$_gpath" ;;
                esac
                if [ ! -x "$_gpath" ]; then
                    fail "adapter's guard argument not executable: '$_g' (resolved: $_gpath)"
                    guards_ok=0
                fi
            done
            [ "$guards_ok" = "1" ] || continue
            adapter_decision() { printf '%s' "$1" | bash "$path" "${_tokens[@]:1}" 2>/dev/null | jq -r '.decision // empty'; }
            case " ${_tokens[*]:1} " in
                *safety_guard.sh*)
                    [ "$(adapter_decision '{"toolCall":{"name":"run_command","args":{"CommandLine":"rm -rf /"}}}')" = "deny" ] \
                        || fail "adapter+safety_guard did NOT deny a known-bad command ($cmd) — fail-open"
                    [ "$(adapter_decision '{"toolCall":{"name":"run_command","args":{"CommandLine":"ls -la"}}}')" = "allow" ] \
                        || fail "adapter+safety_guard did NOT allow a known-good command ($cmd)"
                    [ "$(adapter_decision '{"toolCall":{"name":"run_command","args":{"CommandLine":"sudo reboot"}}}')" = "ask" ] \
                        || fail "adapter+safety_guard did NOT map the confirmation tier to ask ($cmd)"
                    ;;
            esac
            case " ${_tokens[*]:1} " in
                *privacy_guard.sh*)
                    [ "$(adapter_decision '{"toolCall":{"name":"write_to_file","args":{"TargetFile":"/tmp/_vh_clean.md","CodeContent":"hello world"}}}')" = "allow" ] \
                        || fail "adapter+privacy_guard did not allow clean input ($cmd)"
                    ;;
            esac
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
