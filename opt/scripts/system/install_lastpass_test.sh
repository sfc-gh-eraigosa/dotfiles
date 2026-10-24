#!/usr/bin/env bash
# Test driver for opt/scripts/system/install_lastpass.sh
#
# What must hold:
#   * Linux builds lastpass-cli from the pinned upstream tag into ~/opt/bin —
#     never the distro package: Debian/Ubuntu ship 1.3.7, and since 2024 the
#     LastPass server silently DROPS every item that carries a URL unless the
#     client encrypts it, which 1.5.0+ does. A present lpass below the floor
#     (1.5.0) is therefore replaced, one at or above it on the pin is a no-op;
#   * the clone is pinned to a tag AND its commit: a tag that resolves elsewhere
#     is refused and nothing is installed;
#   * build dependencies come from apt/dnf through sudo; when sudo would need a
#     password and there is no terminal, nothing privileged runs — ONE WARNING,
#     exit 0 (fleet update over ssh must not stall) — unless the headers are
#     already there, in which case the build proceeds without sudo;
#   * macOS goes through brew (Homebrew ships 1.6.x);
#   * install.sh gates it fail-OPEN (gff_on, boolDefault true) as a deps-phase
#     key, and versions.tsv carries the pin with the installer gate registered.
#
# Nothing real is built or installed: uname, sudo, apt-get, dnf, brew, git,
# cmake, pkg-config and lpass are PATH stubs that record their argv.
#
# Run: bash opt/scripts/system/install_lastpass_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/install_lastpass.sh"
INSTALL_SH="${REPO_ROOT}/install.sh"
FEATURES="${REPO_ROOT}/.github/gff/features.yaml"
VERSIONS="${REPO_ROOT}/opt/profiles/versions.tsv"
GATES="${REPO_ROOT}/sdk/fleet/cmd/pins_apply.go"

assert_file_exists "${SCRIPT}" "install_lastpass.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
set +e

# --- source-level ------------------------------------------------------------
assert_grep_negative "never installs the distro lastpass-cli package" 'apt-get install[^|]*lastpass-cli|dnf install[^|]*lastpass-cli' "${SCRIPT}"
assert_grep "clones the upstream repo at a tag" 'git clone[^|]*--branch "v\$\{LPASS_VERSION\}"' "${SCRIPT}"
assert_grep "pins the tag to a commit" 'LPASS_COMMIT' "${SCRIPT}"
assert_grep "apt runs non-interactively" 'DEBIAN_FRONTEND=noninteractive' "${SCRIPT}"
assert_grep_negative "never pipes a remote script into a shell" 'curl[^|]*\|[[:space:]]*(ba)?sh' "${SCRIPT}"
assert_grep "macOS uses brew" 'brew install lastpass-cli' "${SCRIPT}"
assert_grep "installs to ~/opt/bin like the other fetched tools" 'LASTPASS_INSTALL_DIR:-\$\{HOME\}/opt/bin' "${SCRIPT}"
assert_grep "names the floor and why" 'LPASS_MIN=1\.5\.0' "${SCRIPT}"

# --- stubs ------------------------------------------------------------------
TMP="$(mktemp -d "${TMPDIR:-/tmp}/install_lastpass_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
BIN="${TMP}/stubs"; mkdir -p "${BIN}"
LOG="${TMP}/calls.log"
DEST="${TMP}/optbin"            # LASTPASS_INSTALL_DIR
SHADOW="${TMP}/shadow"; mkdir -p "${SHADOW}"   # first on PATH: no real lpass leaks in

