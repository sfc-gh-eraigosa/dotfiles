#!/usr/bin/env bash
# Test driver for opt/scripts/system/oh-my-zsh_update.sh
#
# Every case runs the script for real against throwaway git repos built in a
# temp dir: an "upstream" repo standing in for github.com/ohmyzsh/ohmyzsh and
# a "local" clone standing in for ~/git/oh-my-zsh. No network, no $HOME writes.
#
# Run: bash opt/scripts/system/oh-my-zsh_update_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/oh-my-zsh_update.sh"

# === 1. Syntax check ===
assert_exit_code 0 "oh-my-zsh_update.sh parses with bash -n" bash -n "$SCRIPT"

# Deterministic, hermetic git for the fixtures (no user config, no hooks,
# no prompts).
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@example.invalid
export GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@example.invalid
export GIT_TERMINAL_PROMPT=0

# _commit <repo> <file> <content> — add one commit to <repo>.
_commit() {
    mkdir -p "$(dirname "$1/$2")"
    printf '%s\n' "$3" > "$1/$2"
    git -C "$1" add -A >/dev/null
    git -C "$1" commit -q -m "add $2" >/dev/null
}

# _fixture — build upstream + local clone; echoes the temp root.
# Layout: <root>/upstream (origin), <root>/local (the "oh-my-zsh" clone).
_fixture() {
    local root
    root="$(mktemp -d)"
    git init -q -b master "$root/upstream"
    _commit "$root/upstream" oh-my-zsh.sh "# omz v1"
    git clone -q "$root/upstream" "$root/local"
    echo "$root"
}

# _run <clone-dir> <out-file> — run the script, echo its exit code.
_run() {
    set +e
    bash "$SCRIPT" "$1" >"$2" 2>&1
    local rc=$?
    set -e
    echo "$rc"
}

_head() { git -C "$1" rev-parse HEAD; }

# === 2. Behind upstream: fast-forwards to the upstream tip ===
ROOT="$(_fixture)"
_commit "$ROOT/upstream" plugins.txt "docker fix"
_commit "$ROOT/upstream" themes.txt "new theme"
OUT="$ROOT/out.txt"
RC="$(_run "$ROOT/local" "$OUT")"
assert_eq "$RC" "0" "behind upstream: exits 0"
assert_eq "$(_head "$ROOT/local")" "$(_head "$ROOT/upstream")" \
    "behind upstream: HEAD fast-forwarded to the upstream tip"
assert_grep "behind upstream: reports the update" 'oh-my-zsh.*updated' "$OUT"
assert_grep_negative "behind upstream: no WARNING" 'WARNING' "$OUT"

# === 3. Already current: no-op, exit 0 ===
OUT2="$ROOT/out2.txt"
BEFORE="$(_head "$ROOT/local")"
RC="$(_run "$ROOT/local" "$OUT2")"
assert_eq "$RC" "0" "already current: exits 0"
assert_eq "$(_head "$ROOT/local")" "$BEFORE" "already current: HEAD unchanged"
assert_grep "already current: says so" 'up to date' "$OUT2"

# === 4. Local edits that upstream does not touch survive a fast-forward ===
# A dirty tree alone must not block updates, and an edit to a file upstream
# never touches is the user's business — the fast-forward must keep it.
ROOT4="$(_fixture)"
_commit "$ROOT4/upstream" agnoster.zsh-theme "upstream agnoster"
git -C "$ROOT4/local" pull -q origin master
printf '%s\n' "LOCAL agnoster override" > "$ROOT4/local/agnoster.zsh-theme"
_commit "$ROOT4/upstream" lib.txt "unrelated upstream change"
OUT4="$ROOT4/out.txt"
RC="$(_run "$ROOT4/local" "$OUT4")"
assert_eq "$RC" "0" "dirty-but-untouched file: exits 0"
assert_eq "$(_head "$ROOT4/local")" "$(_head "$ROOT4/upstream")" \
    "dirty-but-untouched file: still fast-forwards"
assert_eq "$(cat "$ROOT4/local/agnoster.zsh-theme")" "LOCAL agnoster override" \
    "dirty-but-untouched file: local edit preserved"

