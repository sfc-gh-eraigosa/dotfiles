#!/usr/bin/env bash
# Test driver for apply-forced-settings.sh — the forced-field settings merge.
#
# Verifies the core guarantee of the AI-config provisioning design
# (docs/mbo/designs/2026-06-02-ai-config-home-provisioning.md §7, D2):
#   forced fields are applied from the repo; undeclared host fields are
#   preserved; bad input fails loud without clobbering the host file.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
# shellcheck source=/dev/null
. "$REPO_ROOT/ai/_test_helpers.sh"

APPLY="$SCRIPT_DIR/apply-forced-settings.sh"

if ! command -v jq >/dev/null 2>&1; then
    echo "SKIP: jq not installed — apply-forced-settings tests skipped"
    exit 0
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# --- Case 1: forced fields win; undeclared host fields are preserved ---
cat > "$tmp/host.json" <<'JSON'
{
  "hooks": { "PreToolUse": [ { "matcher": "OLD" } ] },
  "enabledPlugins": { "superpowers@x": true },
  "theme": "dark",
  "permissions": { "allow": ["Bash(git status:*)"], "deny": ["OLDDENY"], "defaultMode": "auto" }
}
JSON
cat > "$tmp/forced.json" <<'JSON'
{
  "hooks": { "PreToolUse": [ { "matcher": "Bash", "hooks": [ { "type": "command", "command": "$HOME/.claude/hooks/safety_guard.sh" } ] } ] },
  "statusLine": { "type": "command", "command": "bash ~/.claude/statusline-command.sh" },
  "permissions": { "deny": ["Bash(rm -rf /:*)"], "ask": ["Bash(gss push:*)"] }
}
JSON

bash "$APPLY" "$tmp/host.json" "$tmp/forced.json"

assert_eq "$(jq -r '.hooks.PreToolUse[0].matcher' "$tmp/host.json")" "Bash" "forced hooks overwrite host hooks"
assert_eq "$(jq -r '.hooks.PreToolUse[0].hooks[0].command' "$tmp/host.json")" '$HOME/.claude/hooks/safety_guard.sh' "forced hook command applied verbatim"
assert_eq "$(jq -r '.statusLine.command' "$tmp/host.json")" "bash ~/.claude/statusline-command.sh" "forced statusLine added"
assert_eq "$(jq -r '.enabledPlugins."superpowers@x"' "$tmp/host.json")" "true" "host enabledPlugins preserved"
assert_eq "$(jq -r '.theme' "$tmp/host.json")" "dark" "host theme preserved"
assert_eq "$(jq -r '.permissions.allow[0]' "$tmp/host.json")" "Bash(git status:*)" "host permissions.allow preserved"
assert_eq "$(jq -r '.permissions.defaultMode' "$tmp/host.json")" "auto" "host permissions.defaultMode preserved"
assert_eq "$(jq -r '.permissions.deny[0]' "$tmp/host.json")" "Bash(rm -rf /:*)" "forced permissions.deny overwrites"
assert_eq "$(jq -r '.permissions.ask[0]' "$tmp/host.json")" "Bash(gss push:*)" "forced permissions.ask added"

# --- Case 2: missing host file fails loud (caller must seed first) ---
assert_exit_code 1 "missing host file fails" bash "$APPLY" "$tmp/nope.json" "$tmp/forced.json"

# --- Case 3: invalid host JSON fails loud and leaves the host untouched ---
printf 'not valid json{' > "$tmp/bad.json"
before="$(cat "$tmp/bad.json")"
assert_exit_code 1 "invalid host JSON fails" bash "$APPLY" "$tmp/bad.json" "$tmp/forced.json"
assert_eq "$(cat "$tmp/bad.json")" "$before" "invalid host file left unchanged"

# --- Case 4: missing forced file fails loud ---
assert_exit_code 1 "missing forced file fails" bash "$APPLY" "$tmp/host.json" "$tmp/nope.json"

# --- Case 6: permissions.allow is UNIONED (host additions preserved, forced
# entries guaranteed), while deny/ask are still replaced. ---
cat > "$tmp/host6.json" <<'JSON'
{ "permissions": { "allow": ["Bash(host-only:*)", "Bash(gss status:*)"], "ask": ["Bash(gss push:*)"] } }
JSON
cat > "$tmp/forced6.json" <<'JSON'
{ "permissions": { "allow": ["Bash(gss push:*)", "Bash(gss status:*)"], "ask": ["Bash(sudo:*)"] } }
JSON
bash "$APPLY" "$tmp/host6.json" "$tmp/forced6.json"
assert_eq "$(jq -r '.permissions.allow | any(. == "Bash(host-only:*)")' "$tmp/host6.json")" "true" "host-only allow entry preserved (union)"
assert_eq "$(jq -r '.permissions.allow | any(. == "Bash(gss push:*)")' "$tmp/host6.json")" "true" "forced allow entry added (union)"
assert_eq "$(jq -r '[.permissions.allow[] | select(. == "Bash(gss status:*)")] | length' "$tmp/host6.json")" "1" "duplicate allow entry deduped"
assert_eq "$(jq -r '.permissions.ask' "$tmp/host6.json")" "$(printf '[\n  "Bash(sudo:*)"\n]')" "ask is replaced (not unioned)"

# --- Case 5: doc keys (leading underscore) in the forced file are NOT merged
# into the live settings (they are self-documentation, not config). ---
cat > "$tmp/host5.json" <<'JSON'
{ "theme": "x" }
JSON
cat > "$tmp/forced5.json" <<'JSON'
{ "_comment": "explains why hooks use well-known $HOME paths", "statusLine": { "type": "command", "command": "bash ~/.claude/statusline-command.sh" } }
JSON
bash "$APPLY" "$tmp/host5.json" "$tmp/forced5.json"
assert_eq "$(jq -r 'has("_comment")' "$tmp/host5.json")" "false" "forced _comment doc key not merged into live settings"
assert_eq "$(jq -r '.statusLine.command' "$tmp/host5.json")" "bash ~/.claude/statusline-command.sh" "real forced fields still merged when doc key present"

_test_report
