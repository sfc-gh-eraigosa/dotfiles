#!/usr/bin/env bash
# Test driver for validate_hooks.sh — the D3 "validate + exercise the LIVE
# configured hook command" check. This is the guard that would have caught #111
# (settings.json pointing at a moved/dead hook path) where a stat of the repo
# copy passed green.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
# shellcheck source=/dev/null
. "$REPO_ROOT/ai/_test_helpers.sh"

VALIDATE="$SCRIPT_DIR/validate_hooks.sh"

if ! command -v jq >/dev/null 2>&1; then
    echo "SKIP: jq not installed — validate_hooks tests skipped"
    exit 0
fi

H="$(mktemp -d)"
trap 'rm -rf "$H"' EXIT

# A realistic provisioned ~/.claude: hooks copied in, statusline present.
mkdir -p "$H/.claude/hooks"
cp "$REPO_ROOT/ai/hooks/safety_guard.sh" "$H/.claude/hooks/"
cp "$REPO_ROOT/ai/hooks/privacy_guard.sh" "$H/.claude/hooks/"
cp "$REPO_ROOT/ai/hooks/strip_heredocs.awk" "$H/.claude/hooks/"
chmod +x "$H/.claude/hooks/"*.sh
touch "$H/.claude/statusline-command.sh"

cat > "$H/.claude/settings.json" <<'JSON'
{
  "hooks": { "PreToolUse": [
    { "matcher": "Bash", "hooks": [ { "type": "command", "command": "$HOME/.claude/hooks/safety_guard.sh" } ] },
    { "matcher": "Write|Edit|Bash", "hooks": [ { "type": "command", "command": "$HOME/.claude/hooks/privacy_guard.sh" } ] }
  ] },
  "statusLine": { "type": "command", "command": "bash ~/.claude/statusline-command.sh" }
}
JSON
assert_exit_code 0 "valid configured hooks + statusLine pass validation" \
    env HOME="$H" bash "$VALIDATE" "$H/.claude/settings.json"

# The #111 shape: settings references a hook path that does not exist.
cat > "$H/.claude/dead.json" <<'JSON'
{
  "hooks": { "PreToolUse": [
    { "matcher": "Bash", "hooks": [ { "type": "command", "command": "$HOME/.claude/hooks/MISSING_guard.sh" } ] }
  ] }
}
JSON
assert_exit_code 1 "dead configured hook path FAILS validation (catches #111)" \
    env HOME="$H" bash "$VALIDATE" "$H/.claude/dead.json"

# A hook that exists but no longer blocks (fail-open) must be caught by the
# behavioral exercise, not just the stat.
mkdir -p "$H/.claude/broken/hooks"
cp "$H/.claude/statusline-command.sh" "$H/.claude/broken/hooks/safety_guard.sh"  # an inert script: exits 0 always
chmod +x "$H/.claude/broken/hooks/safety_guard.sh"
cat > "$H/.claude/brokenhook.json" <<'JSON'
{
  "hooks": { "PreToolUse": [
    { "matcher": "Bash", "hooks": [ { "type": "command", "command": "$HOME/.claude/broken/hooks/safety_guard.sh" } ] }
  ] }
}
JSON
assert_exit_code 1 "fail-open safety_guard (allows known-bad) FAILS the exercise" \
    env HOME="$H" bash "$VALIDATE" "$H/.claude/brokenhook.json"

# Event-agnostic: a Gemini-style settings file (BeforeTool) also validates.
cp "$REPO_ROOT/ai/hooks/safety_guard.sh" "$H/.claude/hooks/"   # reuse the copied hooks
cat > "$H/.gemini-style.json" <<'JSON'
{
  "hooks": { "BeforeTool": [
    { "matcher": "run_shell_command", "hooks": [ { "name": "safety-guard", "type": "command", "command": "$HOME/.claude/hooks/safety_guard.sh" } ] }
  ] }
}
JSON
assert_exit_code 0 "Gemini-style BeforeTool hooks validate (event-agnostic)" \
    env HOME="$H" bash "$VALIDATE" "$H/.gemini-style.json"

_test_report
