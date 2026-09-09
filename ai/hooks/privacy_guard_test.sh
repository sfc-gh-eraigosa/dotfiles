#!/bin/bash
# Test driver for ai/hooks/privacy_guard.sh
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
    out=$(printf '%s' "$payload" | env -i PATH="$PATH" USER="$ID_USER" HOME="$ID_HOME" HOSTNAME="$ID_HOST" bash "$HOOK" 2>&1)
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

# Shared-dialect tool variants (write_file/replace; used by the antigravity adapter)
pl_write_file() { jq -n --arg fp "$1" --arg c "$2" '{tool_name:"write_file", tool_input:{file_path:$fp, content:$c}}'; }
pl_replace()    { jq -n --arg fp "$1" --arg s "$2" '{tool_name:"replace",    tool_input:{file_path:$fp, old_string:"x", new_string:$s}}'; }
pl_shell()      { jq -n --arg c "$1"               '{tool_name:"run_shell_command", tool_input:{command:$c}}'; }

WIN="C:\\Users\\${ID_USER}\\project"
WSL="/mnt/c/Users/${ID_USER}/project"
UNIXHOME="$ID_HOME/secret/path"            # /home/alice/...
USERLEAK="deployed by $ID_USER on host"
HOSTLEAK="runs on $ID_HOST in the lab"

assert_json() {
    local expected="$1" payload="$2" label="$3" pattern="$4" out rc
    set +e
    out=$(printf '%s' "$payload" | env -i PATH="$PATH" USER="$ID_USER" HOME="$ID_HOME" HOSTNAME="$ID_HOST" bash "$HOOK" 2>&1)
    rc=$?
    set -e
    if [ "$rc" = "$expected" ] && echo "$out" | grep -q "$pattern"; then
        echo "PASS: $label (exit $rc + JSON match)"; PASS=$((PASS+1))
    else
        echo "FAIL: $label (expected $expected + pattern '$pattern', got $rc) :: $out"; FAIL=$((FAIL+1))
    fi
}

# ============================ ALLOWED (exit 0) =================================
assert 0 "$(pl_write "$TRACKED" 'Path is $HOME/git/dotfiles and ~/.ssh/config')" "tracked: \$HOME + ~ variables"
assert 0 "$(pl_write_file "$TRACKED" 'Path is $HOME/git/dotfiles and ~/.ssh/config')" "tracked: write_file (shared dialect)"
assert_json 0 "$(pl_write_file "$TRACKED" 'Path is $HOME/git/dotfiles and ~/.ssh/config')" "tracked: write_file (shared dialect) JSON check" '{"decision": "allow"}'
assert 0 "$(pl_write "$TRACKED" 'Use ${USER} or <user> as the account placeholder')" "tracked: \${USER} + <user> placeholder"
assert 0 "$(pl_write "$TRACKED" 'See C:\Users\<user>\AppData for the path')"   "tracked: Windows path with <user> placeholder"
assert 0 "$(pl_write "$TRACKED" 'export DB_PASS=${DB_PASS}  # from env')"        "tracked: password references a variable"
assert 0 "$(pl_write "$TRACKED" 'password: <your-password-here>')"              "tracked: password placeholder"
assert 0 "$(pl_write "$LOCAL"   "leaked $WSL and $USERLEAK")"                   "ignored file: leak allowed (local only)"
assert 0 "$(pl_write "$OUTSIDE" "leaked $WIN and $UNIXHOME")"                   "outside any repo: leak allowed (local only)"
assert 0 "$(pl_bash  "ls -la $WSL && grep alice /etc/passwd")"                  "non-publish Bash: out of scope"
assert 0 "$(pl_shell "ls -la $WSL && grep alice /etc/passwd")"                  "non-publish shell (shared dialect): out of scope"
assert 0 "$(pl_bash  'git commit -m "fix: tidy the docs"')"                     "clean commit message"
assert 0 "$(pl_write "$TRACKED" 'Generic system path /home/linuxbrew/.linuxbrew is fine')" "tracked: non-user system /home path"
assert 0 "$(jq -n '{tool_name:"Read", tool_input:{file_path:"/x"}}')"           "non-write/non-bash tool passes through"

