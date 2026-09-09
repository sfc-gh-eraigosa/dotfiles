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

# BASE_DIR is exported (so child scripts can reuse it). Accept both the
# combined `export BASE_DIR=...` and the SC2155-safe split form
# (`BASE_DIR=...` then `export BASE_DIR` on its own line).
assert_grep "BASE_DIR is exported" \
    '^export[[:space:]]+BASE_DIR([[:space:]]|=|$)' "$INSTALL"

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

# Existing-file backup pattern: when ~/.claude settings.json exists as a
# real file, the installer backs it up before applying the forced-field
# merge, so re-running is safe on a machine with pre-existing config.
# Settings provisioning lives in the per-tool skill installers (install.sh
# delegates to them), so assert the guard there. Antigravity needs no such
# guard: agy owns its settings file and we only render hooks.json (wholly
# repo-owned) — but its aliases link must keep the backup pattern.
CLAUDE_SKILLS="${SELF_DIR}/opt/scripts/system/install_claude_skills.sh"
ANTIGRAVITY_SKILLS="${SELF_DIR}/opt/scripts/system/install_antigravity_skills.sh"
assert_grep "claude settings backup-on-conflict guard present" \
    'settings\.json\.bak' "$CLAUDE_SKILLS"
assert_grep "antigravity aliases backup-on-conflict guard present" \
    'aliases\.sh\.bak' "$ANTIGRAVITY_SKILLS"

# === Install stamp (fleet F1) must be invoked, and be the last action ===
assert_grep "install.sh invokes the install stamp" \
    'opt/scripts/system/install-stamp\.sh' "$INSTALL"
# The stamp must be the final executable block: anything after it could fail
# AFTER a success marker was written.
STAMP_LINE="$(grep -n 'install-stamp\.sh' "$INSTALL" | tail -1 | cut -d: -f1)"
TAIL_CODE="$(tail -n "+$((STAMP_LINE + 1))" "$INSTALL" | sed 's/#.*//' | tr -d '[:space:]')"
assert_eq "$TAIL_CODE" "fi" \
    "nothing runs after the stamp (only its closing 'fi')"

# === Self-sufficient PATH (fleet update regression) ===
# install.sh WRITES into ~/opt/bin (yq, sops, kubectl, every sdk/ binary) and
# ~/.local/bin (pipx CLIs) and then immediately CONSUMES those tools. Those dirs
# are put on PATH only by ~/.profile — a LOGIN-shell file. `fleet update <host>`
# runs install.sh over `ssh -t host "... ./install.sh"`, a NON-login shell, so a
# real run produced "yq not resolvable after install", "sync-plugins: 'yq' not
# found" and "install_ai_teams: yq is required" on a host where yq had just been
# installed correctly. install.sh must therefore fix its own PATH.
#
# The block is EXECUTED here, extracted verbatim from the shipped install.sh
# rather than re-implemented, so deleting or breaking it fails these cases.
PATH_BLOCK="$(awk '/^# --- Self-sufficient PATH/{f=1} f{print} f&&/^export PATH$/{exit}' "$INSTALL")"
assert_grep "install.sh carries the self-sufficient PATH block" \
    '^# --- Self-sufficient PATH' "$INSTALL"

# run_path_block <initial PATH> -> resulting PATH, with HOME pinned to a fixture.
run_path_block() {
    env HOME=/fixture/home PATH="$1" bash -c "$PATH_BLOCK"'; printf %s "$PATH"'
}

assert_eq "$(run_path_block '/usr/local/bin:/usr/bin:/bin')" \
    "/fixture/home/.local/bin:/fixture/home/opt/bin:/usr/local/bin:/usr/bin:/bin" \
    "non-login PATH gains ~/opt/bin and ~/.local/bin"

# Repeated fleet runs must not accumulate duplicate entries.
CONFIGURED_PATH="/fixture/home/.local/bin:/fixture/home/opt/bin:/usr/bin:/bin"
assert_eq "$(run_path_block "$CONFIGURED_PATH")" "$CONFIGURED_PATH" \
    "already-configured PATH is unchanged (no duplicates)"

assert_eq "$(run_path_block '/fixture/home/opt/bin:/usr/bin')" \
    "/fixture/home/.local/bin:/fixture/home/opt/bin:/usr/bin" \
    "only the missing dir is prepended"

# A neighbouring dir that merely shares a prefix must not read as "present".
assert_eq "$(run_path_block '/fixture/home/opt/bin-extra:/usr/bin')" \
    "/fixture/home/.local/bin:/fixture/home/opt/bin:/fixture/home/opt/bin-extra:/usr/bin" \
    "substring match does not count as present"

# The block must precede the first consumer of an installed tool: the gff
# early-export probes `command -v gff`, and gff lives in ~/opt/bin.
PATH_BLOCK_LINE="$(grep -n '^# --- Self-sufficient PATH' "$INSTALL" | head -1 | cut -d: -f1)"
GFF_PROBE_LINE="$(grep -n 'command -v gff' "$INSTALL" | head -1 | cut -d: -f1)"
assert_eq "$([ -n "$PATH_BLOCK_LINE" ] && [ -n "$GFF_PROBE_LINE" ] && \
    [ "$PATH_BLOCK_LINE" -lt "$GFF_PROBE_LINE" ] && echo before || echo after)" \
    "before" "PATH block runs before the gff early-export probe"

# === 8. Directly-executed helper scripts must carry the exec bit ===
# install.sh invokes some helpers in command position ("${BASE_DIR}/foo.sh")
# rather than via `bash foo.sh`. A 100644 mode on any of those turns into a
# runtime "Permission denied" that the `|| echo WARNING` guards swallow, so the
# component silently never installs (this bit herdr: install_herdr.sh shipped
# non-executable and every install printed "Permission denied ... continuing").
# Sourced (`. path`) and `bash path` call sites are exempt by construction:
# the pattern only matches a quoted path at the start of a command.
DIRECT_SCRIPTS="$(grep -oE '^[[:space:]]*"\$\{BASE_DIR\}/[^"]+\.sh"' "$INSTALL" \
    | sed -e 's#^[[:space:]]*"\${BASE_DIR}/##' -e 's#"$##' | sort -u)"
assert_eq "$([ -n "$DIRECT_SCRIPTS" ] && echo found || echo none)" "found" \
    "install.sh has directly-invoked helper scripts to check"

NON_EXEC=""
for rel in $DIRECT_SCRIPTS; do
    [ -f "${SELF_DIR}/${rel}" ] || continue
    [ -x "${SELF_DIR}/${rel}" ] || NON_EXEC="${NON_EXEC}${rel} "
done
assert_eq "${NON_EXEC% }" "" \
    "every directly-invoked helper in install.sh is executable"

_test_report
