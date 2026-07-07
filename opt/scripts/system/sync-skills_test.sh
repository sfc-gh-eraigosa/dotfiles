#!/usr/bin/env bash
# Test driver for opt/scripts/system/sync-skills.sh
#
# sync-skills.sh has no --dry-run flag, so we isolate side effects by
# pointing HOME at a tempdir for the duration of the run. The script
# only writes under "$HOME/.gemini/config/skills" and "$HOME/.claude/skills",
# so swapping HOME is a clean sandbox.
#
# Run: bash opt/scripts/system/sync-skills_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/sync-skills.sh"

# === 1. Syntax check ===
assert_exit_code 0 "sync-skills.sh parses with bash -n" \
    bash -n "$SCRIPT"

# === 2. --help exits cleanly without touching HOME ===
assert_exit_code 0 "sync-skills.sh --help exits 0" \
    bash "$SCRIPT" --help

# === 3. Run inside a sandboxed HOME ===
TMP_HOME="$(mktemp -d)"
trap 'rm -rf "$TMP_HOME"' EXIT

# Run the script with sandboxed HOME. Suppress output, but capture exit.
if HOME="$TMP_HOME" bash "$SCRIPT" >/dev/null 2>&1; then
    echo "PASS: sync-skills.sh runs cleanly with sandboxed HOME"
    PASS=$((PASS + 1))
else
    echo "FAIL: sync-skills.sh failed under sandboxed HOME"
    FAIL=$((FAIL + 1))
fi

# === 4. Sandbox isolation — outputs land under TMP_HOME, not real ~ ===
assert_file_exists "${TMP_HOME}/.claude/skills" \
    "skills directory created under sandboxed HOME (.claude)"
assert_file_exists "${TMP_HOME}/.gemini/config/skills" \
    "skills directory created under sandboxed HOME (.gemini/config)"

# === 5. Skills are SYMLINKS, not copies ===
# Pick the first symlinked entry under .claude/skills and verify it's a
# symlink pointing back to the repo (not a copy of the directory).
FIRST_LINK=$(find "${TMP_HOME}/.claude/skills" -maxdepth 1 -type l | head -1)
if [ -n "$FIRST_LINK" ] && [ -L "$FIRST_LINK" ]; then
    echo "PASS: at least one skill is a symlink (not a copy): $(basename "$FIRST_LINK")"
    PASS=$((PASS + 1))
    LINK_TARGET=$(readlink "$FIRST_LINK")
    case "$LINK_TARGET" in
        "$REPO_ROOT"/*)
            echo "PASS: skill symlink points back into the dotfiles repo"
            PASS=$((PASS + 1))
            ;;
        *)
            echo "FAIL: skill symlink points outside the repo: $LINK_TARGET"
            FAIL=$((FAIL + 1))
            ;;
    esac
else
    echo "FAIL: no skill symlinks found in ${TMP_HOME}/.claude/skills"
    FAIL=$((FAIL + 1))
fi

# === 6. Real ~/.claude was NOT touched ===
# If the sandbox leaked, the real user's HOME would have new symlinks at
# the same timestamps. We only verify the test's HOME swap mechanism by
# asserting the script's destination references go through "$HOME".
assert_grep "sync-skills.sh uses \$HOME (not hardcoded /home/wenlock)" \
    'SKILLS_DESTS=\("\$\{HOME\}/\.gemini/config/skills"[[:space:]]+"\$\{HOME\}/\.claude/skills"' \
    "$SCRIPT"

_test_report
