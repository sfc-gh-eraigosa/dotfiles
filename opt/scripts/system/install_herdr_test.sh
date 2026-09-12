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
#   * the plugins mode installs each row of ai/herdr/plugins.tsv whose own
#     gff flag (install.herdr-plugin.<name>) is on, at its pinned ref; a plugin
#     already at that ref is a no-op, an undeclared flag means never, and a
#     malformed row never reaches the herdr CLI;
#   * the config mode appends each enabled plugin's keybinding fragment, and
#     only those;
#   * the gff wiring is complete: every flag exists in features.yaml, each
#     install.sh key is in exactly one install-phase list, the integrations
#     block runs AFTER install_antigravity_skills.sh (which re-renders
#     hooks.json and drops herdr's entry), and the plugins step runs after
#     both the herdr binary and install_rust.sh.
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

# --- plugins mode: manifest rows, each behind its own gff flag ----------------
PLUG_FIX="${TMP}/plugins"
mkdir -p "${PLUG_FIX}/bin" "${PLUG_FIX}/keys"
cat > "${PLUG_FIX}/features.yaml" <<'YAML'
sets:
  - area: install
    features:
      - path: install.herdr-plugin.file-viewer
        description: fixture
        boolDefault: true
      - path: install.herdr-plugin.navigator
        description: fixture
        boolDefault: false
      - path: install.herdr-plugin.flaky
        description: fixture
        boolDefault: true
      - path: install.herdr-plugin.reviewr
        description: fixture
        boolDefault: true
      - path: install.herdr-plugin.ohmyzsh
        description: fixture
        boolDefault: true
      - path: install.herdr-plugin.maconly
        description: fixture
        boolDefault: true
      - path: install.herdr-plugin.subdir
        description: fixture
        boolDefault: true
YAML
cat > "${PLUG_FIX}/plugins.tsv" <<'TSV'
# name       plugin_id          repo                        ref
file-viewer  herdr-file-viewer  smarzban/herdr-file-viewer  v1.16.0

navigator    herdr-navigator    thanhdat77/herdr-navigator  v0.3.6
undeclared   some.plugin        owner/undeclared            v1.0.0
TSV
printf '# fv keys\n[[keys.command]]\nkey = "prefix+f"\ntype = "plugin_action"\ncommand = "herdr-file-viewer.open-file-viewer"\n' \
    > "${PLUG_FIX}/keys/file-viewer.toml"
printf '[[keys.command]]\nkey = "prefix+n"\ntype = "plugin_action"\ncommand = "herdr-navigator.open"\n' \
    > "${PLUG_FIX}/keys/navigator.toml"
printf '[[keys.command]]\nkey = "prefix+u"\ntype = "plugin_action"\ncommand = "some.plugin.x"\n' \
    > "${PLUG_FIX}/keys/undeclared.toml"
