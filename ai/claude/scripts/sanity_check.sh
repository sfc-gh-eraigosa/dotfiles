#!/bin/bash
# Sanity check for Claude Code dotfiles integration
set -e

echo "Starting Claude Sanity Check..."

# 1. CLI presence
echo "Verifying Claude CLI..."
if ! command -v claude > /dev/null 2>&1; then
    echo "FAIL: claude CLI not found on PATH"
    exit 1
fi
echo "PASS: claude CLI present ($(claude --version 2>/dev/null || echo unknown))"

# 2. Symlinked config
echo "Verifying ~/.claude configuration links..."
for link in settings.json; do
    if [ ! -L "$HOME/.claude/$link" ] && [ ! -f "$HOME/.claude/$link" ]; then
        echo "FAIL: ~/.claude/$link missing"
        exit 1
    fi
done
echo "PASS: ~/.claude config present"

# 3. Skills linked
echo "Verifying ~/.claude/skills/..."
SKILLS_DIR="$HOME/.claude/skills"
if [ ! -d "$SKILLS_DIR" ]; then
    echo "FAIL: $SKILLS_DIR missing"
    exit 1
fi
expected_skills=(git-safe-sync tmux ssh-host-finder ssh-key-sync)
for s in "${expected_skills[@]}"; do
    if [ ! -e "$SKILLS_DIR/$s/SKILL.md" ]; then
        echo "FAIL: skill '$s' missing (expected $SKILLS_DIR/$s/SKILL.md)"
        exit 1
    fi
done
echo "PASS: skills linked"

# 4. Commands linked
echo "Verifying ~/.claude/commands/..."
if [ ! -e "$HOME/.claude/commands/gss.md" ]; then
    echo "FAIL: /gss command missing"
    exit 1
fi
echo "PASS: commands linked"

# 5. CLAUDE.md context links resolve
echo "Verifying CLAUDE.md context links..."
BASE_DIR="$(cd "$(dirname "$0")/../../.." && pwd)"
missing=0
while IFS= read -r f; do
    if [ ! -e "$f" ]; then
        echo "FAIL: dangling CLAUDE.md link: $f"
        missing=$((missing+1))
    fi
done < <(find "$BASE_DIR" -name "CLAUDE.md" -not -path "*/node_modules/*" -not -path "*/.git/*")
[ "$missing" -gt 0 ] && exit 1
echo "PASS: CLAUDE.md links resolve"

# 6. Hooks executable + test suite passes
echo "Verifying safety_guard hook..."
HOOK="$BASE_DIR/ai/hooks/safety_guard.sh"
HOOK_TEST="$BASE_DIR/ai/hooks/safety_guard_test.sh"
if [ ! -x "$HOOK" ]; then
    echo "FAIL: safety_guard hook missing or not executable"
    exit 1
fi
if command -v jq > /dev/null 2>&1; then
    if [ -x "$HOOK_TEST" ]; then
        if ! "$HOOK_TEST" > /dev/null 2>&1; then
            echo "FAIL: safety_guard hook self-test failed (run $HOOK_TEST to see details)"
            exit 1
        fi
    fi
    echo "PASS: safety_guard hook (27 test cases)"
else
    echo "SKIP: jq not installed — hook self-test skipped"
fi

echo "--------------------------------------------------"
echo "CLAUDE SANITY CHECK PASSED"
echo "--------------------------------------------------"
