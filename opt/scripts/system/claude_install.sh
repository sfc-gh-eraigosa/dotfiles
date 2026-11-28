#!/bin/bash
#
# Claude Code CLI Installation Script
#
# Cross-platform installer for Claude Code (https://claude.com/claude-code):
#   - macOS:       installed via Homebrew cask 'claude-code' (see opt/profiles/Brewfile)
#                  This script verifies presence and updates if needed.
#   - Linux:       Debian/Ubuntu, Raspberry Pi (arm64/armv7l), and NVIDIA Jetson — installed via npm.
#   - Windows WSL: Treated as Linux — installed via npm.
#
# SINGLE CANONICAL BINARY: this installer is the one source of truth for the
# `claude` binary. After (re)installing the canonical copy, it removes the
# official *native* installer's copy (the `curl claude.ai/install.sh` layout at
# ~/.local/bin/claude -> ~/.local/share/claude/...) when present, so a machine
# never ends up with two `claude` binaries + two self-update mechanisms and a
# PATH-order foot-gun. Auth/config under ~/.claude/ is never touched. The
# `claude` shell wrapper (ai/claude/aliases.sh) resolves the binary via PATH;
# `claude-config doctor` reports which binary wins and flags any duplicates.
#
# Re-running this script is safe and idempotent: it updates an existing install
# rather than re-installing from scratch, and the cleanup is a no-op once there
# is only one binary.

NPM_PKG="@anthropic-ai/claude-code"

# shellcheck source=../../lib/npm_allow_scripts.sh
_npm_allow_lib="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../lib" 2>/dev/null && pwd)/npm_allow_scripts.sh"
[ -f "${_npm_allow_lib}" ] && . "${_npm_allow_lib}"

# Detect platform -> echoes one of: macos | wsl | linux | unknown
detect_platform() {
    case "$(uname -s)" in
        Darwin*) echo "macos" ;;
        Linux*)
            if grep -qi microsoft /proc/version 2>/dev/null; then
                echo "wsl"
            else
                echo "linux"
            fi
            ;;
        *) echo "unknown" ;;
    esac
}

OS_KIND="$(detect_platform)"

install_via_brew() {
    if ! command -v brew &> /dev/null; then
        echo "WARNING: Homebrew not found on macOS. Falling back to npm install."
        install_via_npm
        return
    fi

    if brew list --cask claude-code &> /dev/null; then
        echo "claude-code cask already installed. Checking for updates..."
        brew upgrade --cask claude-code 2>/dev/null || true
        # `brew upgrade` is a no-op when current, so repair a truncated binary.
        local bp; bp="$(brew --prefix 2>/dev/null)/bin/claude"
        if [ -e "$bp" ] && ! claude_binary_healthy "$bp"; then
            echo "WARNING: $bp is broken (empty file or no version output). Reinstalling the cask..."
            brew reinstall --cask claude-code || true
        fi
    else
        echo "Installing claude-code cask..."
        brew install --cask claude-code || {
            echo "WARNING: brew install failed. Falling back to npm install."
            install_via_npm
        }
    fi
}

install_via_npm() {
    # Ensure node is available via nvm
    export NVM_DIR="$HOME/.nvm"
    # shellcheck disable=SC1091
    [ -s "$NVM_DIR/nvm.sh" ] && . "$NVM_DIR/nvm.sh"

    if ! command -v npm &> /dev/null; then
        if command -v nvm &> /dev/null; then
            echo "npm not found; installing latest LTS node via nvm..."
            nvm install --lts
            nvm alias default 'lts/*'
            nvm use default
        else
            echo "ERROR: npm/node not available and nvm not initialized. Install nvm first."
            exit 1
        fi
    fi

    # Before the install/update, so its postinstall (which installs the claude
    # binary) is allowed by name and npm stops listing it as unreviewed.
    if command -v npm_allow_scripts >/dev/null 2>&1; then
        npm_allow_scripts "$NPM_PKG"
    fi

    if npm ls -g --depth=0 "$NPM_PKG" &> /dev/null; then
        echo "$NPM_PKG already installed globally. Updating..."
        npm update -g "$NPM_PKG"
    else
        echo "Installing $NPM_PKG globally via npm..."
        npm install -g "$NPM_PKG"
    fi

    # `npm ls` only reads package metadata, and `npm update` is a no-op when
    # the version is current — so a truncated install (every file 0 bytes,
    # left by a crash/`wsl --shutdown` before the writes were flushed) would
    # survive every re-run. Check the binary itself and force a reinstall.
    local prefix; prefix="$(npm config get prefix 2>/dev/null || true)"
    local bin="$prefix/bin/claude"
    if [ -n "$prefix" ] && [ -e "$bin" ] && ! claude_binary_healthy "$bin"; then
        echo "WARNING: $bin is broken (empty file or no version output) — likely a truncated install."
        echo "Reinstalling $NPM_PKG with --force..."
        npm install -g --force "$NPM_PKG"
        if ! claude_binary_healthy "$bin"; then
            echo "ERROR: $bin is still broken after a forced reinstall."
            return 1
        fi
        echo "Repaired: $("$bin" --version 2>/dev/null | head -1)"
    fi
}