# ============================ BLOCKED (exit 2) =================================
assert 2 "$(pl_write "$TRACKED" "build dir is $WIN")"           "tracked Write: C:\\Users\\<name> (Rule A)"
assert 2 "$(pl_write_file "$TRACKED" "build dir is $WIN")"      "tracked write_file: C:\\Users\\<name> (Rule A)"
assert 2 "$(pl_write "$TRACKED" "wsl path $WSL here")"          "tracked Write: /mnt/c/Users/<name> (Rule A)"
assert 2 "$(pl_edit  "$TRACKED" "see $WSL for details")"        "tracked Edit: WSL user path (Rule A)"
assert 2 "$(pl_replace "$TRACKED" "see $WSL for details")"      "tracked replace: WSL user path (Rule A)"
assert 2 "$(pl_write "$TRACKED" "config lives at $UNIXHOME")"   "tracked Write: real /home/<you> (Rule B)"
assert 2 "$(pl_write "$TRACKED" "$USERLEAK")"                   "tracked Write: bare login username (Rule C)"
assert 2 "$(pl_write "$TRACKED" "$HOSTLEAK")"                   "tracked Write: bare hostname (Rule C)"
assert 2 "$(pl_bash  "gh pr create --title x --body '$WSL is the path'")" "gh pr create body: WSL user path"
assert 2 "$(pl_shell "gh pr create --title x --body '$WSL is the path'")" "gh pr create body (shared dialect): WSL user path"
assert 2 "$(pl_bash  "git commit -m 'docs: path $WIN'")"        "git commit message: Windows user path"
assert 2 "$(pl_write "$TRACKED" 'key AKIA1234567890ABCDEF rotated')" "tracked Write: AWS access key (Rule D)"
assert 2 "$(pl_write "$TRACKED" '-----BEGIN OPENSSH PRIVATE KEY-----')" "tracked Write: PEM private key (Rule D)"
assert 2 "$(pl_write "$TRACKED" 'ghp_abcdefghijklmnopqrstuvwxyz0123456789')" "tracked Write: GitHub PAT (Rule D)"
assert 2 "$(pl_write "$TRACKED" 'password=hunter2SuperSecretValue')" "tracked Write: hard-coded password (Rule D heuristic)"

# ============ REGRESSION: hook must run under its production invocation ========
# The asserts above all prepend `bash "$HOOK"`. Production does NOT: the harness
# runs `$HOME/.claude/hooks/privacy_guard.sh` directly, so the shebang selects the
# interpreter. With NO shebang the kernel falls back to /bin/sh (dash on Debian),
# which chokes on the bash arrays / [[ ]] and the guard crashes — failing CLOSED
# on every clean publishing command. Exercise that path directly so the bug can
# never regress unseen. (See issue: privacy_guard crashes under dash.)
if head -1 "$HOOK" | grep -q '^#!'; then
    echo "PASS: hook declares a #! shebang"; PASS=$((PASS+1))
else
    echo "FAIL: hook is missing a #! shebang (production execs it directly -> /bin/sh)"; FAIL=$((FAIL+1))
fi
# Direct (shebang-driven) exec of a clean publishing payload must ALLOW (exit 0),
# not crash. Pre-fix, a shebang-less executable yields ENOEXEC (126).
set +e
rc_direct=$(printf '%s' "$(pl_bash 'git commit -m "fix: tidy the docs"')" \
    | env -i PATH="$PATH" USER="$ID_USER" HOME="$ID_HOME" HOSTNAME="$ID_HOST" "$HOOK" > /dev/null 2>&1; echo $?)
set -e
if [ "$rc_direct" = 0 ]; then
    echo "PASS: clean commit allowed under shebang-driven direct exec (exit 0)"; PASS=$((PASS+1))
else
    echo "FAIL: direct exec of hook did not allow a clean commit (got $rc_direct) -- shebang/interpreter regression"; FAIL=$((FAIL+1))
fi

# ============================ GAPS (probe 2026-09-04) ===========================
# Every case below got through the original hook. Each is one shape a leak can
# take that the original never looked at. Grouped by WHY it got through.