# === 5. Diverged (local commit + upstream commit): warns, leaves HEAD alone ===
ROOT5="$(_fixture)"
_commit "$ROOT5/local" local.txt "local only"
_commit "$ROOT5/upstream" up.txt "upstream only"
LOCAL_HEAD="$(_head "$ROOT5/local")"
OUT5="$ROOT5/out.txt"
RC="$(_run "$ROOT5/local" "$OUT5")"
assert_eq "$RC" "0" "diverged: exits 0 (never fails the install)"
assert_eq "$(_head "$ROOT5/local")" "$LOCAL_HEAD" "diverged: HEAD unchanged (no merge commit)"
assert_grep "diverged: prints a WARNING" 'WARNING' "$OUT5"
assert_grep "diverged: names the clone dir in the warning" "$ROOT5/local" "$OUT5"
assert_eq "$(git -C "$ROOT5/local" rev-list --count HEAD)" "2" \
    "diverged: history untouched (2 commits: base + local)"
assert_grep "diverged: the warning says it is local commits" 'local commit' "$OUT5"

# === 5b. The dotfiles agnoster copy is healed so the clone fast-forwards ===
# Regression (all 4 fleet hosts, 15 commits behind): older .zshrc copied
# opt/themes/agnoster.zsh-theme over the TRACKED themes/agnoster.zsh-theme on
# every shell start, and upstream e6561f57 changed that same file, so
# `merge --ff-only` refused forever. When that copy is the clone's ONLY
# modification it is disposable (.zshrc now keeps it in custom/themes/), so
# the script restores the upstream file and fast-forwards.
ROOT5B="$(_fixture)"
_commit "$ROOT5B/upstream" themes/agnoster.zsh-theme "upstream agnoster v1"
git -C "$ROOT5B/local" pull -q origin master
printf '%s\n' "DOTFILES agnoster copy" > "$ROOT5B/local/themes/agnoster.zsh-theme"
_commit "$ROOT5B/upstream" themes/agnoster.zsh-theme "upstream agnoster v2 (conda fix)"
OUT5B="$ROOT5B/out.txt"
RC="$(_run "$ROOT5B/local" "$OUT5B")"
assert_eq "$RC" "0" "dotfiles theme copy: exits 0"
assert_eq "$(_head "$ROOT5B/local")" "$(_head "$ROOT5B/upstream")" \
    "dotfiles theme copy: healed and fast-forwarded to the upstream tip"
assert_eq "$(cat "$ROOT5B/local/themes/agnoster.zsh-theme")" "upstream agnoster v2 (conda fix)" \
    "dotfiles theme copy: the tracked theme is upstream's again"
assert_eq "$(git -C "$ROOT5B/local" status --porcelain --untracked-files=no)" "" \
    "dotfiles theme copy: the clone is clean afterwards"
assert_grep "dotfiles theme copy: says it restored the theme" 'restored themes/agnoster\.zsh-theme' "$OUT5B"
assert_grep_negative "dotfiles theme copy: no WARNING" 'WARNING' "$OUT5B"

# === 5c. Some OTHER modified file upstream also changed: still refuses ===
# Only the disposable theme copy is healed; a real local edit is never thrown
# away, and the warning must name the file instead of blaming "local commits".
ROOT5C="$(_fixture)"
_commit "$ROOT5C/upstream" lib/git.zsh "upstream git lib v1"
git -C "$ROOT5C/local" pull -q origin master
printf '%s\n' "LOCAL git lib edit" > "$ROOT5C/local/lib/git.zsh"
_commit "$ROOT5C/upstream" lib/git.zsh "upstream git lib v2"
HEAD5C="$(_head "$ROOT5C/local")"
OUT5C="$ROOT5C/out.txt"
RC="$(_run "$ROOT5C/local" "$OUT5C")"
assert_eq "$RC" "0" "modified file upstream changed: exits 0"
assert_eq "$(_head "$ROOT5C/local")" "$HEAD5C" "modified file upstream changed: HEAD unchanged"
assert_eq "$(cat "$ROOT5C/local/lib/git.zsh")" "LOCAL git lib edit" \
    "modified file upstream changed: the local edit is preserved"
assert_grep "modified file upstream changed: prints a WARNING" 'WARNING' "$OUT5C"
assert_grep "modified file upstream changed: says modified tracked files" 'modified tracked file' "$OUT5C"
assert_grep "modified file upstream changed: names the file" 'lib/git\.zsh' "$OUT5C"
assert_grep_negative "modified file upstream changed: does not blame local commits" 'local commit' "$OUT5C"