# Exit 0 when the claude binary at $1 is usable: a non-empty file (test -s
# follows symlinks) whose `--version` prints a version number. A 0-byte
# executable exits 0 and prints nothing, so presence + exit code are not
# enough to tell a working install from a truncated one.
claude_binary_healthy() {
    [ -s "$1" ] || return 1
    "$1" --version 2>/dev/null | grep -q '[0-9]'
}

# Resolve the path to the dotfiles-orchestrated ("canonical") claude binary —
# the npm-global (Linux/WSL) or brew (macOS) one — explicitly NOT a copy under
# ~/.local. Echoes the path, or nothing if none can be confirmed. Tests/hosts
# may override the result via $CLAUDE_CANONICAL_BIN.
resolve_canonical_claude() {
    if [ -n "${CLAUDE_CANONICAL_BIN:-}" ]; then
        echo "$CLAUDE_CANONICAL_BIN"
        return 0
    fi
    # macOS: the brew cask binary.
    if [ "$OS_KIND" = "macos" ] && command -v brew &> /dev/null; then
        local bp; bp="$(brew --prefix 2>/dev/null)/bin/claude"
        [ -x "$bp" ] && { echo "$bp"; return 0; }
    fi
    # Linux/WSL (and macOS npm fallback): the npm global bin.
    local prefix; prefix="$(npm config get prefix 2>/dev/null || true)"
    if [ -n "$prefix" ] && [ -x "$prefix/bin/claude" ]; then
        echo "$prefix/bin/claude"
        return 0
    fi
    # Last resort: the first claude on PATH that is not under ~/.local.
    local c
    for c in $(type -ap claude 2>/dev/null); do
        case "$c" in
            "$HOME/.local/"*) continue ;;
            *) [ -x "$c" ] && { echo "$c"; return 0; } ;;
        esac
    done
    return 0
}

# Echo the node version (e.g. 24.18.0) of a path inside an nvm tree
# (<NVM_DIR>/versions/node/v<ver>/...), or nothing when it is not one.
nvm_node_version_of() {
    case "$1" in
        */versions/node/v*/*)
            printf '%s\n' "$1" | sed 's|.*/versions/node/v\([0-9][0-9.]*\)/.*|\1|' ;;
    esac
}

# Exit 0 when dotted version $1 is strictly lower than $2 (numeric per field;
# awk, not `sort -V`, which BSD sort lacks).
version_lt() {
    awk -v a="$1" -v b="$2" 'BEGIN {
        na = split(a, x, "."); nb = split(b, y, "."); n = (na > nb) ? na : nb
        for (i = 1; i <= n; i++) {
            if (x[i] + 0 < y[i] + 0) exit 0
            if (x[i] + 0 > y[i] + 0) exit 1
        }
        exit 1
    }'
}

