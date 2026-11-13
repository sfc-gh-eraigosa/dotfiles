#!/usr/bin/env bash
# ==============================================================================
# macOS toolchain preflight — make sure Homebrew can compile C/C++ from source
# ==============================================================================
# Why this exists (install.sh on an Intel Mac, Nov 2026): Homebrew ships no
# Intel bottles any more, so `brew bundle` compiles most formulas locally. Two
# host-side faults then broke EVERY C++ formula (mpdecimal -> python -> jc, uv,
# just, clang-format) while install.sh carried on with non-fatal WARNINGs:
#
#   1. A stale <CLT>/usr/include/c++/ directory left behind by an old Command
#      Line Tools release (three files from 2023). clang looks for the libc++
#      headers in the toolchain FIRST and never falls back to the SDK copy when
#      that directory exists, so every `#include <cstdint>` failed with
#      "'cstdint' file not found". The current CLT package does not own that
#      path (`pkgutil --files com.apple.pkg.CLTools_Executables` lists nothing
#      under it), so moving it aside is the fix — clang then finds the full
#      header set under $(xcrun --show-sdk-path)/usr/include/c++/v1.
#   2. An ancient /Applications/Xcode.app (14.0.1 on macOS 26). Homebrew refuses
#      ANY source build while an Xcode older than its minimum is present, even
#      with a current CLT selected: "Your Xcode (14.0.1) is too outdated. Please
#      update to Xcode 26.6 (or delete it)".
#
# This script repairs (1) when it safely can and prints the exact remediation
# for (2). It is FAIL-OPEN: it always exits 0 so install.sh continues, and brew
# still reports its own errors later if the host is still broken.
#
# Usage: macos-toolchain-preflight.sh [--dry-run]
#   --dry-run   report what would change; move nothing.
#
# Env overrides (tests, and manual runs on odd hosts):
#   PREFLIGHT_UNAME          uname -s result            (default: uname -s)
#   PREFLIGHT_DEVDIR         active developer dir       (default: xcode-select -p)
#   PREFLIGHT_SDKROOT        SDK path                   (default: xcrun --show-sdk-path)
#   PREFLIGHT_CXX            C++ compiler to probe      (default: clang++)
#   PREFLIGHT_SUDO           privilege prefix for mv    (default: "sudo -n"; "" = none)
#   PREFLIGHT_XCODE_APP      Xcode bundle to inspect    (default: /Applications/Xcode.app)
#   PREFLIGHT_XCODE_VERSION  Xcode version to assume    (default: read from the bundle)
#   PREFLIGHT_MACOS_VERSION  macOS version to assume    (default: sw_vers -productVersion)
set -u

TAG="[toolchain-preflight]"
DRY_RUN=0
for _arg in "$@"; do
    case "$_arg" in
        --dry-run) DRY_RUN=1 ;;
        -h|--help) sed -n '2,40p' "$0"; exit 0 ;;
    esac
done

say()  { printf '%s %s\n' "$TAG" "$*"; }
warn() { printf '%s WARNING: %s\n' "$TAG" "$*" >&2; }

UNAME="${PREFLIGHT_UNAME:-$(uname -s)}"
[ "$UNAME" = "Darwin" ] || exit 0

CXX="${PREFLIGHT_CXX:-clang++}"
# "${VAR-default}" (no colon) so an explicitly EMPTY PREFLIGHT_SUDO means "no sudo".
SUDO="${PREFLIGHT_SUDO-sudo -n}"
XCODE_APP="${PREFLIGHT_XCODE_APP:-/Applications/Xcode.app}"

# --- 0. Is any developer directory selected at all? ---------------------------
DEVDIR="${PREFLIGHT_DEVDIR:-$(xcode-select -p 2>/dev/null || true)}"
if [ -z "$DEVDIR" ] || [ ! -d "$DEVDIR" ]; then
    warn "no usable developer directory (xcode-select -p -> '${DEVDIR:-<none>}')."
    warn "Install the Command Line Tools:  xcode-select --install"
    exit 0
fi
SDKROOT="${PREFLIGHT_SDKROOT:-$(xcrun --show-sdk-path 2>/dev/null || true)}"