# assert_env: assert with extra environment for the hook subprocess.
# usage: assert_env <expected_code> <payload-json> <label> [VAR=value ...]
assert_env() {
    local expected="$1" payload="$2" label="$3" out rc; shift 3
    set +e
    out=$(printf '%s' "$payload" | env -i PATH="$PATH" USER="$ID_USER" HOME="$ID_HOME" HOSTNAME="$ID_HOST" "$@" bash "$HOOK" 2>&1)
    rc=$?
    set -e
    if [ "$rc" = "$expected" ]; then
        echo "PASS: $label (exit $rc)"; PASS=$((PASS+1))
    else
        echo "FAIL: $label (expected $expected, got $rc) :: $out"; FAIL=$((FAIL+1))
    fi
}
# Bash payload that also carries the working directory, as Claude's does.
pl_bash_cwd() { jq -n --arg c "$1" --arg d "$2" '{tool_name:"Bash", tool_input:{command:$c}, cwd:$d}'; }

# --- Class 1: file writes that never pass through the Write/Edit tools ---------
# The original gate only fired for Write/Edit. A heredoc, sed -i, tee, or an
# interpreter one-liner puts the same bytes in the same tracked file.
assert 2 "$(pl_bash "cat > $TRACKED <<'EOF'
home is $ID_HOME
EOF")"                                                              "Bash heredoc into a TRACKED file"
assert 2 "$(pl_bash "sed -i 's|X|$ID_HOME|' $TRACKED")"           "sed -i into a TRACKED file" # portability-ok: a string fixture the hook judges, never executed
assert 2 "$(pl_bash "echo '$ID_HOME' | tee -a $TRACKED")"          "tee into a TRACKED file"
assert 2 "$(pl_bash "python3 -c \"open('$TRACKED','w').write('$ID_HOME')\"")" "interpreter write into a TRACKED file"
assert 0 "$(pl_bash "cat > $LOCAL <<'EOF'
home is $ID_HOME
EOF")"                                                              "Bash heredoc into a GITIGNORED file stays allowed"
assert 0 "$(pl_bash "printf '%s' '$ID_HOME' > /dev/null")"         "redirect to /dev/null is not a file write"

# --- Class 2: the commit is judged by its CONTENT, not just its message --------
SCAN_REPO="$(mktemp -d)"
git -C "$SCAN_REPO" init -q
git -C "$SCAN_REPO" -c user.name=A -c user.email=a@example.com commit -q --allow-empty -m seed --no-verify
printf 'deployed from %s\n' "$ID_HOME" > "$SCAN_REPO/leak.md"
git -C "$SCAN_REPO" add leak.md
assert 2 "$(pl_bash_cwd 'git commit -m "docs: tidy"' "$SCAN_REPO")"  "git commit with a clean MESSAGE but a leak STAGED"
printf 'fix on %s\n' "$ID_HOST" > "$SCAN_REPO/msg.txt"
git -C "$SCAN_REPO" reset -q leak.md
assert 2 "$(pl_bash_cwd "git commit -F $SCAN_REPO/msg.txt" "$SCAN_REPO")" "git commit -F reads the message FILE"
git -C "$SCAN_REPO" add leak.md
git -C "$SCAN_REPO" -c user.name=A -c user.email=a@example.com commit -q -m "sneaky" --no-verify

# A git trailer carrying the repo's CONFIGURED user.email is not a leak: that
# address is on every author line already. Anyone else's address still is.
TRAILER_REPO="$(mktemp -d)"
git -C "$TRAILER_REPO" init -q
git -C "$TRAILER_REPO" config user.name "Alice"
git -C "$TRAILER_REPO" config user.email "alice@example.com"
assert 0 "$(pl_bash_cwd 'git commit --allow-empty -m "docs: tidy" -m "Signed-off-by: Alice <alice@example.com>"' "$TRAILER_REPO")" "git commit: Signed-off-by trailer with the configured git email is allowed"
assert 2 "$(pl_bash_cwd 'git commit --allow-empty -m "docs: tidy" -m "Co-authored-by: Bob <bob@corp.example>"' "$TRAILER_REPO")" "git commit: a trailer naming SOMEONE ELSE is refused (angle brackets are not a placeholder)"
assert 2 "$(pl_bash_cwd 'git commit --allow-empty -m "docs: mail alice@example.com if it breaks"' "$TRAILER_REPO")" "git commit: the configured email OUTSIDE a trailer is refused"

