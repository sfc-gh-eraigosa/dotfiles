#!/usr/bin/env bash
set -euo pipefail
cd "$(cd "$(dirname "$0")/.." && pwd -P)"
command -v protoc >/dev/null 2>&1 || {
  echo "protoc not found — apt-get install protobuf-compiler / brew install protobuf" >&2
  exit 1
}
GOBIN="${PWD}/.bin" go install google.golang.org/protobuf/cmd/protoc-gen-go  # version from go.mod
mkdir -p gen  # protoc does not create the --go_out root itself
PATH="${PWD}/.bin:${PATH}" protoc \
  --proto_path=proto \
  --go_out=gen --go_opt=paths=source_relative \
  proto/gff/v1/features.proto
