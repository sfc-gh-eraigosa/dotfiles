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
# Mock the package manager (apt-get) and sudo so we can observe install calls
# without touching the real system. sudo just runs its remaining args.
cat > "${TMPDIR_TEST}/mocks/sudo" <<'EOF'
#!/usr/bin/env bash
exec "$@"
EOF
chmod +x "${TMPDIR_TEST}/mocks/sudo"
cat > "${TMPDIR_TEST}/mocks/apt-get" <<'EOF'
#!/usr/bin/env bash
echo "APT-GET $*" >> "${PKG_LOG:?}"
EOF
chmod +x "${TMPDIR_TEST}/mocks/apt-get"
printf 'ssh-ed25519 AAAA-k\n' > "${TMPDIR_TEST}/keys_fixture2"

# stub gff: logs every call; `list` serves $GFF_LIST_FILE; `version` ok.
cat > "${TMPDIR_TEST}/mocks/gff" <<'EOF'
#!/usr/bin/env bash
echo "$*" >> "${GFF_LOG:?}"
case "$1" in
  version) exit 0 ;;
  list) [ -f "${GFF_LIST_FILE:-/nonexistent}" ] && cat "$GFF_LIST_FILE"; exit 0 ;;
  get) echo "${GFF_GET_VALUE:-true}"; exit 0 ;;
esac
exit 0
EOF
chmod +x "${TMPDIR_TEST}/mocks/gff"
export GFF_LOG="${TMPDIR_TEST}/gff.log"; : > "$GFF_LOG"

# native-first: sshd already running -> no apt-get install call
cat > "${TMPDIR_TEST}/mocks/systemctl" <<'EOF'
#!/usr/bin/env bash
[ "$1" = "is-active" ] && exit 0
exit 0
EOF
chmod +x "${TMPDIR_TEST}/mocks/systemctl"
PKG_LOG="${TMPDIR_TEST}/pkg.log"; : > "$PKG_LOG"
PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" HOME="$FAKE_HOME" PKG_LOG="$PKG_LOG" \
  GFF_BIN="${TMPDIR_TEST}/mocks/gff" \
  SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture2" bash "$SSHD_SETUP" enable >/dev/null 2>&1
assert_eq "$(cat "$PKG_LOG")" "" "enable skips install when sshd already running"
assert_grep "enable records the gff flag" "set install.network.sshd true" "$GFF_LOG"
assert_exit_code 1 "enable writes no ~/.sshd.env" test -f "${FAKE_HOME}/.sshd.env"

# NOT running + NOT installed -> installs openssh-server via apt-get
cat > "${TMPDIR_TEST}/mocks/systemctl" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  is-active) exit 3 ;;
  list-unit-files) echo "ssh.service enabled"; exit 0 ;;
  *) exit 0 ;;
esac
EOF
chmod +x "${TMPDIR_TEST}/mocks/systemctl"
cat > "${TMPDIR_TEST}/mocks/pgrep" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "${TMPDIR_TEST}/mocks/pgrep"
: > "$PKG_LOG"
INST_HOME="${TMPDIR_TEST}/insthome"; mkdir -p "$INST_HOME"
PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" HOME="$INST_HOME" PKG_LOG="$PKG_LOG" \
  GFF_BIN="${TMPDIR_TEST}/mocks/gff" \
  SSHD_SETUP_SSHD_PATH=/nonexistent SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture2" \
  bash "$SSHD_SETUP" enable >/dev/null 2>&1
case "$(cat "$PKG_LOG")" in
  *"install -y openssh-server"*) echo "PASS: enable installs openssh-server when absent"; PASS=$((PASS+1));;
  *) echo "FAIL: enable did not apt-get install openssh-server (got: $(cat "$PKG_LOG"))"; FAIL=$((FAIL+1));;
esac
# restore a "running" systemctl + present pgrep for any later cases
cat > "${TMPDIR_TEST}/mocks/systemctl" <<'EOF'
#!/usr/bin/env bash
[ "$1" = "is-active" ] && exit 0
exit 0
EOF
chmod +x "${TMPDIR_TEST}/mocks/systemctl"
rm -f "${TMPDIR_TEST}/mocks/pgrep"

# dry-run mutates nothing: fresh HOME stays empty
DRY_HOME="${TMPDIR_TEST}/dryhome"; mkdir -p "$DRY_HOME"
PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" HOME="$DRY_HOME" \
  SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture2" bash "$SSHD_SETUP" --dry-run enable >/dev/null 2>&1
assert_exit_code 1 "dry-run wrote no sshd.env" test -f "${DRY_HOME}/.sshd.env"
: > "$GFF_LOG"
OUT=$(PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" HOME="$DRY_HOME" GFF_BIN="${TMPDIR_TEST}/mocks/gff" \
  SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture2" bash "$SSHD_SETUP" --dry-run enable 2>&1)
