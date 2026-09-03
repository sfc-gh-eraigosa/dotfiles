#!/usr/bin/env bash
# Test driver for install_antigravity_skills.sh — verifies guard scripts (and
# the antigravity_adapter dialect bridge) are copied into
# ~/.gemini/config/hooks/, hooks.json is rendered from the repo template with
# __HOME__ substituted (agy does not expand env vars in hook commands), the
# aliases symlink lands in ~/.config/antigravity/, and legacy Gemini CLI
# artifacts are cleaned up.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
# shellcheck source=/dev/null
. "$REPO_ROOT/ai/_test_helpers.sh"

INSTALLER="$SCRIPT_DIR/install_antigravity_skills.sh"

if ! command -v jq >/dev/null 2>&1; then
    echo "SKIP: jq not installed — install_antigravity_skills tests skipped"
    exit 0
fi

A="$(mktemp -d)"
B="$(mktemp -d)"
C="$(mktemp -d)"
D="$(mktemp -d)"
trap 'rm -rf "$A" "$B" "$C" "$D"' EXIT

run_install() { HOME="$1" XDG_CONFIG_HOME="$1/.config" bash "$INSTALLER" >/dev/null 2>&1; }

# --- A: fresh host ---
run_install "$A"
assert_file_exists "$A/.gemini/config/hooks/privacy_guard.sh" "privacy_guard copied into ~/.gemini/config/hooks"
assert_file_exists "$A/.gemini/config/hooks/safety_guard.sh" "safety_guard copied into ~/.gemini/config/hooks"
assert_file_exists "$A/.gemini/config/hooks/antigravity_adapter.sh" "antigravity_adapter copied into ~/.gemini/config/hooks"
assert_file_exists "$A/.gemini/config/hooks/strip_heredocs.awk" "strip_heredocs.awk sibling copied"
assert_in_subshell "hooks are real files, not symlinks" \
    "[ -f '$A/.gemini/config/hooks/privacy_guard.sh' ] && [ ! -L '$A/.gemini/config/hooks/privacy_guard.sh' ]"
assert_file_exists "$A/.gemini/config/hooks.json" "hooks.json rendered into ~/.gemini/config"
assert_in_subshell "hooks.json is valid JSON" "jq -e . '$A/.gemini/config/hooks.json' >/dev/null"
assert_grep_negative "no unsubstituted __HOME__ in hooks.json" "__HOME__" "$A/.gemini/config/hooks.json"
assert_eq "$(jq -r '[.[] | objects | .PreToolUse[]?.hooks[]?.command] | map(select(test("antigravity_adapter.sh safety_guard.sh privacy_guard.sh"))) | length' "$A/.gemini/config/hooks.json")" \
    "1" "single hook entry runs both guards through the adapter"
assert_in_subshell "hook commands use the rendered real HOME" \
    "jq -r '[.[] | objects | .PreToolUse[]?.hooks[]?.command] | first' '$A/.gemini/config/hooks.json' | grep -qF '$A/.gemini/config/hooks/'"
assert_in_subshell "aliases.sh copied (not symlinked) into ~/.config/antigravity" \
    "[ -f '$A/.config/antigravity/aliases.sh' ] && [ ! -L '$A/.config/antigravity/aliases.sh' ]"
assert_file_exists "$A/.gemini/config/statusline-command.sh" "statusline shim copied"
assert_file_exists "$A/.gemini/antigravity-cli/settings.json" "agy settings.json created"
assert_eq "$(jq -r '.statusLine.command' "$A/.gemini/antigravity-cli/settings.json")" \
    "bash ~/.gemini/config/statusline-command.sh" "statusLine command configured in settings.json"
assert_eq "$(jq -r '.statusLine.enabled' "$A/.gemini/antigravity-cli/settings.json")" \
    "true" "statusLine enabled in settings.json"

# --- A: agy-parity F2 — first run seeds the tracked settings template ---
S="$A/.gemini/antigravity-cli/settings.json"
assert_eq "$(jq -r '.toolPermission' "$S")" "request-review" "template: toolPermission seeded"
assert_eq "$(jq -r '.editorMode' "$S")" "vim" "template: editorMode vim"
assert_eq "$(jq -r '.allowNonWorkspaceAccess' "$S")" "true" "template: allowNonWorkspaceAccess true"
assert_eq "$(jq -r '.notifications' "$S")" "true" "template: notifications on"
assert_eq "$(jq -r '.enableTerminalSandbox' "$S")" "false" "template: sandbox off"
assert_in_subshell "template: allow list carries command(gss status)" \
    "jq -e '.permissions.allow | index(\"command(gss status)\")' '$S' >/dev/null"
assert_in_subshell "template: no _comment key survives the merge" \
    "! jq -e '._comment' '$S' >/dev/null 2>&1"

# --- A: agy-parity F3 — forced deny/ask policy applied on a fresh host ---
assert_in_subshell "forced: deny carries command(rm -rf /)" \
    "jq -e '.permissions.deny | index(\"command(rm -rf /)\")' '$S' >/dev/null"
assert_in_subshell "forced: ask carries command(git push --force)" \
    "jq -e '.permissions.ask | index(\"command(git push --force)\")' '$S' >/dev/null"
assert_in_subshell "forced: ask carries command(sudo)" \
    "jq -e '.permissions.ask | index(\"command(sudo)\")' '$S' >/dev/null"

# --- A: idempotency (re-run changes nothing structurally) ---
run_install "$A"
assert_in_subshell "re-run keeps hooks.json valid" "jq -e . '$A/.gemini/config/hooks.json' >/dev/null"

