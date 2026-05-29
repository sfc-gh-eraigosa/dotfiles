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
assert_eq "$INSTALL_COUNT" "12" "plans install for all 12 plugins"

ENABLE_COUNT="$(printf '%s' "$OUT" | grep -c 'DRY-RUN: claude plugin enable ')"
assert_eq "$ENABLE_COUNT" "12" "plans enable for all 12 plugins"

GEMINI_COUNT="$(printf '%s' "$OUT" | grep -c 'DRY-RUN: gemini extensions install ')"
assert_eq "$GEMINI_COUNT" "7" "plans install for all 7 gemini extension sources"

# --- Behavioral: idempotent skip + no-hang (hermetic, fake CLIs on PATH) -------
# Real `yq` still resolves (the fakes only shadow claude/gemini), so the manifest
# is parsed for real. The fake gemini reports superpowers as already installed and
# reads stdin on install — if install.sh ever attached an interactive stdin the
# read would block and this test would hang, so completing here is itself the
# hang-guard assertion.
FAKE_BIN="$(mktemp -d)"
cat > "$FAKE_BIN/claude" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat > "$FAKE_BIN/gemini" <<'EOF'
#!/usr/bin/env bash
if [ "$1 $2" = "extensions list" ]; then
    echo "✓ superpowers (5.1.0)" >&2   # real gemini prints the list to stderr
elif [ "$1 $2" = "extensions install" ]; then
    IFS= read -r _ </dev/stdin 2>/dev/null || true   # must hit EOF, never block
    case "$3" in
        # Simulate a source whose extension name differs from its repo basename
        # and is already installed: the name pre-skip cannot catch it, so the
        # install path must treat "already installed" as a quiet skip.
        *gemini-agent-creator*)
            echo 'Extension "agent-creator" is already installed. Please uninstall it first.' >&2
            exit 1 ;;
        # FAKE_HANG=1 makes this source mimic the real gemini CLI: ignore SIGTERM
        # and refuse to die, so only the timeout's -k SIGKILL escalation can stop
        # it. Off by default so the idempotency run (OUT2) stays fast.
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
chmod +x "$FAKE_BIN/claude" "$FAKE_BIN/gemini"
OUT2="$(PATH="$FAKE_BIN:$PATH" bash "$SYNC" 2>&1)"

assert_contains "$OUT2" "(superpowers already installed)" "skips an already-installed gemini extension"
CR_INSTALLS="$(printf '%s' "$OUT2" | grep -c 'INSTALL_CALLED https://github.com/gemini-cli-extensions/code-review')"
assert_eq "$CR_INSTALLS" "1" "installs the duplicated code-review source only once"
SP_INSTALLS="$(printf '%s' "$OUT2" | grep -c 'INSTALL_CALLED https://github.com/obra/superpowers')"
assert_eq "$SP_INSTALLS" "0" "does not reinstall the already-installed superpowers source"

# Name-mismatch source (repo basename != extension name) that is already
# installed: must be reported as a quiet skip, never a WARNING.
assert_contains "$OUT2" "(https://github.com/jduncan-rva/gemini-agent-creator already installed)" "treats name-mismatched 'already installed' as a quiet skip"
assert_not_contains "$OUT2" "WARNING — gemini install https://github.com/jduncan-rva/gemini-agent-creator" "does not warn on a name-mismatched already-installed extension"

# --- Behavioral: timeout actually kills a SIGTERM-ignoring install (-k) --------
# The gemini CLI ignores SIGTERM, so a plain `timeout N` would wait forever. With
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

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