# === 5d. Theme copy PLUS another modification: nothing is healed ===
# The theme is only disposable when it is the clone's sole modification; with
# anything else dirty a human should look, so the tree is left exactly as is.
ROOT5D="$(_fixture)"
_commit "$ROOT5D/upstream" themes/agnoster.zsh-theme "upstream agnoster v1"
_commit "$ROOT5D/upstream" lib/other.zsh "upstream other v1"
git -C "$ROOT5D/local" pull -q origin master
printf '%s\n' "DOTFILES agnoster copy" > "$ROOT5D/local/themes/agnoster.zsh-theme"
printf '%s\n' "LOCAL other edit" > "$ROOT5D/local/lib/other.zsh"
_commit "$ROOT5D/upstream" themes/agnoster.zsh-theme "upstream agnoster v2"
HEAD5D="$(_head "$ROOT5D/local")"
OUT5D="$ROOT5D/out.txt"
RC="$(_run "$ROOT5D/local" "$OUT5D")"
assert_eq "$RC" "0" "theme + other edit: exits 0"
assert_eq "$(_head "$ROOT5D/local")" "$HEAD5D" "theme + other edit: HEAD unchanged"
assert_eq "$(cat "$ROOT5D/local/themes/agnoster.zsh-theme")" "DOTFILES agnoster copy" \
    "theme + other edit: the theme is not restored"
assert_grep "theme + other edit: warning names the conflicting theme file" 'themes/agnoster\.zsh-theme' "$OUT5D"

# === 5e. Local commits AND the theme copy: refuses as local commits, touches nothing ===
ROOT5E="$(_fixture)"
_commit "$ROOT5E/upstream" themes/agnoster.zsh-theme "upstream agnoster v1"
git -C "$ROOT5E/local" pull -q origin master
_commit "$ROOT5E/local" local.txt "local only"
printf '%s\n' "DOTFILES agnoster copy" > "$ROOT5E/local/themes/agnoster.zsh-theme"
_commit "$ROOT5E/upstream" themes/agnoster.zsh-theme "upstream agnoster v2"
HEAD5E="$(_head "$ROOT5E/local")"
OUT5E="$ROOT5E/out.txt"
RC="$(_run "$ROOT5E/local" "$OUT5E")"
assert_eq "$RC" "0" "local commits + theme copy: exits 0"
assert_eq "$(_head "$ROOT5E/local")" "$HEAD5E" "local commits + theme copy: HEAD unchanged"
assert_eq "$(cat "$ROOT5E/local/themes/agnoster.zsh-theme")" "DOTFILES agnoster copy" \
    "local commits + theme copy: the working tree is left untouched"
assert_grep "local commits + theme copy: the warning says local commits" 'local commit' "$OUT5E"

# === 6. Upstream unreachable (offline): warns, exit 0, HEAD unchanged ===
ROOT6="$(_fixture)"
git -C "$ROOT6/local" remote set-url origin "$ROOT6/does-not-exist"
HEAD6="$(_head "$ROOT6/local")"
OUT6="$ROOT6/out.txt"
RC="$(_run "$ROOT6/local" "$OUT6")"
assert_eq "$RC" "0" "fetch failure: exits 0"
assert_eq "$(_head "$ROOT6/local")" "$HEAD6" "fetch failure: HEAD unchanged"
assert_grep "fetch failure: prints a WARNING" 'WARNING' "$OUT6"

# === 7. No clone at the given path: quiet no-op, exit 0 ===
MISSING="$(mktemp -d)/nope"
OUT7="$(mktemp)"
RC="$(_run "$MISSING" "$OUT7")"
assert_eq "$RC" "0" "missing clone: exits 0"
assert_grep_negative "missing clone: no WARNING (nothing to update is not an error)" 'WARNING' "$OUT7"

# === 8. A directory that is not a git checkout: quiet no-op, exit 0 ===
NOTGIT="$(mktemp -d)"
OUT8="$(mktemp)"
RC="$(_run "$NOTGIT" "$OUT8")"
assert_eq "$RC" "0" "non-git dir: exits 0"

# === 9. Default location: ~/.oh-my-zsh wins over ~/git/oh-my-zsh, like .zshrc ===
ROOT9="$(_fixture)"
FAKE_HOME="$(mktemp -d)"
mkdir -p "$FAKE_HOME/git"
git clone -q "$ROOT9/upstream" "$FAKE_HOME/.oh-my-zsh"
git clone -q "$ROOT9/upstream" "$FAKE_HOME/git/oh-my-zsh"
_commit "$ROOT9/upstream" later.txt "after both clones"
OUT9="$ROOT9/out.txt"
set +e
HOME="$FAKE_HOME" bash "$SCRIPT" >"$OUT9" 2>&1
RC=$?
set -e
assert_eq "$RC" "0" "default location: exits 0"
assert_eq "$(_head "$FAKE_HOME/.oh-my-zsh")" "$(_head "$ROOT9/upstream")" \
    "default location: ~/.oh-my-zsh was updated"
