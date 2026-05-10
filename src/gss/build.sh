#!/bin/bash
set -e
BIN_DIR="/home/wenlock/opt/bin"
mkdir -p "$BIN_DIR"
echo "Building gss..."
go build -o "$BIN_DIR/gss" main.go
echo "gss built and installed to $BIN_DIR/gss"
