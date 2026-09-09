#!/usr/bin/env bash
# Test driver for strip-agy-rc-appends.sh — removes agy's hardcoded-$HOME
# PATH append from SYMLINKED (repo-managed) rc files only. Sandboxed HOME.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
# shellcheck source=/dev/null
. "$REPO_ROOT/ai/_test_helpers.sh"

STRIP="$SCRIPT_DIR/strip-agy-rc-appends.sh"

H="$(mktemp -d)"
trap 'rm -rf "$H"' EXIT
mkdir -p "$H/repo"

# Repo-managed rc: a symlink whose target carries the agy append.
cat > "$H/repo/.zshrc" <<'RC'
# repo zshrc content
[ -f "$HOME/.zshrc.local" ] && source "$HOME/.zshrc.local"
true


# Added by Antigravity CLI installer
export PATH="/home/someuser/.local/bin:$PATH"
RC
ln -s "$H/repo/.zshrc" "$H/.zshrc"

# Repo-managed .zprofile: zsh LOGIN shells read this, and agy appends here too.
cat > "$H/repo/.zprofile" <<'RC'
# repo zprofile content
[ -f "$HOME/.profile" ] && emulate sh -c '. "$HOME/.profile"'


# Added by Antigravity CLI installer
export PATH="/home/someuser/.local/bin:$PATH"
RC
ln -s "$H/repo/.zprofile" "$H/.zprofile"

# Host-owned REAL rc with the same append: must be left alone.
cat > "$H/.bashrc" <<'RC'
# host-owned bashrc

# Added by Antigravity CLI installer
export PATH="/home/someuser/.local/bin:$PATH"
RC

# Repo-managed rc WITHOUT the append: must remain byte-identical.
printf '# clean profile\n' > "$H/repo/.profile"
ln -s "$H/repo/.profile" "$H/.profile"
before_clean="$(cat "$H/repo/.profile")"

assert_exit_code 0 "strip script runs clean" env HOME="$H" bash "$STRIP"

assert_grep_negative "append removed from symlinked .zshrc target" \
    'Added by Antigravity CLI installer' "$H/repo/.zshrc"
assert_grep "unrelated content preserved in .zshrc" 'repo zshrc content' "$H/repo/.zshrc"
assert_in_subshell "no dangling blank lines at EOF" \
    "[ \"\$(tail -c 6 '$H/repo/.zshrc')\" = 'true' ] || [ \"\$(tail -n1 '$H/repo/.zshrc')\" = 'true' ]"
assert_in_subshell ".zshrc is still a symlink (not replaced by a file)" "[ -L '$H/.zshrc' ]"

assert_grep_negative "append removed from symlinked .zprofile target" \
    'Added by Antigravity CLI installer' "$H/repo/.zprofile"
assert_grep "unrelated content preserved in .zprofile" 'repo zprofile content' "$H/repo/.zprofile"
assert_in_subshell ".zprofile is still a symlink (not replaced by a file)" "[ -L '$H/.zprofile' ]"

assert_grep "host-owned real .bashrc keeps its append" \
    'Added by Antigravity CLI installer' "$H/.bashrc"

assert_eq "$(cat "$H/repo/.profile")" "$before_clean" "clean symlinked rc left byte-identical"

# Idempotency: second run changes nothing further.
after_first="$(cat "$H/repo/.zshrc")"
after_first_zp="$(cat "$H/repo/.zprofile")"
env HOME="$H" bash "$STRIP" >/dev/null
assert_eq "$(cat "$H/repo/.zshrc")" "$after_first" "second run is a no-op"
assert_eq "$(cat "$H/repo/.zprofile")" "$after_first_zp" "second run is a no-op for .zprofile"

_test_report