# --- 1. Stale toolchain libc++ directory shadowing the SDK headers ------------
# Only the CLT layout keeps libc++ at <DEVDIR>/usr/include/c++; a full Xcode
# keeps it inside Toolchains/XcodeDefault.xctoolchain and is not probed here.
SHADOW="${DEVDIR}/usr/include/c++"
if [ -d "$SHADOW" ] && [ ! -e "${SHADOW}/v1/cstdint" ] \
   && [ -n "$SDKROOT" ] && [ -e "${SDKROOT}/usr/include/c++/v1/cstdint" ]; then
    DEST="${SHADOW}.stale-$(date +%Y%m%d%H%M%S)"
    say "stale libc++ directory ${SHADOW} has no v1/cstdint but the SDK does;"
    say "it shadows ${SDKROOT}/usr/include/c++/v1 and breaks every C++ build."
    if [ "$DRY_RUN" -eq 1 ]; then
        say "dry-run: would move it to ${DEST}"
    else
        if [ -n "$SUDO" ]; then
            # shellcheck disable=SC2086 # SUDO is an intentionally word-split command prefix
            $SUDO mv "$SHADOW" "$DEST" 2>/dev/null
        else
            mv "$SHADOW" "$DEST" 2>/dev/null
        fi
        if [ -d "$SHADOW" ]; then
            warn "could not move it (no cached sudo?). Run by hand, then re-run install.sh:"
            warn "  sudo mv '${SHADOW}' '${DEST}'"
        else
            say "moved it to ${DEST} (delete that once a C++ build succeeds)."
        fi
    fi
fi

# --- 2. Can the compiler see the C++ standard library at all? -----------------
cxx_compiles() {
    local src rc
    src="$(mktemp "${TMPDIR:-/tmp}/preflight.XXXXXX")" || return 1
    printf '#include <cstdint>\nint main(){return 0;}\n' > "$src"
    "$CXX" -x c++ -std=c++11 -c "$src" -o /dev/null >/dev/null 2>&1
    rc=$?
    rm -f "$src"
    return $rc
}
if command -v "$CXX" >/dev/null 2>&1; then
    if cxx_compiles; then
        say "C++ toolchain OK (${CXX} compiles <cstdint>; developer dir ${DEVDIR})."
    else
        warn "${CXX} cannot compile a trivial <cstdint> program; every Homebrew C++"
        warn "source build will fail. Reinstall the Command Line Tools:"
        warn "  sudo rm -rf /Library/Developer/CommandLineTools && xcode-select --install"
        warn "then:  sudo xcode-select -s /Library/Developer/CommandLineTools"
    fi
else
    warn "${CXX} not found on PATH; install the Command Line Tools:  xcode-select --install"
fi

# --- 3. An Xcode too old for Homebrew blocks source builds even with a good CLT
if [ -d "$XCODE_APP" ]; then
    XCODE_VER="${PREFLIGHT_XCODE_VERSION:-}"
    if [ -z "$XCODE_VER" ] && [ -f "${XCODE_APP}/Contents/version.plist" ]; then
        XCODE_VER="$(defaults read "${XCODE_APP}/Contents/version.plist" CFBundleShortVersionString 2>/dev/null || true)"
    fi
    MACOS_VER="${PREFLIGHT_MACOS_VERSION:-$(sw_vers -productVersion 2>/dev/null || true)}"
    XCODE_MAJOR="${XCODE_VER%%.*}"
    MACOS_MAJOR="${MACOS_VER%%.*}"
    # Xcode's major tracks the macOS major since Xcode 26 and trailed it by one
    # before that (Xcode 15 <-> macOS 14, 16 <-> 15), so anything older than
    # macOS_major-1 is certainly below Homebrew's minimum for this OS.
    case "${XCODE_MAJOR}${MACOS_MAJOR}" in
        *[!0-9]*|"") : ;;   # unparsable: stay quiet
        *)
            if [ "$XCODE_MAJOR" -lt $((MACOS_MAJOR - 1)) ]; then
                warn "${XCODE_APP} is Xcode ${XCODE_VER} on macOS ${MACOS_VER}: too old for Homebrew,"
                warn "which refuses to compile anything while it is present (even with a current CLT)."
                warn "Either update Xcode from the App Store, or remove it and use the CLT alone:"
                warn "  sudo mv '${XCODE_APP}' ~/.Trash/  &&  sudo xcode-select -s /Library/Developer/CommandLineTools"
            fi ;;
    esac
fi
exit 0
