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

# === 7. manifest: Linux ships an X11 clipboard tool ===
# herdr's Linux clipboard path is wl-copy -> xclip -> xsel, and only THEN the
# OSC 52 escape -- which gnome-terminal/VTE silently drops (VTE #2495). Without
# one of these installed every herdr mouse-copy shows a "copied" toast while the
# clipboard keeps its old contents (herdr #2399). .tmux.conf's copy-pipe binds
# also assume xsel. Both must stay in the APT column; macOS has pbcopy.
MANIFEST="${REPO_ROOT}/opt/profiles/packages.tsv"
for tool in xclip xsel; do
    if awk '!/^[[:space:]]*(#|$)/ {print $2}' "$MANIFEST" | grep -qx "$tool"; then
        echo "PASS: packages.tsv installs $tool on apt hosts"
        PASS=$((PASS + 1))
    else
        echo "FAIL: packages.tsv APT column is missing $tool (herdr/tmux clipboard)"
        FAIL=$((FAIL + 1))
    fi
done

# === 8. pkg-install-apt: sudo / already-installed behavior ===
# Regression: when sudo could not authenticate (no tty, no cached credential)
# the grouped install failed and the script retried EVERY package on its own
# (1 update + 1 grouped + 32 single sudo failures per run). When sudo IS usable
# the full list must still go to one grouped `apt-get install` every run: that
# upgrades already-installed packages (gh stays on the pinned upstream release).
# The SUT is copied into a scratch tree so its sibling setup_gh_apt_repo.sh is a
# logging stub, and sudo/apt-get/dpkg-query/id are PATH stubs — nothing
# touches the real system.
APT_T="${TMPDIR_TEST}/apt"
APT_STUBS="${APT_T}/stubs"
mkdir -p "${APT_T}/opt/bin" "${APT_T}/opt/scripts/system" "${APT_STUBS}"
cp "$PKG_INSTALL_APT" "${APT_T}/opt/bin/pkg-install-apt"
chmod +x "${APT_T}/opt/bin/pkg-install-apt"
cat > "${APT_T}/opt/scripts/system/setup_gh_apt_repo.sh" <<'EOF'
#!/usr/bin/env bash
echo "gh-repo-setup" >> "$STUB_LOG"
EOF
chmod +x "${APT_T}/opt/scripts/system/setup_gh_apt_repo.sh"
printf '%s\n' '# brew apt' 'jq jq' 'htop htop' 'tmux tmux' 'mas -' > "${APT_T}/manifest.tsv"

# id -u: FAKE_UID (default non-root), so a CI container running as root
# still exercises the non-root sudo paths.
cat > "${APT_STUBS}/id" <<'EOF'
#!/usr/bin/env bash
[ "${1:-}" = "-u" ] && { echo "${FAKE_UID:-1000}"; exit 0; }
exec /usr/bin/id "$@"
EOF
# sudo: FAKE_SUDO_CACHED=1 is "credential cached / NOPASSWD"; otherwise every
# call fails the way sudo does with no tty to prompt on.
cat > "${APT_STUBS}/sudo" <<'EOF'
#!/usr/bin/env bash
echo "sudo $*" >> "$STUB_LOG"
[ "${1:-}" = "-n" ] && shift
[ "${FAKE_SUDO_CACHED:-0}" = "1" ] || { echo "sudo: a password is required" >&2; exit 1; }
exec "$@"
EOF
# apt-get: FAKE_APT_GROUP_FAIL=1 fails any install of more than one package
# (apt is all-or-nothing, so one bad name sinks the whole batch).
cat > "${APT_STUBS}/apt-get" <<'EOF'
#!/usr/bin/env bash
echo "apt-get $*" >> "$STUB_LOG"
n=0; seen_install=0
for a in "$@"; do
  case "$a" in
    install) seen_install=1 ;;
    -*) ;;
    *) [ "$seen_install" = "1" ] && n=$((n + 1)) ;;
  esac
done
[ "${FAKE_APT_GROUP_FAIL:-0}" = "1" ] && [ "$n" -gt 1 ] && exit 100
exit 0
EOF
# dpkg-query -W -f='${Status}' <pkg>: installed iff <pkg> is in FAKE_INSTALLED.
cat > "${APT_STUBS}/dpkg-query" <<'EOF'
#!/usr/bin/env bash
for pkg in "$@"; do :; done
case " ${FAKE_INSTALLED:-} " in
  *" ${pkg} "*) printf 'install ok installed'; exit 0 ;;