# shellcheck disable=SC2016 # the stubs expand their own $@ when they run
{
    printf '#!/bin/sh\ncase "$1" in -s) echo "${STUB_OS:-Linux}" ;; -m) echo aarch64 ;; *) echo "${STUB_OS:-Linux}" ;; esac\n' > "${BIN}/uname"
    # sudo: `-n true` succeeds unless STUB_SUDO_NEEDS_PASSWORD=1; otherwise runs the command.
    printf '#!/bin/sh\necho "sudo $*" >> "$STUB_LOG"\nif [ "$1" = "-n" ]; then [ "${STUB_SUDO_NEEDS_PASSWORD:-0}" != "1" ]; exit $?; fi\nexec "$@"\n' > "${BIN}/sudo"
    printf '#!/bin/sh\necho "apt-get $*" >> "$STUB_LOG"\n[ "$1" = install ] && touch "$STUB_TMP/deps-installed"\nexit 0\n' > "${BIN}/apt-get"
    printf '#!/bin/sh\necho "dnf $*" >> "$STUB_LOG"\n[ "$1" = install ] && touch "$STUB_TMP/deps-installed"\nexit 0\n' > "${BIN}/dnf"
    printf '#!/bin/sh\necho "brew $*" >> "$STUB_LOG"\nmkdir -p "$STUB_DEST"; printf "#!/bin/sh\\necho \\"LastPass CLI v1.6.1\\"\\n" > "$STUB_DEST/lpass"; chmod +x "$STUB_DEST/lpass"\n' > "${BIN}/brew"
    # pkg-config --exists: headers present when deps-installed exists or STUB_DEPS_PRESENT=1
    printf '#!/bin/sh\n[ "${STUB_DEPS_PRESENT:-0}" = 1 ] || [ -f "$STUB_TMP/deps-installed" ]\n' > "${BIN}/pkg-config"
    printf '#!/bin/sh\nexit 0\n' > "${BIN}/gcc"
    printf '#!/bin/sh\nexit 0\n' > "${BIN}/make"
    # git: `clone … <dir>` makes the dir; `-C <dir> rev-parse HEAD` prints the pinned commit (or STUB_GIT_COMMIT)
    cat > "${BIN}/git" <<'EOS'
#!/bin/sh
echo "git $*" >> "$STUB_LOG"
case "$1" in
    clone) for last; do :; done; mkdir -p "$last"
        # upstream v1.6.1 pins.h, abridged: the array the installer patches;
        # STUB_PINS_H picks a layout a later tag might have
        case "${STUB_PINS_H:-upstream}" in
            none) ;;
            upstream) printf '#ifndef PINS_H\n#define PINS_H\nconst char *PK_PINS[] = {\n\t/* current lastpass.com primary (leaf) */\n\t"YDjIAXSYj+mh+25FGifAiKN4oNOAj+as6gQv4naQG0M=",\n\t/* GlobalSign ECC OV SSL CA 2018 intermediate CA */\n\t"OD/WDbD3VsfMwwNzzy9MWd9JXppKB77Vb3ST2wn9meg="\n};\n#endif\n' > "$last/pins.h" ;;
            spaces) printf 'const char *PK_PINS[] = {\n    "YDjIAXSYj+mh+25FGifAiKN4oNOAj+as6gQv4naQG0M=",\n    "OD/WDbD3VsfMwwNzzy9MWd9JXppKB77Vb3ST2wn9meg="  \n};\n' > "$last/pins.h" ;;
            two-arrays) printf 'const char *PK_PINS[] = {\n\t"OD/WDbD3VsfMwwNzzy9MWd9JXppKB77Vb3ST2wn9meg="\n};\nconst char *LEAF_PINS[] = {\n\t"YDjIAXSYj+mh+25FGifAiKN4oNOAj+as6gQv4naQG0M="\n};\n' > "$last/pins.h" ;;
            odd) printf 'const char *PK_PINS[] = {\n\t"YDjIAXSYj+mh+25FGifAiKN4oNOAj+as6gQv4naQG0M=", "OD/WDbD3VsfMwwNzzy9MWd9JXppKB77Vb3ST2wn9meg="\n};\n' > "$last/pins.h" ;;
        esac
        exit 0 ;;
    -C) [ "$3" = rev-parse ] && echo "${STUB_GIT_COMMIT:-feaeacf1549da07a646505cc531e63d6dfdc0f3f}" ;;
