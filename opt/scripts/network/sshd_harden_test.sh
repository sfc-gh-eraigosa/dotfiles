#!/usr/bin/env bash
# Test driver for opt/scripts/network/sshd_harden.sh (gff install.network.sshd-hardening).
#
# What must hold:
#   * no sshd_config, or one without an `Include sshd_config.d`  -> SKIP, exit 0, nothing written;
#   * no authorized key for the invoking user                     -> REFUSED, exit 2, nothing written
#     (the lock-out guard: passwords go off only when a key can get back in);
#   * apply                                                       -> 10-dotfiles-hardening.conf with the
#     directives, `sshd -t` before the reload, one reload;
#   * apply again                                                 -> "already", no reload;
#   * `sshd -t` fails                                             -> exit 1, the drop-in is rolled back,
#     no reload;
#   * a foreign file at the drop-in path                          -> backed up before it is replaced;
#   * effective `sshd -T` still says passwordauthentication yes   -> WARNING (something earlier wins);
#   * SSHD_ALLOW_USERS overrides the allow-list; `-` omits it;
#   * --dry-run prints, writes nothing; --status reads only; --uninstall removes + reloads.
#
# Nothing touches the host: SSHD_HARDEN_ETC points at a sandbox, SSHD_HARDEN_SUDO is
# empty, and sshd / systemctl are recording fakes.
#
# Run: bash opt/scripts/network/sshd_harden_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/sshd_harden.sh"
assert_file_exists "${SCRIPT}" "sshd_harden.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
assert_grep_negative "no hardcoded home path" '/home/[a-z]|/Users/[a-z]' "${SCRIPT}"
set +e

TMP="$(mktemp -d "${TMPDIR:-/tmp}/sshd_harden_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
LOG="${TMP}/calls.log"

# Recording fakes.
FAKES="${TMP}/fakes"; mkdir -p "${FAKES}"
cat > "${FAKES}/sshd" <<'FAKE'
#!/bin/sh
echo "sshd $*" >> "${FAKE_LOG}"
case "$1" in
  -t) exit "${FAKE_SSHD_T_FAIL:-0}" ;;
  -T) if [ -n "${FAKE_SSHD_T_OUT:-}" ]; then cat "${FAKE_SSHD_T_OUT}"; else
        printf 'passwordauthentication no\npermitrootlogin no\nkbdinteractiveauthentication no\nmaxauthtries 3\n'; fi ;;
