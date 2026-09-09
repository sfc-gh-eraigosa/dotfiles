#!/usr/bin/env bash
# Test driver for opt/bin/claude-pick. Exercises discovery, session naming,
# clone-target parsing and the launch command without starting tmux or Claude.
# Run: bash opt/bin/claude-pick_test.sh
set -u
SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"
T="${SELF_DIR}/claude-pick"

assert_exit_code 0 "claude-pick parses with bash -n" bash -n "$T"
set +e   # assert_exit_code leaves errexit ON; later `$(...)`; rc=$? would abort

# --help exits 0 and explains the no-flags contract
assert_exit_code 0 "--help exits 0" bash "$T" --help
set +e
OUT=$(bash "$T" --help 2>&1)
case "$OUT" in *"aliases.sh"*) echo "PASS: help points at the aliases.sh wrapper"; PASS=$((PASS+1));;
  *) echo "FAIL: help does not mention aliases.sh (got: $OUT)"; FAIL=$((FAIL+1));; esac

# --- fixture: a normal clone, a worktree, a plain dir, a dotted repo name ---
TMPD="$(mktemp -d)"
trap 'rm -rf "$TMPD"' EXIT
mkdir -p "$TMPD/roots/a" "$TMPD/roots/b" "$TMPD/notarepo"
git init -q "$TMPD/roots/a/plain"
git -C "$TMPD/roots/a/plain" -c user.email=t@t -c user.name=t commit -q --allow-empty -m init
git init -q "$TMPD/roots/b/dotted.github.io"
git -C "$TMPD/roots/b/dotted.github.io" -c user.email=t@t -c user.name=t commit -q --allow-empty -m init
# a git WORKTREE: its .git is a FILE, which a `[ -d .git ]` test would miss
git -C "$TMPD/roots/a/plain" worktree add -q -b wt "$TMPD/roots/a/plain-wt" >/dev/null 2>&1
assert_file_exists "$TMPD/roots/a/plain-wt/.git" "fixture worktree created"
if [ -d "$TMPD/roots/a/plain-wt/.git" ]; then
    echo "FAIL: fixture worktree .git is a directory, test would not prove the fix"; FAIL=$((FAIL+1))
else
    echo "PASS: fixture worktree .git is a file (the case PR #47 missed)"; PASS=$((PASS+1))
fi

export CLAUDE_PICK_ROOTS="$TMPD/roots"
LIST="$TMPD/list.txt"
bash "$T" --list > "$LIST" 2>&1
assert_grep "--list finds a normal clone" "roots/a/plain$" "$LIST"
assert_grep "--list finds a git worktree" "roots/a/plain-wt$" "$LIST"
assert_grep "--list finds a dotted repo name" "roots/b/dotted.github.io$" "$LIST"
assert_grep_negative "--list skips non-repo dirs" "notarepo" "$LIST"
assert_grep_negative "--list does not descend into .git" "/\.git/" "$LIST"

# the default depth must reach gss's <owner>/<repo>/<feature>/<user>/<worker>
mkdir -p "$TMPD/roots/owner/repo/feat/user"
git init -q "$TMPD/roots/owner/repo/feat/user/worker"
bash "$T" --list > "$LIST" 2>&1
assert_grep "--list reaches gss-depth worker worktrees" "roots/owner/repo/feat/user/worker$" "$LIST"

# --- session naming: tmux rewrites '.' and ':' at CREATE but not at LOOKUP ---
OUT=$(bash "$T" --print "$TMPD/roots/b/dotted.github.io" 2>&1)
case "$OUT" in *"-s claude-dotted-github-io "*)
    echo "PASS: dots sanitized out of the tmux session name"; PASS=$((PASS+1));;
  *) echo "FAIL: session name not sanitized (got: $OUT)"; FAIL=$((FAIL+1));; esac
assert_grep_negative "session name carries no dots" "\-s claude-dotted\." <(printf '%s\n' "$OUT")

OUT=$(CLAUDE_PICK_SESSION_PREFIX=cc bash "$T" --print "$TMPD/roots/a/plain" 2>&1)
case "$OUT" in *"-s cc-plain "*) echo "PASS: CLAUDE_PICK_SESSION_PREFIX honored"; PASS=$((PASS+1));;
  *) echo "FAIL: session prefix (got: $OUT)"; FAIL=$((FAIL+1));; esac

