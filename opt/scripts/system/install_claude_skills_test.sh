#!/usr/bin/env bash
# Test driver for install_claude_skills.sh — verifies the copy-not-symlink hook
# install and the forced-field settings merge against a throwaway $HOME.
#
# Guarantees under test (docs/designs/2026-06-02-ai-config-home-provisioning.md):
#   - hooks land in ~/.claude/hooks/ as executable *copies* (not symlinks)
#   - ~/.claude/settings.json is a real host-owned file (not a symlink)
#   - the forced subset (hooks/statusLine/deny/ask) is applied, referencing
#     well-known $HOME paths (no repo-internal path)
#   - undeclared host fields (enabledPlugins, theme) are preserved, and a stale
#     repo-internal hook path is reconciled
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
# shellcheck source=/dev/null
. "$REPO_ROOT/ai/_test_helpers.sh"

INSTALLER="$SCRIPT_DIR/install_claude_skills.sh"

if ! command -v jq >/dev/null 2>&1; then
    echo "SKIP: jq not installed — install_claude_skills tests skipped"
    exit 0
fi

A="$(mktemp -d)"
B="$(mktemp -d)"
trap 'rm -rf "$A" "$B"' EXIT

run_install() { # $1 = HOME dir
    HOME="$1" XDG_CONFIG_HOME="$1/.config" bash "$INSTALLER" >/dev/null 2>&1
}

# --- A: fresh host (no prior settings) ---
run_install "$A"
assert_file_exists "$A/.claude/hooks/safety_guard.sh" "safety_guard copied into ~/.claude/hooks"
assert_file_exists "$A/.claude/hooks/privacy_guard.sh" "privacy_guard copied into ~/.claude/hooks"
assert_in_subshell "hooks are real files, not symlinks" \
    "[ -f '$A/.claude/hooks/safety_guard.sh' ] && [ ! -L '$A/.claude/hooks/safety_guard.sh' ]"
assert_in_subshell "copied hook is executable" "[ -x '$A/.claude/hooks/safety_guard.sh' ]"
assert_in_subshell "settings.json is a real file, not a symlink" \
    "[ -f '$A/.claude/settings.json' ] && [ ! -L '$A/.claude/settings.json' ]"
assert_eq "$(jq -r '.hooks.PreToolUse[0].hooks[0].command' "$A/.claude/settings.json")" \
    '$HOME/.claude/hooks/safety_guard.sh' "safety_guard wired to well-known \$HOME path"
assert_eq "$(jq -r '[.hooks.PreToolUse[].hooks[].command] | map(select(test("privacy_guard"))) | length' "$A/.claude/settings.json")" \
    "1" "privacy_guard wired"
assert_eq "$(jq -r '.permissions.deny | any(. == "Bash(rm -rf /:*)")' "$A/.claude/settings.json")" \
    "true" "forced permissions.deny applied"
assert_grep_negative "no repo-internal path in fresh settings" "git/dotfiles" "$A/.claude/settings.json"

# --- B: existing host with customizations + a stale repo-internal hook path ---
mkdir -p "$B/.claude"
cat > "$B/.claude/settings.json" <<'JSON'
{
  "enabledPlugins": { "custom@x": true },
  "theme": "myCustomTheme",
  "hooks": { "PreToolUse": [ { "matcher": "Bash", "hooks": [ { "type": "command", "command": "$HOME/git/dotfiles/ai/claude/hooks/safety_guard.sh" } ] } ] }
}
JSON
run_install "$B"
assert_eq "$(jq -r '.enabledPlugins."custom@x"' "$B/.claude/settings.json")" "true" "host enabledPlugins preserved"
assert_eq "$(jq -r '.theme' "$B/.claude/settings.json")" "myCustomTheme" "host theme preserved"
assert_eq "$(jq -r '.hooks.PreToolUse[0].hooks[0].command' "$B/.claude/settings.json")" \
    '$HOME/.claude/hooks/safety_guard.sh' "stale repo-internal hook path reconciled"
assert_grep_negative "no repo-internal path after merge" "git/dotfiles" "$B/.claude/settings.json"

_test_report