# --- Class 3: publishing verbs the original regex did not know -----------------
# The leak above is now an unpushed commit; every verb that would publish it
# must be judged on what it publishes.
assert 2 "$(pl_bash_cwd 'git push origin HEAD' "$SCAN_REPO")"          "git push judges the outgoing commits"
assert 2 "$(pl_bash_cwd 'gss push' "$SCAN_REPO")"                      "gss push (the repo's own canonical publisher)"
assert 2 "$(pl_bash_cwd 'gss feature checkpoint' "$SCAN_REPO")"        "gss feature checkpoint (push + PR)"
printf 'see %s\n' "$ID_HOME" > "$SCAN_REPO/body.md"
assert 2 "$(pl_bash "gh pr create --title x --body-file $SCAN_REPO/body.md")" "gh pr create --body-file reads the body FILE"
assert 2 "$(pl_bash "gh release create v1 --notes 'built at $ID_HOME'")"  "gh release create notes"
assert 2 "$(pl_bash "gh gist create $SCAN_REPO/body.md")"                "gh gist create publishes a file verbatim"

# --- Class 4: identity shapes the word-boundary rule missed --------------------
assert 2 "$(pl_write "$TRACKED" "host ${ID_USER}gigabyte is up")"     "username as a PREFIX of a longer token"
assert 2 "$(pl_write "$TRACKED" "host ${ID_HOST}pi answered")"        "hostname as a PREFIX of a longer token"
git -C "$SCAN_REPO" config user.email "$ID_USER@example.com"
assert 2 "$(pl_write "$SCAN_REPO/doc.md" "contact $ID_USER@example.com")" "the repo's git user.email"
assert 2 "$(pl_write "$SCAN_REPO/doc.md" "contact someone.else@example.com")" "ANY email address in tracked content"
assert 0 "$(pl_write "$SCAN_REPO/doc.md" "contact <user>@example.com or user@example.com")" "placeholder / example.com addresses stay allowed"

# --- Class 5: secret shapes the list did not cover ------------------------------
# Fixtures are BUILT at runtime so the literal never appears in this file.
SK_ANT="sk-ant-api03-$(printf 'A%.0s' $(seq 1 48))"
SK_PROJ="sk-proj-$(printf 'B%.0s' $(seq 1 40))"
JWT='eyJ''hbGciOiJIUzI1NiJ9.eyJ''zdWIiOiIxMjM0NTY3ODkwIn0.c2lnbmF0dXJlLXNpZ25hdHVyZQ'
assert 2 "$(pl_write "$TRACKED" "key $SK_ANT")"                        "Anthropic API key"
assert 2 "$(pl_write "$TRACKED" "key $SK_PROJ")"                       "OpenAI project key"
assert 2 "$(pl_write "$TRACKED" "auth: Bearer $JWT")"                  "JWT"
assert 2 "$(pl_write "$TRACKED" "hook https://hooks.slack.com/services/T0000/B0000/$(printf 'X%.0s' $(seq 1 24))")" "Slack incoming webhook"
assert 2 "$(pl_write "$TRACKED" "db postgres://admin:s3cretpw@db.example.com/prod")" "credential inside a URL"

# --- Class 6: the hook's own posture ---------------------------------------------
CFG="$(mktemp -d)"
printf 'projectcodename\n' > "$CFG/identity"
assert_env 2 "$(pl_write "$TRACKED" "internal name projectcodename")" "extra identity tokens from the identity file" PRIVACY_GUARD_CONFIG_DIR="$CFG"
printf '%s\n' "$ID_USER" > "$CFG/allow"
assert_env 0 "$(pl_write "$TRACKED" "$USERLEAK")"                     "an allowlisted token is not a leak" PRIVACY_GUARD_CONFIG_DIR="$CFG"
LOGF="$CFG/blocks.log"
assert_env 2 "$(pl_write "$TRACKED" "$USERLEAK")"                     "a deny is logged" PRIVACY_GUARD_LOG="$LOGF"
if [ -s "$LOGF" ] && grep -q "Login username" "$LOGF"; then echo "PASS: block log records the category"; PASS=$((PASS+1))
else echo "FAIL: block log missing or empty ($LOGF)"; FAIL=$((FAIL+1)); fi
# No jq -> no way to know what is being written -> refuse, do not wave through.
set +e
out=$(printf '%s' "$(pl_write "$TRACKED" "$USERLEAK")" | env -i PATH=/nonexistent USER="$ID_USER" HOME="$ID_HOME" HOSTNAME="$ID_HOST" /bin/bash "$HOOK" 2>&1); rc=$?
set -e
if [ "$rc" = 2 ]; then echo "PASS: fails CLOSED when jq is unavailable (exit 2)"; PASS=$((PASS+1))
else echo "FAIL: without jq the hook must fail closed (got $rc) :: $out"; FAIL=$((FAIL+1)); fi
rm -rf "$SCAN_REPO" "$CFG"

