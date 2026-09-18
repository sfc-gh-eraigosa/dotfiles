#!/usr/bin/env bash
# Test driver for sync-plugins.sh --dry-run. Mirrors safety_guard_test.sh style.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SYNC="${SCRIPT_DIR}/sync-plugins.sh"

PASS=0
FAIL=0

assert_contains() {
    local haystack="$1" needle="$2" desc="$3"
    if printf '%s' "$haystack" | grep -qF -- "$needle"; then
        echo "PASS: $desc"; PASS=$((PASS+1))
    else
        echo "FAIL: $desc (missing: $needle)"; FAIL=$((FAIL+1))
    fi
}

assert_not_contains() {
    local haystack="$1" needle="$2" desc="$3"
    if printf '%s' "$haystack" | grep -qF -- "$needle"; then
        echo "FAIL: $desc (unexpectedly present: $needle)"; FAIL=$((FAIL+1))
    else
        echo "PASS: $desc"; PASS=$((PASS+1))
    fi
}

assert_eq() {
    local got="$1" want="$2" desc="$3"
    if [ "$got" = "$want" ]; then
        echo "PASS: $desc"; PASS=$((PASS+1))
    else
        echo "FAIL: $desc (got '$got' want '$want')"; FAIL=$((FAIL+1))
    fi
}

OUT="$(bash "$SYNC" --dry-run 2>&1)"

assert_contains "$OUT" "DRY-RUN: claude plugin marketplace add anthropics/claude-plugins-official" "adds the official marketplace"
assert_contains "$OUT" "DRY-RUN: claude plugin install superpowers@claude-plugins-official" "installs superpowers"
assert_contains "$OUT" "DRY-RUN: claude plugin install mcp-apps@claude-plugins-official" "installs mcp-apps"

INSTALL_COUNT="$(printf '%s' "$OUT" | grep -c 'DRY-RUN: claude plugin install ')"
assert_eq "$INSTALL_COUNT" "13" "plans install for all 13 plugins"

ENABLE_COUNT="$(printf '%s' "$OUT" | grep -c 'DRY-RUN: claude plugin enable ')"
assert_eq "$ENABLE_COUNT" "13" "plans enable for all 13 plugins"

AGY_COUNT="$(printf '%s' "$OUT" | grep -c 'DRY-RUN: agy plugin install ')"
assert_eq "$AGY_COUNT" "7" "plans install for all 7 antigravity plugin sources"

# --- Behavioral: idempotent skip + no-hang (hermetic, fake CLIs on PATH) -------
# Real `yq` still resolves (the fakes only shadow claude/agy), so the manifest
# is parsed for real. The fake agy reports superpowers as already installed and
# reads stdin on install — if install.sh ever attached an interactive stdin the
# read would block and this test would hang, so completing here is itself the
# hang-guard assertion.
FAKE_BIN="$(mktemp -d)"
cat > "$FAKE_BIN/claude" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat > "$FAKE_BIN/agy" <<'EOF'
#!/usr/bin/env bash
if [ "$1 $2" = "plugin list" ]; then
    echo "superpowers (1.0.0)"
elif [ "$1 $2" = "plugin install" ]; then
    IFS= read -r _ </dev/stdin 2>/dev/null || true   # must hit EOF, never block
    case "$3" in
        # Simulate a source whose plugin name differs from its repo basename
        # and is already installed: the name pre-skip cannot catch it, so the
        # install path must treat "already installed" as a quiet skip.
        *gemini-agent-creator*)
            echo 'Plugin "agent-creator" is already installed. Please uninstall it first.' >&2
            exit 1 ;;
        # FAKE_HANG=1 makes this source mimic a SIGTERM-ignoring CLI: refuse to
        # die, so only the timeout's -k SIGKILL escalation can stop it. Off by
        # default so the idempotency run (OUT2) stays fast.
        *mcp-toolbox*)
            if [ "${FAKE_HANG:-0}" = "1" ]; then
                trap '' TERM
                for _ in $(seq 1 120); do sleep 0.2; done
            fi
            echo "INSTALL_CALLED $3" ;;
        *) echo "INSTALL_CALLED $3" ;;
    esac
fi
EOF
chmod +x "$FAKE_BIN/claude" "$FAKE_BIN/agy"
OUT2="$(PATH="$FAKE_BIN:$PATH" bash "$SYNC" 2>&1)"

