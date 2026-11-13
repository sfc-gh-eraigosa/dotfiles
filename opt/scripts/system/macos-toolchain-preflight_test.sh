#!/usr/bin/env bash
# Test driver for opt/scripts/system/macos-toolchain-preflight.sh
#
# The script is exercised for real against throwaway fixture directories: every
# host probe it makes (uname, xcode-select, xcrun, the compiler, sudo, the Xcode
# bundle, version numbers) has a PREFLIGHT_* override, so these cases run the
# same on Linux CI as on a Mac and never touch /Library.
#
# Run: bash opt/scripts/system/macos-toolchain-preflight_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/macos-toolchain-preflight.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# Fake compilers: one that always succeeds, one that always fails.
CXX_OK="${WORK}/cxx-ok";   printf '#!/bin/sh\nexit 0\n' > "$CXX_OK";   chmod +x "$CXX_OK"
CXX_BAD="${WORK}/cxx-bad"; printf '#!/bin/sh\nexit 1\n' > "$CXX_BAD"; chmod +x "$CXX_BAD"

# Build a CLT-shaped developer dir + SDK. $1 = "stale" | "healthy" | "none"
# (state of <DEVDIR>/usr/include/c++). Echoes the fixture root.
_fixture() {
    local mode="$1" root
    root="$(mktemp -d "${WORK}/fx.XXXXXX")"
    mkdir -p "${root}/dev/usr/bin" "${root}/sdk/usr/include/c++/v1"
    : > "${root}/sdk/usr/include/c++/v1/cstdint"
    case "$mode" in
        stale)   mkdir -p "${root}/dev/usr/include/c++/v1"
                 : > "${root}/dev/usr/include/c++/v1/__sso_allocator" ;;
        healthy) mkdir -p "${root}/dev/usr/include/c++/v1"
                 : > "${root}/dev/usr/include/c++/v1/cstdint" ;;
        none)    : ;;
    esac
    echo "$root"
}

# Run the script with a fixture. $1 = fixture root, rest = extra args.
# Sets RC (exit status) and OUT (file holding stdout+stderr) for the caller —
# deliberately NOT via $(...), so both survive into the calling scope.
_run() {
    local root="$1"; shift
    OUT="${root}/out.txt"
    PREFLIGHT_UNAME=Darwin \
    PREFLIGHT_DEVDIR="${root}/dev" \
    PREFLIGHT_SDKROOT="${root}/sdk" \
    PREFLIGHT_CXX="${PREFLIGHT_CXX:-$CXX_OK}" \
    PREFLIGHT_SUDO="" \
    PREFLIGHT_XCODE_APP="${PREFLIGHT_XCODE_APP:-${root}/no-xcode}" \
    bash "$SCRIPT" "$@" > "$OUT" 2>&1
    RC=$?
}

# === 1. Syntax ===
assert_exit_code 0 "preflight parses with bash -n" bash -n "$SCRIPT"

# === 2. Not macOS: silent no-op, exit 0 ===
RC="$(PREFLIGHT_UNAME=Linux bash "$SCRIPT" > "${WORK}/linux.txt" 2>&1; echo $?)"
assert_eq "$RC" "0" "non-Darwin exits 0"
assert_eq "$(wc -c < "${WORK}/linux.txt" | tr -d ' ')" "0" "non-Darwin prints nothing"

# === 3. Stale toolchain libc++ dir is moved aside ===
FX="$(_fixture stale)"
_run "$FX"
assert_eq "$RC" "0" "stale dir: exit 0 (fail-open)"
assert_exit_code 1 "stale dir: <DEVDIR>/usr/include/c++ no longer exists" test -d "${FX}/dev/usr/include/c++"
assert_eq "$(ls -d "${FX}"/dev/usr/include/c++.stale-* 2>/dev/null | wc -l | tr -d ' ')" "1" \
    "stale dir: exactly one timestamped .stale-* sibling created"
assert_file_exists "$(ls -d "${FX}"/dev/usr/include/c++.stale-*)/v1/__sso_allocator" \
    "stale dir: contents preserved in the moved copy"
