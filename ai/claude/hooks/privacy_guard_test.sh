#!/bin/bash
# Test driver for ai/claude/hooks/privacy_guard.sh
#
# Hermetic: builds a throwaway git repo (so the tracked-vs-ignored file gate is
# real) and runs the hook with a CONTROLLED identity (USER/HOME/HOSTNAME) so the
# identity rules fire deterministically on any host / CI runner.
#
# exit 0 = all pass, 1 = a failure.
set -u

HOOK_DIR="$(cd "$(dirname "$0")" && pwd)"
HOOK="$HOOK_DIR/privacy_guard.sh"
PASS=0
FAIL=0

# Controlled identity for the hook subprocess (NOT the test's own shell).
ID_USER="alice"
ID_HOME="/home/alice"
ID_HOST="alicebox"

# Hermetic repo: doc.md is tracked-able, local/ is gitignored.
REPO="$(mktemp -d)"
git -C "$REPO" init -q
printf 'local/\n' > "$REPO/.gitignore"
mkdir -p "$REPO/local"
TRACKED="$REPO/doc.md"          # check-ignore -> not ignored -> SCANNED
LOCAL="$REPO/local/note.md"     # check-ignore -> ignored     -> skipped
OUTSIDE="$(mktemp -d)/loose.md" # not in a repo               -> skipped

cleanup() { rm -rf "$REPO" "$(dirname "$OUTSIDE")"; }
trap cleanup EXIT

# usage: assert <expected_code> <payload-json> <label>
assert() {
    local expected="$1" payload="$2" label="$3" out rc
    set +e
    out=$(printf '%s' "$payload" | env -i PATH="$PATH" USER="$ID_USER" HOME="$ID_HOME" HOSTNAME="$ID_HOST" "$HOOK" 2>&1)
    rc=$?
    set -e
    if [ "$rc" = "$expected" ]; then
        echo "PASS: $label (exit $rc)"; PASS=$((PASS+1))
    else
        echo "FAIL: $label (expected $expected, got $rc) :: $out"; FAIL=$((FAIL+1))
    fi
}

pl_write() { jq -n --arg fp "$1" --arg c "$2" '{tool_name:"Write",  tool_input:{file_path:$fp, content:$c}}'; }
pl_edit()  { jq -n --arg fp "$1" --arg s "$2" '{tool_name:"Edit",   tool_input:{file_path:$fp, old_string:"x", new_string:$s}}'; }
pl_bash()  { jq -n --arg c "$1"               '{tool_name:"Bash",   tool_input:{command:$c}}'; }

WIN='C:\Users\edwar\project'
WSL='/mnt/c/Users/edwar/project'
UNIXHOME="$ID_HOME/secret/path"            # /home/alice/...
USERLEAK="deployed by $ID_USER on host"
HOSTLEAK="runs on $ID_HOST in the lab"

# ============================ ALLOWED (exit 0) =================================
assert 0 "$(pl_write "$TRACKED" 'Path is $HOME/git/dotfiles and ~/.ssh/config')" "tracked: \$HOME + ~ variables"
assert 0 "$(pl_write "$TRACKED" 'Use ${USER} or <user> as the account placeholder')" "tracked: \${USER} + <user> placeholder"
assert 0 "$(pl_write "$TRACKED" 'See C:\Users\<user>\AppData for the path')"   "tracked: Windows path with <user> placeholder"
assert 0 "$(pl_write "$TRACKED" 'export DB_PASS=${DB_PASS}  # from env')"        "tracked: password references a variable"
assert 0 "$(pl_write "$TRACKED" 'password: <your-password-here>')"              "tracked: password placeholder"
assert 0 "$(pl_write "$LOCAL"   "leaked $WSL and $USERLEAK")"                   "ignored file: leak allowed (local only)"
assert 0 "$(pl_write "$OUTSIDE" "leaked $WIN and $UNIXHOME")"                   "outside any repo: leak allowed (local only)"
assert 0 "$(pl_bash  "ls -la $WSL && grep alice /etc/passwd")"                  "non-publish Bash: out of scope"
assert 0 "$(pl_bash  'git commit -m "fix: tidy the docs"')"                     "clean commit message"
assert 0 "$(pl_write "$TRACKED" 'Generic system path /home/linuxbrew/.linuxbrew is fine')" "tracked: non-user system /home path"
assert 0 "$(jq -n '{tool_name:"Read", tool_input:{file_path:"/x"}}')"           "non-write/non-bash tool passes through"

# ============================ BLOCKED (exit 2) =================================
assert 2 "$(pl_write "$TRACKED" "build dir is $WIN")"           "tracked Write: C:\\Users\\<name> (Rule A)"
assert 2 "$(pl_write "$TRACKED" "wsl path $WSL here")"          "tracked Write: /mnt/c/Users/<name> (Rule A)"
assert 2 "$(pl_edit  "$TRACKED" "see $WSL for details")"        "tracked Edit: WSL user path (Rule A)"
assert 2 "$(pl_write "$TRACKED" "config lives at $UNIXHOME")"   "tracked Write: real /home/<you> (Rule B)"
assert 2 "$(pl_write "$TRACKED" "$USERLEAK")"                   "tracked Write: bare login username (Rule C)"
assert 2 "$(pl_write "$TRACKED" "$HOSTLEAK")"                   "tracked Write: bare hostname (Rule C)"
assert 2 "$(pl_bash  "gh pr create --title x --body '$WSL is the path'")" "gh pr create body: WSL user path"
assert 2 "$(pl_bash  "git commit -m 'docs: path $WIN'")"        "git commit message: Windows user path"
assert 2 "$(pl_write "$TRACKED" 'key AKIA1234567890ABCDEF rotated')" "tracked Write: AWS access key (Rule D)"
assert 2 "$(pl_write "$TRACKED" '-----BEGIN OPENSSH PRIVATE KEY-----')" "tracked Write: PEM private key (Rule D)"
assert 2 "$(pl_write "$TRACKED" 'ghp_abcdefghijklmnopqrstuvwxyz0123456789')" "tracked Write: GitHub PAT (Rule D)"
assert 2 "$(pl_write "$TRACKED" 'password=hunter2SuperSecretValue')" "tracked Write: hard-coded password (Rule D heuristic)"

echo "----"
echo "privacy_guard_test: $PASS passed, $FAIL failed"
[ "$FAIL" = 0 ] && exit 0 || exit 1