assert_contains "$OUT2" "(superpowers already installed)" "skips an already-installed antigravity plugin"
CR_INSTALLS="$(printf '%s' "$OUT2" | grep -c 'INSTALL_CALLED https://github.com/gemini-cli-extensions/code-review')"
assert_eq "$CR_INSTALLS" "1" "installs the duplicated code-review source only once"
SP_INSTALLS="$(printf '%s' "$OUT2" | grep -c 'INSTALL_CALLED https://github.com/obra/superpowers')"
assert_eq "$SP_INSTALLS" "0" "does not reinstall the already-installed superpowers source"

# Name-mismatch source (repo basename != extension name) that is already
# installed: must be reported as a quiet skip, never a WARNING.
assert_contains "$OUT2" "(https://github.com/jduncan-rva/gemini-agent-creator already installed)" "treats name-mismatched 'already installed' as a quiet skip"
assert_not_contains "$OUT2" "WARNING — agy install https://github.com/jduncan-rva/gemini-agent-creator" "does not warn on a name-mismatched already-installed plugin"

# --- Behavioral: timeout actually kills a SIGTERM-ignoring install (-k) --------
# A SIGTERM-ignoring CLI would make a plain `timeout N` wait forever. With
# FAKE_HANG=1 the mcp-toolbox fake ignores SIGTERM too. A short timeout (2s) + kill
# grace (2s) must SIGKILL it. The outer `timeout 40` is the test's own backstop:
# if -k were missing the inner run would hang and this would exit 124, failing the
# completion assertion loudly instead of wedging the whole suite.
OUT3="$(FAKE_HANG=1 SYNC_PLUGINS_TIMEOUT=2 SYNC_PLUGINS_KILL_GRACE=2 \
        PATH="$FAKE_BIN:$PATH" timeout 40 bash "$SYNC" 2>&1)"
T3RC=$?
rm -rf "$FAKE_BIN"
assert_eq "$T3RC" "0" "sync completes (no hang) when an install ignores SIGTERM"
assert_contains "$OUT3" "timed out after 2s" "escalates to SIGKILL on a SIGTERM-ignoring install"

# --- Behavioral: plugin calls run under setsid (no controlling terminal) -------
# The real claude/agy plugin subcommands open /dev/tty directly and render an
# interactive TUI when a controlling terminal is present — bypassing the
# </dev/null stdin guard and wedging the unattended installer (observed as a
# job-control-stopped `claude`). sync-plugins must run them under `setsid` so the
# CLIs have no controlling terminal and fall back to non-interactive mode. Verify
# the wiring with a `setsid` shim that records its use (and shadows any real one,
# so this also asserts the intent on macOS where setsid is absent).
GUARD_BIN="$(mktemp -d)"
cat > "$GUARD_BIN/setsid" <<'EOF'
#!/usr/bin/env bash
echo "SETSID_USED" >&2
[ "$1" = "-w" ] && shift   # sync-plugins is expected to pass -w; then exec the rest
exec "$@"
EOF
cat > "$GUARD_BIN/claude" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat > "$GUARD_BIN/agy" <<'EOF'
#!/usr/bin/env bash
[ "$1 $2" = "plugin list" ] && exit 0
exit 0
EOF
chmod +x "$GUARD_BIN/setsid" "$GUARD_BIN/claude" "$GUARD_BIN/agy"
GUARDOUT="$(PATH="$GUARD_BIN:$PATH" bash "$SYNC" 2>&1)"
rm -rf "$GUARD_BIN"
assert_contains "$GUARDOUT" "SETSID_USED" "runs plugin commands under setsid to detach the controlling terminal"

# --- Behavioral: a /dev/tty-grabbing CLI must NOT hang the installer -----------
# End-to-end proof of the fix: run sync_claude under a real pseudo-terminal (so a
# controlling terminal exists, as in an interactive `./install.sh`) with a fake
# claude that blocks forever IF it can open its controlling terminal, and prints
# a sentinel only when it cannot. With setsid the controlling terminal is gone,
# so the fake runs headless and sync completes; without setsid it would block
# (and the sentinel would never appear). Linux-only — skipped where `script` or
# `setsid` are unavailable (e.g. macOS).
if command -v script >/dev/null 2>&1 && command -v setsid >/dev/null 2>&1; then
    TTY_BIN="$(mktemp -d)"
    cat > "$TTY_BIN/claude" <<'EOF'
#!/usr/bin/env bash
if : <>/dev/tty 2>/dev/null; then
    sleep 600            # controlling terminal reachable -> mimic the hanging TUI
else
    echo "headless-ok"   # no controlling terminal -> setsid detached it