# --- the launch command must NOT carry launch flags (aliases.sh owns them) ---
OUT=$(bash "$T" --print "$TMPD/roots/a/plain" 2>&1)
case "$OUT" in *--remote-control*|*dangerously-skip-permissions*)
    echo "FAIL: launch flags leaked into the command (got: $OUT)"; FAIL=$((FAIL+1));;
  *) echo "PASS: no launch flags — wrapper owns them, so no duplicate --remote-control"; PASS=$((PASS+1));; esac
case "$OUT" in *"\"\$SHELL\" -ic 'claude; exec \"\$SHELL\" -i'"*)
    echo "PASS: bare claude under an interactive shell"; PASS=$((PASS+1));;
  *) echo "FAIL: inner command shape (got: $OUT)"; FAIL=$((FAIL+1));; esac

# a worktree passed by path resolves instead of being mangled into a clone URL
OUT=$(bash "$T" --print "$TMPD/roots/a/plain-wt" 2>&1)
case "$OUT" in *"-s claude-plain-wt "*) echo "PASS: worktree path accepted as a repo"; PASS=$((PASS+1));;
  *) echo "FAIL: worktree path not resolved (got: $OUT)"; FAIL=$((FAIL+1));; esac
# a subdirectory resolves up to the work-tree root
mkdir -p "$TMPD/roots/a/plain/sub/dir"
OUT=$(bash "$T" --print "$TMPD/roots/a/plain/sub/dir" 2>&1)
case "$OUT" in *"-s claude-plain "*) echo "PASS: subdir resolves to the work-tree root"; PASS=$((PASS+1));;
  *) echo "FAIL: subdir resolution (got: $OUT)"; FAIL=$((FAIL+1));; esac

# --- clone-target parsing: paths are rejected, never coerced into a URL ---
for bad in "$TMPD/nope/typo" "./nope" "../nope"; do
    OUT=$(bash "$T" --print "$bad" 2>&1); RC=$?
    if [ "$RC" -ne 0 ] && [ "${OUT#*looks like a path}" != "$OUT" ]; then
        echo "PASS: path-shaped target rejected ($bad)"; PASS=$((PASS+1))
    else
        echo "FAIL: path-shaped target not rejected: $bad (rc=$RC got: $OUT)"; FAIL=$((FAIL+1))
    fi
    case "$OUT" in *"github.com/$TMPD"*|*"github.com/./"*|*"github.com/../"*)
        echo "FAIL: path was coerced into a clone URL ($bad)"; FAIL=$((FAIL+1));; esac
done
OUT=$(bash "$T" --print "not a repo spec" 2>&1); RC=$?
assert_eq "$RC" 1 "free text rejected"
OUT=$(bash "$T" --print "o/r/extra" 2>&1); RC=$?
assert_eq "$RC" 1 "three-segment target rejected"

# --- clone-and-launch: reuses an existing clone, derives the name correctly ---
export CLAUDE_PICK_CLONE_DIR="$TMPD/clones"
mkdir -p "$CLAUDE_PICK_CLONE_DIR"
git init -q "$CLAUDE_PICK_CLONE_DIR/r"
git -C "$CLAUDE_PICK_CLONE_DIR/r" -c user.email=t@t -c user.name=t commit -q --allow-empty -m init
OUT=$(bash "$T" --print "owner/r" 2>&1)
case "$OUT" in *"reusing existing clone"*) echo "PASS: owner/repo reuses an existing clone"; PASS=$((PASS+1));;
  *) echo "FAIL: existing clone not reused (got: $OUT)"; FAIL=$((FAIL+1));; esac
case "$OUT" in *"-s claude-r "*) echo "PASS: clone target resolves to its work tree"; PASS=$((PASS+1));;
  *) echo "FAIL: clone target session name (got: $OUT)"; FAIL=$((FAIL+1));; esac
# a trailing slash must not yield an empty name (which would clone into the root)
OUT=$(bash "$T" --print "https://github.com/owner/r/" 2>&1)
case "$OUT" in *"-s claude-r "*) echo "PASS: trailing slash stripped from clone name"; PASS=$((PASS+1));;
  *) echo "FAIL: trailing slash handling (got: $OUT)"; FAIL=$((FAIL+1));; esac
# ...and neither must a .git suffix
OUT=$(bash "$T" --print "git@github.com:owner/r.git" 2>&1)
case "$OUT" in *"-s claude-r "*) echo "PASS: .git suffix stripped from clone name"; PASS=$((PASS+1));;
  *) echo "FAIL: .git suffix handling (got: $OUT)"; FAIL=$((FAIL+1));; esac

