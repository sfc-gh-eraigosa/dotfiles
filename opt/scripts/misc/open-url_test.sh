#!/usr/bin/env bash
# Test driver for opt/scripts/misc/open-url
#
# The script uses `exec` for the happy paths, which makes runtime
# behavior hard to intercept without forking platform-specific binaries.
# We mock `uname` via PATH-shadowing for the dispatch tests and inspect
# source for the right `command -v` guards.
#
# Run: bash opt/scripts/misc/open-url_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/open-url"

# === 1. Syntax check ===
assert_exit_code 0 "open-url parses with bash -n" \
    bash -n "$SCRIPT"

# === 2. Error case: no URL → exit 2 ===
set +e
bash "$SCRIPT" >/dev/null 2>&1
RC=$?
set -e
assert_eq "$RC" "2" "no URL argument exits 2"

# === 3. Mock dispatch: Darwin path ===
# When uname returns Darwin, the script execs `open`. We shadow both so
# the test observes the call without actually launching a browser. With
# PATH containing only our stubs, `exec open "$URL"` runs our stub.
TMPDIR_TEST="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_TEST"' EXIT
TRACE="${TMPDIR_TEST}/trace.log"

cat > "${TMPDIR_TEST}/uname" <<'EOF'
#!/usr/bin/env bash
echo "Darwin"
EOF
cat > "${TMPDIR_TEST}/open" <<EOF
#!/usr/bin/env bash
echo "open: \$*" >> "$TRACE"
EOF
chmod +x "${TMPDIR_TEST}/uname" "${TMPDIR_TEST}/open"

PATH="${TMPDIR_TEST}:/usr/bin:/bin" bash "$SCRIPT" "https://example.com" >/dev/null 2>&1
if grep -q "open: https://example.com" "$TRACE" 2>/dev/null; then
    echo "PASS: Darwin dispatch execs 'open'"
    PASS=$((PASS + 1))
else
    echo "FAIL: Darwin dispatch did not exec 'open' (trace: $(cat "$TRACE" 2>/dev/null))"
    FAIL=$((FAIL + 1))
fi

# === 4. Mock dispatch: Linux + xdg-open ===
rm -f "$TRACE"
cat > "${TMPDIR_TEST}/uname" <<'EOF'
#!/usr/bin/env bash
echo "Linux"
EOF
cat > "${TMPDIR_TEST}/xdg-open" <<EOF
#!/usr/bin/env bash
echo "xdg-open: \$*" >> "$TRACE"
EOF
chmod +x "${TMPDIR_TEST}/uname" "${TMPDIR_TEST}/xdg-open"

# Ensure WSL detection returns false: empty WSL_DISTRO_NAME + a /proc/version
# that doesn't match. We can't easily mock /proc/version, but on a real
# Linux CI runner it won't contain "microsoft" or "wsl". On WSL hosts the
# test still passes if wslview etc. aren't on our test PATH.
PATH="${TMPDIR_TEST}:/usr/bin:/bin" WSL_DISTRO_NAME="" \
    bash "$SCRIPT" "https://example.com" >/dev/null 2>&1
if grep -q "xdg-open: https://example.com" "$TRACE" 2>/dev/null; then
    echo "PASS: Linux dispatch tries xdg-open"
    PASS=$((PASS + 1))
else
    # On a WSL host the test would route to wslview; treat that path as a
    # PASS too if the trace shows wslview was tried.
    if grep -q "wslview" "$TRACE" 2>/dev/null; then
        echo "PASS: WSL host detected; dispatch tried wslview (acceptable)"
        PASS=$((PASS + 1))
    else
        echo "FAIL: Linux dispatch did not try xdg-open (trace: $(cat "$TRACE" 2>/dev/null))"
        FAIL=$((FAIL + 1))
    fi
fi

# === 5. Source-level guards: WSL detection + opener fallbacks ===
assert_grep "is_wsl helper present" \
    'is_wsl\(\)' "$SCRIPT"
assert_grep "WSL path tries wslview before explorer.exe" \
    'command -v wslview' "$SCRIPT"
assert_grep "WSL fallback to explorer.exe" \
    'command -v explorer\.exe' "$SCRIPT"
assert_grep "Linux non-WSL path uses xdg-open" \
    'command -v xdg-open' "$SCRIPT"
assert_grep "Darwin branch uses 'open'" \
    'Darwin\)' "$SCRIPT"

_test_report
