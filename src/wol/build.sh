#!/bin/bash
set -e

# Determine the directory of this script
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# Use HOME variable to define the binary directory
BIN_DIR="${HOME}/opt/bin"

VERSION=$(cat "$SCRIPT_DIR/VERSION")
COMMIT=$(git -C "$SCRIPT_DIR" rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
DIRTY="false"
if ! git -C "$SCRIPT_DIR" diff --quiet 2>/dev/null; then
    DIRTY="true"
fi

LDFLAGS="-X github.com/wenlock/dotfiles/wol/cmd.Version=$VERSION \
         -X github.com/wenlock/dotfiles/wol/cmd.Commit=$COMMIT \
         -X github.com/wenlock/dotfiles/wol/cmd.BuildDate=$DATE \
         -X github.com/wenlock/dotfiles/wol/cmd.Dirty=$DIRTY"

mkdir -p "$BIN_DIR"
echo "Building wol v$VERSION..."
cd "$SCRIPT_DIR"
go build -ldflags "$LDFLAGS" -o "$BIN_DIR/wol" main.go
echo "wol built and installed to $BIN_DIR/wol"
