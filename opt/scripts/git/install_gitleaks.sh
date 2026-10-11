#!/usr/bin/env bash
# install_gitleaks.sh — put `gitleaks` on this machine so the privacy guard
# (agent hook + git hooks, via ai/hooks/privacy_rules.sh) judges secrets with
# its upstream-maintained ruleset instead of only our 16 built-in shapes.
#
# Run by install.sh behind the gff flag install.git.gitleaks (default on).
#
# Usage: install_gitleaks.sh            ensure installed (idempotent, fast)
#        install_gitleaks.sh --off      flag is false: install nothing and write
#                                       ~/.config/privacy_guard/gitleaks = off so
#                                       the hooks skip a binary that is present anyway
#
# Environment:
#   GITLEAKS_INSTALL_METHOD  auto (default) | apt | brew | release
#                            auto: apt when apt has a candidate, else brew, else release
#   GITLEAKS_VERSION         release version for the download path (default:
#                            opt/profiles/versions.tsv via pins_want, else 8.21.2)
#   GITLEAKS_FORCE=1         reinstall the release binary even when already at it
#   GITLEAKS_INSTALL_DIR     where the release binary goes (default ~/opt/bin)
#   GITLEAKS_RELEASE_BASE    release URL base (tests point it at a fixture)
#
# The release path is sha256-verified against the upstream checksums file and
# fails closed on any mismatch. No `apt-get update`: one package, keep it fast.
#
# Converges (objective `pins`): a gitleaks in GITLEAKS_INSTALL_DIR (ours) that
# differs from the wanted version is replaced from the release; one already on
# it is a no-op. A gitleaks that resolves OUTSIDE that dir (apt/brew — apt's
# build prints "version is set by build process") is left alone: another
# package manager owns it. WANT=latest resolves via the GitHub releases API.
set -u

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PINS_LIB="${SELF_DIR}/../../lib/pins.sh"
if [ -f "${PINS_LIB}" ]; then
    # shellcheck source=../../lib/pins.sh
    . "${PINS_LIB}"
fi
# Fail-open: without the lib, the hardcoded default below still applies.
command -v pins_want >/dev/null 2>&1 || pins_want() { printf '%s\n' "${2:-}"; }
command -v pins_latest_gh >/dev/null 2>&1 || pins_latest_gh() { return 1; }

METHOD="${GITLEAKS_INSTALL_METHOD:-auto}"
VERSION="${GITLEAKS_VERSION:-$(pins_want gitleaks 8.21.2)}"
VERSION="${VERSION#v}"
INSTALL_DIR="${GITLEAKS_INSTALL_DIR:-$HOME/opt/bin}"
RELEASE_BASE="${GITLEAKS_RELEASE_BASE:-https://github.com/gitleaks/gitleaks/releases/download}"
CFG_DIR="${PRIVACY_GUARD_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/privacy_guard}"
MARKER="$CFG_DIR/gitleaks"

ok()   { echo "OK:   $*"; }
info() { echo "INFO: $*"; }
warn() { echo "WARN: $*" >&2; }
die()  { echo "FAIL: $*" >&2; exit 1; }

if [ "${1:-}" = "--off" ]; then
    mkdir -p "$CFG_DIR" && printf 'off\n' > "$MARKER"
    ok "gitleaks disabled for the privacy guard ($MARKER = off); nothing installed"
    exit 0
fi
[ -f "$MARKER" ] && { rm -f "$MARKER"; info "removed $MARKER (flag is on again)"; }

# resolve_version: turn VERSION=latest into a concrete release, and refuse
# anything that is not x.y.z before it reaches a URL.
resolve_version() {
    if [ "$VERSION" = "latest" ]; then
        VERSION="$(pins_latest_gh gitleaks/gitleaks)" || die "could not resolve the latest gitleaks release"
    fi
    case "$VERSION" in *[!0-9.]*|"") die "GITLEAKS_VERSION must look like 8.21.2 (got '$VERSION')" ;; esac
}
# "gitleaks version 8.21.2" / "8.21.2" -> 8.21.2 ("" when there is none)
version_of() { "$1" version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -n1 || true; }

OWN="$INSTALL_DIR/gitleaks"
FOUND="$(command -v gitleaks 2>/dev/null || true)"
CONVERGE=0
if [ -x "$OWN" ]; then
    # Ours: converge it to the wanted version via the release download.
    resolve_version
    HAVE="$(version_of "$OWN")"
    if [ "$HAVE" = "$VERSION" ] && [ "${GITLEAKS_FORCE:-0}" != "1" ]; then
        ok "gitleaks already at $HAVE: $OWN"
        exit 0
    fi
    info "Updating gitleaks ${HAVE:-unknown} -> $VERSION in $INSTALL_DIR..."
    CONVERGE=1