esac
exit 0
EOS
    # cmake: `--build <dir>` drops a runnable lpass reporting STUB_BUILT_VERSION (default 1.6.1)
    cat > "${BIN}/cmake" <<'EOS'
#!/bin/sh
echo "cmake $*" >> "$STUB_LOG"
if [ "$1" = "--build" ]; then
    # the built binary carries pins.h as it was at build time, as the real one does
    # (STUB_BUILD_IGNORES_PINS=1: a build that compiled some other header)
    mkdir -p "$2"; { printf '#!/bin/sh\necho "LastPass CLI v%s"\n' "${STUB_BUILT_VERSION:-1.6.1}"; [ "${STUB_BUILD_IGNORES_PINS:-0}" = 1 ] || sed "s/^/# /" "$2/../pins.h" 2>/dev/null; } > "$2/lpass"; chmod +x "$2/lpass"
fi
exit 0
EOS
} 2>/dev/null
chmod +x "${BIN}"/*
# lpass shadow: STUB_LPASS_VERSION plays a distro lpass on PATH; otherwise runs
# the one the script installed into DEST once that exists, and fails until then.
# shellcheck disable=SC2016
printf '#!/bin/sh\nif [ -x "$STUB_DEST/lpass" ]; then exec "$STUB_DEST/lpass" "$@"; fi\nif [ -n "${STUB_LPASS_VERSION:-}" ]; then echo "LastPass CLI v${STUB_LPASS_VERSION}"; exit 0; fi\nexit 1\n' > "${SHADOW}/lpass"
chmod +x "${SHADOW}/lpass"

run_lp() {  # run_lp [VAR=value ...]  (stdin is not a terminal)
    : > "${LOG}"; rm -rf "${DEST}" "${TMP}/deps-installed"
    env -u LASTPASS_FORCE STUB_LOG="${LOG}" STUB_TMP="${TMP}" STUB_DEST="${DEST}" \
        LASTPASS_INSTALL_DIR="${DEST}" HOME="${TMP}/home" TMPDIR="${TMP}" \
        PATH="${SHADOW}:${BIN}:/usr/bin:/bin" "$@" bash "${SCRIPT}" 2>&1 </dev/null
}

# 1. Fresh Debian-like host, sudo cached: deps via apt, clone at the tag, commit checked, built, installed.
out="$(run_lp)"; rc=$?
assert_eq "${rc}" "0" "fresh apt host exits 0"
assert_eq "$(grep -c '^sudo env DEBIAN_FRONTEND=noninteractive apt-get install' "${LOG}")" "1" "build deps installed once through sudo, non-interactively"
assert_eq "$(grep -c 'apt-get install.*lastpass-cli' "${LOG}")" "0" "the distro lastpass-cli package is never installed"
assert_eq "$(grep -c '^git clone .*--branch v1.6.1 .*lastpass/lastpass-cli' "${LOG}")" "1" "clones upstream at the pinned tag"
assert_eq "$(grep -c '^git -C .* rev-parse HEAD' "${LOG}")" "1" "checks the tag's commit"
assert_eq "$(grep -c '^cmake --build' "${LOG}")" "1" "builds once"
assert_file_exists "${DEST}/lpass" "lpass lands in LASTPASS_INSTALL_DIR"
assert_eq "$(printf '%s' "${out}" | grep -c 'Success! lastpass-cli 1.6.1')" "1" "success names the built version"
# LastPass moved lastpass.com to a GlobalSign E46 chain in October 2026; 1.6.1's
# pins.h predates it, so a stock build fails every login (upstream #740/#741).
assert_eq "$(grep -c 'l9l5rsCxnWfS7st1kW3EqURDe6gYtprAmX8et0XpFIA=' "${DEST}/lpass")" "1" "the build pins GlobalSign GCC E46 OV TLS CA 2025"
assert_eq "$(grep -c '4EoCLOMvTM8sf2BGKHuCijKpCfXnUUR/g/0scfb9gXM=' "${DEST}/lpass")" "1" "the build pins GlobalSign Root E46"
assert_eq "$(grep -c 'OD/WDbD3VsfMwwNzzy9MWd9JXppKB77Vb3ST2wn9meg=' "${DEST}/lpass")" "1" "upstream's own pins are kept"
# Each pin is its own element: upstream's last entry gained its comma (adjacent
# literals would otherwise concatenate into one bogus pin), and ours end in one.
assert_eq "$(grep -c '^# 	"OD/WDbD3VsfMwwNzzy9MWd9JXppKB77Vb3ST2wn9meg=",$' "${DEST}/lpass")" "1" "upstream's last pin gains its comma"
assert_eq "$(grep -c '^# 	"l9l5rsCxnWfS7st1kW3EqURDe6gYtprAmX8et0XpFIA=",$' "${DEST}/lpass")" "1" "the intermediate pin is its own element"
assert_eq "$(grep -c '^# 	"4EoCLOMvTM8sf2BGKHuCijKpCfXnUUR/g/0scfb9gXM=",$' "${DEST}/lpass")" "1" "the root pin is its own element"

# 2. apt's 1.3.7 is on PATH: below the floor, so it is replaced, and the message says why.
out="$(run_lp STUB_LPASS_VERSION=1.3.7)"; rc=$?
assert_eq "${rc}" "0" "host with distro 1.3.7 exits 0"
assert_eq "$(grep -c '^cmake --build' "${LOG}")" "1" "a lpass below 1.5.0 is replaced by a build"
assert_eq "$(printf '%s' "${out}" | grep -c '1.3.7 is below 1.5.0')" "1" "says the present version is below the floor"
assert_eq "$(printf '%s' "${out}" | grep -ci 'URL')" "1" "says what the old version loses (items with a URL)"

# 3. Already on the pin in the install dir, with the CA pins: no-op, nothing privileged, no clone.
mkdir -p "${DEST}"; printf '#!/bin/sh\necho "LastPass CLI v1.6.1"\n# "l9l5rsCxnWfS7st1kW3EqURDe6gYtprAmX8et0XpFIA=",\n# "4EoCLOMvTM8sf2BGKHuCijKpCfXnUUR/g/0scfb9gXM="\n' > "${DEST}/lpass"; chmod +x "${DEST}/lpass"
: > "${LOG}"
out="$(env STUB_LOG="${LOG}" STUB_TMP="${TMP}" STUB_DEST="${DEST}" LASTPASS_INSTALL_DIR="${DEST}" HOME="${TMP}/home" PATH="${SHADOW}:${BIN}:/usr/bin:/bin" bash "${SCRIPT}" 2>&1 </dev/null)"; rc=$?
assert_eq "${rc}" "0" "already-installed exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'already installed: .*(1.6.1)')" "1" "already-installed is reported"
assert_eq "$(wc -l < "${LOG}" | tr -d ' ')" "0" "already-installed touches neither sudo, git nor cmake"

# 3b. A 1.6.1 built before the pins (every host until 2026-10-24): rebuilt once, and says why.
printf '#!/bin/sh\necho "LastPass CLI v1.6.1"\n' > "${DEST}/lpass"; chmod +x "${DEST}/lpass"
: > "${LOG}"
out="$(env STUB_LOG="${LOG}" STUB_TMP="${TMP}" STUB_DEST="${DEST}" LASTPASS_INSTALL_DIR="${DEST}" HOME="${TMP}/home" TMPDIR="${TMP}" PATH="${SHADOW}:${BIN}:/usr/bin:/bin" bash "${SCRIPT}" 2>&1 </dev/null)"; rc=$?
assert_eq "${rc}" "0" "unpinned 1.6.1 exits 0"
assert_eq "$(grep -c '^cmake --build' "${LOG}")" "1" "an unpinned 1.6.1 is rebuilt"
assert_eq "$(printf '%s' "${out}" | grep -c 'lacks the LastPass CA pins')" "1" "says the present build lacks the pins"
assert_eq "$(grep -c 'l9l5rsCxnWfS7st1kW3EqURDe6gYtprAmX8et0XpFIA=' "${DEST}/lpass")" "1" "the rebuilt lpass carries the pin"

# 3c. pins.h is not where the patch expects it (upstream moved it): refuse rather than build an lpass that cannot log in.
out="$(run_lp STUB_PINS_H=none)"; rc=$?
assert_eq "${rc}" "1" "no pins.h exits 1"
assert_eq "$(grep -c '^cmake' "${LOG}")" "0" "nothing is built without pins.h"

# 3d. Another layout a later tag might ship: spaces, trailing whitespace — patched, each pin its own element.
out="$(run_lp STUB_PINS_H=spaces)"; rc=$?
assert_eq "${rc}" "0" "space-indented pins.h exits 0"
assert_eq "$(grep -c '^#     "OD/WDbD3VsfMwwNzzy9MWd9JXppKB77Vb3ST2wn9meg=",$' "${DEST}/lpass")" "1" "space-indented: the last pin gains its comma (trailing space dropped)"

# 3e. A second array after PK_PINS: only PK_PINS gets the CA pins.
out="$(run_lp STUB_PINS_H=two-arrays)"; rc=$?
assert_eq "${rc}" "0" "two arrays exits 0"
assert_eq "$(sed -n '/LEAF_PINS/,/};/p' "${DEST}/lpass" | grep -c 'l9l5rs\|4EoCLO')" "0" "the second array is left alone"
assert_eq "$(sed -n '/PK_PINS/,/};/p' "${DEST}/lpass" | grep -c 'l9l5rs\|4EoCLO')" "2" "PK_PINS carries both pins"

# 3f. A line the patch cannot read (two pins on one line): refuse, build nothing.
out="$(run_lp STUB_PINS_H=odd)"; rc=$?
assert_eq "${rc}" "1" "unreadable pins.h exits 1"
assert_eq "$(grep -c '^cmake' "${LOG}")" "0" "nothing is built from an unreadable pins.h"

# 3g. The build compiled something other than the patched header: refuse to install it.
out="$(run_lp STUB_BUILD_IGNORES_PINS=1)"; rc=$?
assert_eq "${rc}" "1" "a build without the pins exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'does not carry the LastPass CA pins')" "1" "says the build lacks the pins"
installed_unpinned=no; [ -e "${DEST}/lpass" ] && installed_unpinned=yes
assert_eq "${installed_unpinned}" "no" "an unpinned build is not installed"

# 3h. An unpinned 1.6.1 that cannot be rebuilt (no sudo, no tty, no headers): say it stays broken.
rm -f "${TMP}/deps-installed"; mkdir -p "${DEST}"; printf '#!/bin/sh\necho "LastPass CLI v1.6.1"\n' > "${DEST}/lpass"; chmod +x "${DEST}/lpass"
: > "${LOG}"
out="$(env STUB_LOG="${LOG}" STUB_TMP="${TMP}" STUB_DEST="${DEST}" STUB_SUDO_NEEDS_PASSWORD=1 LASTPASS_INSTALL_DIR="${DEST}" HOME="${TMP}/home" TMPDIR="${TMP}" PATH="${SHADOW}:${BIN}:/usr/bin:/bin" bash "${SCRIPT}" 2>&1 </dev/null)"; rc=$?
assert_eq "${rc}" "0" "unpinned and unbuildable exits 0 (fleet update must not stall)"
assert_eq "$(printf '%s' "${out}" | grep -c 'cannot log in to LastPass')" "1" "warns that the present lpass cannot log in"

# 3i. Another pinned version: no extra pins apply (the bump drops the patch).
assert_eq "$(LASTPASS_VERSION=1.7.0 bash -c '. /dev/null; LPASS_VERSION=1.7.0; '"$(sed -n '/^LPASS_EXTRA_PINS=()/,/^fi$/p' "${SCRIPT}")"'; echo ${#LPASS_EXTRA_PINS[@]}')" "0" "a version other than 1.6.1 carries no extra pins"

# 4. The tag resolves to another commit: refuse, install nothing.
out="$(run_lp STUB_GIT_COMMIT=0000000000000000000000000000000000000000)"; rc=$?
assert_eq "${rc}" "1" "commit mismatch exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'refusing')" "1" "commit mismatch is refused in so many words"
assert_eq "$(grep -c '^cmake' "${LOG}")" "0" "nothing is built on a commit mismatch"
installed_after_mismatch=no; [ -e "${DEST}/lpass" ] && installed_after_mismatch=yes
assert_eq "${installed_after_mismatch}" "no" "nothing installed on a commit mismatch"

# 5. sudo needs a password, no terminal, headers missing: ONE warning, exit 0, nothing built.
out="$(run_lp STUB_SUDO_NEEDS_PASSWORD=1)"; rc=$?
assert_eq "${rc}" "0" "no-sudo/no-tty exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'WARNING: install_lastpass: sudo needs a password and there is no terminal')" "1" "no-sudo/no-tty warns once"
assert_eq "$(grep -c 'apt-get\|cmake\|git clone' "${LOG}")" "0" "no-sudo/no-tty touches neither apt nor the build"

# 6. sudo needs a password but the headers are already present: build anyway, no sudo.
out="$(run_lp STUB_SUDO_NEEDS_PASSWORD=1 STUB_DEPS_PRESENT=1)"; rc=$?
assert_eq "${rc}" "0" "headers present, no sudo: exits 0"
assert_eq "$(grep -c '^cmake --build' "${LOG}")" "1" "headers present: builds without privileges"
assert_eq "$(grep -c '^sudo env' "${LOG}")" "0" "headers present: sudo is never used for a package install"

# 7. The build produced a different version than pinned: refuse.
out="$(run_lp STUB_BUILT_VERSION=1.5.0)"; rc=$?
assert_eq "${rc}" "1" "built version off the pin exits 1"

# 8. macOS: brew, no sudo, no build.
out="$(run_lp STUB_OS=Darwin)"; rc=$?
assert_eq "${rc}" "0" "macOS exits 0"
assert_eq "$(grep -c '^brew install lastpass-cli$' "${LOG}")" "1" "macOS installs the brew formula"
assert_eq "$(grep -c 'sudo\|cmake' "${LOG}")" "0" "macOS never calls sudo or cmake"
assert_eq "$(printf '%s' "${out}" | grep -c "brew's lpass lacks the LastPass CA pins")" "1" "macOS warns when brew's lpass lacks the pins"

# 9. Unsupported OS is refused.
out="$(run_lp STUB_OS=FreeBSD)"; rc=$?
assert_eq "${rc}" "1" "unsupported OS exits 1"

# --- pins + gff wiring ----------------------------------------------------------
assert_grep "versions.tsv pins lastpass-cli" '^lastpass-cli +pin +1\.6\.1 +gh:lastpass/lastpass-cli +opt/scripts/system/install_lastpass\.sh +lpass --version' "${VERSIONS}"
assert_grep "fleet pins apply knows the installer's gff gate" '"opt/scripts/system/install_lastpass\.sh": *\{key: "install\.tools\.lastpass"\}' "${GATES}"
assert_grep "features.yaml declares install.tools.lastpass" 'path: install\.tools\.lastpass$' "${FEATURES}"
assert_eq "$(awk '$2 == "path:" { k = $3 } k == "install.tools.lastpass" && $1 == "boolDefault:" { print $2; exit }' "${FEATURES}")" \
    "true" "install.tools.lastpass is on by default (boolDefault true)"
assert_grep "install.sh gates lastpass fail-open with gff_on" 'gff_on install\.tools\.lastpass;' "${INSTALL_SH}"
assert_eq "$(grep -E '^_IP_DEPS_FLAGS=' "${INSTALL_SH}" | grep -c -w 'INSTALL_TOOLS_LASTPASS')" "1" \
    "INSTALL_TOOLS_LASTPASS is a deps-phase key"

_test_report
