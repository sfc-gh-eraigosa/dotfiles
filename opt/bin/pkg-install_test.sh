#!/usr/bin/env bash
# Test driver for opt/bin/pkg-install (and apt/brew variants).
#
# pkg-install is a thin dispatcher to the platform-specific installer.
# We mock uname/apt-get/brew via PATH shadowing so the test doesn't
# touch the real package manager.
#
# Run: bash opt/bin/pkg-install_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

PKG_INSTALL="${SELF_DIR}/pkg-install"
PKG_INSTALL_APT="${SELF_DIR}/pkg-install-apt"
PKG_INSTALL_BREW="${SELF_DIR}/pkg-install-brew"

# === 1. Syntax checks ===
assert_exit_code 0 "pkg-install parses with bash -n" \
    bash -n "$PKG_INSTALL"
assert_exit_code 0 "pkg-install-apt parses with bash -n" \
    bash -n "$PKG_INSTALL_APT"
assert_exit_code 0 "pkg-install-brew parses with bash -n" \
    bash -n "$PKG_INSTALL_BREW"

# === 2. pkg-install: unknown platform → exit 1 with a meaningful message ===
TMPDIR_TEST="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_TEST"' EXIT

cat > "${TMPDIR_TEST}/uname" <<'EOF'
#!/usr/bin/env bash
echo "FreeBSD"
EOF
chmod +x "${TMPDIR_TEST}/uname"

set +e
OUT=$(PATH="${TMPDIR_TEST}:/usr/bin:/bin" bash "$PKG_INSTALL" 2>&1)
RC=$?
set -e
assert_eq "$RC" "1" "unknown platform exits 1"
case "$OUT" in
    *"unsupported platform"*)
        echo "PASS: unknown platform prints meaningful error"
        PASS=$((PASS + 1))
        ;;
    *)
        echo "FAIL: unknown platform error message missing 'unsupported platform' (got: $OUT)"
        FAIL=$((FAIL + 1))
        ;;
esac

# Sandbox PATH: TMPDIR first (so our stubs win), then real /usr/bin:/bin
# (so bash, grep, etc. are still findable). The script uses `command -v
# apt-get` which checks PATH; we make it miss by NOT supplying a stub
# and ensuring apt-get isn't on /usr/bin or /bin in our test env. On a
# system that actually has apt-get installed we'd accidentally find it,
# so the "missing apt-get" tests below use a hardened sandbox: a tiny
# PATH containing only the stubs we explicitly placed (plus a shim dir
# with bash/grep aliases).
SANDBOX_PATH="${TMPDIR_TEST}/sandbox"
mkdir -p "$SANDBOX_PATH"
# Symlink the essentials so subprocesses still find bash, grep, etc.,
# without exposing apt-get/brew.
for tool in bash sh grep sed awk cat head tr ls mkdir rm chmod readlink dirname basename printf echo find tail cut; do
    if command -v "$tool" >/dev/null 2>&1; then
        ln -sf "$(command -v "$tool")" "${SANDBOX_PATH}/${tool}"
    fi
done

# === 3. pkg-install: Linux without apt-get → exit 1 ===
cat > "${TMPDIR_TEST}/uname" <<'EOF'
#!/usr/bin/env bash
echo "Linux"
EOF
chmod +x "${TMPDIR_TEST}/uname"

set +e
OUT=$(PATH="${TMPDIR_TEST}:${SANDBOX_PATH}" bash "$PKG_INSTALL" 2>&1)
RC=$?
set -e
assert_eq "$RC" "1" "Linux without apt-get exits 1"
case "$OUT" in
    *"no supported package manager"*)
        echo "PASS: Linux+no-apt prints meaningful error"
        PASS=$((PASS + 1))
        ;;
    *)
        echo "FAIL: Linux+no-apt error message missing (got: $OUT)"
        FAIL=$((FAIL + 1))
        ;;
esac

# === 4. pkg-install-apt: missing apt-get → exit 1 with hint ===
set +e
OUT=$(PATH="${SANDBOX_PATH}" bash "$PKG_INSTALL_APT" 2>&1)
RC=$?
set -e
assert_eq "$RC" "1" "pkg-install-apt exits 1 when apt-get is absent"
case "$OUT" in
    *"apt-get not found"*)
        echo "PASS: pkg-install-apt explains the missing apt-get"
        PASS=$((PASS + 1))
        ;;
    *)
        echo "FAIL: pkg-install-apt error message did not mention apt-get (got: $OUT)"
        FAIL=$((FAIL + 1))
        ;;
esac

# === 5. pkg-install-brew: missing brew → exit 1 with hint ===
set +e
OUT=$(PATH="${SANDBOX_PATH}" bash "$PKG_INSTALL_BREW" 2>&1)
RC=$?
set -e
assert_eq "$RC" "1" "pkg-install-brew exits 1 when brew is absent"
case "$OUT" in
    *"Homebrew not found"*)
        echo "PASS: pkg-install-brew explains the missing brew"
        PASS=$((PASS + 1))
        ;;
    *)
        echo "FAIL: pkg-install-brew error message did not mention Homebrew (got: $OUT)"
        FAIL=$((FAIL + 1))
        ;;
esac

# === 6. pkg-install-apt: missing manifest → exit 1 ===
# Mock apt-get so the manifest check is reached.
cat > "${TMPDIR_TEST}/apt-get" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "${TMPDIR_TEST}/apt-get"

set +e
OUT=$(PATH="${TMPDIR_TEST}:${SANDBOX_PATH}" \
    PKG_MANIFEST="/nonexistent-manifest.tsv" \
    bash "$PKG_INSTALL_APT" 2>&1)
RC=$?
set -e
assert_eq "$RC" "1" "pkg-install-apt exits 1 on missing manifest"
case "$OUT" in
    *"manifest not found"*)
        echo "PASS: pkg-install-apt explains the missing manifest"
        PASS=$((PASS + 1))
        ;;
    *)
        echo "FAIL: pkg-install-apt missing-manifest message wrong (got: $OUT)"
        FAIL=$((FAIL + 1))
        ;;
esac

_test_report
