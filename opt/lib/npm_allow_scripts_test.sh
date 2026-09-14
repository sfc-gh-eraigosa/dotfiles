#!/usr/bin/env bash
# Test driver for opt/lib/npm_allow_scripts.sh
#
# What must hold (fleet history 2026-09-11, spark + pi on npm >= 11.16):
#   * a package is added to the user-level `allow-scripts` list, so npm stops
#     printing "install scripts not yet covered by allowScripts" on every run
#     and the postinstall keeps running if npm ever makes the list strict;
#   * entries already present are left alone (no duplicates, no rewrite);
#   * an npm that predates the key (11.13 on gigabyte/nano) is never asked to
#     set it — it would warn about an unknown config key instead;
#   * both installers that npm-install a package with a postinstall call it.
#
# `npm` is a stub that keeps its user config in a file.
#
# Run: bash opt/lib/npm_allow_scripts_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

LIB="${SELF_DIR}/npm_allow_scripts.sh"
assert_file_exists "${LIB}" "npm_allow_scripts.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${LIB}"
set +e

TMP="$(mktemp -d "${TMPDIR:-/tmp}/npm_allow_scripts_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
BIN="${TMP}/stubs"
mkdir -p "${BIN}"
STATE="${TMP}/allow-scripts"
LOG="${TMP}/calls.log"

# The stub supports exactly the three calls the helper makes. KNOWS_KEY=no
# makes it an npm that predates allow-scripts.
cat > "${BIN}/npm" <<EOF
#!/bin/sh
echo "npm \$*" >> "${LOG}"
case "\$1 \$2" in
  "config ls") [ "\${KNOWS_KEY:-yes}" = yes ] && echo 'allow-scripts = [""]'; echo 'also = null' ;;
  "config get") cat "${STATE}" 2>/dev/null ;;
  "config set") printf '%s' "\${3#allow-scripts=}" > "${STATE}" ;;
esac
exit 0
EOF
chmod +x "${BIN}/npm"

# allow <pkg> [KNOWS_KEY]: run the helper in a fresh shell with the stub on PATH.
allow() {
    : > "${LOG}"
    PATH="${BIN}:/usr/bin:/bin" KNOWS_KEY="${2:-yes}" bash -c '. "$1"; npm_allow_scripts "$2"' _ "${LIB}" "$1"
}

rm -f "${STATE}"
allow @anthropic-ai/claude-code
assert_eq "$(cat "${STATE}")" "@anthropic-ai/claude-code" "an empty list gains the package"

allow @googleworkspace/cli
assert_eq "$(cat "${STATE}")" "@anthropic-ai/claude-code,@googleworkspace/cli" "a second package is appended, comma-separated"

allow @anthropic-ai/claude-code
assert_grep_negative "a package already listed is not set again" '^npm config set' "${LOG}"
assert_eq "$(cat "${STATE}")" "@anthropic-ai/claude-code,@googleworkspace/cli" "the list is unchanged"

rm -f "${STATE}"
allow @anthropic-ai/claude-code no
assert_grep_negative "an npm without the key is never asked to set it" '^npm config set' "${LOG}"

# The user-level list is the one `npm install -g` reads.
allow @anthropic-ai/claude-code
assert_grep "writes the user-level config" '^npm config set allow-scripts=.* --location=user$' "${LOG}"

# --- no npm at all is a no-op, not an error ------------------------------------
rc=0
PATH="/nonexistent" /bin/bash -c '. "$1"; npm_allow_scripts pkg' _ "${LIB}" || rc=$?
assert_eq "${rc}" "0" "no npm on PATH is a quiet no-op"

# --- wiring ----------------------------------------------------------------------
assert_grep "claude_install.sh allows claude-code's postinstall" \
    'npm_allow_scripts "\$NPM_PKG"' "${REPO_ROOT}/opt/scripts/system/claude_install.sh"
assert_grep "google-cli-setup.sh allows gws's postinstall" \
    'npm_allow_scripts "@googleworkspace/cli"' "${REPO_ROOT}/opt/scripts/system/google-cli-setup.sh"

_test_report
