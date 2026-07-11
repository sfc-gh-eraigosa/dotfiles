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
trap 'rm -rf "$A" "$B"' EXIT

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

# --- A: idempotency (re-run changes nothing structurally) ---
run_install "$A"
assert_in_subshell "re-run keeps hooks.json valid" "jq -e . '$A/.gemini/config/hooks.json' >/dev/null"

# --- B: legacy Gemini CLI artifacts are cleaned up ---
mkdir -p "$B/.gemini/hooks" "$B/.gemini/policies" "$B/.gemini/commands" \
         "$B/.config/gemini" "$B/.agents/skills"
touch "$B/.gemini/hooks/privacy_guard.sh"
ln -s "$REPO_ROOT/ai/hooks/safety_guard.sh" "$B/.gemini/policies/safety.toml"   # repo-pointing link
ln -s "$REPO_ROOT/ai/antigravity/aliases.sh" "$B/.config/gemini/aliases.sh"
ln -s "$REPO_ROOT/ai/skills/sync-skills" "$B/.agents/skills/sync-skills"        # repo-pointing skill link
echo '{ "ui": { "theme": "myTheme" } }' > "$B/.gemini/settings.json"             # host-owned, must survive
run_install "$B"
assert_in_subshell "legacy ~/.gemini/hooks removed" "[ ! -e '$B/.gemini/hooks' ]"
assert_in_subshell "legacy repo-pointing policy link removed" "[ ! -e '$B/.gemini/policies/safety.toml' ]"
assert_in_subshell "legacy ~/.config/gemini aliases link removed" "[ ! -e '$B/.config/gemini/aliases.sh' ]"
assert_in_subshell "legacy ~/.agents/skills repo link removed" "[ ! -e '$B/.agents/skills/sync-skills' ]"
assert_eq "$(jq -r '.ui.theme' "$B/.gemini/settings.json")" "myTheme" "host-owned legacy settings.json left alone"

_test_report