# Re-point a ~/.local/bin/claude symlink that targets the npm global of an
# OLDER nvm node (e.g. .../node/v24.16.0/lib/node_modules/...). `npm update -g`
# only updates the CURRENT node's global, and ~/.local/bin precedes nvm on
# PATH, so such a link silently pins claude to the old version forever (seen on
# a Pi: link -> v24.16.0 at 2.1.191 while v24.18.0 held 2.1.330). Re-pointing
# (not removing) keeps the path working for non-interactive ssh shells that
# never load nvm. Acts ONLY when the canonical claude is itself an nvm npm
# global on a strictly NEWER node than the link's target — so it can never
# move the link to an older node (e.g. an nvm `default` alias older than the
# node the user linked by hand). Every skip says why.
repoint_stale_nvm_link() {
    local link="$1" target="$2"
    local canonical; canonical="$(resolve_canonical_claude)"
    if [ -z "$canonical" ] || [ ! -x "$canonical" ]; then
        echo "  Skipping stale-nvm re-point of $link: no canonical claude confirmed."
        return 0
    fi
    case "$canonical" in
        "$HOME/.local/"*)
            echo "  Skipping stale-nvm re-point of $link: canonical also resolves under ~/.local."
            return 0 ;;
    esac
    local prefix="${canonical%/bin/claude}"
    case "$target" in
        "$prefix"/*|"$canonical") return 0 ;;  # already current
    esac
    local have want
    have="$(nvm_node_version_of "$target")"
    want="$(nvm_node_version_of "$canonical")"
    if [ -z "$have" ] || [ -z "$want" ]; then
        echo "  Skipping stale-nvm re-point of $link: canonical $canonical is not an nvm node global."
        return 0
    fi
    if ! version_lt "$have" "$want"; then
        echo "  Skipping stale-nvm re-point of $link: its node v$have is not older than the canonical v$want."
        return 0
    fi
    echo "Re-pointing $link from node v$have to the canonical claude on node v$want:"
    echo "  $target -> $canonical"
    ln -sfn "$canonical" "$link" \
        || echo "WARNING: could not re-point $link; claude on PATH may stay on node v$have."
}

# Remove the official native-installer copy of claude so only the canonical
# (npm/brew) binary remains. Guarded + idempotent + non-destructive to config:
#   - acts only on a ~/.local/bin/claude that is a SYMLINK INTO
#     ~/.local/share/claude/ (the native-installer layout); a convenience
#     symlink to the canonical binary is left alone;
#   - a symlink into an nvm node's npm global (<NVM_DIR>/versions/node/...)
#     is never removed, only re-pointed at the canonical claude when that is
#     on a newer node (see repoint_stale_nvm_link);
#   - removes nothing unless a canonical claude OUTSIDE ~/.local is confirmed
#     (so a failed install never leaves the user with no claude);
#   - never touches ~/.claude/ (auth + settings live there).
cleanup_conflicting_installs() {
    local native_link="$HOME/.local/bin/claude"
    local native_data="$HOME/.local/share/claude"
    local native_state="$HOME/.local/state/claude"

    # No native symlink => nothing to consolidate.
    [ -L "$native_link" ] || return 0

    local target; target="$(readlink "$native_link" 2>/dev/null || true)"
    # A relative link target is relative to the link's own directory.
    case "$target" in
        /*|"") ;;
        *) target="$(dirname "$native_link")/$target" ;;
    esac
    case "$target" in
        */.local/share/claude/*) ;;          # native installer — candidate
        */versions/node/*)                   # npm global of SOME nvm node (any NVM_DIR) — maybe stale
            repoint_stale_nvm_link "$native_link" "$target"
            return 0 ;;
        *) return 0 ;;                        # points elsewhere (e.g. canonical) — keep
    esac

    local canonical; canonical="$(resolve_canonical_claude)"
    if [ -z "$canonical" ] || [ ! -x "$canonical" ]; then
        echo "  Skipping native-install cleanup: no canonical claude confirmed (leaving ~/.local install intact)."
        return 0
    fi
    case "$canonical" in
        "$HOME/.local/"*)
            echo "  Skipping native-install cleanup: canonical also resolves under ~/.local."
            return 0 ;;
    esac

    echo "Consolidating to a single Claude binary (canonical: $canonical)."
    echo "Removing the non-orchestrated native install:"
    echo "  - $native_link -> $target"
    rm -f "$native_link"
    if [ -d "$native_data" ]; then echo "  - $native_data/"; rm -rf "$native_data"; fi
    if [ -d "$native_state" ]; then echo "  - $native_state/"; rm -rf "$native_state"; fi
    echo "Done. ~/.claude/ (auth + settings) was not touched."
}

main() {
    set -e
    echo "Detected platform: $OS_KIND"

    case "$OS_KIND" in
        macos) install_via_brew ;;
        linux|wsl) install_via_npm ;;
        *)
            echo "ERROR: Unsupported platform: $(uname -s)"
            exit 1
            ;;
    esac

    # Enforce a single canonical binary (remove any native-installer copy).
    cleanup_conflicting_installs

    # Verify
    if command -v claude &> /dev/null; then
        if ! claude_binary_healthy "$(command -v claude)"; then
            echo "ERROR: $(command -v claude) is broken (empty file or no version output)."
            echo "       Reinstall it: npm install -g --force $NPM_PKG  (macOS: brew reinstall --cask claude-code)"
            echo "       'claude-config doctor' shows which binary is in use."
            exit 1
        fi
        echo "Claude Code installed successfully!"
        claude --version
        echo "Run 'claude' to authenticate and get started."
        echo "Tip: 'claude-config doctor' reports which binary is in use."
    else
        echo "WARNING: 'claude' is not on PATH after install. You may need to open a new shell."
        echo "         npm global bin: $(npm config get prefix 2>/dev/null)/bin"
    fi
}

# Run only when executed directly; sourcing (e.g. the test driver) loads the
# functions without performing an install.
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi
