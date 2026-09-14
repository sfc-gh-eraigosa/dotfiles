#!/usr/bin/env bash
# Test driver for opt/profiles/.locale.sh (locale_fallback)
#
# What must hold (fleet history 2026-09-13 + the #326 review):
#   * a forwarded LC_ALL the host lacks is dropped and a working LANG is kept
#     (the WSL host: LC_ALL=en_US.UTF-8, only C.UTF-8 generated);
#   * a bad LC_CTYPE alone is dropped and a working LANG is kept (macOS
#     Terminal's LC_CTYPE=UTF-8 over ssh);
#   * a bad LANG falls back to C.UTF-8;
#   * a working locale is left alone, silently; -q never prints;
#   * it behaves the same under bash, dash (the Pi's GUI login reads .profile)
#     and zsh — the three shells that source it;
#   * install.sh, .profile, .bashrc and .zshrc all call it.
#
# `locale` is a stub that behaves like glibc's on a host with only C, C.utf8
# and POSIX: an unknown LC_ALL, LC_CTYPE or LANG makes it complain on stderr.
#
# Run: bash opt/profiles/.locale_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

LIB="${SELF_DIR}/.locale.sh"
assert_file_exists "${LIB}" ".locale.sh exists"
assert_exit_code 0 "parses under bash -n" bash -n "${LIB}"
assert_exit_code 0 "parses under dash -n (sourced by .profile at GUI login)" dash -n "${LIB}"
set +e

TMP="$(mktemp -d "${TMPDIR:-/tmp}/locale_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
cat > "${TMP}/locale" <<'STUB'
#!/bin/sh
ok() { case "$1" in "" | C | POSIX | C.UTF-8 | C.utf8) return 0 ;; esac; return 1; }
if [ "${1:-}" = "-a" ]; then printf 'C\nC.utf8\nPOSIX\n'; exit 0; fi
ok "${LC_ALL:-}" || echo "locale: Cannot set LC_ALL to default locale: No such file or directory" >&2
ok "${LC_CTYPE:-}" || echo "locale: Cannot set LC_CTYPE to default locale: No such file or directory" >&2
ok "${LANG:-}" || echo "locale: Cannot set LC_CTYPE to default locale: No such file or directory" >&2
echo "LANG=${LANG:-}"
STUB
chmod +x "${TMP}/locale"

# run <shell> [-q] <env assignments...> -> message lines, then
# "LANG=<v> LC_ALL=<v> LC_CTYPE=<v>"
run() {
    _sh="$1"
    shift
    _q=""
    if [ "$1" = "-q" ]; then _q="-q"; shift; fi
    # zsh must run as sh-compatible here only for the stub-free parts; the
    # function itself is sourced exactly as .zshrc does.
    env -i PATH="${TMP}:/usr/bin:/bin" "$@" "${_sh}" -c \
        '. "$1"; locale_fallback $2; printf "LANG=%s LC_ALL=%s LC_CTYPE=%s\n" "${LANG:-}" "${LC_ALL:-}" "${LC_CTYPE:-}"' \
        _ "${LIB}" "${_q}" </dev/null 2>&1
}

for SH in bash dash zsh; do
    command -v "${SH}" >/dev/null 2>&1 || { echo "SKIP: ${SH} not installed"; continue; }

    out="$(run "${SH}" LANG=C.UTF-8 LC_ALL=en_US.UTF-8)"
    assert_eq "$(printf '%s\n' "${out}" | tail -1)" "LANG=C.UTF-8 LC_ALL= LC_CTYPE=" \
        "${SH}: an unavailable LC_ALL is dropped, the working LANG kept"
    assert_eq "$(printf '%s\n' "${out}" | grep -c 'dropped LC_ALL=en_US.UTF-8, using LANG=C.UTF-8')" "1" \
        "${SH}: the message names exactly what was dropped"

    out="$(run "${SH}" LANG=POSIX LC_CTYPE=UTF-8)"
    assert_eq "$(printf '%s\n' "${out}" | tail -1)" "LANG=POSIX LC_ALL= LC_CTYPE=" \
        "${SH}: a bad LC_CTYPE alone is dropped, LANG kept (macOS client)"

    out="$(run "${SH}" LANG=en_US.UTF-8)"
    assert_eq "$(printf '%s\n' "${out}" | tail -1)" "LANG=C.UTF-8 LC_ALL= LC_CTYPE=" \
        "${SH}: an unavailable LANG falls back to C.UTF-8"

    out="$(run "${SH}" LANG=C.UTF-8)"
    assert_eq "${out}" "LANG=C.UTF-8 LC_ALL= LC_CTYPE=" "${SH}: a working locale is left alone, silently"

    out="$(run "${SH}" -q LANG=C.UTF-8 LC_ALL=en_US.UTF-8)"
    assert_eq "${out}" "LANG=C.UTF-8 LC_ALL= LC_CTYPE=" "${SH}: -q fixes it without a word"
done

# No locale binary at all (minimal containers): a quiet no-op.
out="$(env -i PATH=/nonexistent LANG=bogus /bin/sh -c '. "$1"; locale_fallback; echo "rc=$?"' _ "${LIB}" 2>&1)"
assert_eq "${out}" "rc=0" "no locale binary: a quiet no-op"

# --- wiring ----------------------------------------------------------------------
INSTALL="${REPO_ROOT}/install.sh"
assert_grep "install.sh sources the helper from the repo" '^\. "\$\{BASE_DIR\}/opt/profiles/\.locale\.sh"' "${INSTALL}"
assert_grep "install.sh calls it (with its message)" '^locale_fallback$' "${INSTALL}"
LF_LINE="$(grep -n '^locale_fallback$' "${INSTALL}" | head -1 | cut -d: -f1)"
GFF_LINE="$(grep -n '^\. "\${BASE_DIR}/opt/lib/gff.sh"' "${INSTALL}" | head -1 | cut -d: -f1)"
assert_eq "$([ -n "${LF_LINE}" ] && [ -n "${GFF_LINE}" ] && [ "${LF_LINE}" -lt "${GFF_LINE}" ] && echo before || echo after)" \
    "before" "install.sh falls back before it starts using tools"
for P in .profile .bashrc .zshrc; do
    assert_grep "${P} calls locale_fallback quietly" '\. "\$HOME/\.locale\.sh" && locale_fallback -q' "${SELF_DIR}/${P}"
done
assert_exit_code 0 ".profile still parses under dash -n" dash -n "${SELF_DIR}/.profile"
set +e

_test_report
