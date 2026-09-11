#!/usr/bin/env bash
# ==============================================================================
# Rust Setup — rustup + the stable toolchain into ~/.cargo and ~/.rustup
# ==============================================================================
# Why this exists:
#   * herdr-file-viewer (`install_herdr.sh plugins`) publishes prebuilt
#     binaries for x86_64 Linux and macOS only. Every aarch64 host (Spark,
#     Jetson, 64-bit Pi) builds it from source with cargo, and it needs Rust
#     1.96+. Rust is installed per user by rustup, like goenv/pyenv/nvm.
#   * rustup's documented install pipes sh.rustup.rs straight into a shell,
#     which this repo avoids. We fetch the same rustup-init binary for the host
#     triple from static.rust-lang.org and verify it against the SHA-256
#     published beside it. Fail-closed: no valid checksum, no install.
#   * --no-modify-path is load-bearing. Without it rustup appends
#     `. "$HOME/.cargo/env"` to every rc file it finds, and ours are symlinks
#     into this repo, so the append lands in the working tree (unguarded, it
#     breaks every shell on a host without Rust). opt/profiles/.profile and
#     .zshrc put ~/.cargo/bin on PATH instead.
#   * The minimal profile (rustc, cargo, rust-std) is enough to build; add
#     more with `rustup component add clippy rustfmt`.
#   * Re-runs are cheap: an existing rustup is left alone unless its rustc is
#     older than RUST_MIN_VERSION, which triggers `rustup update`. rustup
#     keeps itself current with `rustup update`; RUST_UPDATE=1 runs it here.
#
# Safe to re-run. Env overrides:
#   RUST_TOOLCHAIN     toolchain to install / update (default: stable)
#   RUST_MIN_VERSION   oldest acceptable rustc (default: 1.96.0, herdr-file-viewer's MSRV)
#   RUST_UPDATE=1      run `rustup update <toolchain>` even when rustc is new enough
#   CARGO_HOME         cargo home (default: ~/.cargo; rustup's own variable)
#   RUSTUP_HOME        rustup home (default: ~/.rustup; rustup's own variable)
#   RUST_HOST_TRIPLE   override the detected host triple (tests)
#   RUSTUP_INIT_BASE   rustup-init download base (default: https://static.rust-lang.org/rustup/dist)
set -e

RUST_TOOLCHAIN="${RUST_TOOLCHAIN:-stable}"
RUST_MIN_VERSION="${RUST_MIN_VERSION:-1.96.0}"
CARGO_HOME="${CARGO_HOME:-${HOME}/.cargo}"
RUSTUP_INIT_BASE="${RUSTUP_INIT_BASE:-https://static.rust-lang.org/rustup/dist}"
export CARGO_HOME

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}$*${NC}"; }
ok()   { echo -e "${GREEN}$*${NC}"; }
warn() { echo -e "${YELLOW}install_rust: $*${NC}" >&2; }
die()  { echo -e "${RED}install_rust: $*${NC}" >&2; exit 1; }

TMP_DIR=""
# `if`, not `[ … ] &&`: under set -e a false && in the EXIT trap would turn a
# successful run (nothing to clean) into exit status 1.
cleanup() { if [ -n "${TMP_DIR}" ]; then rm -rf "${TMP_DIR}"; fi; }
trap cleanup EXIT

host_triple() {
    if [ -n "${RUST_HOST_TRIPLE:-}" ]; then
        echo "${RUST_HOST_TRIPLE}"
        return
    fi
    case "$(uname -s)" in
        Linux)
            case "$(uname -m)" in
                x86_64)        echo "x86_64-unknown-linux-gnu" ;;
                arm64|aarch64) echo "aarch64-unknown-linux-gnu" ;;
                armv7l)        echo "armv7-unknown-linux-gnueabihf" ;;
                armv6l)        echo "arm-unknown-linux-gnueabihf" ;;
                *) die "unsupported arch $(uname -m)" ;;
            esac ;;
        Darwin)
            case "$(uname -m)" in
                x86_64)        echo "x86_64-apple-darwin" ;;
                arm64|aarch64) echo "aarch64-apple-darwin" ;;
                *) die "unsupported arch $(uname -m)" ;;
            esac ;;
        *) die "unsupported OS $(uname -s)" ;;
    esac
}

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum < "$1" | awk '{ print $1 }'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 < "$1" | awk '{ print $1 }'
    elif command -v openssl >/dev/null 2>&1; then
        openssl dgst -sha256 < "$1" | awk '{ print $NF }'
    else
        die "SHA-256 verification requires sha256sum, shasum, or openssl"
    fi
}

