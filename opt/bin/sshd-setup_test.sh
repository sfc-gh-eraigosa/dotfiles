#!/usr/bin/env bash
# Test driver for opt/bin/sshd-setup. Mocks external commands via PATH
# shadowing (same pattern as pkg-install_test.sh). Run: bash opt/bin/sshd-setup_test.sh
set -u
SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"
SSHD_SETUP="${SELF_DIR}/sshd-setup"
TMPDIR_TEST="$(mktemp -d)"; trap 'rm -rf "$TMPDIR_TEST"' EXIT

# Sandbox PATH: essentials only, so `command -v gh` / `git` / `ufw` genuinely
# miss when a test needs them absent (real ones live in /usr/bin).
SANDBOX_PATH="${TMPDIR_TEST}/sandbox"
mkdir -p "$SANDBOX_PATH" "${TMPDIR_TEST}/mocks"
for tool in bash sh env grep sed awk cat wc tr ls mkdir rm chmod touch head tail cut printf echo uname pgrep dirname basename readlink find sort; do
    if command -v "$tool" >/dev/null 2>&1; then
        ln -sf "$(command -v "$tool")" "${SANDBOX_PATH}/${tool}"
    fi
done

assert_exit_code 0 "sshd-setup parses with bash -n" bash -n "$SSHD_SETUP"

# usage on no args -> exit 1, mentions subcommands
set +e; OUT=$(bash "$SSHD_SETUP" 2>&1); RC=$?; set -e
assert_eq "$RC" "1" "no args exits 1"
case "$OUT" in *"status|enable|keys"*) echo "PASS: usage lists subcommands"; PASS=$((PASS+1));;
  *) echo "FAIL: usage missing subcommand list (got: $OUT)"; FAIL=$((FAIL+1));; esac

# platform detection: wsl via fake /proc/version
echo "Linux version 6.6 (Microsoft@Microsoft.com)" > "${TMPDIR_TEST}/proc_version"
OUT=$(SSHD_SETUP_PROC_VERSION="${TMPDIR_TEST}/proc_version" bash "$SSHD_SETUP" _detect 2>&1)
assert_eq "$OUT" "wsl" "detects wsl from microsoft /proc/version"
echo "Linux version 6.6 (gcc)" > "${TMPDIR_TEST}/proc_version"
OUT=$(SSHD_SETUP_PROC_VERSION="${TMPDIR_TEST}/proc_version" bash "$SSHD_SETUP" _detect 2>&1)
assert_eq "$OUT" "linux" "detects plain linux"

# === Task 2: status (read-only probes) ===
# status with mocked systemctl "active" -> reports running, exit 0
cat > "${TMPDIR_TEST}/mocks/systemctl" <<'EOF'
#!/usr/bin/env bash
[ "$1" = "is-active" ] && exit 0
exit 0
EOF
chmod +x "${TMPDIR_TEST}/mocks/systemctl"
OUT=$(PATH="${TMPDIR_TEST}/mocks:$PATH" bash "$SSHD_SETUP" status 2>&1)
case "$OUT" in *"running"*) echo "PASS: status reports running"; PASS=$((PASS+1));;
  *) echo "FAIL: status missing 'running' (got: $OUT)"; FAIL=$((FAIL+1));; esac

# status with everything absent -> still exit 0, reports not installed
cat > "${TMPDIR_TEST}/mocks/systemctl" <<'EOF'
#!/usr/bin/env bash
exit 3
EOF
chmod +x "${TMPDIR_TEST}/mocks/systemctl"
cat > "${TMPDIR_TEST}/mocks/pgrep" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "${TMPDIR_TEST}/mocks/pgrep"
cat > "${TMPDIR_TEST}/mocks/sshd" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
# NOTE: mocks/sshd intentionally NOT created and not executable — we want
# `command -v sshd` to miss; remove it in case a later test adds one.
rm -f "${TMPDIR_TEST}/mocks/sshd"
set +e
OUT=$(PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" SSHD_SETUP_SSHD_PATH=/nonexistent bash "$SSHD_SETUP" status 2>&1); RC=$?
set -e
assert_eq "$RC" "0" "status exits 0 even when sshd absent"
case "$OUT" in *"not installed"*) echo "PASS: status reports not installed"; PASS=$((PASS+1));;
  *) echo "FAIL: status missing 'not installed' (got: $OUT)"; FAIL=$((FAIL+1));; esac

# === Task 3: GitHub account derivation ===
# gh present wins
cat > "${TMPDIR_TEST}/mocks/gh" <<'EOF'
#!/usr/bin/env bash
echo "gh-user"
EOF
chmod +x "${TMPDIR_TEST}/mocks/gh"
OUT=$(PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" bash "$SSHD_SETUP" _github_user 2>&1)
assert_eq "$OUT" "gh-user" "gh api user wins precedence"

# gh absent -> origin owner (https)
rm -f "${TMPDIR_TEST}/mocks/gh"
cat > "${TMPDIR_TEST}/mocks/git" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  "remote get-url origin") echo "https://github.com/origin-owner/repo.git" ;;
  "config github.user") exit 1 ;;
