#!/usr/bin/env bash
# ==============================================================================
# herdr Setup — install the `herdr` agent terminal workspace into ~/opt/bin
# ==============================================================================
# Why this exists:
#   * herdr (https://github.com/herdrdev/herdr, Apache-2.0) ships static release
#     binaries for linux + macOS on x86_64 and aarch64. There is no apt package;
#     brew, mise and nix packages exist upstream, but this repo has no mise/nix
#     surface and a brew copy would lag the "track latest" policy below. One
#     script covers every host we run (WSL, macOS, Jetson, 64-bit Pi).
#   * The upstream installer (herdr.dev/install.sh, piped straight into a shell)
#     works, but cannot pin a version, lands in ~/.local/bin, and is a pattern
#     this repo avoids. We fetch the same release asset ourselves and verify the
#     SHA-256 herdr publishes in its manifest (https://herdr.dev/latest.json).
#     Fail-closed: no published checksum, no install.
#   * Tracks LATEST by default so `fleet update` keeps every host on the same
#     release. herdr's client/server protocol is versioned, so `herdr --remote`
#     between hosts on different releases is the thing to avoid. Pin with
#     HERDR_VERSION=x.y.z when a release misbehaves; the manifest carries
#     checksums for older releases too.
#   * `herdr update` (herdr's self-updater) writes over whichever binary it is
#     run from. That is harmless here — the next install.sh / fleet update
#     re-converges the host on the manifest version.
#
# Modes:
#   install_herdr.sh                # the binary            (install.sh --phase deps)
#   install_herdr.sh integrations   # agent integrations    (install.sh --phase config)
#   install_herdr.sh config         # managed config.toml   (install.sh --phase config)
#
#   `config` renders ai/herdr/config.toml into ~/.config/herdr/config.toml so
#   herdr follows the host terminal's light/dark appearance with the fleet
#   Solarized palette (a dark catppuccin sidebar on a Solarized Light terminal
#   is unreadable). The host OWNS the file: it is written when missing or when
#   it still carries the "managed by dotfiles" marker; a hand-edited file
#   without the marker is left alone (HERDR_CONFIG_FORCE=1 reclaims it, keeping
#   a .bak). A running server is reloaded best-effort so the change is live.
#
#   `integrations` runs `herdr integration install <id>` for each id in
#   HERDR_INTEGRATIONS whose agent CLI is present on this host. Ordering
#   relative to install_antigravity_skills.sh no longer matters: that script
#   MERGES its `guards` entry into ~/.gemini/config/hooks.json and preserves
#   every other named hook (herdr's included). Claude's SessionStart hook
#   likewise survives the forced-settings merge (only hooks.PreToolUse /
#   DirectoryAdded are replaced).
#
# Safe to re-run. Env overrides:
#   HERDR_VERSION       release to install (default: latest from the manifest)
#   HERDR_INSTALL_DIR   target dir (default: ~/opt/bin)
#   HERDR_FORCE=1       reinstall even when the wanted version is already present
#   HERDR_INTEGRATIONS  space-separated integration ids (default: claude antigravity-cli)
#   HERDR_CONFIG_DIR    herdr config dir (default: ${XDG_CONFIG_HOME:-~/.config}/herdr)
#   HERDR_CONFIG_TEMPLATE  template to render (default: ai/herdr/config.toml in this repo)
#   HERDR_THEME_DARK    dark theme / no-report fallback (default: solarized)
#   HERDR_THEME_LIGHT   light theme (default: solarized-light)
#   HERDR_CONFIG_FORCE=1  overwrite a hand-edited (unmanaged) config.toml, keeping a .bak
#   HERDR_MANIFEST_URL  manifest location (default: https://herdr.dev/latest.json)
#   HERDR_MANIFEST_FILE use a local manifest file instead of fetching (tests)
set -e