# --- B: legacy Gemini CLI artifacts are cleaned up ---
mkdir -p "$B/.gemini/hooks" "$B/.gemini/policies" "$B/.gemini/commands" \
         "$B/.config/gemini" "$B/.agents/skills" "$B/.gemini/antigravity-cli"
touch "$B/.gemini/hooks/privacy_guard.sh"
ln -s "$REPO_ROOT/ai/hooks/safety_guard.sh" "$B/.gemini/policies/safety.toml"   # repo-pointing link
ln -s "$REPO_ROOT/ai/antigravity/aliases.sh" "$B/.config/gemini/aliases.sh"
ln -s "$REPO_ROOT/ai/skills/sync-skills" "$B/.agents/skills/sync-skills"        # repo-pointing skill link
echo '{ "ui": { "theme": "myTheme" } }' > "$B/.gemini/settings.json"             # host-owned, must survive
echo '{ "colorScheme": "light", "permissions": { "allow": ["command(mytool)"], "deny": ["command(host-only)"] } }' \
    > "$B/.gemini/antigravity-cli/settings.json"    # pre-existing keys: colorScheme + host allow preserved, host deny replaced
run_install "$B"
assert_in_subshell "legacy ~/.gemini/hooks removed" "[ ! -e '$B/.gemini/hooks' ]"
assert_in_subshell "legacy repo-pointing policy link removed" "[ ! -e '$B/.gemini/policies/safety.toml' ]"
assert_in_subshell "legacy ~/.config/gemini aliases link removed" "[ ! -e '$B/.config/gemini/aliases.sh' ]"
assert_in_subshell "legacy ~/.agents/skills repo link removed" "[ ! -e '$B/.agents/skills/sync-skills' ]"
assert_eq "$(jq -r '.ui.theme' "$B/.gemini/settings.json")" "myTheme" "host-owned legacy settings.json left alone"
assert_file_exists "$B/.gemini/antigravity-cli/settings.json.bak" "original agy settings backed up"
assert_eq "$(jq -r '.colorScheme' "$B/.gemini/antigravity-cli/settings.json")" "light" "existing settings keys preserved"
assert_eq "$(jq -r '.statusLine.enabled' "$B/.gemini/antigravity-cli/settings.json")" "true" "statusLine configured when settings.json pre-exists"
assert_in_subshell "existing host: template NOT applied (no toolPermission)" \
    "! jq -e '.toolPermission' '$B/.gemini/antigravity-cli/settings.json' >/dev/null 2>&1"
SB="$B/.gemini/antigravity-cli/settings.json"
assert_in_subshell "existing host: own allow entry survives the union" \
    "jq -e '.permissions.allow | index(\"command(mytool)\")' '$SB' >/dev/null"
assert_in_subshell "existing host: forced allow entry added by the union" \
    "jq -e '.permissions.allow | index(\"command(gss push)\")' '$SB' >/dev/null"
assert_in_subshell "existing host: host deny entry REPLACED (policy is immutable)" \
    "! jq -e '.permissions.deny | index(\"command(host-only)\")' '$SB' >/dev/null 2>&1"
assert_in_subshell "existing host: forced deny carries command(mkfs)" \
    "jq -e '.permissions.deny | index(\"command(mkfs)\")' '$SB' >/dev/null"

# --- C: agy-parity F4 — hooks.json is MERGED: a foreign named hook (herdr's
# state-reporting integration) survives, and a stale repo `guards` entry is
# replaced by the freshly rendered one ---
mkdir -p "$C/.gemini/config"
cat > "$C/.gemini/config/hooks.json" <<'JSON'
{
  "herdr": { "PreToolUse": [ { "matcher": "*", "hooks": [ { "type": "command", "command": "herdr-hook" } ] } ] },
  "guards": { "PreToolUse": [ { "matcher": "stale", "hooks": [ { "type": "command", "command": "stale" } ] } ] }
}
JSON
run_install "$C"
HJ="$C/.gemini/config/hooks.json"
assert_in_subshell "merge: hooks.json still valid JSON" "jq -e . '$HJ' >/dev/null"
assert_eq "$(jq -r 'keys | join(",")' "$HJ")" "guards,herdr" "merge: foreign herdr hook preserved next to guards"
assert_eq "$(jq -r '.herdr.PreToolUse[0].hooks[0].command' "$HJ")" "herdr-hook" "merge: herdr command untouched"
assert_in_subshell "merge: stale guards matcher replaced by the rendered one" \
    "jq -r '.guards.PreToolUse[0].matcher' '$HJ' | grep -q '^run_command|'"
assert_in_subshell "merge: rendered guards command uses the real HOME" \
    "jq -r '.guards.PreToolUse[0].hooks[0].command' '$HJ' | grep -qF '$C/.gemini/config/hooks/antigravity_adapter.sh'"
assert_grep_negative "merge: no unsubstituted __HOME__" "__HOME__" "$HJ"

# --- D: an unparseable pre-existing hooks.json is set aside, not merged into ---
mkdir -p "$D/.gemini/config"
echo 'not json {' > "$D/.gemini/config/hooks.json"
run_install "$D"
assert_in_subshell "invalid hooks.json: recreated as valid JSON with guards" \
    "jq -e '.guards' '$D/.gemini/config/hooks.json' >/dev/null"
assert_file_exists "$D/.gemini/config/hooks.json.invalid" "invalid hooks.json: original set aside as .invalid"

_test_report
