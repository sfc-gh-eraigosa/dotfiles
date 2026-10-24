#!/usr/bin/env bash
# ==============================================================================
# LastPass CLI Setup — build `lpass` (lastpass-cli) from the pinned upstream tag
# ==============================================================================
# Why this exists:
#   * `lpass` is the official LastPass command-line client (lastpass/lastpass-cli,
#     GPL-2.0): `lpass show --password <entry>` in scripts, `lpass ls` in a shell.
#   * NOT the distro package. Debian/Ubuntu/Raspberry Pi OS/Jetson ship 1.3.7
#     (Nov 2023). Since 2024 the LastPass server expects the URL field encrypted
#     (lastpass-cli 1.5.0, "write encrypted url"); a 1.3.7 client posts it in
#     the clear, the server answers 200 and then silently DROPS the item. Every
#     login with a URL vanishes while notes survive — found on 2026-10-17 when
#     `k3sops secrets sync` wrote 10 items and the two admin logins never
#     appeared. So Linux builds the pinned tag from source into ~/opt/bin (first
#     on PATH, ahead of /usr/bin), and a present lpass below LPASS_MIN is replaced.
#   * macOS uses Homebrew, whose formula is current (1.6.x).
#   * Pinned (versions.tsv row `lastpass-cli`) to a tag AND that tag's commit:
#     a tag that resolves elsewhere is refused and nothing is installed.
#   * Patched with LastPass's current CA pins (LPASS_EXTRA_PINS). lpass refuses
#     any server key not in its compiled-in pins.h, and in October 2026
#     lastpass.com moved to a GlobalSign E46 chain that 1.6.1 does not pin, so a
#     stock build fails every call with "SSL peer certificate or SSH remote key
#     was not OK" (upstream issue #740, PR #741). The patch only adds pins;
#     chain and hostname verification are unchanged. A 1.6.1 built without them
#     is rebuilt once. Drop the patch when a release carries the pins.
#
# Safe to re-run: a lpass at the pin in LASTPASS_INSTALL_DIR is a no-op. Like
# pkg-install-apt, when sudo would need a password and there is no terminal to
# ask on, no privileged command runs — ONE WARNING, exit 0 — so an unattended
# `fleet update` never stalls here; if the build headers are already present
# the build goes ahead without sudo.
#
# Environment:
#   LASTPASS_VERSION      tag to build (default: versions.tsv `lastpass-cli`, else 1.6.1)
#   LASTPASS_COMMIT       the commit that tag must resolve to (default: 1.6.1's)
#   LASTPASS_INSTALL_DIR  target dir (default: ~/opt/bin)
#   LASTPASS_FORCE=1      rebuild even when the pinned version is present
#   LASTPASS_REPO         clone URL (default: https://github.com/lastpass/lastpass-cli.git)
set -e

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PINS_LIB="${SELF_DIR}/../../lib/pins.sh"
if [ -f "${PINS_LIB}" ]; then
    # shellcheck source=../../lib/pins.sh
    . "${PINS_LIB}"
fi
# Fail-open: without the lib (or the manifest) the hardcoded default applies.
command -v pins_want >/dev/null 2>&1 || pins_want() { printf '%s\n' "${2:-}"; }

LPASS_VERSION="${LASTPASS_VERSION:-$(pins_want lastpass-cli 1.6.1)}"
LPASS_VERSION="${LPASS_VERSION#v}"
# The commit v1.6.1 resolves to upstream; a different pin needs its own.
if [ -n "${LASTPASS_COMMIT:-}" ]; then
    LPASS_COMMIT="${LASTPASS_COMMIT}"
elif [ "${LPASS_VERSION}" = "1.6.1" ]; then
    LPASS_COMMIT=feaeacf1549da07a646505cc531e63d6dfdc0f3f
else
    LPASS_COMMIT=""