# --- Class 7: gitleaks delegation + the time budget ----------------------------
# Secret SHAPES come from gitleaks when it is on PATH (its ruleset is ~10x ours
# and maintained upstream); our 16 shapes stay as the floor. A stub gitleaks
# proves the contract without depending on the real binary being installed:
# it flags any text containing LEAKME and honours the CLI we call.
GL_REPO="$(mktemp -d)"; git -C "$GL_REPO" init -q
GL_DOC="$GL_REPO/doc.md"
STUBS="$(mktemp -d)"
export STUB_GL_LOG="$STUBS/gitleaks.log"
cat > "$STUBS/gitleaks" <<'SHIM'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${STUB_GL_LOG:-/dev/null}"
if [ "${STUB_GL_LEGACY:-0}" = 1 ] && [ "$1" = stdin ]; then
  echo 'Error: unknown command "stdin" for "gitleaks"' >&2; exit 1
fi
report=""; prev=""
for a in "$@"; do [ "$prev" = "--report-path" ] && report="$a"; prev="$a"; done
text="$(cat)"
if printf '%s' "$text" | grep -q LEAKME; then
  [ -n "$report" ] && printf '[{"RuleID":"stub-rule","Description":"Stub Secret","Secret":"LEAKME","File":"","StartLine":1}]\n' > "$report"
  exit 2
fi
[ -n "$report" ] && printf '[]\n' > "$report"
exit 0
SHIM
chmod +x "$STUBS/gitleaks"
GLPATH="$STUBS:$PATH"

assert_env 2 "$(pl_write "$GL_DOC" "token LEAKME here")"           "gitleaks finding denies the write"                  PATH="$GLPATH" STUB_GL_LOG="$STUB_GL_LOG"
out=$(printf '%s' "$(pl_write "$GL_DOC" "token LEAKME here")" | env -i PATH="$GLPATH" USER="$ID_USER" HOME="$ID_HOME" HOSTNAME="$ID_HOST" STUB_GL_LOG="$STUB_GL_LOG" bash "$HOOK" 2>&1) || true
if printf '%s' "$out" | grep -q "stub-rule"; then echo "PASS: the deny names the gitleaks rule"; PASS=$((PASS+1))
else echo "FAIL: deny should name the gitleaks rule :: $out"; FAIL=$((FAIL+1)); fi
if grep -q -- '--exit-code 2' "$STUB_GL_LOG" && grep -q -- '--no-banner' "$STUB_GL_LOG"; then echo "PASS: gitleaks is called with --no-banner --exit-code 2"; PASS=$((PASS+1))
else echo "FAIL: unexpected gitleaks CLI :: $(cat "$STUB_GL_LOG")"; FAIL=$((FAIL+1)); fi
assert_env 0 "$(pl_write "$GL_DOC" "nothing secret here")"         "gitleaks clean => allowed"                          PATH="$GLPATH"
assert_env 2 "$(pl_write "$GL_DOC" "key AKIAIOSFODNN7EXAMPLE1")"   "builtin shapes remain the floor when gitleaks is clean" PATH="$GLPATH"
assert_env 0 "$(pl_write "$GL_DOC" "token LEAKME here")"           "PRIVACY_GUARD_GITLEAKS=0 disables the delegation"   PATH="$GLPATH" PRIVACY_GUARD_GITLEAKS=0
GLCFG="$(mktemp -d)"; printf 'off\n' > "$GLCFG/gitleaks"
assert_env 0 "$(pl_write "$GL_DOC" "token LEAKME here")"           "config file gitleaks=off disables the delegation"    PATH="$GLPATH" PRIVACY_GUARD_CONFIG_DIR="$GLCFG"
assert_env 2 "$(pl_write "$GL_DOC" "token LEAKME here")"           "legacy gitleaks (no stdin subcommand) falls back to detect --pipe" PATH="$GLPATH" STUB_GL_LEGACY=1 STUB_GL_LOG="$STUB_GL_LOG"
if grep -q -- 'detect --pipe' "$STUB_GL_LOG"; then echo "PASS: legacy fallback used detect --pipe"; PASS=$((PASS+1))
else echo "FAIL: legacy fallback did not call detect --pipe :: $(cat "$STUB_GL_LOG")"; FAIL=$((FAIL+1)); fi
: > "$STUB_GL_LOG"
printf '[extend]\nuseDefault = true\n' > "$GL_REPO/.gitleaks.toml"
assert_env 0 "$(pl_write "$GL_DOC" "nothing secret here")"         "a repo .gitleaks.toml is passed to gitleaks"        PATH="$GLPATH" STUB_GL_LOG="$STUB_GL_LOG"
if grep -q -- "-c $GL_REPO/.gitleaks.toml" "$STUB_GL_LOG"; then echo "PASS: gitleaks received -c <repo>/.gitleaks.toml"; PASS=$((PASS+1))
else echo "FAIL: gitleaks did not receive the repo config :: $(cat "$STUB_GL_LOG")"; FAIL=$((FAIL+1)); fi
assert_env 0 "$(pl_write "$LOCAL" "token LEAKME here")"            "gitleaks is NOT run for a gitignored file (no cost where nothing is judged)" PATH="$GLPATH" STUB_GL_LOG="$STUBS/none.log"
[ ! -s "$STUBS/none.log" ] && { echo "PASS: gitleaks was not invoked for the ignored file"; PASS=$((PASS+1)); } || { echo "FAIL: gitleaks ran for an ignored file"; FAIL=$((FAIL+1)); }

