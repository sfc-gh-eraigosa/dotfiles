#!/bin/bash
# Test driver for ai/githooks/{pre-commit,commit-msg,pre-push}.
#
# These hooks are the content-at-boundary layer: they scan what git is about to
# RECORD or PUBLISH, regardless of how the text got into the tree. The agent
# hook (privacy_guard.sh) sees only the tool call that wrote a file; a heredoc,
# `sed -i`, or `gss push` never passes through it. Git does see all of those.
#
# Hermetic: a throwaway repo + a throwaway bare "origin", a throwaway HOME
# carrying a controlled git identity, and core.hooksPath pointed straight at
# the repo's ai/githooks so the hooks under test are the ones in the tree.
#
# exit 0 = all pass, 1 = a failure.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
PASS=0
FAIL=0

# Controlled identity for every git subprocess (NOT the test's own shell).
ID_USER="alice"
ID_HOST="alicebox"
ID_EMAIL="alice@example.com"

WORK="$(mktemp -d)"
HOME_T="$WORK/home"
REPO="$WORK/repo"
ORIGIN="$WORK/origin.git"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT

mkdir -p "$HOME_T"
printf '[user]\n\tname = Alice\n\temail = %s\n[init]\n\tdefaultBranch = main\n' "$ID_EMAIL" > "$HOME_T/.gitconfig"

# g: run git with the controlled identity and the hooks under test.
g() {
    env -i PATH="$PATH" HOME="$HOME_T" USER="$ID_USER" HOSTNAME="$ID_HOST" \
        GIT_CONFIG_GLOBAL="$HOME_T/.gitconfig" \
        git -c core.hooksPath="$HERE" "$@"
}

git init -q --bare "$ORIGIN"
mkdir -p "$REPO"
g -C "$REPO" init -q
g -C "$REPO" remote add origin "$ORIGIN"
printf 'seed\n' > "$REPO/README.md"
g -C "$REPO" add README.md
g -C "$REPO" commit -q -m "seed" --no-verify
g -C "$REPO" push -q origin HEAD --no-verify

# usage: expect <ok|blocked> <label> -- <git args...>
expect() {
    local want="$1" label="$2"; shift 2; [ "$1" = "--" ] && shift
    local out rc good=1
    out=$(g "$@" 2>&1)
    rc=$?
    if [ "$want" = ok ] && [ "$rc" -eq 0 ]; then good=0; fi
    if [ "$want" = blocked ] && [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "BLOCKED by privacy_guard"; then good=0; fi
    if [ "$good" -eq 0 ]; then
        echo "PASS: $label"; PASS=$((PASS+1))
    else
        echo "FAIL: $label (want $want, exit $rc) :: $out"; FAIL=$((FAIL+1))
    fi
}

stage() { # $1 = file, $2 = content
    printf '%s\n' "$2" > "$REPO/$1"
    g -C "$REPO" add "$1"
}

# ============================ pre-commit ========================================
# Content reaches the index by ANY route; the hook must judge the index itself.
stage leak.md "config lives at /home/$ID_USER/.ssh"
expect blocked "pre-commit: staged real home path is refused" -- -C "$REPO" commit -q -m "docs: paths"
g -C "$REPO" reset -q leak.md; rm -f "$REPO/leak.md"

stage leak.md "built on $ID_HOST last night"
expect blocked "pre-commit: staged hostname is refused" -- -C "$REPO" commit -q -m "docs: host"
g -C "$REPO" reset -q leak.md; rm -f "$REPO/leak.md"

stage leak.md "contact $ID_EMAIL for access"
expect blocked "pre-commit: staged git-config email is refused (identity from git config)" -- -C "$REPO" commit -q -m "docs: contact"
g -C "$REPO" reset -q leak.md; rm -f "$REPO/leak.md"

stage leak.md "host ${ID_USER}gigabyte answered on :22"
expect blocked "pre-commit: username as a PREFIX of a longer token is refused" -- -C "$REPO" commit -q -m "docs: fleet"
g -C "$REPO" reset -q leak.md; rm -f "$REPO/leak.md"

stage clean.md 'path is $HOME/.ssh, user is ${USER}, host is <host>'
expect ok "pre-commit: placeholders are allowed" -- -C "$REPO" commit -q -m "docs: placeholders"

# A leak that was ALREADY committed and is untouched by this commit is not this
# commit's fault; only ADDED lines are judged, or every later commit is blocked.
printf 'old %s line\n' "/home/$ID_USER" > "$REPO/history.md"
g -C "$REPO" add history.md
g -C "$REPO" commit -q -m "pre-existing" --no-verify
printf 'old %s line\nnew clean line\n' "/home/$ID_USER" > "$REPO/history.md"
g -C "$REPO" add history.md
expect ok "pre-commit: only ADDED lines are judged, pre-existing leaks do not block" -- -C "$REPO" commit -q -m "docs: append"

# ============================ commit-msg ========================================
expect blocked "commit-msg: home path in the message is refused" -- -C "$REPO" commit -q --allow-empty -m "docs: see /home/$ID_USER/notes"
expect blocked "commit-msg: message from a FILE is judged too" -- -C "$REPO" commit -q --allow-empty -F "$(printf 'fix on %s\n' "$ID_HOST" > "$WORK/msg" && echo "$WORK/msg")"
expect ok "commit-msg: clean message passes" -- -C "$REPO" commit -q --allow-empty -m "docs: tidy"

# ============================ pre-push ==========================================
# Everything committed with --no-verify above must still be caught at the
# boundary that actually publishes it. This is the layer gss push / checkpoint
# hit, because they exec git push.
#
# First put the history so far (which carries the old history.md leak) on
# origin, bypassing the hook: the point of the next cases is what happens to
# NEW commits on top of a remote that already has a leak in its past.
g -C "$REPO" push -q origin HEAD --no-verify

stage pushed.md "deployed from /home/$ID_USER"
g -C "$REPO" commit -q -m "sneaky" --no-verify
expect blocked "pre-push: a leaked commit (made with --no-verify) is refused at push" -- -C "$REPO" push -q origin HEAD
g -C "$REPO" reset -q --hard HEAD~1

stage after.md "all clean here"
g -C "$REPO" commit -q -m "docs: clean follow-up"
expect ok "pre-push: clean commits pass even when older history (already on origin) has a leak" -- -C "$REPO" push -q origin HEAD

g -C "$REPO" checkout -q -b feature
stage newbranch.md "host is $ID_HOST"
g -C "$REPO" commit -q -m "branch" --no-verify
expect blocked "pre-push: a NEW branch (no remote counterpart) is judged against what origin lacks" -- -C "$REPO" push -q -u origin feature
g -C "$REPO" reset -q --hard HEAD~1

# ============================ escape hatch ======================================
stage skip.md "/home/$ID_USER on purpose"
out=$(env -i PATH="$PATH" HOME="$HOME_T" USER="$ID_USER" HOSTNAME="$ID_HOST" \
      GIT_CONFIG_GLOBAL="$HOME_T/.gitconfig" PRIVACY_GUARD_SKIP=1 \
      git -c core.hooksPath="$HERE" -C "$REPO" commit -q -m "reviewed exception" 2>&1); rc=$?
if [ "$rc" -eq 0 ]; then echo "PASS: PRIVACY_GUARD_SKIP=1 is an explicit, loud bypass"; PASS=$((PASS+1))
else echo "FAIL: PRIVACY_GUARD_SKIP=1 bypass (exit $rc) :: $out"; FAIL=$((FAIL+1)); fi

echo "----"
echo "githooks_test: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
