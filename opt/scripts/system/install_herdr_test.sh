#!/usr/bin/env bash
# Test driver for opt/scripts/system/install_herdr.sh
#
# What must hold:
#   * the download is verified against the SHA-256 herdr publishes in its
#     manifest, and a version+target with no published checksum is REFUSED
#     (fail-closed) rather than installed unverified;
#   * "already on the wanted version" is a cheap no-op (fleet update re-runs
#     install.sh on every host, so this path is the common one);
#   * the integrations mode only touches agent CLIs that exist on the host;
#   * the config mode seeds a dotfiles-MANAGED ~/.config/herdr/config.toml that
#     lets herdr follow the host terminal's light/dark appearance with the
#     fleet Solarized palette; it converges a managed file, never clobbers a
#     hand-edited one (no marker) unless HERDR_CONFIG_FORCE=1, and reloads a
#     running server best-effort;
#   * the gff wiring is complete: all three flags exist in features.yaml, each key is
#     in exactly one install-phase list, and the integrations block runs AFTER
#     install_antigravity_skills.sh (which re-renders hooks.json and drops
#     herdr's entry).
#
# Network is never touched: the manifest is fed via HERDR_MANIFEST_FILE and
# the "installed" herdr is a stub that only answers --version.
#
# Run: bash opt/scripts/system/install_herdr_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/install_herdr.sh"
INSTALL_SH="${REPO_ROOT}/install.sh"
FEATURES="${REPO_ROOT}/.github/gff/features.yaml"

assert_file_exists "${SCRIPT}" "install_herdr.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
# assert_exit_code leaves errexit ON when it returns; the functional cases below
# capture non-zero exits on purpose, so switch it back off.
set +e

# --- source-level: the trust model -------------------------------------------
assert_grep "reads the manifest herdr publishes" 'herdr\.dev/latest\.json' "${SCRIPT}"
assert_grep "verifies SHA-256 of the download" 'checksum mismatch' "${SCRIPT}"
assert_grep "fails closed when the manifest has no checksum" \
    'refusing to install an unverified binary' "${SCRIPT}"
assert_grep_negative "never pipes a remote script into a shell" \
    'curl[^|]*\|[[:space:]]*(ba)?sh' "${SCRIPT}"
assert_grep "covers the 64-bit Pi / Nano / Spark (aarch64)" 'arm64\|aarch64' "${SCRIPT}"
assert_grep "skips (not fails) on 32-bit ARM, which has no upstream build" \
    'armv7l\|armv6l' "${SCRIPT}"
assert_grep "installs to ~/opt/bin like the other fetched tools" \
    'HERDR_INSTALL_DIR:-\$\{HOME\}/opt/bin' "${SCRIPT}"
assert_grep "supports a version pin" 'HERDR_VERSION:-latest' "${SCRIPT}"

# --- functional: a fixture manifest, no network -----------------------------
TMP="$(mktemp -d "${TMPDIR:-/tmp}/install_herdr_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
FIXTURE="${TMP}/latest.json"
cat > "${FIXTURE}" <<'JSON'
{
  "version": "0.8.2",
  "assets": { "linux-x86_64": "https://example.invalid/herdr", "linux-aarch64": "https://example.invalid/herdr",
              "macos-x86_64": "https://example.invalid/herdr", "macos-aarch64": "https://example.invalid/herdr" },
  "sha256": { "linux-x86_64": "976150a14d490c94b243ea2e1a7eb2dfb67f12e36b182db90936f6728e6aecf4",
              "linux-aarch64": "f55610658e1c2e0d2aaef730b4b2ab885f7f8ba00285ab372bfb14f2e3d5b40d",
              "macos-x86_64": "ab50262c8190cd7aa9056d249d255c08c328c3e8716de9cfa29db4f131b8e2c1",
              "macos-aarch64": "a5d4f4d504d8b309c91f811050559300faba31258425f53c50852fc96f6ae574" },
  "releases": {
    "0.8.2": { "assets": {}, "sha256": {} },
    "0.7.5": { "assets": { "linux-x86_64": "https://example.invalid/old" }, "sha256": {} }
  }
}
JSON

# A stub herdr that only knows --version, standing in for an installed binary.
STUB_DIR="${TMP}/bin"
mkdir -p "${STUB_DIR}"
cat > "${STUB_DIR}/herdr" <<'SH'
#!/bin/sh
case "$1" in
  --version) echo "herdr 0.8.2" ;;
  integration) echo "stub: herdr $*"; exit 0 ;;
  server) echo "stub: herdr $*"; exit 0 ;;
  *) exit 1 ;;
