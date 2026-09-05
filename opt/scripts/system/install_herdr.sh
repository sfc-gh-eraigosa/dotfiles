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
#   HERDR_MANIFEST_URL  manifest location (default: https://herdr.dev/latest.json)
#   HERDR_MANIFEST_FILE use a local manifest file instead of fetching (tests)
set -e

MODE="${1:-install}"
HERDR_VERSION="${HERDR_VERSION:-latest}"
INSTALL_DIR="${HERDR_INSTALL_DIR:-${HOME}/opt/bin}"
HERDR_INTEGRATIONS="${HERDR_INTEGRATIONS:-claude antigravity-cli}"
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

case "${MODE}" in
    install)      install_binary ;;
    integrations) install_integrations ;;
    *) die "unknown mode '${MODE}' (expected: install | integrations)" ;;
esac