assert_eq "$(git -C "$FAKE_HOME/git/oh-my-zsh" rev-list --count HEAD)" "1" \
    "default location: ~/git/oh-my-zsh left alone when ~/.oh-my-zsh exists"

# === 10. Default location fallback: ~/git/oh-my-zsh when ~/.oh-my-zsh is absent ===
ROOT10="$(_fixture)"
FAKE_HOME10="$(mktemp -d)"
mkdir -p "$FAKE_HOME10/git"
git clone -q "$ROOT10/upstream" "$FAKE_HOME10/git/oh-my-zsh"
_commit "$ROOT10/upstream" later.txt "after clone"
OUT10="$ROOT10/out.txt"
set +e
HOME="$FAKE_HOME10" bash "$SCRIPT" >"$OUT10" 2>&1
RC=$?
set -e
assert_eq "$RC" "0" "fallback location: exits 0"
assert_eq "$(_head "$FAKE_HOME10/git/oh-my-zsh")" "$(_head "$ROOT10/upstream")" \
    "fallback location: ~/git/oh-my-zsh was updated"

# === 11. gff wiring: flag declared, gated in install.sh, in exactly one phase list ===
INSTALL_SH="${REPO_ROOT}/install.sh"
FEATURES="${REPO_ROOT}/.github/gff/features.yaml"
assert_grep "features.yaml declares install.shell.oh-my-zsh-update" \
    'path: install\.shell\.oh-my-zsh-update$' "${FEATURES}"
assert_grep "install.sh gates the update on install.shell.oh-my-zsh-update" \
    'gff_on install\.shell\.oh-my-zsh-update;' "${INSTALL_SH}"
assert_grep "install.sh calls oh-my-zsh_update.sh" \
    'opt/scripts/system/oh-my-zsh_update\.sh' "${INSTALL_SH}"
# Updating an external clone is a deps step (not repo content). A key in
# NEITHER list runs in BOTH docker phases; a key in both is contradictory.
deps_line="$(grep -E '^_IP_DEPS_FLAGS=' "${INSTALL_SH}")"
config_line="$(grep -E '^_IP_CONFIG_FLAGS=' "${INSTALL_SH}")"
assert_eq "$(printf '%s' "${deps_line}" | grep -c -w 'INSTALL_SHELL_OH_MY_ZSH_UPDATE')" "1" \
    "INSTALL_SHELL_OH_MY_ZSH_UPDATE is in _IP_DEPS_FLAGS"
assert_eq "$(printf '%s' "${config_line}" | grep -c -w 'INSTALL_SHELL_OH_MY_ZSH_UPDATE')" "0" \
    "INSTALL_SHELL_OH_MY_ZSH_UPDATE is NOT in _IP_CONFIG_FLAGS"
# Ordering: the update must run AFTER the .gitrepos block that creates the clone,
# or a fresh machine's first install.sh run never updates anything.
gitrepos_line="$(grep -n 'gff_on install\.system\.gitrepos;' "${INSTALL_SH}" | head -1 | cut -d: -f1)"
update_line="$(grep -n 'gff_on install\.shell\.oh-my-zsh-update;' "${INSTALL_SH}" | head -1 | cut -d: -f1)"
if [ -n "${gitrepos_line}" ] && [ -n "${update_line}" ] && [ "${update_line}" -gt "${gitrepos_line}" ]; then
    assert_eq "ordered" "ordered" "oh-my-zsh update runs after the gitrepos block (line ${update_line} > ${gitrepos_line})"
else
    assert_eq "update@${update_line:-missing} gitrepos@${gitrepos_line:-missing}" "ordered" \
        "oh-my-zsh update runs after the gitrepos block"
fi

# === 12. .zshrc keeps the dotfiles agnoster theme OUT of the tracked tree ===
# oh-my-zsh loads $ZSH_CUSTOM/themes/<name> before $ZSH/themes/<name>, and
# custom/ is gitignored upstream — so copying there gives the same prompt
# without dirtying the clone (the cause of the never-fast-forwards bug).
ZSHRC="${REPO_ROOT}/opt/profiles/.zshrc"
assert_grep ".zshrc copies the agnoster theme into custom/themes" \
    'ZSH_CUSTOM:-\$ZSH/custom\}/themes' "${ZSHRC}"
assert_grep_negative ".zshrc never copies into the tracked \$ZSH/themes" \
    'cp .*"\$ZSH/themes/' "${ZSHRC}"
if command -v zsh >/dev/null 2>&1; then
    assert_exit_code 0 ".zshrc parses with zsh -n" zsh -n "${ZSHRC}"
    set +e
fi

_test_report
