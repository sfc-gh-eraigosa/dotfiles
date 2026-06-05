#!/usr/bin/env bash
# Test driver for install.sh — source-level guards only.
#
# install.sh mutates the host (apt installs, chsh, ~/.* symlinks). We
# deliberately do NOT execute it from this driver. Instead we:
#   1. syntax-check with bash -n
#   2. grep for forbidden patterns CLAUDE.md calls out
#   3. assert structural shape of an idempotent installer
#
# Run: bash install_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=ai/_test_helpers.sh
. "${SELF_DIR}/ai/_test_helpers.sh"

INSTALL="${SELF_DIR}/install.sh"

# === 1. Syntax check ===
assert_exit_code 0 "install.sh parses with bash -n" bash -n "$INSTALL"

# === 2. Source-level guards (per CLAUDE.md) ===
# Never `git add -A` / `git add .` inside the installer.
assert_grep_negative "no 'git add -A' in install.sh" \
    'git[[:space:]]+add[[:space:]]+-A([[:space:]]|$)' "$INSTALL"
assert_grep_negative "no 'git add .' in install.sh" \
    'git[[:space:]]+add[[:space:]]+\.([[:space:]]|$)' "$INSTALL"

# No hardcoded home paths. CLAUDE.md mandates $HOME / ~ / $DOTFILES_DIR.
assert_grep_negative "no hardcoded /home/wenlock in install.sh" \
    '/home/wenlock' "$INSTALL"
assert_grep_negative "no hardcoded /Users/eraigosa in install.sh" \
    '/Users/eraigosa' "$INSTALL"

# Every top-level `cd` must use $BASE_DIR or $HOME (interpolated or as
# the variable itself). Subshell `(cd /tmp/zsh ...)` is fine for build
# dirs we created ourselves earlier in the same script, so the check is
# scoped to NON-/tmp absolute cd targets.
# Pattern: a literal `cd /` that isn't $HOME / $BASE_DIR / /tmp.
# We allow `cd "${HOME}/..."`, `cd "${BASE_DIR}/..."`, `cd /tmp/...`.
BAD_CD=$(grep -nE '(^|[[:space:](])cd[[:space:]]+/' "$INSTALL" \
    | grep -vE 'cd[[:space:]]+/tmp' \
    | grep -vE 'cd[[:space:]]+"?\$\{?(HOME|BASE_DIR)' \
    | grep -vE '"\$\{?HOME' \
    || true)
assert_eq "$BAD_CD" "" "every absolute cd uses \$HOME / \$BASE_DIR / /tmp"

# BASE_DIR is exported (so child scripts can reuse it).
assert_grep "BASE_DIR is exported" \
    '^export[[:space:]]+BASE_DIR=' "$INSTALL"

# === 3. Idempotency shape ===
# An installer that re-runs cleanly must guard symlink creation: the
# canonical pattern checks `-L` AND `readlink == expected target`
# before recreating. install.sh uses this for ~/opt (lines around the
# `Ensure ~/opt is a symlink to the repo's opt directory` block).
assert_grep "symlink idempotency guard present" \
    '\[[[:space:]]+-L[[:space:]]+"\$\{HOME\}/opt"[[:space:]]+\][[:space:]]+&&[[:space:]]+\[[[:space:]]+"\$\(readlink' \
    "$INSTALL"

# `ln -sf` should be used (overwrite-safe), never `ln -s` alone (which
# fails on re-run when the link already exists). One miss here means a
# second `install.sh` run errors out instead of being idempotent.
NAKED_LN_S=$(grep -nE '(^|[[:space:]])ln[[:space:]]+-s[[:space:]]+[^f]' "$INSTALL" || true)
assert_eq "$NAKED_LN_S" "" "no naked 'ln -s' (need -sf for idempotency)"

# Existing-file backup pattern: when ~/.claude or ~/.gemini settings.json
# exists as a real file, the installer backs it up before applying the
# forced-field merge, so re-running is safe on a machine with pre-existing
# config. Settings provisioning now lives in the per-tool skill installers
# (install.sh delegates to them), so assert the guard there.
CLAUDE_SKILLS="${SELF_DIR}/opt/scripts/system/install_claude_skills.sh"
GEMINI_SKILLS="${SELF_DIR}/opt/scripts/system/install_gemini_skills.sh"
assert_grep "claude settings backup-on-conflict guard present" \
    'settings\.json\.bak' "$CLAUDE_SKILLS"
assert_grep "gemini settings backup-on-conflict guard present" \
    'settings\.json\.bak' "$GEMINI_SKILLS"

_test_report
