#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
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
    echo "WARNING: 'go' not found; skipping gdash build."
    exit 0
fi

# Version comes from the module's git release tag (sdk/gdash/vX.Y.Z).
# shellcheck source=sdk/version.sh
. "$SCRIPT_DIR/../version.sh"
VERSION="$(sdk_version "gdash" "$SCRIPT_DIR")"
COMMIT=$(git -C "$SCRIPT_DIR" rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
DIRTY="false"
if ! git -C "$SCRIPT_DIR" diff --quiet 2>/dev/null; then
    DIRTY="true"
fi

PKG="github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/version"
LDFLAGS="-X $PKG.Version=$VERSION -X $PKG.Commit=$COMMIT -X $PKG.BuildDate=$DATE -X $PKG.Dirty=$DIRTY"

mkdir -p "$BIN_DIR"
echo "Building gdash v$VERSION with $($GO_BIN version)..."
cd "$SCRIPT_DIR"
"$GO_BIN" build -ldflags "$LDFLAGS" -o "$BIN_DIR/gdash" main.go
echo "gdash built and installed to $BIN_DIR/gdash"