MODE="${1:-install}"
HERDR_VERSION="${HERDR_VERSION:-latest}"
INSTALL_DIR="${HERDR_INSTALL_DIR:-${HOME}/opt/bin}"
HERDR_INTEGRATIONS="${HERDR_INTEGRATIONS:-claude antigravity-cli}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
CONFIG_DIR="${HERDR_CONFIG_DIR:-${XDG_CONFIG_HOME:-${HOME}/.config}/herdr}"
CONFIG_TEMPLATE="${HERDR_CONFIG_TEMPLATE:-${REPO_ROOT}/ai/herdr/config.toml}"
HERDR_THEME_DARK="${HERDR_THEME_DARK:-solarized}"
HERDR_THEME_LIGHT="${HERDR_THEME_LIGHT:-solarized-light}"
MANAGED_MARKER="managed by dotfiles"
MANIFEST_URL="${HERDR_MANIFEST_URL:-https://herdr.dev/latest.json}"
RELEASE_BASE="https://github.com/herdrdev/herdr/releases/download"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}$*${NC}"; }
ok()   { echo -e "${GREEN}$*${NC}"; }
warn() { echo -e "${YELLOW}install_herdr: $*${NC}" >&2; }
die()  { echo -e "${RED}install_herdr: $*${NC}" >&2; exit 1; }

# install.sh may run from a non-login shell (`fleet update` over ssh) where
# ~/opt/bin is not yet on PATH; make sure our own binary is resolvable.
case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *) PATH="${INSTALL_DIR}:${PATH}" ;;
esac

TMP_DIR=""
# `if`, not `[ … ] &&`: under set -e a false && in the EXIT trap would turn a
# successful run (nothing to clean) into exit status 1.
cleanup() { if [ -n "${TMP_DIR}" ]; then rm -rf "${TMP_DIR}"; fi; }
trap cleanup EXIT

# ------------------------------------------------------------------------------
# Manifest access. herdr.dev/latest.json carries the current version plus, per
# target, the asset URL and SHA-256 — and a `releases.<version>` map with the
# same for older releases, so a pinned install can still be checksummed.
# jq is preferred (installed by the common core); python3 is the fallback.
# ------------------------------------------------------------------------------
MANIFEST=""
load_manifest() {
    if [ -n "${HERDR_MANIFEST_FILE:-}" ]; then
        [ -r "${HERDR_MANIFEST_FILE}" ] || die "manifest file not readable: ${HERDR_MANIFEST_FILE}"
        MANIFEST="${HERDR_MANIFEST_FILE}"
        return
    fi
    command -v curl >/dev/null 2>&1 || die "curl is required to fetch the release manifest"
    TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/install_herdr.XXXXXX")"
    MANIFEST="${TMP_DIR}/latest.json"
    curl -fsSL "${MANIFEST_URL}" -o "${MANIFEST}" || die "could not fetch ${MANIFEST_URL}"
}

# manifest_field <version|latest> <version|assets|sha256> <target>
manifest_field() {
    if command -v jq >/dev/null 2>&1; then
        if [ "$1" = "latest" ]; then
            case "$2" in
                version) jq -r '.version // empty' "${MANIFEST}" ;;
                *)       jq -r --arg k "$2" --arg t "$3" '.[$k][$t] // empty' "${MANIFEST}" ;;
            esac
        else
            case "$2" in
                version) jq -r --arg v "$1" 'if .releases[$v] then $v else empty end' "${MANIFEST}" ;;
                *)       jq -r --arg v "$1" --arg k "$2" --arg t "$3" '.releases[$v][$k][$t] // empty' "${MANIFEST}" ;;
            esac
        fi
    elif command -v python3 >/dev/null 2>&1; then
        python3 - "${MANIFEST}" "$1" "$2" "${3:-}" <<'PY'
import json, sys
path, ver, key, target = sys.argv[1:5]
with open(path) as fh:
    doc = json.load(fh)
node = doc if ver == "latest" else doc.get("releases", {}).get(ver)
if node is None:
    print("")
elif key == "version":
    print(doc.get("version", "") if ver == "latest" else ver)
else:
    print((node.get(key) or {}).get(target) or "")
PY
    else
        die "jq or python3 is required to read the release manifest"
    fi
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

installed_version() {
    if [ -x "${INSTALL_DIR}/herdr" ]; then
        "${INSTALL_DIR}/herdr" --version 2>/dev/null | awk '{ print $2 }'
    fi
}