esac
SH
chmod +x "${STUB_DIR}/herdr"

# 1. Already on the manifest version -> no-op, exit 0, no download attempted.
out="$(HERDR_MANIFEST_FILE="${FIXTURE}" HERDR_INSTALL_DIR="${STUB_DIR}" bash "${SCRIPT}" 2>&1)"; rc=$?
assert_eq "${rc}" "0" "already-installed path exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'already installed')" "1" "already-installed path says so"

# 2. Pinned to a release the manifest does not list -> refuse (fail closed).
EMPTY_DIR="${TMP}/empty"; mkdir -p "${EMPTY_DIR}"
out="$(HERDR_MANIFEST_FILE="${FIXTURE}" HERDR_INSTALL_DIR="${EMPTY_DIR}" HERDR_VERSION=9.9.9 bash "${SCRIPT}" 2>&1)"; rc=$?
assert_eq "${rc}" "1" "unknown pinned release exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'not in the manifest')" "1" "unknown pinned release is reported"

# 3. Pinned to a listed release whose checksum is missing for this target ->
#    refuse rather than download an unverifiable binary.
out="$(HERDR_MANIFEST_FILE="${FIXTURE}" HERDR_INSTALL_DIR="${EMPTY_DIR}" HERDR_VERSION=0.7.5 bash "${SCRIPT}" 2>&1)"; rc=$?
assert_eq "${rc}" "1" "missing checksum exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'refusing to install an unverified binary')" "1" \
    "missing checksum is the stated reason"

# 4. integrations mode: an id whose agent CLI is absent is skipped, not failed;
#    one whose CLI is present is handed to `herdr integration install`.
out="$(HERDR_INSTALL_DIR="${STUB_DIR}" HERDR_INTEGRATIONS="no-such-agent-zz" bash "${SCRIPT}" integrations 2>&1)"; rc=$?
assert_eq "${rc}" "0" "integrations: absent agent CLI -> exit 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'not on this host; skipping')" "1" "integrations: absent agent CLI is skipped"

# `sh` is always present, so an id mapped to it exercises the install call.
out="$(HERDR_INSTALL_DIR="${STUB_DIR}" HERDR_INTEGRATIONS="sh" bash "${SCRIPT}" integrations 2>&1)"; rc=$?
assert_eq "${rc}" "0" "integrations: present agent CLI -> exit 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'stub: herdr integration install sh')" "1" \
    "integrations: present agent CLI runs 'herdr integration install <id>'"

# 5. integrations mode with no herdr at all is a warning, not a failure
#    (install.tools.herdr may be off while the integrations flag is on).
out="$(HERDR_INSTALL_DIR="${EMPTY_DIR}" PATH="${EMPTY_DIR}:/usr/bin:/bin" bash "${SCRIPT}" integrations 2>&1)"; rc=$?
assert_eq "${rc}" "0" "integrations: no herdr -> exit 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'herdr is not installed; skipping')" "1" "integrations: no herdr is reported"

# --- config mode: managed config.toml ----------------------------------------
TEMPLATE="${REPO_ROOT}/ai/herdr/config.toml"
assert_file_exists "${TEMPLATE}" "config template is tracked at ai/herdr/config.toml"
assert_grep "template turns on host light/dark following" '^auto_switch = true' "${TEMPLATE}"
assert_grep "template carries the managed marker" 'managed by dotfiles' "${TEMPLATE}"
assert_grep "template renders the dark theme from a token" '@HERDR_THEME_DARK@' "${TEMPLATE}"
assert_grep "template renders the light theme from a token" '@HERDR_THEME_LIGHT@' "${TEMPLATE}"
assert_grep "template ships no hardcoded home path" '^# managed by dotfiles' "${TEMPLATE}"
assert_grep "template sets the prefix to ctrl+a (tmux muscle memory, no ctrl+b clash)" '^prefix = "ctrl\+a"$' "${TEMPLATE}"

