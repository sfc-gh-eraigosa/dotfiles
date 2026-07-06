#!/bin/bash
# antigravity_adapter.sh — translate Antigravity CLI hook payloads to the
# shared guard contract, run a guard, and translate the verdict back.
#
# Antigravity CLI (agy) hooks speak a different dialect than Claude Code /
# the retired Gemini CLI:
#   - stdin:  JSON {toolCall: {name, args}, ...}         (guards expect
#             {tool_name, tool_input})
#   - stdout: JSON {"decision": "allow"|"deny"|"ask", "reason": ...}
#             (guards signal block via exit code 2 + stderr)
#
# Rather than fork the guards per assistant, this adapter keeps
# safety_guard.sh / privacy_guard.sh as the single shared rule set:
#
#   hooks.json:  antigravity_adapter.sh <guard.sh>
#
# Tool-name / argument mapping (Antigravity -> shared contract):
#   run_command                  -> run_shell_command  {command: args.CommandLine}
#   write_to_file                -> write_file  {file_path: args.TargetFile,
#                                                content: args.CodeContent}
#   replace_file_content /       -> replace     {file_path: args.TargetFile,
#   multi_replace_file_content                   new_string: joined
#                                                args.ReplacementChunks[].ReplacementContent}
#
# Dependencies: jq, bash 3.2+

set -u

GUARD="${1:-}"
if [ -z "$GUARD" ] || [ ! -x "$GUARD" ]; then
    # Misconfiguration must not silently allow: surface it but do not block.
    echo "{\"decision\": \"allow\", \"reason\": \"antigravity_adapter: guard not found or not executable: ${GUARD}\"}"
    exit 0
fi

PAYLOAD="$(cat)"

if ! command -v jq >/dev/null 2>&1; then
    echo '{"decision": "allow", "reason": "antigravity_adapter: jq not installed; guard skipped"}'
    exit 0
fi

TOOL="$(printf '%s' "$PAYLOAD" | jq -r '.toolCall.name // empty')"

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
                  new_string: (([$tc.args.ReplacementChunks[]?.ReplacementContent] | join("\n"))
                               + ($tc.args.ReplacementContent // ""))}}
  else
    {tool_name: ($tc.name // ""), tool_input: ($tc.args // {})}
  end' 2>/dev/null)"

if [ -z "$TRANSLATED" ]; then
    echo '{"decision": "allow", "reason": "antigravity_adapter: could not parse hook payload"}'
    exit 0
fi

STDERR_FILE="$(mktemp)"
printf '%s' "$TRANSLATED" | "$GUARD" >/dev/null 2>"$STDERR_FILE"
GUARD_EXIT=$?
REASON="$(head -c 1000 "$STDERR_FILE" | tr '\n' ' ' | sed 's/"/\\"/g')"
rm -f "$STDERR_FILE"

if [ "$GUARD_EXIT" -eq 2 ]; then
    echo "{\"decision\": \"deny\", \"reason\": \"${REASON:-Blocked by guard $(basename "$GUARD")}\"}"
else
    echo '{"decision": "allow"}'
fi
exit 0
