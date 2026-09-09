#!/usr/bin/env bash
# Test driver for opt/profiles/.profile and opt/profiles/.zprofile
#
# Covers two regressions found on DGX Spark (see
# docs/mbo/designs/spark-gpu-support.md):
#
#   1. `.profile` used to `. /etc/environment`. That file is a pam_env(8)
#      key=value TABLE, not a shell script, and its `PATH="..."` line was
#      executed as an assignment — CLOBBERING the PATH that PAM and
#      /etc/profile.d/*.sh had already built. On Spark that silently
#      dropped /usr/local/cuda/bin (nvcc, ncu, cuda-gdb) from every login
#      shell. It now PARSES the table and never lets it overwrite PATH.
#
#   2. Ubuntu's stock /etc/zsh/zprofile is comment-only on some releases,
#      so zsh login shells never sourced /etc/profile and never ran any
#      /etc/profile.d drop-in. `.zprofile` now sources it explicitly.
#
# Plus the PATH dedupe that makes (2) safe to re-source.
#
# The env-file loop reads $DOTFILES_ENV_FILE (default /etc/environment) so
# these cases run hermetically in CI, not just on a Spark.
#
# Run: bash opt/profiles/profile_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

PROFILE="${SELF_DIR}/.profile"
# Resolve sh absolutely: one case below starts with an EMPTY PATH, and
# `env -i` would otherwise be unable to find the interpreter itself.
SH_BIN="$(command -v sh)"
ZPROFILE="${SELF_DIR}/.zprofile"
TMPHOME=$(mktemp -d)
trap 'rm -rf "${TMPHOME}"' EXIT

# === syntax ===
assert_file_exists "${PROFILE}" ".profile exists"
assert_exit_code 0 ".profile parses with bash -n" bash -n "${PROFILE}"
if command -v dash >/dev/null 2>&1; then
    # The load-bearing one: dash is what `sh -l` actually runs on Debian/Ubuntu.
    assert_exit_code 0 ".profile parses with dash -n (POSIX)" dash -n "${PROFILE}"
fi
assert_file_exists "${ZPROFILE}" ".zprofile exists"
if command -v zsh >/dev/null 2>&1; then
    assert_exit_code 0 ".profile parses with zsh -n" zsh -n "${PROFILE}"
    assert_exit_code 0 ".zprofile parses with zsh -n" zsh -n "${ZPROFILE}"
fi

# Source .profile hermetically with a fixture env-file and a controlled PATH,
# then print one variable. Nothing from the host leaks in (env -i).
run_profile() {  # <env_file> <initial_PATH> <var_to_print>
    env -i HOME="${TMPHOME}" PATH="$2" TERM=dumb TERM_PROGRAM=vscode \
        DOTFILES_ENV_FILE="$1" \
        "${SH_BIN}" -c ". '${PROFILE}' >/dev/null 2>&1; printf '%s' \"\${$3:-}\""
}

# === /etc/environment is parsed, not sourced ===
ENVFILE="${TMPHOME}/environment"
cat > "${ENVFILE}" <<'EOF'
PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
LANG="en_US.UTF-8"
EDITOR=vim
export SOME_EXPORTED="yes"
# a comment
   INDENTED=ignored-fail-safe
NOT_AN_ASSIGNMENT
BAD-KEY=skipped
EOF

GOT_PATH=$(run_profile "${ENVFILE}" "/sentinel/bin:/usr/bin:/bin" PATH)
case ":${GOT_PATH}:" in
    *:/sentinel/bin:*) assert_eq "kept" "kept" "/etc/environment does NOT clobber PATH (sentinel survives)" ;;
    *) assert_eq "clobbered" "kept" "/etc/environment does NOT clobber PATH (sentinel survives)" ;;
esac

assert_eq "$(run_profile "${ENVFILE}" "/usr/bin:/bin" LANG)" "en_US.UTF-8" \
    "quoted value from /etc/environment is exported, quotes stripped"
assert_eq "$(run_profile "${ENVFILE}" "/usr/bin:/bin" EDITOR)" "vim" \
    "unquoted value from /etc/environment is exported"
assert_eq "$(run_profile "${ENVFILE}" "/usr/bin:/bin" SOME_EXPORTED)" "yes" \
    "leading 'export ' in /etc/environment is handled"
assert_eq "$(run_profile "${ENVFILE}" "/usr/bin:/bin" INDENTED)" "" \
    "indented line is skipped (fail-safe, not mis-parsed)"
assert_eq "$(run_profile "${ENVFILE}" "/usr/bin:/bin" NOT_AN_ASSIGNMENT)" "" \
    "line with no '=' is skipped"

# PATH fallback: only when we genuinely have none.
GOT_FALLBACK=$(run_profile "${ENVFILE}" "" PATH)
case ":${GOT_FALLBACK}:" in
    *:/usr/local/sbin:*) assert_eq "used" "used" "empty PATH falls back to /etc/environment's PATH" ;;
    *) assert_eq "unused" "used" "empty PATH falls back to /etc/environment's PATH" ;;
esac

# A missing env file must not break the profile.
assert_exit_code 0 "missing /etc/environment is tolerated" \
    env -i HOME="${TMPHOME}" PATH=/usr/bin:/bin TERM=dumb TERM_PROGRAM=vscode \
        DOTFILES_ENV_FILE="${TMPHOME}/does-not-exist" \
        "${SH_BIN}" -c ". '${PROFILE}' >/dev/null 2>&1"

# === PATH dedupe ===
DEDUPED=$(run_profile "${ENVFILE}" "/a:/b:/a:/c::/b:/a" PATH)
count_entry() { echo "$1" | tr ':' '\n' | grep -c "^$2\$"; }
assert_eq "$(count_entry "${DEDUPED}" "/a")" "1" "dedupe: /a appears exactly once"
assert_eq "$(count_entry "${DEDUPED}" "/b")" "1" "dedupe: /b appears exactly once"
assert_eq "$(count_entry "${DEDUPED}" "/c")" "1" "dedupe: /c appears exactly once"
assert_eq "$(echo "${DEDUPED}" | tr ':' '\n' | grep -c '^$')" "0" \
    "dedupe: no empty PATH field (implicit CWD) survives"
# First-occurrence order preserved: /a before /b before /c.
assert_eq "$(echo "${DEDUPED}" | tr ':' '\n' | grep -n '^/[abc]$' | sed 's/:.*//' | tr '\n' ' ' | \
            { read -r i j k _; [ "$i" -lt "$j" ] && [ "$j" -lt "$k" ] && echo ordered || echo unordered; })" \
    "ordered" "dedupe: first-occurrence order is preserved"

# Dedupe must be idempotent — a re-sourced profile (tmux, `su -`) stays clean.
assert_eq "$(echo "${DEDUPED}" | tr ':' '\n' | sort | uniq -d | grep -c . || true)" "0" \
    "dedupe: resulting PATH has no duplicates at all"

# === .zprofile sources /etc/profile (so profile.d drop-ins reach zsh) ===
assert_grep ".zprofile sources /etc/profile for profile.d drop-ins" \
    '^\[ -r /etc/profile \] && emulate sh -c' "${ZPROFILE}"

_test_report
