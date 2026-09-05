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
cp "$REPO_ROOT/ai/hooks/privacy_rules.sh" "$H/.claude/hooks/"   # the guard fails closed without its rule library
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

# Event-agnostic: a legacy Gemini-style settings file (BeforeTool) also validates.
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

# Antigravity hooks.json layout (named hooks, no top-level "hooks" key), with
# the adapter bridging agy's {toolCall}/{decision} dialect to the shared guards.
mkdir -p "$H/.gemini/config/hooks"
cp "$REPO_ROOT/ai/hooks/safety_guard.sh" "$REPO_ROOT/ai/hooks/privacy_guard.sh" "$REPO_ROOT/ai/hooks/privacy_rules.sh" \
   "$REPO_ROOT/ai/hooks/strip_heredocs.awk" "$REPO_ROOT/ai/hooks/antigravity_adapter.sh" \
   "$H/.gemini/config/hooks/"
chmod +x "$H/.gemini/config/hooks/"*.sh
cat > "$H/.gemini/config/hooks.json" <<'JSON'
{
  "guards": { "PreToolUse": [
    { "matcher": "run_command|write_to_file|replace_file_content|multi_replace_file_content|edit_file", "hooks": [ { "type": "command", "command": "$HOME/.gemini/config/hooks/antigravity_adapter.sh safety_guard.sh privacy_guard.sh" } ] }
  ] }
}
JSON
assert_exit_code 0 "Antigravity hooks.json validates (adapter + both guards, bare names)" \
    env HOME="$H" bash "$VALIDATE" "$H/.gemini/config/hooks.json"

# Adapter configured WITHOUT a guard argument: the exact fail-open shape the
# validator exists to catch (adapter alone answers ask/allow for everything).
cat > "$H/.gemini/config/noguard.json" <<'JSON'
{
  "guards": { "PreToolUse": [
    { "matcher": "run_command", "hooks": [ { "type": "command", "command": "$HOME/.gemini/config/hooks/antigravity_adapter.sh" } ] }
  ] }
}
JSON
assert_exit_code 1 "adapter with NO guard argument FAILS validation" \
    env HOME="$H" bash "$VALIDATE" "$H/.gemini/config/noguard.json"

# Antigravity fail-open shape: adapter wired to an inert guard must FAIL.
cp "$H/.claude/statusline-command.sh" "$H/.gemini/config/hooks/safety_guard_inert.sh"
chmod +x "$H/.gemini/config/hooks/safety_guard_inert.sh"
mv "$H/.gemini/config/hooks/safety_guard.sh" "$H/.gemini/config/hooks/_real_safety_guard.sh"
mv "$H/.gemini/config/hooks/safety_guard_inert.sh" "$H/.gemini/config/hooks/safety_guard.sh"
assert_exit_code 1 "Antigravity fail-open guard behind the adapter FAILS the exercise" \
    env HOME="$H" bash "$VALIDATE" "$H/.gemini/config/hooks.json"

_test_report
