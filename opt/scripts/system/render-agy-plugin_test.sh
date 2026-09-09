#!/usr/bin/env bash
# Test driver for render-agy-plugin.sh — renders the repo's Claude slash
# commands (ai/claude/commands/*.md) and scope:account memories
# (ai/claude/memory/*.md) into a local Antigravity plugin
# (plugin.json + commands/*.toml + rules/AGENTS.md). agy-parity units 5–6.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
# shellcheck source=/dev/null
. "$REPO_ROOT/ai/_test_helpers.sh"

R="$SCRIPT_DIR/render-agy-plugin.sh"

if ! command -v jq >/dev/null 2>&1; then
    echo "SKIP: jq not installed — render-agy-plugin tests skipped"
    exit 0
fi

OUT="$(mktemp -d)"
trap 'rm -rf "$OUT"' EXIT

# === Renders against the real repo sources ===
assert_file_exists "$R" "render-agy-plugin.sh exists"
assert_exit_code 0 "renderer runs against the repo" bash "$R" "$REPO_ROOT" "$OUT/dotfiles"
assert_file_exists "$OUT/dotfiles/plugin.json" "plugin.json written"
assert_in_subshell "plugin.json is valid JSON" "jq -e . '$OUT/dotfiles/plugin.json' >/dev/null"
assert_eq "$(jq -r .name "$OUT/dotfiles/plugin.json")" "dotfiles" "plugin name is dotfiles"

# === Commands: one TOML per Claude command, Claude-only syntax rewritten ===
n_md=$(find "$REPO_ROOT/ai/claude/commands" -maxdepth 1 -name '*.md' | wc -l | tr -d ' ')
n_toml=$(find "$OUT/dotfiles/commands" -maxdepth 1 -name '*.toml' | wc -l | tr -d ' ')
assert_eq "$n_toml" "$n_md" "one TOML per command ($n_md)"
assert_grep "sync.toml has a description" '^description = "' "$OUT/dotfiles/commands/sync.toml"
assert_grep "sync.toml description came from the frontmatter" '^description = "Stage, commit, and push' "$OUT/dotfiles/commands/sync.toml"
assert_grep "sync.toml has a prompt block" '^prompt = """' "$OUT/dotfiles/commands/sync.toml"
# (sync.md's body has a legitimate markdown `---` rule, so the leak check is on a
# frontmatter KEY, not the fence.)
assert_grep_negative "no frontmatter key leaks into the prompt" '^allowed-tools:' "$OUT/dotfiles/commands/sync.toml"
assert_grep_negative "no frontmatter description line leaks into the prompt" '^description: ' "$OUT/dotfiles/commands/sync.toml"
# shellcheck disable=SC2016  # literal patterns; no expansion wanted
assert_grep_negative "no literal \$ARGUMENTS (agy does not substitute it)" '\$ARGUMENTS' "$OUT/dotfiles/commands/sync.toml"
assert_grep_negative "no Claude !-backtick injection lines" '^!`' "$OUT/dotfiles/commands/sync.toml"
# shellcheck disable=SC2016  # literal backticks in the expected text
assert_grep "injection rewritten as an instruction" 'Run `git status --short --branch` first' "$OUT/dotfiles/commands/sync.toml"
assert_grep "arguments rewritten as prose" 'the arguments the user passed to this command' "$OUT/dotfiles/commands/sync.toml"
if command -v python3 >/dev/null 2>&1 && python3 -c 'import tomllib' 2>/dev/null; then
    assert_in_subshell "every TOML parses with description + prompt" \
        "for f in '$OUT'/dotfiles/commands/*.toml; do python3 -c 'import sys,tomllib; d=tomllib.load(open(sys.argv[1],\"rb\")); assert d[\"description\"] and d[\"prompt\"]' \"\$f\" || exit 1; done"
fi

# === Rules: account memories become an always-on rules file ===
assert_file_exists "$OUT/dotfiles/rules/AGENTS.md" "rules/AGENTS.md written"
assert_grep "rules carry an account memory section" '^## gss land flow' "$OUT/dotfiles/rules/AGENTS.md"
assert_grep_negative "rules have no frontmatter fences" '^---$' "$OUT/dotfiles/rules/AGENTS.md"
assert_grep_negative "MEMORY.md index is not inlined as a section" '^## MEMORY$' "$OUT/dotfiles/rules/AGENTS.md"

# === Idempotent + stale cleanup ===
touch "$OUT/dotfiles/commands/stale.toml"
bash "$R" "$REPO_ROOT" "$OUT/dotfiles" >/dev/null
assert_in_subshell "stale TOML removed on re-render" "[ ! -e '$OUT/dotfiles/commands/stale.toml' ]"
assert_eq "$(find "$OUT/dotfiles/commands" -maxdepth 1 -name '*.toml' | wc -l | tr -d ' ')" "$n_md" "re-render keeps exactly one TOML per command"

# === Usage errors ===
assert_exit_code 1 "missing args -> exit 1" bash "$R"
assert_exit_code 1 "missing repo commands dir -> exit 1" bash "$R" "$OUT/nonexistent-repo" "$OUT/x"

_test_report