fi
exit 0
EOF
    cat > "$TTY_BIN/agy" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
    chmod +x "$TTY_BIN/claude" "$TTY_BIN/agy"
    # Short per-call timeout bounds the pre-fix blocking case; outer `timeout 60`
    # backstops the whole pty run so a regression fails loudly instead of wedging.
    TTYOUT="$(SYNC_PLUGINS_TIMEOUT=3 SYNC_PLUGINS_KILL_GRACE=2 PATH="$TTY_BIN:$PATH" \
              timeout 60 script -qec "bash '$SYNC'" /dev/null </dev/null 2>&1 | tr -d '\r')"
    TTYRC=$?
    rm -rf "$TTY_BIN"
    assert_eq "$TTYRC" "0" "sync completes under a pty when the CLI would grab /dev/tty"
    assert_contains "$TTYOUT" "headless-ok" "setsid detaches the controlling terminal so claude runs headless"
fi

# --- Behavioral: macOS keg-only util-linux setsid is discovered via brew -------
# On macOS `command -v setsid` fails (no native setsid), so sync-plugins must
# probe Homebrew's keg-only util-linux (`brew --prefix util-linux`) — otherwise
# the detach guard no-ops and a fresh `claude plugin install` hangs on /dev/tty.
# Simulate macOS: a curated PATH with NO setsid, a fake `brew` whose
# `--prefix util-linux` points at a keg holding a fake setsid, and fake CLIs.
# Assert the keg setsid actually gets invoked.
KEG_ROOT="$(mktemp -d)"
mkdir -p "$KEG_ROOT/bin"
cat > "$KEG_ROOT/bin/setsid" <<'EOF'
#!/usr/bin/env bash
echo "KEG_SETSID_USED" >&2
[ "$1" = "-w" ] && shift   # sync-plugins passes -w; then exec the rest
exec "$@"
EOF
chmod +x "$KEG_ROOT/bin/setsid"
KEGBIN="$(mktemp -d)"
# Curated PATH: symlink the real tools sync-plugins needs, but deliberately NOT
# setsid (so the keg probe is exercised) and NOT timeout (so GUARD = setsid only).
for _t in bash sh env yq grep egrep awk sed tr cat head cut sort uniq dirname basename readlink mktemp; do
    _src="$(command -v "$_t" 2>/dev/null)" && ln -s "$_src" "$KEGBIN/$_t" 2>/dev/null
done
cat > "$KEGBIN/brew" <<EOF
#!/usr/bin/env bash
[ "\$1" = "--prefix" ] && [ "\$2" = "util-linux" ] && { printf '%s\n' "$KEG_ROOT"; exit 0; }
exit 0
EOF
cat > "$KEGBIN/claude" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat > "$KEGBIN/agy" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$KEGBIN/brew" "$KEGBIN/claude" "$KEGBIN/agy"
KEGOUT="$(PATH="$KEGBIN" bash "$SYNC" 2>&1 || true)"
rm -rf "$KEGBIN" "$KEG_ROOT"
assert_contains "$KEGOUT" "KEG_SETSID_USED" "discovers keg-only util-linux setsid via 'brew --prefix' when setsid is off PATH (macOS)"

# --- Behavioral: warns when uvx is missing and an AWS plugin is enabled -------
# dotfiles#312: deploy-on-aws/aws-serverless/aws-core's MCP servers launch via
# `uvx`, which nothing in this repo installs or checks for — on a host without
# it, those servers fail to connect at every future session start with a
# confusing "Executable not found in $PATH: uvx", naming a binary the user
# never chose to depend on. sync-plugins must warn once, actionably, at sync
# time instead. Curated PATH (coreutils + yq, no uvx) makes this deterministic
# regardless of whether uvx happens to be installed on the machine running
# this test suite.
NOUVX_BIN="$(mktemp -d)"
for _t in bash sh env yq grep egrep awk sed tr cat head cut sort uniq dirname basename readlink mktemp xargs; do
    _src="$(command -v "$_t" 2>/dev/null)" && ln -s "$_src" "$NOUVX_BIN/$_t" 2>/dev/null
done
NOUVXOUT="$(PATH="$NOUVX_BIN" bash "$SYNC" --dry-run 2>&1)"
rm -rf "$NOUVX_BIN"
assert_contains "$NOUVXOUT" "uvx is not on PATH" "warns when uvx is missing and an AWS (uvx-dependent) plugin is enabled"
UVX_WARNING="$(printf '%s\n' "$NOUVXOUT" | grep "uvx is not on PATH")"
assert_contains "$UVX_WARNING" "deploy-on-aws" "warning names deploy-on-aws"
assert_contains "$UVX_WARNING" "aws-serverless" "warning names aws-serverless"
assert_contains "$UVX_WARNING" "aws-core" "warning names aws-core"

