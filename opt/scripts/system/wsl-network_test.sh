#!/usr/bin/env bash
# =============================================================================
# wsl-network_test.sh - test driver for the sshd / WSL-networking gff flags.
#
#   STATIC (always, incl. Linux CI) - the wiring is present in source.
#
# Run standalone: bash opt/scripts/system/wsl-network_test.sh
# =============================================================================
set -u

SCRIPT_DIR=$(cd -- "$(dirname "$0")" && pwd -P)
REPO_ROOT=$(cd -- "${SCRIPT_DIR}/../../.." && pwd -P)
. "${REPO_ROOT}/ai/_test_helpers.sh"

INSTALL_SH="${REPO_ROOT}/install.sh"

echo "==== STATIC ===="
# F1: install.sh requires a working gff instead of fail-open.
# shellcheck disable=SC2016  # literal pattern, not a shell expansion
assert_grep "install.sh requires gff" 'gff_require "\$\{BASE_DIR\}" \|\| exit 1' "$INSTALL_SH"
assert_grep_negative "install.sh no longer fails open on a gff build error" \
  'gff build failed; all components will run' "$INSTALL_SH"

_test_report