# 6. Fresh host: no config.toml -> seeded with the Solarized pair, dark fallback.
CFG_DIR="${TMP}/cfg-fresh"
out="$(HERDR_INSTALL_DIR="${EMPTY_DIR}" HERDR_CONFIG_DIR="${CFG_DIR}" bash "${SCRIPT}" config 2>&1)"; rc=$?
assert_eq "${rc}" "0" "config: fresh host exits 0"
assert_file_exists "${CFG_DIR}/config.toml" "config: fresh host gets config.toml"
assert_grep "config: dark fallback theme is solarized" '^name = "solarized"$' "${CFG_DIR}/config.toml"
assert_grep "config: auto_switch is on" '^auto_switch = true$' "${CFG_DIR}/config.toml"
assert_grep "config: light sibling is solarized-light" '^light_name = "solarized-light"$' "${CFG_DIR}/config.toml"
assert_grep "config: dark sibling is solarized" '^dark_name = "solarized"$' "${CFG_DIR}/config.toml"
assert_grep "config: written file carries the managed marker" '^# managed by dotfiles' "${CFG_DIR}/config.toml"
assert_grep_negative "config: no unrendered tokens remain" '@HERDR_THEME_' "${CFG_DIR}/config.toml"
assert_grep "config: rendered file keeps the ctrl+a prefix" '^prefix = "ctrl\+a"$' "${CFG_DIR}/config.toml"
assert_grep_negative "config: no herdr -> no reload attempted" 'reload' <(printf '%s\n' "${out}")

# 7. Re-run on a managed file is a converge no-op.
before="$(cat "${CFG_DIR}/config.toml")"
out="$(HERDR_INSTALL_DIR="${EMPTY_DIR}" HERDR_CONFIG_DIR="${CFG_DIR}" bash "${SCRIPT}" config 2>&1)"; rc=$?
assert_eq "${rc}" "0" "config: re-run exits 0"
assert_eq "$(cat "${CFG_DIR}/config.toml")" "${before}" "config: re-run leaves a managed file byte-identical"
assert_eq "$(printf '%s' "${out}" | grep -c 'up to date')" "1" "config: re-run reports up to date"

# 8. Theme overrides render into the managed file.
CFG_DIR2="${TMP}/cfg-override"
out="$(HERDR_INSTALL_DIR="${EMPTY_DIR}" HERDR_CONFIG_DIR="${CFG_DIR2}" HERDR_THEME_DARK=nord HERDR_THEME_LIGHT=one-light bash "${SCRIPT}" config 2>&1)"; rc=$?
assert_eq "${rc}" "0" "config: theme override exits 0"
assert_grep "config: HERDR_THEME_DARK renders name" '^name = "nord"$' "${CFG_DIR2}/config.toml"
assert_grep "config: HERDR_THEME_DARK renders dark_name" '^dark_name = "nord"$' "${CFG_DIR2}/config.toml"
assert_grep "config: HERDR_THEME_LIGHT renders light_name" '^light_name = "one-light"$' "${CFG_DIR2}/config.toml"

# 9. A hand-edited config (no managed marker) is left alone, with a warning.
CFG_DIR3="${TMP}/cfg-hand"
mkdir -p "${CFG_DIR3}"
printf '[theme]\nname = "dracula"\n' > "${CFG_DIR3}/config.toml"
out="$(HERDR_INSTALL_DIR="${EMPTY_DIR}" HERDR_CONFIG_DIR="${CFG_DIR3}" bash "${SCRIPT}" config 2>&1)"; rc=$?
assert_eq "${rc}" "0" "config: hand-edited file -> exit 0"
assert_grep "config: hand-edited file is untouched" '^name = "dracula"$' "${CFG_DIR3}/config.toml"
assert_eq "$(printf '%s' "${out}" | grep -c 'leaving it alone')" "1" "config: hand-edited file is reported, not clobbered"
assert_eq "$(printf '%s' "${out}" | grep -c 'HERDR_CONFIG_FORCE=1')" "1" "config: warning names the override"

# 10. HERDR_CONFIG_FORCE=1 replaces a hand-edited file but keeps a backup.
out="$(HERDR_INSTALL_DIR="${EMPTY_DIR}" HERDR_CONFIG_DIR="${CFG_DIR3}" HERDR_CONFIG_FORCE=1 bash "${SCRIPT}" config 2>&1)"; rc=$?
assert_eq "${rc}" "0" "config: force exits 0"
assert_grep "config: force writes the managed file" '^# managed by dotfiles' "${CFG_DIR3}/config.toml"
assert_grep "config: force keeps a backup of the hand-edited file" '^name = "dracula"$' "${CFG_DIR3}/config.toml.bak"

