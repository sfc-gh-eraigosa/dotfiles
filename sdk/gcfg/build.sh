#!/usr/bin/env bash
set -e

# Determine the directory of this script
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# Use HOME variable to define the binary directory
BIN_DIR="${HOME}/opt/bin"

# Drop any inherited GOROOT/GOTOOLCHAIN so the resolved `go` binary uses its
# own matching stdlib (prevents brew-go vs goenv-go version-mismatch).
unset GOROOT GOTOOLCHAIN

# Prefer goenv-managed go (respects repo-root .go-version); fall back to PATH.
if command -v goenv >/dev/null 2>&1; then
    GO_BIN="$(goenv which go 2>/dev/null || command -v go || true)"
else
    GO_BIN="$(command -v go || true)"
fi
if [ -z "$GO_BIN" ]; then
    echo "WARNING: 'go' not found; skipping gcfg build."
    exit 0
fi

# Version comes from the module's git release tag (sdk/gcfg/vX.Y.Z) —
# the single source of truth. See sdk/version.sh for why the old VERSION
# file was removed (it had to be committed to a protected branch).
# shellcheck source=sdk/version.sh
. "$SCRIPT_DIR/../version.sh"
VERSION="$(sdk_version "gcfg" "$SCRIPT_DIR")"
COMMIT=$(git -C "$SCRIPT_DIR" rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
DIRTY="false"
if ! git -C "$SCRIPT_DIR" diff --quiet 2>/dev/null; then
    DIRTY="true"
fi

# Version metadata is stamped into the single source of truth,
# internal/version. No build vars live in package cmd or package main.
LDFLAGS="-X github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/version.Version=$VERSION \
         -X github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/version.Commit=$COMMIT \
         -X github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/version.BuildDate=$DATE \
         -X github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/version.Dirty=$DIRTY"

mkdir -p "$BIN_DIR"
echo "Building gcfg v$VERSION with $($GO_BIN version)..."
cd "$SCRIPT_DIR"
"$GO_BIN" build -ldflags "$LDFLAGS" -o "$BIN_DIR/gcfg" main.go
echo "gcfg built and installed to $BIN_DIR/gcfg"
