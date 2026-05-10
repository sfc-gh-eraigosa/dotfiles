#!/bin/bash
set -e

# Use HOME variable to define the binary directory
BIN_DIR="${HOME}/opt/bin"

mkdir -p "$BIN_DIR"
echo "Building gss..."
go build -o "$BIN_DIR/gss" main.go
echo "gss built and installed to $BIN_DIR/gss"