# 11. A running server (socket present) is reloaded best-effort after a write.
CFG_DIR4="${TMP}/cfg-live"
mkdir -p "${CFG_DIR4}"; : > "${CFG_DIR4}/herdr.sock"
out="$(HERDR_INSTALL_DIR="${STUB_DIR}" HERDR_CONFIG_DIR="${CFG_DIR4}" bash "${SCRIPT}" config 2>&1)"; rc=$?
assert_eq "${rc}" "0" "config: live server path exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'stub: herdr server reload-config')" "1" \
    "config: running server gets 'herdr server reload-config'"

# --- gff wiring -------------------------------------------------------------
assert_grep "features.yaml declares install.tools.herdr" 'path: install\.tools\.herdr$' "${FEATURES}"
assert_grep "features.yaml declares install.tools.herdr-integrations" \
    'path: install\.tools\.herdr-integrations$' "${FEATURES}"
assert_grep "install.sh gates the binary on install.tools.herdr" 'gff_on install\.tools\.herdr;' "${INSTALL_SH}"
assert_grep "install.sh gates integrations on install.tools.herdr-integrations" \
    'gff_on install\.tools\.herdr-integrations;' "${INSTALL_SH}"
assert_grep "features.yaml declares install.tools.herdr-config" \
    'path: install\.tools\.herdr-config$' "${FEATURES}"
assert_grep "install.sh gates the managed config on install.tools.herdr-config" \
    'gff_on install\.tools\.herdr-config;' "${INSTALL_SH}"
assert_grep "install.sh runs install_herdr.sh in config mode" 'install_herdr\.sh" config' "${INSTALL_SH}"

# Phase lists: a downloaded CLI is a deps step; writing into ~/.claude and
# ~/.gemini is a config step. A key in NEITHER list runs in BOTH phases (the
# docker/AGENTS.md #217 bug), and a key in both is contradictory.
deps_line="$(grep -E '^_IP_DEPS_FLAGS=' "${INSTALL_SH}")"
config_line="$(grep -E '^_IP_CONFIG_FLAGS=' "${INSTALL_SH}")"
assert_eq "$(printf '%s' "${deps_line}" | grep -c -w 'INSTALL_TOOLS_HERDR')" "1" \
    "INSTALL_TOOLS_HERDR is in _IP_DEPS_FLAGS"
assert_eq "$(printf '%s' "${config_line}" | grep -c -w 'INSTALL_TOOLS_HERDR')" "0" \
    "INSTALL_TOOLS_HERDR is NOT in _IP_CONFIG_FLAGS"
assert_eq "$(printf '%s' "${config_line}" | grep -c -w 'INSTALL_TOOLS_HERDR_INTEGRATIONS')" "1" \
    "INSTALL_TOOLS_HERDR_INTEGRATIONS is in _IP_CONFIG_FLAGS"
assert_eq "$(printf '%s' "${deps_line}" | grep -c -w 'INSTALL_TOOLS_HERDR_INTEGRATIONS')" "0" \
    "INSTALL_TOOLS_HERDR_INTEGRATIONS is NOT in _IP_DEPS_FLAGS"
assert_eq "$(printf '%s' "${config_line}" | grep -c -w 'INSTALL_TOOLS_HERDR_CONFIG')" "1" \
    "INSTALL_TOOLS_HERDR_CONFIG is in _IP_CONFIG_FLAGS"
assert_eq "$(printf '%s' "${deps_line}" | grep -c -w 'INSTALL_TOOLS_HERDR_CONFIG')" "0" \
    "INSTALL_TOOLS_HERDR_CONFIG is NOT in _IP_DEPS_FLAGS"

# Ordering: the integrations block must come after install_antigravity_skills.sh
# is invoked, or the hooks.json re-render undoes it on every run.
agy_line="$(grep -n 'opt/scripts/system/install_antigravity_skills.sh"$' "${INSTALL_SH}" | head -1 | cut -d: -f1)"
integ_line="$(grep -n 'install_herdr.sh" integrations' "${INSTALL_SH}" | head -1 | cut -d: -f1)"
if [ -n "${agy_line}" ] && [ -n "${integ_line}" ] && [ "${integ_line}" -gt "${agy_line}" ]; then
    assert_eq "ordered" "ordered" "herdr integrations run after install_antigravity_skills.sh (line ${integ_line} > ${agy_line})"
else
    assert_eq "integrations@${integ_line:-missing} agy@${agy_line:-missing}" "ordered" \
        "herdr integrations run after install_antigravity_skills.sh"
fi

_test_report
