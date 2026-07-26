#!/usr/bin/env bash
set -euo pipefail

# Build gff into a temp dir and run e2e tests with the compiled binary.
# Usage: bash sdk/gff/scripts/e2e.sh

# Determine the module root (where go.mod lives).
mod_root="$(cd "$(dirname "$0")/.." && pwd -P)"

# Build into a temp dir so tests can find it via $GFF_E2E_BIN.
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

gff_bin="${tmp_dir}/gff"
(cd "$mod_root" && go build -o "$gff_bin" .)

# Export the binary path for the test harness.
export GFF_E2E_BIN="$gff_bin"

# Run tests with the e2e build tag from the module root.
(cd "$mod_root" && go test -tags e2e -v ./e2e)