esac
FAKE
cat > "${FAKES}/systemctl" <<'FAKE'
#!/bin/sh
echo "systemctl $*" >> "${FAKE_LOG}"
case "$1" in is-active) exit 0 ;; esac
FAKE
chmod +x "${FAKES}"/*

# fresh <name> -> a sandbox /etc/ssh with an Include line and a keyed home.
fresh() {
    ETC="${TMP}/$1/etc/ssh"; HOMEDIR="${TMP}/$1/home"
    mkdir -p "${ETC}/sshd_config.d" "${HOMEDIR}/.ssh"
    printf '# sandbox sshd_config\nInclude %s/sshd_config.d/*.conf\nSubsystem sftp internal-sftp\n' "${ETC}" > "${ETC}/sshd_config"
    printf 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFAKEFAKEFAKEFAKEFAKEFAKEFAKEFAKEFAKEFAKEFAKE test@fixture\n' > "${HOMEDIR}/.ssh/authorized_keys"
    : > "${LOG}"
}
DROPIN() { echo "${ETC}/sshd_config.d/10-dotfiles-hardening.conf"; }

run() {  # run [VAR=value ...] [-- args...]
    env -u SSHD_ALLOW_USERS -u FAKE_SSHD_T_FAIL -u FAKE_SSHD_T_OUT \
        HOME="${HOMEDIR}" FAKE_LOG="${LOG}" \
        SSHD_HARDEN_ETC="${ETC}" SSHD_HARDEN_SUDO="" \
        SSHD_HARDEN_SSHD="${FAKES}/sshd" SSHD_HARDEN_SYSTEMCTL="${FAKES}/systemctl" \
        "$@" 2>&1
}

# 1. No sshd_config at all -> SKIP, exit 0, nothing written.
fresh noconf; rm "${ETC}/sshd_config"
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "no sshd_config: exit 0"
assert_in_subshell "no sshd_config: says SKIP" "printf '%s' '${out//\'/}' | grep -q 'SKIP'"
[ ! -e "$(DROPIN)" ] && echo "PASS: no sshd_config: nothing written" || echo "FAIL: no sshd_config: wrote a drop-in"

# 2. sshd_config without an Include -> SKIP (a drop-in would be ignored).
fresh noinclude; printf 'PasswordAuthentication yes\n' > "${ETC}/sshd_config"
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "no Include: exit 0"
assert_in_subshell "no Include: says SKIP and names Include" "printf '%s' '${out//\'/}' | grep -q 'SKIP.*Include'"
[ ! -e "$(DROPIN)" ] && echo "PASS: no Include: nothing written" || echo "FAIL: no Include: wrote a drop-in"

# 3. No authorized key -> REFUSED, exit 2, nothing written (the lock-out guard).
fresh nokey; : > "${HOMEDIR}/.ssh/authorized_keys"
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "2" "no authorized key: exit 2"
assert_in_subshell "no authorized key: names authorized_keys" "printf '%s' '${out//\'/}' | grep -q 'authorized_keys'"
[ ! -e "$(DROPIN)" ] && echo "PASS: no authorized key: nothing written" || echo "FAIL: no authorized key: wrote a drop-in"
assert_grep_negative "no authorized key: no reload" 'systemctl reload' "${LOG}"

# 4. Apply -> the drop-in, sshd -t first, one reload.
fresh apply
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "apply: exit 0"
assert_file_exists "$(DROPIN)" "apply: drop-in written"
for d in 'PasswordAuthentication no' 'KbdInteractiveAuthentication no' 'PermitRootLogin no' 'PermitEmptyPasswords no' 'PubkeyAuthentication yes' 'MaxAuthTries 3' 'LoginGraceTime 30'; do
    assert_grep "apply: ${d}" "^${d}\$" "$(DROPIN)"
done
assert_grep "apply: AllowUsers is the invoking user" "^AllowUsers $(id -un)\$" "$(DROPIN)"
assert_grep "apply: the file says it is managed" 'dotfiles' "$(DROPIN)"
assert_grep "apply: sshd -t ran" '^sshd -t' "${LOG}"
assert_in_subshell "apply: sshd -t ran BEFORE the reload" "[ \"\$(grep -n -m1 'sshd -t' '${LOG}' | cut -d: -f1)\" -lt \"\$(grep -n -m1 'systemctl reload' '${LOG}' | cut -d: -f1)\" ]"
assert_eq "$(grep -c 'systemctl reload' "${LOG}")" "1" "apply: exactly one reload"
assert_in_subshell "apply: reports the effective values" "printf '%s' '${out//\'/}' | grep -q 'passwordauthentication no'"

# 5. Apply again -> already, no reload.
: > "${LOG}"
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "re-apply: exit 0"
assert_in_subshell "re-apply: says already" "printf '%s' '${out//\'/}' | grep -qi 'already'"
assert_grep_negative "re-apply: no reload" 'systemctl reload' "${LOG}"

# 6. sshd -t fails -> exit 1, drop-in rolled back, no reload.
fresh badconf
out="$(run FAKE_SSHD_T_FAIL=1 bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "1" "sshd -t fails: exit 1"
[ ! -e "$(DROPIN)" ] && echo "PASS: sshd -t fails: drop-in rolled back" || echo "FAIL: sshd -t fails: drop-in left in place"
assert_grep_negative "sshd -t fails: no reload" 'systemctl reload' "${LOG}"

# 7. A foreign file at the drop-in path is backed up, then replaced; a failed -t restores it.
fresh foreign; printf 'PasswordAuthentication yes\n' > "$(DROPIN)"
out="$(run bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "foreign drop-in: exit 0"
assert_in_subshell "foreign drop-in: a .bak copy exists" "ls '${ETC}/sshd_config.d/' | grep -q '10-dotfiles-hardening.conf.bak-'"
assert_grep "foreign drop-in: replaced" '^PasswordAuthentication no$' "$(DROPIN)"
fresh foreignfail; printf 'PasswordAuthentication yes\n' > "$(DROPIN)"
out="$(run FAKE_SSHD_T_FAIL=1 bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "1" "foreign + sshd -t fails: exit 1"
assert_grep "foreign + sshd -t fails: the old file is back" '^PasswordAuthentication yes$' "$(DROPIN)"

# 8. Effective config still says yes -> WARNING, exit 0.
fresh override; printf 'passwordauthentication yes\npermitrootlogin no\n' > "${TMP}/tout"
out="$(run FAKE_SSHD_T_OUT="${TMP}/tout" bash "${SCRIPT}")"; rc=$?
assert_eq "${rc}" "0" "override: exit 0"
assert_in_subshell "override: WARNING names passwordauthentication" "printf '%s' '${out//\'/}' | grep -q 'WARNING.*passwordauthentication'"

# 9. SSHD_ALLOW_USERS: a list, and `-` for none.
fresh allow
run SSHD_ALLOW_USERS="alice bob" bash "${SCRIPT}" >/dev/null
assert_grep "SSHD_ALLOW_USERS list" '^AllowUsers alice bob$' "$(DROPIN)"
fresh allownone
run SSHD_ALLOW_USERS="-" bash "${SCRIPT}" >/dev/null
assert_grep_negative "SSHD_ALLOW_USERS=- omits AllowUsers" '^AllowUsers' "$(DROPIN)"

# 10. --dry-run prints and writes nothing; --status reads only; --uninstall removes + reloads.
fresh dry
out="$(run bash "${SCRIPT}" --dry-run)"; rc=$?
assert_eq "${rc}" "0" "dry-run: exit 0"
assert_in_subshell "dry-run: prints the rendered file" "printf '%s' '${out//\'/}' | grep -q '^PasswordAuthentication no'"
[ ! -e "$(DROPIN)" ] && echo "PASS: dry-run: nothing written" || echo "FAIL: dry-run: wrote a drop-in"
assert_grep_negative "dry-run: no reload" 'systemctl reload' "${LOG}"
fresh status; printf 'passwordauthentication yes\npermitrootlogin without-password\n' > "${TMP}/tout-status"
out="$(run FAKE_SSHD_T_OUT="${TMP}/tout-status" bash "${SCRIPT}" --status)"; rc=$?
assert_eq "${rc}" "0" "status: exit 0"
assert_in_subshell "status: shows effective values" "printf '%s' '${out//\'/}' | grep -q 'passwordauthentication'"
assert_in_subshell "status: says not applied" "printf '%s' '${out//\'/}' | grep -qi 'not applied'"
assert_in_subshell "status: no 'wins' WARNING while not applied (nothing to be overridden yet)" "! printf '%s' '${out//\'/}' | grep -q 'WARNING'"
[ ! -e "$(DROPIN)" ] && echo "PASS: status: nothing written" || echo "FAIL: status: wrote a drop-in"
fresh uninstall; run bash "${SCRIPT}" >/dev/null; : > "${LOG}"
out="$(run bash "${SCRIPT}" --uninstall)"; rc=$?
assert_eq "${rc}" "0" "uninstall: exit 0"
[ ! -e "$(DROPIN)" ] && echo "PASS: uninstall: drop-in removed" || echo "FAIL: uninstall: drop-in still there"
assert_eq "$(grep -c 'systemctl reload' "${LOG}")" "1" "uninstall: one reload"

_test_report
