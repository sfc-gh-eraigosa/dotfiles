#!/bin/bash
set -e

# Determine the directory of this script
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# Use HOME variable to define the binary directory
BIN_DIR="${HOME}/opt/bin"

mkdir -p "$BIN_DIR"
echo "Building gss..."
cd "$SCRIPT_DIR"
go build -o "$BIN_DIR/gss" main.go
echo "gss built and installed to $BIN_DIR/gss"
