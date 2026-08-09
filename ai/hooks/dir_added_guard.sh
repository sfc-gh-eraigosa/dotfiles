#!/usr/bin/env bash
# DirectoryAdded hook (Claude Code >= v2.1.221; never fires on older versions).
# Warns when a directory registered mid-session (/add-dir) is a sensitive
# credential/config path, injecting a reminder into model context. Never blocks.
# Input (stdin JSON): { hook_event_name, session_id, cwd, directory_path, source, ... }
# See docs/claude-code-support.md for the version-support policy.

set -u

command -v jq >/dev/null 2>&1 || exit 0

INPUT="$(cat 2>/dev/null || true)"
[ -n "$INPUT" ] || exit 0

DIR="$(printf '%s' "$INPUT" | jq -r '.directory_path // .directory // .path // empty' 2>/dev/null)"
[ -n "$DIR" ] || exit 0

# Normalize: expand a leading ~ and strip a trailing slash. The quoted tilde
# is deliberate — we are matching a literal "~" in the JSON payload string.
# shellcheck disable=SC2088
case "$DIR" in
  "~"|"~/"*) DIR="${HOME}${DIR#\~}" ;;
esac
DIR="${DIR%/}"

SENSITIVE=""
for s in "$HOME/.ssh" "$HOME/.aws" "$HOME/.gnupg" "$HOME/.config/gss" "$HOME/.kube" "$HOME/.docker"; do
  if [ "$DIR" = "$s" ] || case "$DIR" in "$s"/*) true ;; *) false ;; esac; then
    SENSITIVE="$s"
    break
  fi
done

[ -n "$SENSITIVE" ] || exit 0

jq -n --arg dir "$DIR" --arg root "$SENSITIVE" '{
  systemMessage: "dir_added_guard: \($dir) is under the sensitive path \($root)",
  hookSpecificOutput: {
    hookEventName: "DirectoryAdded",
    additionalContext: "The directory \($dir) added to this session is under \($root), which holds credentials or security-sensitive config. Do not read, display, or commit secret material from it; reference files by path only, and never copy key/token contents into the conversation, commits, or PRs."
  }
}'
exit 0