# Security must not cost time silently. Every judged call is timed and logged;
# over budget => a visible SLOW warning (never a block).
TLOG="$STUBS/timing.log"
assert_env 0 "$(pl_write "$GL_DOC" "nothing secret here")"         "a timed run is logged"                              PATH="$GLPATH" PRIVACY_GUARD_TIMING_LOG="$TLOG"
if [ -s "$TLOG" ] && grep -Eq $'\tagent:Write\t[0-9]+\t' "$TLOG"; then echo "PASS: timing log has tool, ms and bytes"; PASS=$((PASS+1))
else echo "FAIL: timing log missing or malformed :: $(cat "$TLOG" 2>/dev/null)"; FAIL=$((FAIL+1)); fi
rc=0; out=$(printf '%s' "$(pl_write "$GL_DOC" "nothing secret here")" | env -i PATH="$GLPATH" USER="$ID_USER" HOME="$ID_HOME" HOSTNAME="$ID_HOST" PRIVACY_GUARD_TIMING_LOG="$TLOG" PRIVACY_GUARD_BUDGET_MS=0 bash "$HOOK" 2>&1) || rc=$?
if [ "$rc" = 0 ] && printf '%s' "$out" | grep -q "SLOW"; then echo "PASS: over budget => SLOW warning, still allowed"; PASS=$((PASS+1))
else echo "FAIL: over budget should warn SLOW and allow (rc $rc) :: $out"; FAIL=$((FAIL+1)); fi
out=$(printf '%s' "$(pl_write "$GL_DOC" "nothing secret here")" | env -i PATH="$GLPATH" USER="$ID_USER" HOME="$ID_HOME" HOSTNAME="$ID_HOST" PRIVACY_GUARD_TIMING_LOG="$TLOG" bash "$HOOK" 2>&1) || true
if ! printf '%s' "$out" | grep -q "SLOW"; then echo "PASS: within the default budget => no warning"; PASS=$((PASS+1))
else echo "FAIL: default budget should not warn :: $out"; FAIL=$((FAIL+1)); fi
# The real binary, when present: a shape our 16 built-ins do NOT know.
if command -v gitleaks >/dev/null 2>&1; then
    TW="SK$(head -c 64 /dev/urandom | od -An -tx1 | tr -d ' \n' | cut -c1-32)"
    assert 2 "$(pl_write "$GL_DOC" "twilio key $TW")" "REAL gitleaks: a Twilio API key (absent from the built-in shapes) is refused"
    assert 0 "$(pl_write "$GL_DOC" "plain prose about keys and tokens, none present")" "REAL gitleaks: ordinary prose passes"
else
    echo "SKIP: real gitleaks not on PATH (CI installs it; locally: make install or install_gitleaks.sh)"
fi
rm -rf "$GL_REPO" "$STUBS" "$GLCFG"

echo "----"
echo "privacy_guard_test: $PASS passed, $FAIL failed"
[ "$FAIL" = 0 ] && exit 0 || exit 1