# ------------------------------------------------------------------------------
# Mode: install the binary
# ------------------------------------------------------------------------------
install_binary() {
    # --- OS / arch detection (matches the herdr release asset naming) ---
    case "$(uname -s)" in
        Linux)  HERDR_OS="linux" ;;
        Darwin) HERDR_OS="macos" ;;
        *) die "unsupported OS $(uname -s)" ;;
    esac
    case "$(uname -m)" in
        x86_64)        HERDR_ARCH="x86_64" ;;
        arm64|aarch64) HERDR_ARCH="aarch64" ;;
        armv7l|armv6l)
            # herdr publishes no 32-bit ARM build. A 64-bit Pi/Nano userland
            # reports aarch64 and is covered above.
            warn "no upstream build for $(uname -m) (32-bit ARM); skipping herdr"
            exit 0 ;;
        *) die "unsupported arch $(uname -m)" ;;
    esac
    TARGET="${HERDR_OS}-${HERDR_ARCH}"

    load_manifest
    WANT="$(manifest_field "${HERDR_VERSION}" version "")"
    [ -n "${WANT}" ] || die "release ${HERDR_VERSION} is not in the manifest (${MANIFEST_URL})"

    HAVE="$(installed_version)"
    if [ "${HAVE}" = "${WANT}" ] && [ "${HERDR_FORCE:-0}" != "1" ]; then
        ok "herdr already installed: ${INSTALL_DIR}/herdr (${HAVE})"
        report_shadowing
        return 0
    fi

    URL="$(manifest_field "${HERDR_VERSION}" assets "${TARGET}")"
    [ -n "${URL}" ] || URL="${RELEASE_BASE}/v${WANT}/herdr-${TARGET}"
    SHA="$(manifest_field "${HERDR_VERSION}" sha256 "${TARGET}")"
    SHA="$(printf '%s' "${SHA}" | tr '[:upper:]' '[:lower:]')"
    # Fail closed: the manifest must publish a checksum for this exact
    # version+target, or we do not install.
    if [ "${#SHA}" -ne 64 ] || [ -n "$(printf '%s' "${SHA}" | tr -d '0-9a-f')" ]; then
        die "manifest has no valid SHA-256 checksum for herdr ${WANT} (${TARGET}); refusing to install an unverified binary"
    fi

    command -v curl >/dev/null 2>&1 || die "curl is required to download herdr"
    [ -n "${TMP_DIR}" ] || TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/install_herdr.XXXXXX")"
    DL="${TMP_DIR}/herdr"

    if [ -n "${HAVE}" ]; then
        info "Updating herdr ${HAVE} -> ${WANT} (${TARGET}) in ${INSTALL_DIR}..."
    else
        info "Installing herdr ${WANT} (${TARGET}) to ${INSTALL_DIR}/herdr..."
    fi
    curl -fsSL "${URL}" -o "${DL}" || die "download failed: ${URL}"

    ACTUAL="$(sha256_of "${DL}")"
    [ "${ACTUAL}" = "${SHA}" ] || die "checksum mismatch for ${URL}: expected ${SHA}, got ${ACTUAL}"
    chmod +x "${DL}"

    mkdir -p "${INSTALL_DIR}"
    # Clear any stale entry (including a dangling symlink) before moving the
    # verified binary into place. `-e` is false for a dangling symlink, so test
    # `-L` too.
    if [ -e "${INSTALL_DIR}/herdr" ] || [ -L "${INSTALL_DIR}/herdr" ]; then
        rm -f "${INSTALL_DIR}/herdr"
    fi
    mv "${DL}" "${INSTALL_DIR}/herdr"

    "${INSTALL_DIR}/herdr" --version >/dev/null 2>&1 || die "installed binary does not run: ${INSTALL_DIR}/herdr"
    ok "Success! $("${INSTALL_DIR}/herdr" --version | head -1) (sha256 verified)"
    report_shadowing
}

# A host bootstrapped with the upstream installer has a second copy in
# ~/.local/bin. Our profiles put ~/opt/bin first, so ours wins in login shells,
# but say so rather than leave two herdr binaries silently diverging.
report_shadowing() {
    RESOLVED="$(command -v herdr 2>/dev/null || true)"
    if [ -n "${RESOLVED}" ] && [ "${RESOLVED}" != "${INSTALL_DIR}/herdr" ]; then
        warn "another herdr is earlier in PATH: ${RESOLVED} (probably the upstream installer's copy). Remove it or let ~/opt/bin win."
    fi
}

