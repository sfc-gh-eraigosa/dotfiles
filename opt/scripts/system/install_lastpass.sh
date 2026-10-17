#!/usr/bin/env bash
# ==============================================================================
# LastPass CLI Setup — install `lpass` (lastpass-cli) from the OS package manager
# ==============================================================================
# Why this exists:
#   * `lpass` is the official LastPass command-line client (lastpass/lastpass-cli,
#     GPL-2.0): `lpass show --password <entry>` in scripts, `lpass ls` in a shell.
#   * Every fleet platform packages it, so unlike sops/yq/glow there is no release
#     binary to fetch and verify: Debian/Ubuntu/Raspberry Pi OS/Jetson carry
#     `lastpass-cli` in apt, Fedora in dnf, macOS in Homebrew. The distro build
#     is also the one linked against the host's own libcurl/openssl/libxml2.
#   * It is a per-tool install.sh step (gff install.tools.lastpass, on by
#     default) rather than a packages.tsv row, so a host can drop it with
#     `gff set install.tools.lastpass false` without touching common-core.
#
# Safe to re-run: exits early when `lpass` already runs. Like pkg-install-apt,
# when sudo would need a password and there is no terminal to ask on, it does
# not touch sudo or apt — ONE WARNING, exit 0 — so an unattended `fleet update`
# never stalls here.
#
# Environment:
#   LASTPASS_FORCE=1   run the package manager even when lpass is present
#                      (upgrades to the candidate version)
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}$*${NC}"; }
ok()   { echo -e "${GREEN}$*${NC}"; }
warn() { echo -e "${RED}WARNING: install_lastpass: $*${NC}" >&2; }
die()  { echo -e "${RED}install_lastpass: $*${NC}" >&2; exit 1; }

# "LastPass CLI v1.3.7" -> 1.3.7 (empty when lpass is absent or does not run)
installed_version() {
    command -v lpass >/dev/null 2>&1 || return 0
    lpass --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -n 1 || true
}

HAVE="$(installed_version)"
if [ -n "${HAVE}" ] && [ "${LASTPASS_FORCE:-0}" != "1" ]; then
    ok "lastpass-cli already installed: $(command -v lpass) (${HAVE})"
    exit 0
fi

# Run a command as root: directly when we already are, via sudo otherwise.
if [ "$(id -u)" -eq 0 ]; then
    as_root() { "$@"; }
else
    as_root() { sudo "$@"; }
fi
# True when sudo can run without a prompt (root, NOPASSWD, or a cached credential).
sudo_noninteractive() {
    [ "$(id -u)" -eq 0 ] || sudo -n true 2>/dev/null
}
# A privileged package install with no way to authenticate: report and leave
# it for an interactive run instead of failing the whole install.
need_root_or_skip() {
    if ! sudo_noninteractive && [ ! -t 0 ]; then
        warn "sudo needs a password and there is no terminal; skipping the lastpass-cli install ($1). Re-run install.sh interactively."
        exit 0
    fi
}

case "$(uname -s)" in
    Darwin)
        command -v brew >/dev/null 2>&1 || die "Homebrew not found — install it first: https://brew.sh/"
        info "Installing lastpass-cli via brew..."
        brew install lastpass-cli
        ;;
    Linux)
        if command -v apt-get >/dev/null 2>&1; then
            need_root_or_skip apt
            info "Installing lastpass-cli via apt..."
            # DEBIAN_FRONTEND=noninteractive: no debconf prompt can block an
            # unattended run. Try the current index first (install.sh's
            # common-core step usually refreshed it moments ago); refresh and
            # retry once when the package or its candidate is not there.
            if ! as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends lastpass-cli; then
                info "apt install failed on the current index; refreshing and retrying once..."
                as_root apt-get update -qq
                as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends lastpass-cli
            fi
        elif command -v dnf >/dev/null 2>&1; then
            need_root_or_skip dnf
            info "Installing lastpass-cli via dnf..."
            as_root dnf install -y -q lastpass-cli
        else
            die "no supported package manager (apt-get, dnf) on this Linux host — install lastpass-cli by hand: https://github.com/lastpass/lastpass-cli"
        fi
        ;;
    *)
        die "unsupported OS $(uname -s)"
        ;;
esac

HAVE="$(installed_version)"
[ -n "${HAVE}" ] || die "lpass not resolvable after install — is the package manager's bin dir on PATH?"
ok "Success! lastpass-cli ${HAVE} at $(command -v lpass)"
