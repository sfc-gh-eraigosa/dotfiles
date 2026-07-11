#!/usr/bin/env bash
# antigravity_adapter.sh — translate Antigravity CLI hook payloads to the
# shared guard contract, run the guards, and translate the verdict back.
#
# Antigravity CLI (agy) hooks speak a different dialect than Claude Code /
# the retired Gemini CLI:
#   - stdin:  JSON {toolCall: {name, args}, ...}         (guards expect
#             {tool_name, tool_input})
#   - stdout: JSON {"decision": "allow"|"deny"|"ask", "reason": ...}
#             (guards signal via exit code: 2 = deny, 3 = ask; stderr
#             carries the reason)
#
# Rather than fork the guards per assistant, this adapter keeps
# safety_guard.sh / privacy_guard.sh as the single shared rule set and runs
# them all off ONE payload translation:
#
#   hooks.json:  antigravity_adapter.sh safety_guard.sh privacy_guard.sh
#
# Guard arguments without a "/" are resolved relative to this script's own
# directory (the installer copies adapter + guards side by side into
# ~/.gemini/config/hooks/), so hooks.json needs no absolute guard paths.
#
# Returning "allow" auto-approves the tool call — this deliberately mirrors
# the retired Gemini CLI trusted-tools.toml tier (routine shell/file tools
# ran without prompts, with the deny rules layered above). Misconfiguration
# (missing jq, missing guard, unparseable payload) degrades to "ask", never
# to silent allow.
#
# Tool-name / argument mapping (Antigravity -> shared contract):
#   run_command                  -> run_shell_command  {command: args.CommandLine}
#   write_to_file                -> write_file  {file_path: args.TargetFile,
#                                                content: args.CodeContent}
#   replace_file_content /       -> replace     {file_path: args.TargetFile,
#   multi_replace_file_content                   new_string: joined
#                                                args.ReplacementChunks[].ReplacementContent}
#   edit_file                    -> replace     {file_path: args.TargetFile,
#                                                new_string: every string arg
#                                                joined (instruction + code)}
#
# Dependencies: jq, bash 3.2+

set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"

# Emit a verdict JSON and exit 0 (agy reads stdout; non-zero would be a hook
# error). jq --arg gives correct escaping for arbitrary reason text.
verdict() {
    local decision="$1" reason="${2:-}"
    if command -v jq >/dev/null 2>&1; then
        jq -cn --arg d "$decision" --arg r "$reason" \
            'if $r == "" then {decision: $d} else {decision: $d, reason: $r} end'
    else
        # jq-less fallback: decisions are fixed tokens; reason is dropped
        # rather than risking broken hand-escaped JSON.
        printf '{"decision": "%s"}\n' "$decision"
    fi
    exit 0
}

if [ "$#" -lt 1 ]; then
    verdict ask "antigravity_adapter: no guard argument configured (check hooks.json)"
fi

if ! command -v jq >/dev/null 2>&1; then
    verdict ask "antigravity_adapter: jq not installed; guards cannot run"
fi

# Resolve guards up front so a misconfigured entry never silently allows.
GUARDS=()
for g in "$@"; do
    case "$g" in
        */*) ;;
        *) g="$SELF_DIR/$g" ;;
    esac
    if [ ! -x "$g" ]; then
        verdict ask "antigravity_adapter: guard not found or not executable: $g"
    fi
    GUARDS+=("$g")
done

PAYLOAD="$(cat)"

TRANSLATED="$(printf '%s' "$PAYLOAD" | jq -c '
  .toolCall as $tc |
  if $tc.name == "run_command" then
    {tool_name: "run_shell_command", tool_input: {command: ($tc.args.CommandLine // "")}}
  elif $tc.name == "write_to_file" then
    {tool_name: "write_file",
     tool_input: {file_path: ($tc.args.TargetFile // ""),
                  content: ($tc.args.CodeContent // "")}}
  elif ($tc.name == "replace_file_content" or $tc.name == "multi_replace_file_content") then
    {tool_name: "replace",
     tool_input: {file_path: ($tc.args.TargetFile // ""),
                  new_string: ([$tc.args.ReplacementChunks[]?.ReplacementContent] | join("\n"))}}
  elif $tc.name == "edit_file" then
    # Arg names beyond TargetFile vary; scan every string argument so the
    # instruction and code content all reach the privacy guard.
    {tool_name: "replace",
     tool_input: {file_path: ($tc.args.TargetFile // ""),
                  new_string: ([$tc.args | to_entries[] | select(.key != "TargetFile") | .value | strings] | join("\n"))}}
  else
    {tool_name: ($tc.name // ""), tool_input: ($tc.args // {})}
  end' 2>/dev/null)"

if [ -z "$TRANSLATED" ]; then
    verdict ask "antigravity_adapter: could not parse hook payload"
fi

ASK_REASON=""
for guard in "${GUARDS[@]}"; do
    # Capture stderr (the guard's reason) while discarding stdout; the
    # pipeline's exit status is the guard's.
    REASON="$(printf '%s' "$TRANSLATED" | "$guard" 2>&1 >/dev/null)"
    GUARD_EXIT=$?
    if [ "$GUARD_EXIT" -eq 2 ]; then
        verdict deny "${REASON:-Blocked by $(basename "$guard")}"
    elif [ "$GUARD_EXIT" -eq 3 ] && [ -z "$ASK_REASON" ]; then
        ASK_REASON="${REASON:-$(basename "$guard") requests confirmation}"
    fi
done

if [ -n "$ASK_REASON" ]; then
    verdict ask "$ASK_REASON"
fi
verdict allow