esac
echo "dpkg-query: no packages found matching ${pkg}" >&2
exit 1
EOF
chmod +x "${APT_STUBS}"/*

# _apt_run [VAR=val ...] — run the SUT with no tty on stdin; sets APT_OUT/APT_RC.
_apt_run() {
    : > "${APT_T}/stublog"
    APT_RC=0
    APT_OUT="$(env PATH="${APT_STUBS}:/usr/bin:/bin" STUB_LOG="${APT_T}/stublog" \
        PKG_MANIFEST="${APT_T}/manifest.tsv" "$@" \
        bash "${APT_T}/opt/bin/pkg-install-apt" </dev/null 2>&1)" || APT_RC=$?
}
_apt_log_count() { grep -c -- "$1" "${APT_T}/stublog" || true; }
# Every sudo call except the non-interactive `sudo -n true` probe.
_apt_sudo_cmds() { grep -- '^sudo' "${APT_T}/stublog" | grep -vc -- '^sudo -n true$' || true; }
_apt_install_line() { grep -- '^apt-get .*install' "${APT_T}/stublog" | head -n 1; }
_apt_has_word() { case " $1 " in *" $2 "*) echo yes ;; *) echo no ;; esac; }

# 8a. All installed + sudo unusable: a plain info line, no WARNING, no apt calls.
_apt_run FAKE_INSTALLED="jq htop tmux" FAKE_SUDO_CACHED=0
assert_eq "$APT_RC" "0" "all installed, no sudo: exits 0"
assert_eq "$(_apt_sudo_cmds)" "0" "all installed, no sudo: no sudo command beyond the -n probe"
assert_eq "$(_apt_log_count '^apt-get')" "0" "all installed, no sudo: never calls apt-get (not even update)"
assert_eq "$(_apt_log_count '^gh-repo-setup')" "0" "all installed, no sudo: skips the gh apt repo setup"
assert_eq "$(printf '%s\n' "$APT_OUT" | grep -c 'WARNING')" "0" "all installed, no sudo: no WARNING"
case "$APT_OUT" in *"all 3 packages installed; no sudo in this session"*) r=0 ;; *) r=1 ;; esac
assert_eq "$r" "0" "all installed, no sudo: says why the upgrade check is skipped"

# 8a2. All installed + sudo usable: update + grouped install of the FULL list
# still runs — that is the upgrade path.
_apt_run FAKE_INSTALLED="jq htop tmux" FAKE_SUDO_CACHED=1
assert_eq "$APT_RC" "0" "all installed, sudo usable: exits 0"
assert_eq "$(_apt_log_count '^gh-repo-setup')" "1" "all installed, sudo usable: runs the gh apt repo setup"
assert_eq "$(_apt_log_count '^apt-get update')" "1" "all installed, sudo usable: refreshes the package lists"
line="$(_apt_install_line)"
assert_eq "$(_apt_has_word "$line" jq)$(_apt_has_word "$line" htop)$(_apt_has_word "$line" tmux)" "yesyesyes" \
    "all installed, sudo usable: grouped install of the full list (upgrades)"
assert_eq "$(_apt_log_count '^sudo env DEBIAN_FRONTEND=noninteractive apt-get install')" "1" \
    "all installed, sudo usable: installs via sudo env DEBIAN_FRONTEND=noninteractive"

# 8b. Some missing + sudo unusable: ONE warning listing only the missing, zero apt calls.
_apt_run FAKE_INSTALLED="htop" FAKE_SUDO_CACHED=0
assert_eq "$APT_RC" "0" "sudo unavailable: exits 0 (non-fatal, like the other failure paths)"
assert_eq "$(_apt_log_count '^apt-get')" "0" "sudo unavailable: zero apt-get calls"
assert_eq "$(_apt_sudo_cmds)" "0" "sudo unavailable: no sudo command beyond the -n probe"
assert_eq "$(_apt_log_count '^gh-repo-setup')" "0" "sudo unavailable: skips the gh apt repo setup"
assert_eq "$(printf '%s\n' "$APT_OUT" | grep -c 'WARNING')" "1" "sudo unavailable: exactly one WARNING"
case "$APT_OUT" in *"sudo needs a password and there is no terminal"*"jq tmux"*) r=0 ;; *) r=1 ;; esac
assert_eq "$r" "0" "sudo unavailable: the warning explains why and lists the missing packages"
case "$APT_OUT" in *htop*) r=1 ;; *) r=0 ;; esac
assert_eq "$r" "0" "sudo unavailable: the already-installed package is not listed"
case "$APT_OUT" in *"retrying packages individually"*) r=1 ;; *) r=0 ;; esac
assert_eq "$r" "0" "sudo unavailable: no per-package retry storm"

# 8c. Some missing + sudo usable: the FULL list is installed, not just the missing.
_apt_run FAKE_INSTALLED="jq" FAKE_SUDO_CACHED=1
assert_eq "$APT_RC" "0" "some missing, sudo usable: exits 0"
line="$(_apt_install_line)"
assert_eq "$(_apt_has_word "$line" jq)$(_apt_has_word "$line" htop)$(_apt_has_word "$line" tmux)" "yesyesyes" \
    "some missing, sudo usable: grouped install of the full list"
assert_eq "$(_apt_log_count '^apt-get update')" "1" "some missing, sudo usable: refreshes the package lists once"

# 8d. Grouped install fails with WORKING sudo: the per-package retry still runs.
_apt_run FAKE_INSTALLED="" FAKE_SUDO_CACHED=1 FAKE_APT_GROUP_FAIL=1
assert_eq "$APT_RC" "0" "grouped failure: exits 0"
assert_eq "$(_apt_log_count '^apt-get .*install')" "4" \
    "grouped failure: 1 grouped + 3 single-package installs"
case "$APT_OUT" in *"retrying packages individually"*) r=0 ;; *) r=1 ;; esac
assert_eq "$r" "0" "grouped failure: announces the retry"

# 8e. Running as root: apt-get directly, no sudo needed (or called).
_apt_run FAKE_UID=0 FAKE_INSTALLED="" FAKE_SUDO_CACHED=0
assert_eq "$APT_RC" "0" "root: exits 0"
assert_eq "$(_apt_log_count '^sudo')" "0" "root: never calls sudo"
assert_eq "$(_apt_log_count '^apt-get .*install')" "1" "root: installs the package list directly"

_test_report
