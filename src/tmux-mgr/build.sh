#!/bin/bash
set -e

DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$DIR/../../" && pwd)"
BIN_DIR="${HOME}/opt/bin"
SKILL_INSTALL_DIR="${HOME}/.agents/skills"

VERSION=$(cat "$DIR/VERSION")
COMMIT=$(git -C "$DIR" rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
DIRTY="false"
if ! git -C "$DIR" diff --quiet 2>/dev/null; then
    DIRTY="true"
fi

LDFLAGS="-X github.com/eraigosa/dotfiles/src/tmux-mgr/cmd.Version=$VERSION \
         -X github.com/eraigosa/dotfiles/src/tmux-mgr/cmd.Commit=$COMMIT \
         -X github.com/eraigosa/dotfiles/src/tmux-mgr/cmd.BuildDate=$DATE \
         -X github.com/eraigosa/dotfiles/src/tmux-mgr/cmd.Dirty=$DIRTY"

if ! command -v go &> /dev/null; then
    echo "WARNING: 'go' command not found. Skipping tmux-mgr build."
    echo "Install Go (https://golang.org/doc/install) and run this script again to enable tmux-mgr."
    exit 0
fi

mkdir -p "$BIN_DIR"
mkdir -p "$SKILL_INSTALL_DIR"

echo "Building tmux-mgr v$VERSION..."
cd "$DIR"
go build -ldflags "$LDFLAGS" -o "$BIN_DIR/tmux-mgr" main.go

echo "Installing tmux skill..."
mkdir -p "$SKILL_INSTALL_DIR"
ln -sfn "$DIR/skill" "$SKILL_INSTALL_DIR/tmux"

echo "tmux-mgr built and skill installed."
