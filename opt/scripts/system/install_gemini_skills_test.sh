#!/usr/bin/env bash
# Test driver for install_gemini_skills.sh — verifies hooks are copied into
# ~/.gemini/hooks/ and the forced-field settings merge runs, dropping
# $GEMINI_PROJECT_DIR for a fixed well-known $HOME path (design doc D4).
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
# shellcheck source=/dev/null
. "$REPO_ROOT/ai/_test_helpers.sh"

INSTALLER="$SCRIPT_DIR/install_gemini_skills.sh"

if ! command -v jq >/dev/null 2>&1; then
    echo "SKIP: jq not installed — install_gemini_skills tests skipped"
    exit 0
fi

A="$(mktemp -d)"
B="$(mktemp -d)"
trap 'rm -rf "$A" "$B"' EXIT

run_install() { HOME="$1" XDG_CONFIG_HOME="$1/.config" bash "$INSTALLER" >/dev/null 2>&1; }

# --- A: fresh host ---
run_install "$A"
assert_file_exists "$A/.gemini/hooks/privacy_guard.sh" "privacy_guard copied into ~/.gemini/hooks"
assert_file_exists "$A/.gemini/hooks/safety_guard.sh" "safety_guard copied into ~/.gemini/hooks"
assert_in_subshell "gemini hooks are real files, not symlinks" \
    "[ -f '$A/.gemini/hooks/privacy_guard.sh' ] && [ ! -L '$A/.gemini/hooks/privacy_guard.sh' ]"
assert_in_subshell "gemini settings.json is a real file, not a symlink" \
    "[ -f '$A/.gemini/settings.json' ] && [ ! -L '$A/.gemini/settings.json' ]"
assert_eq "$(jq -r '[.hooks.BeforeTool[].hooks[].command] | map(select(test("privacy_guard"))) | first' "$A/.gemini/settings.json")" \
    '$HOME/.gemini/hooks/privacy_guard.sh' "privacy_guard wired to well-known \$HOME path"
assert_eq "$(jq -r '[.hooks.BeforeTool[].hooks[].command] | map(select(test("safety_guard"))) | length' "$A/.gemini/settings.json")" \
    "1" "safety_guard wired for Gemini"
assert_grep_negative "no \$GEMINI_PROJECT_DIR in settings" "GEMINI_PROJECT_DIR" "$A/.gemini/settings.json"

# --- B: existing host with a custom field + stale GEMINI_PROJECT_DIR hook ---
mkdir -p "$B/.gemini"
cat > "$B/.gemini/settings.json" <<'JSON'
{
  "ui": { "theme": "myGeminiTheme" },
  "hooks": { "BeforeTool": [ { "matcher": "write_file", "hooks": [ { "name": "privacy-guard", "type": "command", "command": "$GEMINI_PROJECT_DIR/ai/hooks/privacy_guard.sh" } ] } ] }
}
JSON
run_install "$B"
assert_eq "$(jq -r '.ui.theme' "$B/.gemini/settings.json")" "myGeminiTheme" "host ui.theme preserved"
assert_grep_negative "stale \$GEMINI_PROJECT_DIR reconciled away" "GEMINI_PROJECT_DIR" "$B/.gemini/settings.json"

_test_report