assert_grep "stale dir: reports the move" 'moved it to .*c\+\+\.stale-' "$OUT"
assert_grep "stale dir: C++ probe then reports OK" 'C\+\+ toolchain OK' "$OUT"

# === 4. --dry-run reports but moves nothing ===
FX="$(_fixture stale)"
_run "$FX" --dry-run
assert_eq "$RC" "0" "dry-run: exit 0"
assert_file_exists "${FX}/dev/usr/include/c++/v1/__sso_allocator" "dry-run: stale dir left in place"
assert_grep "dry-run: says what it would do" 'dry-run: would move it to' "$OUT"

# === 5. Healthy toolchain dir (has v1/cstdint) is never touched ===
FX="$(_fixture healthy)"
_run "$FX"
assert_file_exists "${FX}/dev/usr/include/c++/v1/cstdint" "healthy dir: left in place"
assert_grep_negative "healthy dir: no move reported" 'moved it to|would move' "$OUT"
assert_grep "healthy dir: reports OK" 'C\+\+ toolchain OK' "$OUT"

# === 6. No toolchain dir at all (normal current CLT) → nothing to do ===
FX="$(_fixture none)"
_run "$FX"
assert_grep_negative "no dir: no move reported" 'moved it to|would move|stale' "$OUT"
assert_grep "no dir: reports OK" 'C\+\+ toolchain OK' "$OUT"

# === 7. Compiler cannot build C++ → remediation printed, still exit 0 ===
FX="$(_fixture none)"
PREFLIGHT_CXX="$CXX_BAD" _run "$FX"
assert_eq "$RC" "0" "broken compiler: exit 0 (fail-open)"
assert_grep "broken compiler: names the CLT reinstall" 'xcode-select --install' "$OUT"
assert_grep_negative "broken compiler: does not claim OK" 'toolchain OK' "$OUT"

# === 8. No developer dir selected → CLT install hint, exit 0 ===
RC="$(PREFLIGHT_UNAME=Darwin PREFLIGHT_DEVDIR="${WORK}/does-not-exist" bash "$SCRIPT" > "${WORK}/nodev.txt" 2>&1; echo $?)"
assert_eq "$RC" "0" "no devdir: exit 0"
assert_grep "no devdir: suggests xcode-select --install" 'xcode-select --install' "${WORK}/nodev.txt"

# === 9. Xcode age check ===
mkdir -p "${WORK}/Xcode.app/Contents"
FX="$(_fixture none)"
PREFLIGHT_XCODE_APP="${WORK}/Xcode.app" PREFLIGHT_XCODE_VERSION=14.0.1 PREFLIGHT_MACOS_VERSION=26.7.2 _run "$FX"
assert_grep "old Xcode 14 on macOS 26: warned" 'Xcode 14\.0\.1 on macOS 26\.7\.2: too old for Homebrew' "$OUT"
assert_grep "old Xcode: remediation names xcode-select -s CLT" 'xcode-select -s /Library/Developer/CommandLineTools' "$OUT"
FX="$(_fixture none)"
PREFLIGHT_XCODE_APP="${WORK}/Xcode.app" PREFLIGHT_XCODE_VERSION=26.1 PREFLIGHT_MACOS_VERSION=26.7.2 _run "$FX"
assert_grep_negative "current Xcode 26 on macOS 26: no warning" 'too old for Homebrew' "$OUT"
FX="$(_fixture none)"
PREFLIGHT_XCODE_APP="${WORK}/Xcode.app" PREFLIGHT_XCODE_VERSION=16.4 PREFLIGHT_MACOS_VERSION=15.6 _run "$FX"
assert_grep_negative "Xcode 16 on macOS 15 (trails by one): no warning" 'too old for Homebrew' "$OUT"
FX="$(_fixture none)"
PREFLIGHT_XCODE_APP="${WORK}/Xcode.app" PREFLIGHT_XCODE_VERSION="" PREFLIGHT_MACOS_VERSION=26.7.2 _run "$FX"
assert_eq "$RC" "0" "unreadable Xcode version: stays quiet, exit 0"
assert_grep_negative "unreadable Xcode version: no warning" 'too old for Homebrew' "$OUT"

_test_report