esac
EOF
chmod +x "${TMPDIR_TEST}/mocks/git"
OUT=$(PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" bash "$SSHD_SETUP" _github_user 2>&1)
assert_eq "$OUT" "origin-owner" "falls back to origin owner (https URL)"

# ssh-style URL parses too
cat > "${TMPDIR_TEST}/mocks/git" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  "remote get-url origin") echo "git@github.com:ssh-owner/repo.git" ;;
  "config github.user") exit 1 ;;
esac
EOF
chmod +x "${TMPDIR_TEST}/mocks/git"
OUT=$(PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" bash "$SSHD_SETUP" _github_user 2>&1)
assert_eq "$OUT" "ssh-owner" "falls back to origin owner (ssh URL)"

# nothing available -> exit 1
cat > "${TMPDIR_TEST}/mocks/git" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "${TMPDIR_TEST}/mocks/git"
set +e
PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" bash "$SSHD_SETUP" _github_user >/dev/null 2>&1; RC=$?
set -e
assert_eq "$RC" "1" "no derivation source exits 1"

# === Task 4: key seeding (refuse-empty, idempotent) ===
# curl mock serves the file named in SSHD_SETUP_KEYS_URL (last arg)
cat > "${TMPDIR_TEST}/mocks/curl" <<'EOF'
#!/usr/bin/env bash
for a in "$@"; do :; done
cat "$a"
EOF
chmod +x "${TMPDIR_TEST}/mocks/curl"
printf 'ssh-ed25519 AAAA-test-key-1\nssh-ed25519 AAAA-test-key-2\n' > "${TMPDIR_TEST}/keys_fixture"
FAKE_HOME="${TMPDIR_TEST}/home"; mkdir -p "$FAKE_HOME"
cat > "${TMPDIR_TEST}/mocks/gh" <<'EOF'
#!/usr/bin/env bash
echo "fixture-user"
EOF
chmod +x "${TMPDIR_TEST}/mocks/gh"

PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" HOME="$FAKE_HOME" \
  SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture" bash "$SSHD_SETUP" keys >/dev/null
assert_eq "$(wc -l < "${FAKE_HOME}/.ssh/authorized_keys" | tr -d ' ')" "2" "first keys run adds 2 lines"
# idempotent: run again, still 2
PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" HOME="$FAKE_HOME" \
  SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture" bash "$SSHD_SETUP" keys >/dev/null
assert_eq "$(wc -l < "${FAKE_HOME}/.ssh/authorized_keys" | tr -d ' ')" "2" "second keys run adds nothing"
# refuse-empty: empty fixture -> exit 1, file untouched
: > "${TMPDIR_TEST}/keys_fixture"
set +e
PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" HOME="$FAKE_HOME" \
  SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture" bash "$SSHD_SETUP" keys >/dev/null 2>&1; RC=$?
set -e
assert_eq "$RC" "1" "empty keys response exits 1"
assert_eq "$(wc -l < "${FAKE_HOME}/.ssh/authorized_keys" | tr -d ' ')" "2" "authorized_keys untouched on empty response"

# === Task 5: enable — native-first, sshd.env, dry-run ===
# native-first: sshd already running -> no pkg-install call
cat > "${TMPDIR_TEST}/mocks/systemctl" <<'EOF'
#!/usr/bin/env bash
[ "$1" = "is-active" ] && exit 0
exit 0
EOF
chmod +x "${TMPDIR_TEST}/mocks/systemctl"
cat > "${TMPDIR_TEST}/mocks/pkg-install" <<'EOF'
#!/usr/bin/env bash
echo "PKG-INSTALL-CALLED" >> "${PKG_LOG:?}"
EOF
chmod +x "${TMPDIR_TEST}/mocks/pkg-install"
printf 'ssh-ed25519 AAAA-k\n' > "${TMPDIR_TEST}/keys_fixture2"
PKG_LOG="${TMPDIR_TEST}/pkg.log"; : > "$PKG_LOG"
PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" HOME="$FAKE_HOME" PKG_LOG="$PKG_LOG" \
  SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture2" bash "$SSHD_SETUP" enable >/dev/null 2>&1
assert_eq "$(cat "$PKG_LOG")" "" "enable skips pkg-install when sshd running"
assert_grep "sshd.env written with SSHD_LOGIN" "SSHD_LOGIN=true" "${FAKE_HOME}/.sshd.env"

# dry-run mutates nothing: fresh HOME stays empty
DRY_HOME="${TMPDIR_TEST}/dryhome"; mkdir -p "$DRY_HOME"
PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" HOME="$DRY_HOME" \
  SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture2" bash "$SSHD_SETUP" --dry-run enable >/dev/null 2>&1
assert_exit_code 1 "dry-run wrote no sshd.env" test -f "${DRY_HOME}/.sshd.env"
assert_exit_code 1 "dry-run wrote no authorized_keys" test -f "${DRY_HOME}/.ssh/authorized_keys"

_test_report
