#!/bin/bash
# Test driver for opt/scripts/git/install_git_hooks.sh.
#
# The installer makes the privacy git hooks GLOBAL: it copies them (plus the
# shared rules library) into ~/.config/git/hooks and points core.hooksPath at
# that directory. Hermetic: HOME and the global git config are throwaways, so
# the real machine is never touched.
#
# exit 0 = all pass, 1 = a failure.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
INSTALLER="$HERE/install_git_hooks.sh"
PASS=0
FAIL=0

WORK="$(mktemp -d)"
HOME_T="$WORK/home"
GCFG="$HOME_T/.gitconfig"
mkdir -p "$HOME_T"
: > "$GCFG"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT

run_installer() {
    env -i PATH="$PATH" HOME="$HOME_T" GIT_CONFIG_GLOBAL="$GCFG" bash "$INSTALLER"
}

check() { # $1 = label, $2 = condition exit code
    if [ "$2" -eq 0 ]; then echo "PASS: $1"; PASS=$((PASS+1))
    else echo "FAIL: $1"; FAIL=$((FAIL+1)); fi
}

out=$(run_installer 2>&1); rc=$?
check "installer exits 0 on a fresh HOME (got $rc) :: $out" "$rc"

DEST="$HOME_T/.config/git/hooks"
for h in pre-commit commit-msg pre-push; do
    if [ -x "$DEST/$h" ]; then r=0; else r=1; fi
    check "installs an executable $h" "$r"
done
if [ -f "$DEST/privacy_rules.sh" ]; then r=0; else r=1; fi
check "installs the shared rules library beside the hooks" "$r"

got="$(env -i PATH="$PATH" HOME="$HOME_T" GIT_CONFIG_GLOBAL="$GCFG" git config --global core.hooksPath)"
if [ "$got" = "$DEST" ]; then r=0; else r=1; fi
check "sets core.hooksPath to the install dir (got '$got')" "$r"

# Idempotent: a second run changes nothing and does not error.
run_installer > /dev/null 2>&1; rc=$?
check "second run is a no-op (exit $rc)" "$rc"

# A repo with its own .git/hooks/<name> (husky, lefthook, hand-written) must
# keep working: a global hooksPath silently replaces the local hook, so the
# installed hook chains to it after its own check passes.
REPO="$WORK/repo"
mkdir -p "$REPO"
env -i PATH="$PATH" HOME="$HOME_T" GIT_CONFIG_GLOBAL="$GCFG" git -C "$REPO" init -q
printf '#!/bin/sh\necho LOCAL-HOOK-RAN >&2\nexit 0\n' > "$REPO/.git/hooks/pre-commit"
chmod +x "$REPO/.git/hooks/pre-commit"
printf 'clean\n' > "$REPO/a.md"
env -i PATH="$PATH" HOME="$HOME_T" GIT_CONFIG_GLOBAL="$GCFG" git -C "$REPO" add a.md
out=$(env -i PATH="$PATH" HOME="$HOME_T" GIT_CONFIG_GLOBAL="$GCFG" USER=alice HOSTNAME=alicebox \
      git -C "$REPO" -c user.name=A -c user.email=a@example.com commit -q -m "x" 2>&1); rc=$?
printf '%s' "$out" | grep -q "LOCAL-HOOK-RAN"; check "chains to the repo-local .git/hooks/pre-commit (exit $rc) :: $out" $?

echo "----"
echo "install_git_hooks_test: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
