#!/usr/bin/env bash
# install-stamp.sh — record that install.sh completed, and at which commit.
#
# Consumed by `fleet status` (docs/mbo/specs/fleet.md F1) to distinguish
# "this host PULLED the latest dotfiles" from "this host actually RAN the
# installer" — two facts that were previously indistinguishable because
# install.sh left no trace of having run.
#
# Contract:
#   - Called as the LAST action of a successful install.sh run, so an
#     aborted run never leaves a false success marker.
#   - Writes only for a full run. A Docker image build invokes install.sh
#     with --phase deps|config to exploit layer caching; stamping there
#     would bake a bogus commit into the cached layer.
#   - Never fails the install: a missing/non-git BASE_DIR exits 0 quietly.
#
# Usage: install-stamp.sh [BASE_DIR]   (defaults to $BASE_DIR, then cwd)
set -u

BASE_DIR="${1:-${BASE_DIR:-$PWD}}"
STAMP_DIR="${HOME}/.local/state/dotfiles"
STAMP_FILE="${STAMP_DIR}/install-stamp"

# Build phases must never stamp. Unset means a normal, full run.
case "${INSTALL_PHASE:-all}" in
    all) ;;
    *)   exit 0 ;;
esac

COMMIT="$(git -C "${BASE_DIR}" rev-parse HEAD 2>/dev/null || true)"
if [ -z "${COMMIT}" ]; then
    # Not a git checkout (tarball install, odd container layout). Nothing
    # meaningful to record, and this must not fail the installer.
    exit 0
fi

BRANCH="$(git -C "${BASE_DIR}" rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)"

mkdir -p "${STAMP_DIR}"
# Single redirect (not append) so re-running replaces rather than grows.
cat > "${STAMP_FILE}" <<EOF
commit=${COMMIT}
installed_at=$(date -u +%s)
branch=${BRANCH}
hostname=$(uname -n)
EOF
chmod 644 "${STAMP_FILE}"