# --- Behavioral: no warning when uvx IS on PATH --------------------------------
UVX_BIN="$(mktemp -d)"
for _t in bash sh env yq grep egrep awk sed tr cat head cut sort uniq dirname basename readlink mktemp xargs; do
    _src="$(command -v "$_t" 2>/dev/null)" && ln -s "$_src" "$UVX_BIN/$_t" 2>/dev/null
done
cat > "$UVX_BIN/uvx" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$UVX_BIN/uvx"
UVXOUT="$(PATH="$UVX_BIN" bash "$SYNC" --dry-run 2>&1)"
rm -rf "$UVX_BIN"
assert_not_contains "$UVXOUT" "uvx is not on PATH" "does not warn when uvx is already on PATH"

# --- Behavioral: transient git index.lock on `claude plugin update` retries --
# Observed on real fleet-update runs: `claude plugin update` failed on a FRESH
# clone dir with "fatal: Unable to create '.../.clone/.git/index.lock': File
# exists" for several plugins in a row, each succeeding again moments later —
# the CLI's own internal clone/checkout racing itself, not a genuinely stuck
# process (that would need a manual `rm`, per the message). One short retry
# recovers the common case instead of leaving a plugin silently stuck on its
# previous version until the next sync.
LOCK_BIN="$(mktemp -d)"
for _t in bash sh env yq grep egrep awk sed tr cat head cut sort uniq dirname basename readlink mktemp xargs; do
    _src="$(command -v "$_t" 2>/dev/null)" && ln -s "$_src" "$LOCK_BIN/$_t" 2>/dev/null
done
COUNT_DIR="$(mktemp -d)"
cat > "$LOCK_BIN/claude" <<EOF
#!/usr/bin/env bash
if [ "\$1 \$2" = "plugin update" ]; then
    name="\$3"
    f="$COUNT_DIR/\${name//[^a-zA-Z0-9]/_}"
    n=0; [ -f "\$f" ] && n=\$(cat "\$f")
    n=\$((n + 1)); echo "\$n" > "\$f"
    case "\$name" in
        deploy-on-aws@claude-plugins-official)
            # Locked on the first attempt only; clear by the retry.
            if [ "\$n" -eq 1 ]; then
                echo "fatal: Unable to create '/home/x/.claude/plugins/cache/temp_subdir_1.clone/.git/index.lock': File exists." >&2
                exit 1
            fi
            exit 0 ;;
        aws-serverless@claude-plugins-official)
            # Always locked: the retry budget must still run out and warn.
            echo "fatal: Unable to create '/home/x/.claude/plugins/cache/temp_subdir_2.clone/.git/index.lock': File exists." >&2
            exit 1 ;;
        superpowers@claude-plugins-official)
            # A real, non-lock failure must never retry.
            echo "some other real failure" >&2
            exit 1 ;;
    esac
fi
exit 0
EOF
chmod +x "$LOCK_BIN/claude"
LOCKOUT="$(SYNC_PLUGINS_LOCK_RETRY_DELAY=0 PATH="$LOCK_BIN:$PATH" bash "$SYNC" 2>&1)"
rm -rf "$LOCK_BIN"
assert_eq "$(cat "$COUNT_DIR/deploy_on_aws_claude_plugins_official")" "2" "a transient index.lock failure is retried exactly once before succeeding"
assert_not_contains "$LOCKOUT" "WARNING — 'claude plugin update deploy-on-aws@claude-plugins-official'" "a transient index.lock failure that clears on retry reports no warning"
assert_eq "$(cat "$COUNT_DIR/aws_serverless_claude_plugins_official")" "2" "a persistent index.lock failure retries up to the budget, not forever"
assert_contains "$LOCKOUT" "WARNING — 'claude plugin update aws-serverless@claude-plugins-official' failed" "a persistent index.lock failure still warns once the retry budget is spent"
assert_eq "$(cat "$COUNT_DIR/superpowers_claude_plugins_official")" "1" "a non-lock failure is never retried"
assert_contains "$LOCKOUT" "WARNING — 'claude plugin update superpowers@claude-plugins-official' failed" "a non-lock failure still warns immediately"
rm -rf "$COUNT_DIR"

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