case "$OUT" in *"DRY: gff set install.network.sshd true"*) echo "PASS: dry-run prints gff set line"; PASS=$((PASS+1));;
  *) echo "FAIL: dry-run missing gff set line (got: $OUT)"; FAIL=$((FAIL+1));; esac
assert_eq "$(cat "$GFF_LOG")" "" "dry-run makes no gff calls"

# no usable gff -> exit 1 with the install.sh hint
set +e
OUT=$(PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" HOME="$DRY_HOME" GFF_BIN="${TMPDIR_TEST}/nogff" \
  SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture2" bash "$SSHD_SETUP" enable 2>&1); RC=$?
set -e
assert_eq "$RC" "1" "enable without gff exits 1"
case "$OUT" in *"sshd-setup: gff not found — run install.sh first"*) echo "PASS: missing gff hint"; PASS=$((PASS+1));;
  *) echo "FAIL: missing gff hint (got: $OUT)"; FAIL=$((FAIL+1));; esac

# WSL enable: nothing detected, WSLNET_ANSWER=nat -> records the mode, prints next step
echo "Linux version 6.6 microsoft-standard-WSL2" > "${TMPDIR_TEST}/proc_wsl"
WSL_HOME="${TMPDIR_TEST}/wslhome"; mkdir -p "$WSL_HOME"
: > "$GFF_LOG"; rm -f "${TMPDIR_TEST}/gff_list"
wsl_enable() {
  PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" HOME="$WSL_HOME" GFF_BIN="${TMPDIR_TEST}/mocks/gff" \
    SSHD_SETUP_PROC_VERSION="${TMPDIR_TEST}/proc_wsl" \
    WSLNET_WSLCONFIG="${TMPDIR_TEST}/none.wslconfig" WSLNET_NETSH="${TMPDIR_TEST}/nonetsh" \
    WSLNET_PS="${TMPDIR_TEST}/nops" WSLNET_K3S_BIN="${TMPDIR_TEST}/nok3s" WSLNET_ANSWER=nat \
    GFF_LIST_FILE="${TMPDIR_TEST}/gff_list" \
    SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture2" bash "$SSHD_SETUP" enable 2>&1
}
set +e; OUT=$(wsl_enable); RC=$?; set -e
assert_eq "$RC" "0" "WSL enable exits 0 (no set -u/-e abort)"
assert_grep "WSL enable records nat" "set install.windows.wsl-network nat" "$GFF_LOG"
case "$OUT" in *"Next: re-run install.sh to apply it on Windows."*) echo "PASS: WSL next-step line"; PASS=$((PASS+1));;
  *) echo "FAIL: WSL next-step line missing (got: $OUT)"; FAIL=$((FAIL+1));; esac

# re-run with the mode already set by this machine: no set, no prompt
printf 'install.windows.wsl-network choice nat user-override\n' > "${TMPDIR_TEST}/gff_list"
: > "$GFF_LOG"
set +e; OUT=$(wsl_enable); RC=$?; set -e
assert_eq "$RC" "0" "WSL re-run exits 0"
if grep -q "set install.windows.wsl-network" "$GFF_LOG"; then
  echo "FAIL: re-run re-set wsl-network"; FAIL=$((FAIL+1))
else echo "PASS: re-run leaves wsl-network alone"; PASS=$((PASS+1)); fi
case "$OUT" in *"How should this Windows host"*) echo "FAIL: re-run prompted"; FAIL=$((FAIL+1));;
  *) echo "PASS: re-run did not prompt"; PASS=$((PASS+1));; esac

# status prints the flag, not sshd.env
OUT=$(PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" HOME="$DRY_HOME" GFF_BIN="${TMPDIR_TEST}/mocks/gff" GFF_GET_VALUE=true bash "$SSHD_SETUP" status 2>&1)
case "$OUT" in *"sshd flag: true"*) echo "PASS: status shows flag"; PASS=$((PASS+1));;
  *) echo "FAIL: status flag (got: $OUT)"; FAIL=$((FAIL+1));; esac
case "$OUT" in *"sshd.env"*) echo "FAIL: status still mentions sshd.env"; FAIL=$((FAIL+1));;
  *) echo "PASS: status drops sshd.env"; PASS=$((PASS+1));; esac
OUT=$(PATH="${TMPDIR_TEST}/mocks:${SANDBOX_PATH}" HOME="$DRY_HOME" GFF_BIN="${TMPDIR_TEST}/nogff" bash "$SSHD_SETUP" status 2>&1)
case "$OUT" in *"sshd flag: unknown (gff not found)"*) echo "PASS: status without gff"; PASS=$((PASS+1));;
  *) echo "FAIL: status without gff (got: $OUT)"; FAIL=$((FAIL+1));; esac
assert_exit_code 1 "dry-run wrote no authorized_keys" test -f "${DRY_HOME}/.ssh/authorized_keys"

_test_report
