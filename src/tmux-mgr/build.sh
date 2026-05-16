#!/bin/bash
set -e

DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$DIR/../../" && pwd)"
BIN_DIR="${HOME}/opt/bin"
SKILL_INSTALL_DIR="${HOME}/.agents/skills"

if ! command -v go &> /dev/null; then
    echo "WARNING: 'go' command not found. Skipping tmux-mgr build."
    echo "Install Go (https://golang.org/doc/install) and run this script again to enable tmux-mgr."
    exit 0
fi

mkdir -p "$BIN_DIR"
mkdir -p "$SKILL_INSTALL_DIR"

echo "Building tmux-mgr..."
cd "$DIR"
go build -o "$BIN_DIR/tmux-mgr" main.go

echo "Installing tmux skill..."
mkdir -p "$SKILL_INSTALL_DIR"
ln -sfn "$DIR/skill" "$SKILL_INSTALL_DIR/tmux"

echo "tmux-mgr built and skill installed."