# ------------------------------------------------------------------------------
# Mode: agent integrations
# ------------------------------------------------------------------------------
# herdr integration id -> the agent CLI whose presence means "install it here".
integration_cli() {
    case "$1" in
        claude)          echo "claude" ;;
        antigravity-cli) echo "agy" ;;
        codex)           echo "codex" ;;
        copilot)         echo "copilot" ;;
        opencode)        echo "opencode" ;;
        cursor)          echo "cursor-agent" ;;
        *)               echo "$1" ;;
    esac
}

install_integrations() {
    HERDR_BIN="${INSTALL_DIR}/herdr"
    [ -x "${HERDR_BIN}" ] || HERDR_BIN="$(command -v herdr 2>/dev/null || true)"
    if [ -z "${HERDR_BIN}" ]; then
        warn "herdr is not installed; skipping integrations (is install.tools.herdr enabled?)"
        return 0
    fi
    status=0
    for id in ${HERDR_INTEGRATIONS}; do
        cli="$(integration_cli "${id}")"
        if ! command -v "${cli}" >/dev/null 2>&1; then
            echo "  herdr integration ${id}: ${cli} not on this host; skipping"
            continue
        fi
        # Idempotent upstream: reports "current" when nothing changed.
        if "${HERDR_BIN}" integration install "${id}"; then
            ok "  herdr integration ${id}: installed"
        else
            warn "integration ${id} failed"
            status=1
        fi
    done
    return "${status}"
}

# ------------------------------------------------------------------------------
# Mode: managed config.toml
# ------------------------------------------------------------------------------
render_config() {
    sed -e "s|@HERDR_THEME_DARK@|${HERDR_THEME_DARK}|g" \
        -e "s|@HERDR_THEME_LIGHT@|${HERDR_THEME_LIGHT}|g" "${CONFIG_TEMPLATE}"
}

# Best-effort: only when a server is up (its API socket exists). Never fails
# the install; the next `herdr` launch reads the file anyway.
reload_running_server() {
    HERDR_BIN="${INSTALL_DIR}/herdr"
    [ -x "${HERDR_BIN}" ] || HERDR_BIN="$(command -v herdr 2>/dev/null || true)"
    [ -n "${HERDR_BIN}" ] || return 0
    [ -e "${CONFIG_DIR}/herdr.sock" ] || return 0
    if "${HERDR_BIN}" server reload-config 2>&1 | sed 's/^/  /'; then
        ok "  herdr: running server reloaded config"
    else
        warn "running server did not reload config (it will apply on next launch)"
    fi
}

install_config() {
    [ -r "${CONFIG_TEMPLATE}" ] || die "config template not readable: ${CONFIG_TEMPLATE}"
    for t in "${HERDR_THEME_DARK}" "${HERDR_THEME_LIGHT}"; do
        case "${t}" in
            *[!a-z0-9-]*|"") die "theme names must match [a-z0-9-]+ (got '${t}')" ;;
        esac
    done
    TARGET="${CONFIG_DIR}/config.toml"
    WANT_CONTENT="$(render_config)"

    if [ -e "${TARGET}" ]; then
        if ! grep -q "${MANAGED_MARKER}" "${TARGET}"; then
            if [ "${HERDR_CONFIG_FORCE:-0}" != "1" ]; then
                warn "${TARGET} is hand-edited (no '${MANAGED_MARKER}' marker); leaving it alone. Set HERDR_CONFIG_FORCE=1 to replace it (a .bak is kept)."
                return 0
            fi
            cp -p "${TARGET}" "${TARGET}.bak"
            info "Replacing hand-edited ${TARGET} (backup: ${TARGET}.bak)..."
        elif [ "$(cat "${TARGET}")" = "${WANT_CONTENT}" ]; then
            ok "herdr config up to date: ${TARGET}"
            return 0
        else
            info "Updating managed ${TARGET}..."
        fi
    else
        info "Seeding ${TARGET} (theme: ${HERDR_THEME_DARK} / ${HERDR_THEME_LIGHT}, follows host appearance)..."
    fi

    mkdir -p "${CONFIG_DIR}"
    # Write-then-rename so a reload never sees a half-written file.
    printf '%s\n' "${WANT_CONTENT}" > "${TARGET}.tmp"
    mv "${TARGET}.tmp" "${TARGET}"
    ok "  herdr config written: ${TARGET}"
    reload_running_server
}

case "${MODE}" in
    install)      install_binary ;;
    integrations) install_integrations ;;
    config)       install_config ;;
    *) die "unknown mode '${MODE}' (expected: install | integrations | config)" ;;
esac
