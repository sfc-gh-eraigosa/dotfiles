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
# shellcheck disable=SC2043 # single config link today; loop kept for future entries
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

# 6. Hooks: the LIVE configured wiring resolves + repo self-tests pass
echo "Verifying hooks..."
HOOK="$BASE_DIR/ai/hooks/safety_guard.sh"
HOOK_TEST="$BASE_DIR/ai/hooks/safety_guard_test.sh"
PRIV_TEST="$BASE_DIR/ai/hooks/privacy_guard_test.sh"
VALIDATE="$BASE_DIR/ai/claude/scripts/validate_hooks.sh"
if [ ! -x "$HOOK" ]; then
    echo "FAIL: safety_guard hook missing or not executable"
    exit 1
fi
# D3: validate the wiring CONFIGURED in the live ~/.claude/settings.json and
# exercise it through the configured command. This is the check that catches
# #111 — a settings.json pointing at a moved/dead hook path that a stat of the
# repo copy (the old check below) would miss while both guards are silently off.
if [ -x "$VALIDATE" ]; then
    if ! "$VALIDATE" "$HOME/.claude/settings.json"; then
        echo "FAIL: configured hook wiring in ~/.claude/settings.json is broken (see above)"
        exit 1
    fi
fi
if command -v jq > /dev/null 2>&1; then
    for t in "$HOOK_TEST" "$PRIV_TEST"; do
        [ -x "$t" ] || continue
        if ! "$t" > /dev/null 2>&1; then
            echo "FAIL: hook self-test failed (run $t to see details)"
            exit 1
        fi
    done
    echo "PASS: hooks wired (validated via the configured command) + self-tests pass"
else
    echo "SKIP: jq not installed — hook self-tests skipped"
fi

echo "--------------------------------------------------"
echo "CLAUDE SANITY CHECK PASSED"
echo "--------------------------------------------------"