# --- no TTY: refuse to run, and say what to do instead ---
OUT=$(bash "$T" "$TMPD/roots/a/plain" < /dev/null 2>&1); RC=$?
assert_eq "$RC" 1 "no-TTY launch refused"
case "$OUT" in *"--print"*) echo "PASS: no-TTY error points at --print"; PASS=$((PASS+1));;
  *) echo "FAIL: no-TTY error unhelpful (got: $OUT)"; FAIL=$((FAIL+1));; esac
OUT=$(bash "$T" --print < /dev/null 2>&1); RC=$?
assert_eq "$RC" 1 "--print without a target refused"

# --- --print must not require tmux or claude (CI has neither) ---
# Regression guard: a blanket preflight made every --print call die with
# "'claude' not found on PATH" on any machine that is not the one that will
# run the session. --print only resolves a repo and prints a command.
MINBIN="$TMPD/minbin"
mkdir -p "$MINBIN"
for b in bash git basename tr sed; do ln -sf "$(command -v "$b")" "$MINBIN/$b"; done
OUT=$(env -i PATH="$MINBIN" HOME="$TMPD" bash "$T" --print "$TMPD/roots/a/plain" 2>&1); RC=$?
assert_eq "$RC" 0 "--print works with neither tmux nor claude on PATH"
case "$OUT" in *"-s claude-plain "*) echo "PASS: --print emits the command without claude present"; PASS=$((PASS+1));;
  *) echo "FAIL: --print under a minimal PATH (got: $OUT)"; FAIL=$((FAIL+1));; esac
# ...but the real launch path still refuses (no tmux, and no terminal)
OUT=$(env -i PATH="$MINBIN" HOME="$TMPD" bash "$T" "$TMPD/roots/a/plain" < /dev/null 2>&1); RC=$?
assert_eq "$RC" 1 "launch path still refuses under a minimal PATH"
set +e

# --- the picker (sourced with CLAUDE_PICK_LIB=1, fzf stubbed) ---
# The picker needs a tty, so exercise cp_pick directly with a fake fzf that
# records the candidate list and echoes back a chosen line.
mkdir -p "$TMPD/bin"
FZF_IN="$TMPD/fzf-stdin.txt"
# shellcheck disable=SC2016  # $FZF_PICK must expand inside the stub, not here
printf '#!/usr/bin/env bash\ncat > %s\nsed -n "${FZF_PICK:-1}p" %s\n' "$FZF_IN" "$FZF_IN" > "$TMPD/bin/fzf"
chmod +x "$TMPD/bin/fzf"

PICK=$(PATH="$TMPD/bin:$PATH" CLAUDE_PICK_LIB=1 bash -c \
    '. "$1"; cp_pick' _ "$T" 2>/dev/null)
case "$PICK" in "$TMPD"/roots/*) echo "PASS: picker returns a repo from the discovered list"; PASS=$((PASS+1));;
  *) echo "FAIL: picker return value (got: $PICK)"; FAIL=$((FAIL+1));; esac
assert_grep "picker offers the clone entry last" "^\[clone a new repo\]$" "$FZF_IN"
assert_grep "picker offers discovered repos" "roots/a/plain$" "$FZF_IN"

# choosing "[clone a new repo]" prompts for a target and reuses an existing clone
CLONE_LINE=$(grep -c '' "$FZF_IN")
PICK=$(printf 'owner/r\n' | PATH="$TMPD/bin:$PATH" FZF_PICK="$CLONE_LINE" CLAUDE_PICK_LIB=1 \
    bash -c '. "$1"; cp_pick' _ "$T" 2>/dev/null)
assert_eq "$PICK" "$CLAUDE_PICK_CLONE_DIR/r" "picker clone entry clones/reuses and returns the work tree"

# sourcing as a library must not launch anything
OUT=$(CLAUDE_PICK_LIB=1 bash -c '. "$1"; echo sourced-ok' _ "$T" 2>&1)
assert_eq "$OUT" "sourced-ok" "CLAUDE_PICK_LIB=1 sources without running main"

# --- empty roots ---
OUT=$(CLAUDE_PICK_ROOTS="$TMPD/does-not-exist" bash "$T" --list 2>&1)
assert_eq "$OUT" "" "--list is empty when no roots exist"

_test_report
