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

    if npm ls -g --depth=0 "$NPM_PKG" &> /dev/null; then
        echo "$NPM_PKG already installed globally. Updating..."
        npm update -g "$NPM_PKG"
    else
        echo "Installing $NPM_PKG globally via npm..."
        npm install -g "$NPM_PKG"
    fi
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

# Remove the official native-installer copy of claude so only the canonical
# (npm/brew) binary remains. Guarded + idempotent + non-destructive to config:
#   - acts only on a ~/.local/bin/claude that is a SYMLINK INTO
#     ~/.local/share/claude/ (the native-installer layout); a convenience
#     symlink to the canonical binary is left alone;
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
    case "$target" in
        */.local/share/claude/*) ;;          # native installer — candidate
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
        echo "Claude Code installed successfully!"
        claude --version || true
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