fi
# Below this the server drops items that carry a URL (see the header).
LPASS_MIN=1.5.0
# SPKI SHA-256 pins (base64) added to pins.h, checked against the live chain
# on 2026-10-24 (openssl s_client | x509 -pubkey | pkey -outform der | sha256).
# For 1.6.1 only: a versions.tsv bump to a release is what drops the patch, so
# re-check that release's pins.h before bumping. Descriptions go into a C
# comment: letters, digits, spaces and ( ) . , ' - only.
LPASS_EXTRA_PINS=()
if [ "${LPASS_VERSION}" = "1.6.1" ]; then
    LPASS_EXTRA_PINS=(
        "l9l5rsCxnWfS7st1kW3EqURDe6gYtprAmX8et0XpFIA=|GlobalSign GCC E46 OV TLS CA 2025 intermediate CA (lastpass.com from 2026-10)"
        "4EoCLOMvTM8sf2BGKHuCijKpCfXnUUR/g/0scfb9gXM=|GlobalSign Root E46 (that intermediate's root)"
    )
fi
INSTALL_DIR="${LASTPASS_INSTALL_DIR:-${HOME}/opt/bin}"
REPO="${LASTPASS_REPO:-https://github.com/lastpass/lastpass-cli.git}"
OWN="${INSTALL_DIR}/lpass"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}$*${NC}"; }
ok()   { echo -e "${GREEN}$*${NC}"; }
warn() { echo -e "${RED}WARNING: install_lastpass: $*${NC}" >&2; }
die()  { echo -e "${RED}install_lastpass: $*${NC}" >&2; exit 1; }

TMP_DIR=""
cleanup() { if [ -n "${TMP_DIR}" ]; then rm -rf "${TMP_DIR}"; fi; }
trap cleanup EXIT

