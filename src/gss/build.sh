#!/bin/bash
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
    echo "WARNING: 'go' not found; skipping gss build."
    exit 0
fi

VERSION=$(cat "$SCRIPT_DIR/VERSION")
COMMIT=$(git -C "$SCRIPT_DIR" rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
DIRTY="false"
if ! git -C "$SCRIPT_DIR" diff --quiet 2>/dev/null; then
    DIRTY="true"
fi

# Version metadata is stamped into the single source of truth,
# internal/version (PR-04; design.md → "Build-time version"). No build
# vars live in package cmd or package main any more.
LDFLAGS="-X github.com/wenlock/dotfiles/gss/internal/version.Version=$VERSION \
         -X github.com/wenlock/dotfiles/gss/internal/version.Commit=$COMMIT \
         -X github.com/wenlock/dotfiles/gss/internal/version.BuildDate=$DATE \
         -X github.com/wenlock/dotfiles/gss/internal/version.Dirty=$DIRTY"

mkdir -p "$BIN_DIR"
echo "Building gss v$VERSION with $($GO_BIN version)..."
cd "$SCRIPT_DIR"
"$GO_BIN" build -ldflags "$LDFLAGS" -o "$BIN_DIR/gss" main.go
echo "gss built and installed to $BIN_DIR/gss"