# version_ge A B: true when dotted version A >= B. Numeric per field, so
# 1.100 > 1.96; a suffix such as "-nightly" is ignored.
version_ge() {
    awk -v a="$1" -v b="$2" 'BEGIN {
        n = split(a, x, "."); m = split(b, y, ".")
        for (i = 1; i <= (n > m ? n : m); i++) {
            if ((x[i] + 0) > (y[i] + 0)) exit 0
            if ((x[i] + 0) < (y[i] + 0)) exit 1
        }
        exit 0
    }'
}

rustc_version() {
    if [ -x "${CARGO_HOME}/bin/rustc" ]; then
        "${CARGO_HOME}/bin/rustc" --version 2>/dev/null | awk '{ print $2 }'
    fi
}

update_toolchain() {
    info "Updating Rust toolchain '${RUST_TOOLCHAIN}' ($1)..."
    "${CARGO_HOME}/bin/rustup" update "${RUST_TOOLCHAIN}" --no-self-update \
        || die "rustup update ${RUST_TOOLCHAIN} failed"
}

install_rustup() {
    TRIPLE="$(host_triple)"
    URL="${RUSTUP_INIT_BASE}/${TRIPLE}/rustup-init"
    command -v curl >/dev/null 2>&1 || die "curl is required to download rustup-init"
    TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/install_rust.XXXXXX")"
    DL="${TMP_DIR}/rustup-init"

    info "Installing rustup + Rust '${RUST_TOOLCHAIN}' (${TRIPLE}) into ${CARGO_HOME}..."
    curl -fsSL "${URL}.sha256" -o "${DL}.sha256" || die "could not fetch ${URL}.sha256"
    SHA="$(awk '{ print $1; exit }' "${DL}.sha256" | tr '[:upper:]' '[:lower:]')"
    # Fail closed: a missing or malformed checksum means we do not install.
    if [ "${#SHA}" -ne 64 ] || [ -n "$(printf '%s' "${SHA}" | tr -d '0-9a-f')" ]; then
        die "no valid SHA-256 checksum at ${URL}.sha256; refusing to run an unverified rustup-init"
    fi
    curl -fsSL "${URL}" -o "${DL}" || die "download failed: ${URL}"
    ACTUAL="$(sha256_of "${DL}")"
    [ "${ACTUAL}" = "${SHA}" ] || die "checksum mismatch for ${URL}: expected ${SHA}, got ${ACTUAL}"
    chmod +x "${DL}"

    # A distro rustc elsewhere in PATH makes rustup-init stop and ask; the
    # profiles put ~/.cargo/bin first, so the rustup toolchain wins anyway.
    # Its chatter (download bars, a PATH how-to we already handle) is kept
    # out of install.sh output and shown only when it fails.
    if ! RUSTUP_INIT_SKIP_PATH_CHECK=yes "${DL}" -y --no-modify-path \
        --profile minimal --default-toolchain "${RUST_TOOLCHAIN}" > "${TMP_DIR}/rustup-init.log" 2>&1; then
        tail -n 20 "${TMP_DIR}/rustup-init.log" >&2
        die "rustup-init failed"
    fi

    HAVE="$(rustc_version)"
    [ -n "${HAVE}" ] || die "rustup-init finished but ${CARGO_HOME}/bin/rustc does not run"
    ok "Success! rustc ${HAVE} (${CARGO_HOME}/bin; rustup-init sha256 verified)"
}

if [ -x "${CARGO_HOME}/bin/rustup" ]; then
    HAVE="$(rustc_version)"
    if [ -z "${HAVE}" ]; then
        update_toolchain "rustup present, no working rustc"
    elif ! version_ge "${HAVE}" "${RUST_MIN_VERSION}"; then
        update_toolchain "rustc ${HAVE} is older than ${RUST_MIN_VERSION}"
    elif [ "${RUST_UPDATE:-0}" = "1" ]; then
        update_toolchain "RUST_UPDATE=1"
    else
        ok "Rust already installed: rustc ${HAVE} (${CARGO_HOME}/bin)"
        exit 0
    fi
    ok "Rust ready: rustc $(rustc_version) (${CARGO_HOME}/bin)"
else
    install_rustup
fi