elif [ -n "$FOUND" ] && [ "${GITLEAKS_FORCE:-0}" != "1" ]; then
    ok "gitleaks already installed: $FOUND ($(gitleaks version 2>/dev/null | head -n1)) — managed outside $INSTALL_DIR; leaving it alone"
    exit 0
fi

apt_candidate() {
    command -v apt-get >/dev/null 2>&1 && command -v apt-cache >/dev/null 2>&1 || return 1
    apt-cache policy gitleaks 2>/dev/null | grep -E '^\s*Candidate:' | grep -vq '(none)'
}

install_apt() {
    info "Installing gitleaks with apt-get..."
    sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq gitleaks
}
install_brew() {
    info "Installing gitleaks with brew..."
    brew install gitleaks
}
sha256_of() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }
install_release() {
    local os arch asset sums tmp want got
    case "$(uname -s)" in Darwin) os=darwin ;; Linux) os=linux ;; *) die "unsupported OS $(uname -s)" ;; esac
    case "$(uname -m)" in
        x86_64|amd64)  arch=x64 ;;
        arm64|aarch64) arch=arm64 ;;
        armv7l)        arch=armv7 ;;
        *) die "unsupported arch $(uname -m)" ;;
    esac
    command -v curl >/dev/null 2>&1 || die "curl is required for the release download"
    resolve_version
    asset="gitleaks_${VERSION}_${os}_${arch}.tar.gz"
    sums="gitleaks_${VERSION}_checksums.txt"
    tmp="$(mktemp -d "${TMPDIR:-/tmp}/install_gitleaks.XXXXXX")"
    info "Downloading gitleaks ${VERSION} (${os}/${arch}) from ${RELEASE_BASE}..."
    curl -fsSL "${RELEASE_BASE}/v${VERSION}/${sums}" -o "$tmp/$sums" || { rm -rf "$tmp"; die "could not fetch $sums"; }
    curl -fsSL "${RELEASE_BASE}/v${VERSION}/${asset}" -o "$tmp/$asset" || { rm -rf "$tmp"; die "could not fetch $asset"; }
    want="$(grep -E "[[:space:]]\*?${asset}\$" "$tmp/$sums" | head -n1 | cut -d' ' -f1 | tr '[:upper:]' '[:lower:]')"
    [ "${#want}" -eq 64 ] || { rm -rf "$tmp"; die "no sha256 for $asset in $sums; refusing an unverified binary"; }
    got="$(sha256_of "$tmp/$asset")"
    [ "$got" = "$want" ] || { rm -rf "$tmp"; die "checksum mismatch for $asset: expected $want, got $got"; }
    tar -C "$tmp" -xzf "$tmp/$asset" gitleaks || { rm -rf "$tmp"; die "could not extract gitleaks from $asset"; }
    mkdir -p "$INSTALL_DIR"
    chmod +x "$tmp/gitleaks"
    mv "$tmp/gitleaks" "$INSTALL_DIR/gitleaks"
    rm -rf "$tmp"
    "$INSTALL_DIR/gitleaks" version >/dev/null 2>&1 || die "installed binary does not run: $INSTALL_DIR/gitleaks"
    ok "gitleaks ${VERSION} installed to $INSTALL_DIR/gitleaks (sha256 verified)"
    case ":$PATH:" in *":$INSTALL_DIR:"*) ;; *) warn "$INSTALL_DIR is not on PATH in this shell; the profiles put ~/opt/bin first in login shells" ;; esac
}

if [ "$CONVERGE" = "1" ]; then
    install_release
else
case "$METHOD" in
    apt)     install_apt || { warn "apt install failed; falling back to the release download"; install_release; } ;;
    brew)    install_brew ;;
    release) install_release ;;
    auto)
        if apt_candidate; then
            install_apt || { warn "apt install failed; falling back to the release download"; install_release; }
        elif command -v brew >/dev/null 2>&1; then
            install_brew
        else
            install_release
        fi ;;
    *) die "GITLEAKS_INSTALL_METHOD must be auto|apt|brew|release (got '$METHOD')" ;;
esac
fi

if command -v gitleaks >/dev/null 2>&1 || [ -x "$INSTALL_DIR/gitleaks" ]; then
    ok "gitleaks ready: $(command -v gitleaks 2>/dev/null || echo "$INSTALL_DIR/gitleaks")"
else
    die "gitleaks is still not on PATH after install"
fi
