#!/usr/bin/env bash
# Test driver for .gitools.sh (git-reset / git-reset-all / git-clean).
# Self-contained: builds a bare origin + clones in a temp dir; no network.

set -u
FRAGMENT="$(cd -- "$(dirname "$0")" && pwd -P)/.gitools.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
FAILS=0

pass() { echo "PASS: $*"; }
failc() { echo "FAIL: $*"; FAILS=$((FAILS + 1)); }

# Isolated git identity for commits made by this driver.
export GIT_CONFIG_GLOBAL="$TMP/gitconfig"
git config --global user.email t@t.test
git config --global user.name t
git config --global init.defaultBranch main

make_origin_and_clone() {  # $1 = workdir name
    git init -q --bare "$TMP/$1-origin.git"
    git clone -q "$TMP/$1-origin.git" "$TMP/$1" 2>/dev/null
    ( cd "$TMP/$1" && echo base > f.txt && git add f.txt && \
      git commit -qm base && git push -q origin main )
}

in_clone() {  # $1 = workdir, rest = command run with fragment loaded
    d="$TMP/$1"; shift
    ( cd "$d" && bash -c ". '$FRAGMENT'; $*" )
}

# --- git-reset: discards local commits + untracked files, syncs to origin ---
make_origin_and_clone r1
( cd "$TMP/r1" && echo local > f.txt && git commit -qam divergent && echo junk > junk.txt )
if in_clone r1 "git-reset" > /dev/null 2>&1 \
   && [ "$(cat "$TMP/r1/f.txt")" = "base" ] && [ ! -f "$TMP/r1/junk.txt" ]; then
    pass "git-reset discards divergence and untracked files"
else
    failc "git-reset did not restore clean origin state"
fi

# --- git-reset outside a repo fails cleanly ---
mkdir -p "$TMP/notrepo"
if ( cd "$TMP/notrepo" && bash -c ". '$FRAGMENT'; git-reset" ) > /dev/null 2>&1; then
    failc "git-reset outside a repo should fail"
else
    pass "git-reset outside a repo fails with error"
fi

# --- git-reset HOME guard: aborts unless the answer is exactly Yes ---
make_origin_and_clone rhome
if ( cd "$TMP/rhome" && HOME="$TMP/rhome" bash -c ". '$FRAGMENT'; echo No | git-reset" ) > /dev/null 2>&1; then
    failc "git-reset on HOME repo should abort without Yes"
else
    pass "git-reset on HOME repo aborts without explicit Yes"
fi

# --- git-clean: resets current branch to upstream without pulling ---
make_origin_and_clone r2
( cd "$TMP/r2" && echo dirty >> f.txt && echo junk > junk.txt )
if in_clone r2 "git-clean" > /dev/null 2>&1 \
   && [ "$(cat "$TMP/r2/f.txt")" = "base" ] && [ ! -f "$TMP/r2/junk.txt" ]; then
    pass "git-clean resets to upstream and removes untracked files"
else
    failc "git-clean did not clean the worktree"
fi

# --- git-reset-all: sweeps child repos, skips non-repos ---
mkdir -p "$TMP/sweep"
git init -q --bare "$TMP/s1-origin.git"; git clone -q "$TMP/s1-origin.git" "$TMP/sweep/s1" 2>/dev/null
( cd "$TMP/sweep/s1" && echo base > f.txt && git add f.txt && git commit -qm base && git push -q origin main )
( cd "$TMP/sweep/s1" && echo junk > junk.txt )
mkdir -p "$TMP/sweep/plain"
out="$( cd "$TMP/sweep" && bash -c ". '$FRAGMENT'; git-reset-all" 2>&1 )"
if [ ! -f "$TMP/sweep/s1/junk.txt" ] && printf '%s' "$out" | grep -q "skip: plain"; then
    pass "git-reset-all resets child repos and skips non-repos"
else
    failc "git-reset-all sweep incorrect: $out"
fi

# --- fragment is side-effect free at source time ---
out="$(bash -c ". '$FRAGMENT'" 2>&1)"
if [ -z "$out" ]; then
    pass "sourcing the fragment produces no output or side effects"
else
    failc "sourcing the fragment printed: $out"
fi

if [ "$FAILS" -gt 0 ]; then
    echo "$FAILS case(s) failed"
    exit 1
fi
echo "All gitools cases passed"