# "LastPass CLI v1.6.1" -> 1.6.1 (empty when the binary does not run)
version_of() { "$1" --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -n 1 || true; }
# version_ge A B: A >= B as dotted numbers
version_ge() { [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n 1)" = "$2" ]; }
# has_pins BIN: every LPASS_EXTRA_PINS key is compiled into BIN (pins.h strings are literals)
has_pins() {
    local p
    for p in ${LPASS_EXTRA_PINS[@]+"${LPASS_EXTRA_PINS[@]}"}; do grep -aqF "${p%%|*}" "$1" 2>/dev/null || return 1; done
}
# pins_awk MODE: the one reader of pins.h's `PK_PINS[] = {` … `};` array, so
# patching and checking agree on its shape. In the array every line is blank,
# a whole `/* … */` comment, or one quoted pin with an optional comma; anything
# else, a second PK_PINS, or no array at all exits 1. MODE patch prints the
# file with a comma on every pin (C allows a trailing one) and the PINS
# (`key|description` lines) it lacks appended; MODE check exits 1 unless each
# of PINS is its own element.
pins_awk() {
    awk -v mode="$1" -v pins="${2:-}" '
        BEGIN { n = split(pins, want, "\n") }
        /^[ \t]*const char \*PK_PINS\[\] = \{[ \t]*$/ { arrays++; inarr = 1; print; next }
        inarr && /^[ \t]*};[ \t]*$/ {
            if (mode == "patch") for (i = 1; i <= n; i++) {
                if (want[i] == "") continue
                k = want[i]; sub(/\|.*/, "", k); d = want[i]; sub(/^[^|]*\|/, "", d)
                if (!(k in have)) { printf "\t/* %s */\n\t\"%s\",\n", d, k; have[k] = 1 }
            }
            inarr = 0; closed++; print; next
        }
        inarr && /^[ \t]*$/ { print; next }
        inarr && /^[ \t]*\/\*.*\*\/[ \t]*$/ { print; next }
        inarr && /^[ \t]*"[A-Za-z0-9+\/=]+",?[ \t]*$/ {
            k = $0; gsub(/[ \t",]/, "", k); have[k] = 1
            line = $0; sub(/[ \t]+$/, "", line); if (line !~ /,$/) line = line ","
            print line; next
        }
        inarr { bad = 1 }
        { print }
        END {
            if (bad || arrays != 1 || closed != 1) exit 1
            if (mode == "check") for (i = 1; i <= n; i++) {
                if (want[i] == "") continue
                k = want[i]; sub(/\|.*/, "", k); if (!(k in have)) exit 1
            }
        }' "$3"
}
# patch_pins PINS_H: add LPASS_EXTRA_PINS to the PK_PINS array (upstream's
# pins stay), then re-read the result: fails closed on any other shape.
patch_pins() {
    local h="$1" p list=""
    [ -f "${h}" ] || return 1
    for p in ${LPASS_EXTRA_PINS[@]+"${LPASS_EXTRA_PINS[@]}"}; do
        if printf '%s' "${p#*|}" | grep -q "[^A-Za-z0-9 ().,'-]"; then return 1; fi
        list="${list}${p}"$'\n'
    done
    [ -n "${list}" ] || return 0
    if ! pins_awk patch "${list}" "${h}" > "${h}.new"; then rm -f "${h}.new"; return 1; fi
    mv "${h}.new" "${h}"
    pins_awk check "${list}" "${h}" >/dev/null
}

case "${LPASS_VERSION}" in
    *[!0-9.]*|"") die "LASTPASS_VERSION must look like 1.6.1 (got '${LPASS_VERSION}')" ;;
esac

# --- macOS: Homebrew owns it ----------------------------------------------------
if [ "$(uname -s)" = "Darwin" ]; then
    if command -v lpass >/dev/null 2>&1 && version_ge "$(version_of lpass)" "${LPASS_MIN}" && [ "${LASTPASS_FORCE:-0}" != "1" ]; then
        ok "lastpass-cli already installed: $(command -v lpass) ($(version_of lpass))"
        has_pins "$(command -v lpass)" || warn "brew's lpass lacks the LastPass CA pins, so every login fails with \"SSL peer certificate … not OK\" until lastpass-cli releases them (upstream #740); not patched on macOS"
        exit 0
    fi
    command -v brew >/dev/null 2>&1 || die "Homebrew not found — install it first: https://brew.sh/"
    info "Installing lastpass-cli via brew..."
    brew install lastpass-cli
    command -v lpass >/dev/null 2>&1 || die "lpass not resolvable after brew install"
    ok "Success! lastpass-cli $(version_of lpass) at $(command -v lpass)"
    has_pins "$(command -v lpass)" || warn "brew's lpass lacks the LastPass CA pins, so every login fails until lastpass-cli releases them (upstream #740); not patched on macOS"
    exit 0
fi
[ "$(uname -s)" = "Linux" ] || die "unsupported OS $(uname -s)"

# --- Linux: build the pinned tag into ~/opt/bin ----------------------------------
BROKEN=""   # set when the lpass already here cannot log in (no CA pins)
HAVE=""
if [ -x "${OWN}" ]; then HAVE="$(version_of "${OWN}")"; fi
if [ "${HAVE}" = "${LPASS_VERSION}" ] && [ "${LASTPASS_FORCE:-0}" != "1" ]; then
    if has_pins "${OWN}"; then
        ok "lastpass-cli already installed: ${OWN} (${HAVE})"
        exit 0
    fi
    info "lpass ${HAVE} at ${OWN} lacks the LastPass CA pins (every login fails since lastpass.com's October 2026 certificate); rebuilding it with them"
    BROKEN="${OWN}"
fi
# A lpass elsewhere on PATH (the distro package) is only good enough at or above the floor, with the pins.
if [ -z "${HAVE}" ] && command -v lpass >/dev/null 2>&1 && [ "${LASTPASS_FORCE:-0}" != "1" ]; then
    FOUND="$(command -v lpass)"
    FOUND_VER="$(version_of lpass)"
    if [ -n "${FOUND_VER}" ] && version_ge "${FOUND_VER}" "${LPASS_MIN}" && has_pins "${FOUND}"; then
        ok "lastpass-cli already installed: ${FOUND} (${FOUND_VER})"
        exit 0
    fi
    if [ -n "${FOUND_VER}" ] && version_ge "${FOUND_VER}" "${LPASS_MIN}"; then
        info "lpass ${FOUND_VER} at ${FOUND} lacks the LastPass CA pins (every login fails); building ${LPASS_VERSION} with them into ${INSTALL_DIR} (first on PATH)"
        BROKEN="${FOUND}"
    else
        info "lpass ${FOUND_VER:-?} is below ${LPASS_MIN} (at ${FOUND}): the LastPass server drops items that carry a URL from that client; building ${LPASS_VERSION} into ${INSTALL_DIR} (first on PATH)"
    fi
fi

# Run a command as root: directly when we already are, via sudo otherwise.
if [ "$(id -u)" -eq 0 ]; then
    as_root() { "$@"; }
else
    as_root() { sudo "$@"; }
fi
sudo_noninteractive() { [ "$(id -u)" -eq 0 ] || sudo -n true 2>/dev/null; }

# Build headers + toolchain present?
have_deps() {
    command -v git >/dev/null 2>&1 && command -v cmake >/dev/null 2>&1 \
        && { command -v gcc >/dev/null 2>&1 || command -v cc >/dev/null 2>&1; } \
        && command -v make >/dev/null 2>&1 \
        && pkg-config --exists libcurl libxml-2.0 openssl 2>/dev/null
}

if ! have_deps; then
    if ! sudo_noninteractive && [ ! -t 0 ]; then
        warn "sudo needs a password and there is no terminal; skipping the lastpass-cli build (needs cmake, pkg-config, gcc, make, libcurl/libxml2/openssl headers). Re-run install.sh interactively."
        [ -z "${BROKEN}" ] || warn "${BROKEN} stays as it is and cannot log in to LastPass (no CA pins) until this build runs"
        exit 0
    fi
    if command -v apt-get >/dev/null 2>&1; then
        info "Installing lastpass-cli build dependencies via apt..."
        as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends \
            git cmake pkg-config gcc make libcurl4-openssl-dev libxml2-dev libssl-dev
    elif command -v dnf >/dev/null 2>&1; then
        info "Installing lastpass-cli build dependencies via dnf..."
        as_root dnf install -y -q git cmake pkgconf-pkg-config gcc make libcurl-devel libxml2-devel openssl-devel
    else
        die "no supported package manager (apt-get, dnf) for the build dependencies — install cmake, pkg-config, gcc, make and the libcurl/libxml2/openssl headers by hand"
    fi
    have_deps || die "build dependencies still missing after the package install"
fi

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/install_lastpass.XXXXXX")"
SRC="${TMP_DIR}/lastpass-cli"
info "Building lastpass-cli ${LPASS_VERSION} from ${REPO} (tag v${LPASS_VERSION})..."
git clone -q --depth 1 --branch "v${LPASS_VERSION}" "${REPO}" "${SRC}" || die "could not clone ${REPO} at tag v${LPASS_VERSION}"
if [ -n "${LPASS_COMMIT}" ]; then
    GOT="$(git -C "${SRC}" rev-parse HEAD)"
    [ "${GOT}" = "${LPASS_COMMIT}" ] || die "tag v${LPASS_VERSION} resolves to ${GOT}, not the pinned ${LPASS_COMMIT}; refusing to build it"
else
    warn "no LASTPASS_COMMIT pinned for ${LPASS_VERSION}; building whatever tag v${LPASS_VERSION} points at"
fi
patch_pins "${SRC}/pins.h" || die "could not add the LastPass CA pins to ${SRC}/pins.h (did upstream move the PK_PINS array?); refusing to build an lpass that cannot log in"
cmake -S "${SRC}" -B "${SRC}/build" -DCMAKE_BUILD_TYPE=Release >/dev/null || die "cmake configure failed"
cmake --build "${SRC}/build" -j"$(nproc 2>/dev/null || echo 2)" >/dev/null || die "cmake build failed"
[ -x "${SRC}/build/lpass" ] || die "the build produced no lpass binary"
BUILT="$(version_of "${SRC}/build/lpass")"
[ "${BUILT}" = "${LPASS_VERSION}" ] || die "built lpass reports ${BUILT:-nothing}, expected ${LPASS_VERSION}; refusing to install it"
has_pins "${SRC}/build/lpass" || die "the built lpass does not carry the LastPass CA pins patched into pins.h; refusing to install it"

mkdir -p "${INSTALL_DIR}"
if [ -e "${OWN}" ] || [ -L "${OWN}" ]; then rm -f "${OWN}"; fi
install -m 0755 "${SRC}/build/lpass" "${OWN}"
ok "Success! lastpass-cli $(version_of "${OWN}") at ${OWN} (built from tag v${LPASS_VERSION}${LPASS_COMMIT:+, commit ${LPASS_COMMIT:0:12}})"