# A gff stub: answers `get <key>` from ${PLUG_FIX}/gff-overrides ("key value"
# per line, what a host's `gff set` would resolve to), exit 2 otherwise —
# the real gff's answer for an unknown key.
cat > "${PLUG_FIX}/bin/gff" <<'SH'
#!/bin/sh
while [ "$#" -gt 0 ]; do case "$1" in --source) shift 2 ;; get) shift; break ;; *) shift ;; esac; done
v="$(awk -v k="$1" '$1 == k { print $2 }' "${GFF_OVERRIDES}" 2>/dev/null)"
[ -n "$v" ] && { echo "$v"; exit 0; }
echo "gff: resolve: unknown flag key: $1" >&2; exit 2
SH
chmod +x "${PLUG_FIX}/bin/gff"
export GFF_OVERRIDES="${PLUG_FIX}/gff-overrides"
: > "${GFF_OVERRIDES}"
# A herdr stub that logs every call; `plugin install fail/...` fails.
STUB_LOG="${PLUG_FIX}/calls.log"
cat > "${PLUG_FIX}/bin/herdr" <<'SH'
#!/bin/sh
echo "$*" >> "${STUB_LOG}"
case "$1" in
  --version) echo "herdr 0.8.2" ;;
  plugin) case "$3" in fail/*) exit 1 ;; esac ;;
esac
exit 0
SH
chmod +x "${PLUG_FIX}/bin/herdr"
export STUB_LOG

# run_plugins <config dir> [VAR=value ...]: plugins mode against the fixtures,
# with no GFF_* leaking in from the caller's environment.
run_plugins() {
    _cfg="$1"; shift
    : > "${STUB_LOG}"
    env -u GFF_INSTALL_HERDR_PLUGIN_FILE_VIEWER -u GFF_INSTALL_HERDR_PLUGIN_NAVIGATOR \
        -u GFF_INSTALL_HERDR_PLUGIN_UNDECLARED -u GFF_INSTALL_HERDR_PLUGIN_FLAKY \
        HERDR_INSTALL_DIR="${PLUG_FIX}/bin" HERDR_CONFIG_DIR="${_cfg}" \
        HERDR_PLUGINS_MANIFEST="${HERDR_PLUGINS_MANIFEST:-${PLUG_FIX}/plugins.tsv}" HERDR_GFF="${PLUG_FIX}/bin/gff" \
        HERDR_FEATURES_FILE="${PLUG_FIX}/features.yaml" "$@" bash "${SCRIPT}" plugins 2>&1
}

# 12. Standalone (no exported GFF_*): the declared defaults decide. file-viewer
#     (default on) installs at its pinned ref; navigator (default off) and the
#     row with no declared flag do not.
out="$(run_plugins "${TMP}/p-fresh")"; rc=$?
assert_eq "${rc}" "0" "plugins: fresh host exits 0"
assert_eq "$(grep -c '^plugin install smarzban/herdr-file-viewer --ref v1.16.0 --yes$' "${STUB_LOG}")" "1" \
    "plugins: default-on plugin is installed at its pinned ref"
assert_eq "$(grep -c 'thanhdat77/herdr-navigator' "${STUB_LOG}")" "0" "plugins: default-off plugin is not installed"
assert_eq "$(grep -c 'owner/undeclared' "${STUB_LOG}")" "0" "plugins: a row without a declared flag never installs"
assert_eq "$(printf '%s' "${out}" | grep -c 'off (gff install.herdr-plugin.navigator)')" "1" \
    "plugins: an off plugin names its flag"

# 13. Exported GFF_* values (what install.sh passes) win over the defaults, and
#     every enabled row is processed (the install call must not eat the
#     manifest loop's stdin).
out="$(run_plugins "${TMP}/p-env" GFF_INSTALL_HERDR_PLUGIN_NAVIGATOR=true)"; rc=$?
assert_eq "${rc}" "0" "plugins: gff override exits 0"
assert_eq "$(grep -c '^plugin install ' "${STUB_LOG}")" "2" "plugins: both enabled rows are installed"
assert_eq "$(grep -c '^plugin install thanhdat77/herdr-navigator --ref v0.3.6 --yes$' "${STUB_LOG}")" "1" \
    "plugins: gff true turns a default-off plugin on"
out="$(run_plugins "${TMP}/p-env" GFF_INSTALL_HERDR_PLUGIN_FILE_VIEWER=false)"; rc=$?
assert_eq "$(grep -c '^plugin install ' "${STUB_LOG}")" "0" "plugins: gff false turns a default-on plugin off"

# 13b. Standalone (no exported GFF_*): a host's `gff set` override, as gff
#      itself resolves it, turns a default-off plugin on — and the exported
#      value, when present, still wins over it. Without a gff binary the
#      declared default is the last resort.
printf 'install.herdr-plugin.navigator true\n' > "${GFF_OVERRIDES}"
out="$(run_plugins "${TMP}/p-gffset")"; rc=$?
assert_eq "${rc}" "0" "plugins: standalone with a gff override exits 0"
assert_eq "$(grep -c '^plugin install thanhdat77/herdr-navigator --ref v0.3.6 --yes$' "${STUB_LOG}")" "1" \
    "plugins: standalone run honours the host's gff set override"
out="$(run_plugins "${TMP}/p-gffset" GFF_INSTALL_HERDR_PLUGIN_NAVIGATOR=false)"
assert_eq "$(grep -c 'thanhdat77/herdr-navigator' "${STUB_LOG}")" "0" "plugins: an exported GFF_* value wins over gff's answer"
out="$(run_plugins "${TMP}/p-gffset" HERDR_GFF=/nonexistent/gff)"
assert_eq "$(grep -c 'thanhdat77/herdr-navigator' "${STUB_LOG}")" "0" "plugins: no gff binary -> declared default (off)"
: > "${GFF_OVERRIDES}"

# 14. Already installed at the pinned ref: a no-op with no herdr call (fleet
#     update re-runs install.sh everywhere; a reinstall rebuilds with cargo).
P_CFG="${TMP}/p-have"; mkdir -p "${P_CFG}"
printf '[{"plugin_id":"herdr-file-viewer","enabled":true,"source":{"requested_ref":"v1.16.0"}}]\n' > "${P_CFG}/plugins.json"
out="$(run_plugins "${P_CFG}")"; rc=$?
assert_eq "${rc}" "0" "plugins: already-installed exits 0"
assert_eq "$(grep -c '^plugin install ' "${STUB_LOG}")" "0" "plugins: already at the pinned ref -> no reinstall"
assert_eq "$(printf '%s' "${out}" | grep -c 'already installed at v1.16.0')" "1" "plugins: already-installed is reported"

# 15. HERDR_PLUGIN_FORCE=1 reinstalls anyway.
out="$(run_plugins "${P_CFG}" HERDR_PLUGIN_FORCE=1)"; rc=$?
assert_eq "$(grep -c '^plugin install smarzban/herdr-file-viewer ' "${STUB_LOG}")" "1" "plugins: HERDR_PLUGIN_FORCE=1 reinstalls"

# 16. Installed at an older ref: moves to the pinned one and says so.
printf '[{"plugin_id":"herdr-file-viewer","enabled":true,"source":{"requested_ref":"v1.15.0"}}]\n' > "${P_CFG}/plugins.json"
out="$(run_plugins "${P_CFG}")"; rc=$?
assert_eq "$(grep -c '^plugin install smarzban/herdr-file-viewer --ref v1.16.0 ' "${STUB_LOG}")" "1" \
    "plugins: an older ref is upgraded to the pinned one"
assert_eq "$(printf '%s' "${out}" | grep -c 'v1.15.0 -> v1.16.0')" "1" "plugins: the upgrade is reported"

# 17. Malformed rows never reach the herdr CLI (option injection), and a
#     failing install is reported without stopping the other rows.
cat > "${PLUG_FIX}/bad.tsv" <<'TSV'
file-viewer  herdr-file-viewer  --upload-pack=evil/x        v1
file-viewer  herdr-file-viewer  smarzban/herdr-file-viewer  --ref=main
file-viewer  herdr-file-viewer  a//b                        v1
file-viewer  herdr-file-viewer  owner/../escape             v1
file-viewer  herdr-file-viewer  smarzban/herdr-file-viewer  v1  linux;rm
file-viewer  herdr-file-viewer  smarzban/herdr-file-viewer
flaky        flaky.plugin       fail/flaky                  v1
file-viewer  herdr-file-viewer  smarzban/herdr-file-viewer  v1.16.0
TSV
out="$(HERDR_PLUGINS_MANIFEST="${PLUG_FIX}/bad.tsv" run_plugins "${TMP}/p-bad")"; rc=$?
assert_eq "${rc}" "1" "plugins: malformed rows or a failed install exit 1"
assert_eq "$(grep -c -- '--upload-pack\|--ref=main\|a//b\|\.\./\|rm' "${STUB_LOG}")" "0" "plugins: malformed rows never reach herdr"
assert_eq "$(printf '%s' "${out}" | grep -c 'flaky (fail/flaky@v1) failed to install')" "1" "plugins: a failed install is reported"
assert_eq "$(grep -c '^plugin install smarzban/herdr-file-viewer --ref v1.16.0 ' "${STUB_LOG}")" "1" \
    "plugins: rows after a bad one still install"

# 18a. herdr builds plugins in a temporary checkout, so links a build step made
#      to $HERDR_PLUGIN_ROOT dangle once herdr moves it into place. The known
#      cases are re-pointed at the final root on every pass.
FX="${TMP}/fixup"; FX_HOME="${FX}/home"
mkdir -p "${FX}/root-reviewr/bin" "${FX}/root-omz/bin" "${FX_HOME}/.local/bin" "${FX}/cfg" "${FX}/elsewhere"
: > "${FX}/root-reviewr/bin/herdr-reviewr"; : > "${FX}/elsewhere/herdr-reviewr"
ln -s "${FX}/gone/checkout/bin/herdr-reviewr" "${FX_HOME}/.local/bin/herdr-reviewr"       # dangling
mkdir -p "${FX_HOME}/.local/state/herdr/plugins/persiyanov.reviewr/bin"
ln -s "${FX}/elsewhere/herdr-reviewr" "${FX_HOME}/.local/state/herdr/plugins/persiyanov.reviewr/bin/herdr-reviewr"  # live
# shellcheck disable=SC2016 # the stub expands HERDR_PLUGIN_ROOT when it runs
printf 'echo "linked from ${HERDR_PLUGIN_ROOT}" > "%s/omz-linked"\n' "${FX}" > "${FX}/root-omz/bin/install-zsh-plugin"
printf '[{"plugin_id":"persiyanov.reviewr","plugin_root":"%s","source":{"requested_ref":"v0.36.2"}},{"plugin_id":"ohmyzsh.shell","plugin_root":"%s","source":{"requested_ref":"abc1234"}}]\n' \
    "${FX}/root-reviewr" "${FX}/root-omz" > "${FX}/cfg/plugins.json"
printf 'reviewr persiyanov.reviewr persiyanov/herdr-reviewr v0.36.2\nohmyzsh ohmyzsh.shell robbyrussell/herdr-ohmyzsh abc1234\n' > "${FX}/plugins.tsv"
out="$(HERDR_PLUGINS_MANIFEST="${FX}/plugins.tsv" run_plugins "${FX}/cfg" HOME="${FX_HOME}" ZSH="${FX}/omz")"; rc=$?
assert_eq "${rc}" "0" "fixup: already-installed plugins with link fix-ups exit 0"
assert_eq "$(readlink "${FX_HOME}/.local/bin/herdr-reviewr")" "${FX}/root-reviewr/bin/herdr-reviewr" \
    "fixup: a dangling reviewr link is re-pointed at the final plugin root"
assert_eq "$(readlink "${FX_HOME}/.local/state/herdr/plugins/persiyanov.reviewr/bin/herdr-reviewr")" "${FX}/elsewhere/herdr-reviewr" \
    "fixup: a live link someone chose is left alone"
if command -v zsh >/dev/null 2>&1; then
    assert_eq "$(cat "${FX}/omz-linked" 2>/dev/null)" "linked from ${FX}/root-omz" \
        "fixup: herdr-ohmyzsh re-runs its own link step from the final root"
fi

# 18. No herdr at all: a warning, not a failure.
out="$(HERDR_INSTALL_DIR="${EMPTY_DIR}" HERDR_CONFIG_DIR="${TMP}/p-none" PATH="${EMPTY_DIR}:/usr/bin:/bin" \
    HERDR_PLUGINS_MANIFEST="${PLUG_FIX}/plugins.tsv" HERDR_FEATURES_FILE="${PLUG_FIX}/features.yaml" \
    bash "${SCRIPT}" plugins 2>&1)"; rc=$?
assert_eq "${rc}" "0" "plugins: no herdr -> exit 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'herdr is not installed; skipping plugins')" "1" "plugins: no herdr is reported"

# --- config mode: each enabled plugin's keybindings ---------------------------
# run_config <config dir> [VAR=value ...]
run_config() {
    _cfg="$1"; shift
    env -u GFF_INSTALL_HERDR_PLUGIN_FILE_VIEWER -u GFF_INSTALL_HERDR_PLUGIN_NAVIGATOR \
        HERDR_INSTALL_DIR="${EMPTY_DIR}" HERDR_CONFIG_DIR="${_cfg}" \
        HERDR_PLUGINS_MANIFEST="${HERDR_PLUGINS_MANIFEST:-${PLUG_FIX}/plugins.tsv}" HERDR_PLUGIN_KEYS_DIR="${PLUG_FIX}/keys" \
        HERDR_GFF="${PLUG_FIX}/bin/gff" HERDR_FEATURES_FILE="${PLUG_FIX}/features.yaml" "$@" bash "${SCRIPT}" config 2>&1
}

# 19. Enabled plugins' fragments are appended; off and undeclared ones are not.
K_CFG="${TMP}/k-cfg"
out="$(run_config "${K_CFG}")"; rc=$?
assert_eq "${rc}" "0" "config: plugin keys render exits 0"
assert_grep "config: an enabled plugin's keybindings are appended" \
    '^command = "herdr-file-viewer\.open-file-viewer"$' "${K_CFG}/config.toml"
assert_grep_negative "config: an off plugin gets no keybindings" 'herdr-navigator\.open' "${K_CFG}/config.toml"
assert_grep_negative "config: a row without a declared flag gets no keybindings" 'some\.plugin' "${K_CFG}/config.toml"
assert_grep "config: the managed marker still leads the file" '^# managed by dotfiles' "${K_CFG}/config.toml"

# 19b. Standalone config: keys follow the host's gff set override too.
printf 'install.herdr-plugin.navigator true\n' > "${GFF_OVERRIDES}"
out="$(run_config "${K_CFG}")"
assert_grep "config: standalone run renders keys for a gff-set plugin" '^command = "herdr-navigator\.open"$' "${K_CFG}/config.toml"
: > "${GFF_OVERRIDES}"
out="$(run_config "${K_CFG}")"
assert_grep_negative "config: clearing the override drops the keys again" 'herdr-navigator\.open' "${K_CFG}/config.toml"

# 20. Flipping a plugin's flag re-renders the managed file both ways.
out="$(run_config "${K_CFG}" GFF_INSTALL_HERDR_PLUGIN_FILE_VIEWER=false GFF_INSTALL_HERDR_PLUGIN_NAVIGATOR=true)"; rc=$?
assert_eq "$(printf '%s' "${out}" | grep -c 'Updating managed')" "1" "config: a flag flip updates the managed file"
assert_grep_negative "config: keys go away with their flag" 'herdr-file-viewer\.open-file-viewer' "${K_CFG}/config.toml"
assert_grep "config: keys arrive with their flag" '^command = "herdr-navigator\.open"$' "${K_CFG}/config.toml"

# 21. The real template + manifest + fragments render a config herdr accepts.
R_CFG="${TMP}/r-cfg/herdr"
out="$(env -u GFF_INSTALL_HERDR_PLUGIN_FILE_VIEWER HERDR_INSTALL_DIR="${EMPTY_DIR}" HERDR_GFF=/nonexistent/gff \
    HERDR_CONFIG_DIR="${R_CFG}" bash "${SCRIPT}" config 2>&1)"; rc=$?
assert_eq "${rc}" "0" "config: the repo's own plugin manifest renders"
assert_grep "config: the repo's file-viewer keys render by default" \
    '^command = "herdr-file-viewer\.open-file-viewer-tab"$' "${R_CFG}/config.toml"
if command -v herdr >/dev/null 2>&1; then
    assert_exit_code 0 "config: herdr config check accepts the rendered file" \
        env XDG_CONFIG_HOME="${TMP}/r-cfg" herdr config check
    set +e
fi

# 22. The optional os column: a row for another OS is skipped (no install,
#      no keybindings) and says why; a subdir path reaches herdr as-is.
cat > "${PLUG_FIX}/os.tsv" <<'TSV'
maconly  mac.plugin     owner/maconly            v1.0.0  macos
subdir   sub.plugin     owner/monorepo/herdr-plugin  v2.0.0
TSV
printf '[[keys.command]]\nkey = "prefix+shift+s"\ntype = "plugin_action"\ncommand = "mac.plugin.open"\n' > "${PLUG_FIX}/keys/maconly.toml"
out="$(HERDR_PLUGINS_MANIFEST="${PLUG_FIX}/os.tsv" run_plugins "${TMP}/p-os" HERDR_HOST_OS=linux)"; rc=$?
assert_eq "${rc}" "0" "os: a row for another OS is not a failure"
assert_eq "$(grep -c 'owner/maconly' "${STUB_LOG}")" "0" "os: a macos row is not installed on linux"
assert_eq "$(printf '%s' "${out}" | grep -c 'maconly: macos only; skipped on linux')" "1" "os: the skip names the OS"
assert_eq "$(grep -c '^plugin install owner/monorepo/herdr-plugin --ref v2.0.0 --yes$' "${STUB_LOG}")" "1" \
    "subdir: owner/repo/subdir is passed to herdr as-is"
out="$(HERDR_PLUGINS_MANIFEST="${PLUG_FIX}/os.tsv" run_plugins "${TMP}/p-os" HERDR_HOST_OS=macos)"; rc=$?
assert_eq "$(grep -c '^plugin install owner/maconly --ref v1.0.0 --yes$' "${STUB_LOG}")" "1" "os: a macos row installs on macos"
O_CFG="${TMP}/o-cfg"
out="$(HERDR_PLUGINS_MANIFEST="${PLUG_FIX}/os.tsv" run_config "${O_CFG}" HERDR_HOST_OS=linux)"
assert_grep_negative "os: no keybindings for a plugin this OS skips" 'mac\.plugin\.open' "${O_CFG}/config.toml"
out="$(HERDR_PLUGINS_MANIFEST="${PLUG_FIX}/os.tsv" run_config "${O_CFG}" HERDR_HOST_OS=macos)"
assert_grep "os: keybindings arrive on the OS the plugin supports" '^command = "mac\.plugin\.open"$' "${O_CFG}/config.toml"

# --- the repo's manifest, fragments and flags agree ---------------------------
MANIFEST="${REPO_ROOT}/ai/herdr/plugins.tsv"
KEYS_DIR="${REPO_ROOT}/ai/herdr/plugins"
assert_file_exists "${MANIFEST}" "plugin manifest is tracked at ai/herdr/plugins.tsv"
rows="$(awk '!/^[[:space:]]*(#|$)/ { print $1, $2, $3, $4, NF, $5 }' "${MANIFEST}")"
assert_eq "$(printf '%s\n' "${rows}" | grep -c 'herdr-file-viewer smarzban/herdr-file-viewer')" "1" \
    "manifest lists herdr-file-viewer"
while read -r name id _repo ref nf os; do
    [ -n "${name}" ] || continue
    case "${nf}" in 4|5) cols=ok ;; *) cols="${nf} columns" ;; esac
    assert_eq "${cols}" "ok" "manifest row ${name} has name, plugin_id, repo, ref and an optional os"
    case "${os}" in ""|any|linux|macos|linux,macos|macos,linux) osok=ok ;; *) osok="bad os '${os}'" ;; esac
    assert_eq "${osok}" "ok" "manifest row ${name} names only known OSes"
    assert_grep "features.yaml declares install.herdr-plugin.${name}" \
        "path: install\\.herdr-plugin\\.${name}\$" "${FEATURES}"
    case "${ref}" in
        v[0-9]*|[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]*) pinned=yes ;;
        *) pinned="no (${ref})" ;;
    esac
    assert_eq "${pinned}" "yes" "manifest row ${name} pins a release tag or commit"
    if [ -f "${KEYS_DIR}/${name}.toml" ]; then
        foreign="$(grep '^command = ' "${KEYS_DIR}/${name}.toml" | grep -c -v -F "command = \"${id}.")"
        assert_eq "${foreign}" "0" "keys for ${name} only bind ${id}'s own actions"
    fi
done <<EOF_ROWS
${rows}
EOF_ROWS
for frag in "${KEYS_DIR}"/*.toml; do
    fname="$(basename "${frag}" .toml)"
    assert_eq "$(printf '%s\n' "${rows}" | awk -v n="${fname}" '$1 == n' | wc -l | tr -d ' ')" "1" \
        "keys fragment ${fname}.toml belongs to a manifest row"
done

# --- install.sh wiring for plugins + Rust --------------------------------------
assert_grep "install.sh runs install_herdr.sh in plugins mode" 'install_herdr\.sh" plugins' "${INSTALL_SH}"
assert_grep "install.sh gates Rust on install.runtime.rust" 'gff_on install\.runtime\.rust;' "${INSTALL_SH}"
assert_grep "features.yaml declares install.runtime.rust" 'path: install\.runtime\.rust$' "${FEATURES}"
assert_eq "$(printf '%s' "${deps_line}" | grep -c -w 'INSTALL_RUNTIME_RUST')" "1" "INSTALL_RUNTIME_RUST is in _IP_DEPS_FLAGS"
assert_eq "$(printf '%s' "${config_line}" | grep -c -w 'INSTALL_RUNTIME_RUST')" "0" "INSTALL_RUNTIME_RUST is NOT in _IP_CONFIG_FLAGS"
assert_eq "$(grep -E '^_IP_(DEPS|CONFIG)_FLAGS=' "${INSTALL_SH}" | grep -c 'HERDR_PLUGIN')" "0" \
    "per-plugin flags are read by install_herdr.sh, so they are in neither phase list"
bin_line="$(grep -n 'opt/scripts/system/install_herdr.sh" || echo' "${INSTALL_SH}" | head -1 | cut -d: -f1)"
rust_line="$(grep -n 'install_rust.sh" ||' "${INSTALL_SH}" | head -1 | cut -d: -f1)"
plug_line="$(grep -n 'install_herdr.sh" plugins' "${INSTALL_SH}" | head -1 | cut -d: -f1)"
if [ -n "${bin_line}" ] && [ -n "${rust_line}" ] && [ -n "${plug_line}" ] \
    && [ "${plug_line}" -gt "${bin_line}" ] && [ "${plug_line}" -gt "${rust_line}" ]; then
    assert_eq "ordered" "ordered" "herdr plugins run after the herdr binary and Rust (line ${plug_line} > ${bin_line}, ${rust_line})"
else
    assert_eq "plugins@${plug_line:-missing} herdr@${bin_line:-missing} rust@${rust_line:-missing}" "ordered" \
        "herdr plugins run after the herdr binary and Rust"
fi

_test_report
