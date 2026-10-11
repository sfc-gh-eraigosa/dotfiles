#!/usr/bin/env bash
# Test driver for opt/lib/pins.sh (the versions.tsv resolver). POSIX-only on
# purpose, like gff_test.sh: pins.sh is sourced by bash installers AND must stay
# dash-safe, so run both:
#   bash opt/lib/pins_test.sh
#   sh   opt/lib/pins_test.sh
# Network is never touched: pins_latest_gh is pointed at a file:// fixture
# (PINS_GH_API) or at a fake curl on PATH.
# shellcheck disable=SC2034 # GFF_*/PINS_* vars are read by the sourced helper
set -u

SCRIPT_DIR=$(cd -- "$(dirname "$0")" && pwd -P)
REPO_ROOT=$(cd -- "${SCRIPT_DIR}/../.." && pwd -P)
HELPER="${SCRIPT_DIR}/pins.sh"
PASS=0
FAIL=0

if [ ! -f "$HELPER" ]; then
    echo "FAIL: helper not found at $HELPER"
    echo "---"
    echo "PASS: 0  FAIL: 1"
    exit 1
fi
# shellcheck source=opt/lib/pins.sh
. "$HELPER"

# usage: assert_out <expected> <actual> <label>
assert_out() {
    if [ "$2" = "$1" ]; then
        echo "PASS: $3"
        PASS=$((PASS+1))
    else
        echo "FAIL: $3 (expected '$1', got '$2')"
        FAIL=$((FAIL+1))
    fi
}
# usage: assert_rc <expected> <actual> <label>
assert_rc() {
    if [ "$2" -eq "$1" ]; then
        echo "PASS: $3 (exit $2)"
        PASS=$((PASS+1))
    else
        echo "FAIL: $3 (expected exit $1, got $2)"
        FAIL=$((FAIL+1))
    fi
}

TMP=$(mktemp -d "${TMPDIR:-/tmp}/pins_test.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

unset GFF_PINS_TRACK GFF_PINS_TRACK_MODE PINS_ROOT 2>/dev/null || true
# Hermetic by default: no real gff lookup unless a test points PINS_GFF at a stub.
PINS_GFF=""
export PINS_GFF

# --- fixture manifest + @file root -------------------------------------------
mkdir -p "$TMP/root/sub"
printf '  1.26.9\n\n' > "$TMP/root/.go-version"
printf 'v4.5.6\n' > "$TMP/root/sub/vfile"
M="$TMP/versions.tsv"
cat > "$M" <<'EOF'
# tool   track   version  upstream  installer  probe...
#plain   pin     9.9.9    -         -          -

go       pin     @.go-version   go:   -   go version
sops     pin     3.13.1   gh:getsops/sops   opt/scripts/system/install_sops.sh   sops --version
vtool    pin     v2.0.1   gh:o/r    -          -
vfile    pin     @sub/vfile     -     -          -
gone     pin     @no/such/file  -     -          -
rust     floor   1.96.0   gh:rust-lang/rust   -   rustc --version
herdr    latest  -        -         opt/scripts/system/install_herdr.sh   herdr --version
lver     latest  5.0.0    gh:o/l    -          -
EOF
PINS_MANIFEST="$M"; export PINS_MANIFEST
PINS_ROOT="$TMP/root"; export PINS_ROOT

# --- pins_manifest -------------------------------------------------------------
assert_out "$M" "$(pins_manifest)" "pins_manifest honours PINS_MANIFEST"
_default=$(unset PINS_MANIFEST; pins_manifest)
assert_out "${REPO_ROOT}/opt/profiles/versions.tsv" \
    "$(cd -- "$(dirname "$_default")" && pwd -P)/$(basename "$_default")" \
    "pins_manifest defaults to <lib>/../profiles/versions.tsv"

# --- pins_strip_v --------------------------------------------------------------
assert_out "1.2.3" "$(pins_strip_v v1.2.3)" "pins_strip_v strips a leading v"
assert_out "1.2.3" "$(pins_strip_v 1.2.3)" "pins_strip_v leaves a bare version alone"
assert_out "latest" "$(pins_strip_v latest)" "pins_strip_v leaves non-version words alone"

# --- pins_want: manifest track -------------------------------------------------
assert_out "3.13.1" "$(pins_want sops 1.0.0)" "plain pin row => its version"
assert_out "1.96.0" "$(pins_want rust 1.0.0)" "floor row => its version"
assert_out "1.26.9" "$(pins_want go 1.0.0)" "@file row => the file's trimmed content"
assert_out "4.5.6" "$(pins_want vfile 1.0.0)" "@file content has its leading v stripped"
assert_out "2.0.1" "$(pins_want vtool 1.0.0)" "leading v in the manifest is stripped"
assert_out "7.7.7" "$(pins_want gone 7.7.7)" "@file that does not exist => fallback"
assert_out "8.8.8" "$(pins_want nosuchtool 8.8.8)" "no row => fallback"
assert_out "8.8.8" "$(pins_want nosuchtool v8.8.8)" "fallback has its leading v stripped too"
assert_out "9.9.9" "$(pins_want '#plain' 9.9.9)" "commented-out rows are ignored"
assert_out "1.0.0" "$(pins_want plain 1.0.0)" "a commented row does not match its bare name"
assert_out "latest" "$(pins_want herdr 0.1.0)" "latest row => latest"
assert_out "latest" "$(pins_want lver 0.1.0)" "latest row with a version => still latest under manifest"
pins_want nosuchtool >/dev/null; assert_rc 0 $? "pins_want always exits 0 (no row, no fallback)"

# --- pins_want: no manifest => fallback (fail-open) ------------------------------
_out=$(PINS_MANIFEST="$TMP/missing.tsv" pins_want sops 3.0.0); _rc=$?
assert_out "3.0.0" "$_out" "missing manifest => fallback"
assert_rc 0 "$_rc" "missing manifest => exit 0"

# --- pins_want: GFF_PINS_TRACK --------------------------------------------------
assert_out "latest" "$(GFF_PINS_TRACK=latest pins_want sops 1.0.0)" "GFF_PINS_TRACK=latest => latest for a pin row"
assert_out "latest" "$(GFF_PINS_TRACK=latest pins_want nosuchtool 1.0.0)" "GFF_PINS_TRACK=latest => latest even with no row"
assert_out "3.13.1" "$(GFF_PINS_TRACK=manifest pins_want sops 1.0.0)" "GFF_PINS_TRACK=manifest honours the row"
assert_out "0.1.0" "$(GFF_PINS_TRACK=pin pins_want herdr 0.1.0)" "GFF_PINS_TRACK=pin + latest row with '-' => fallback"
assert_out "5.0.0" "$(GFF_PINS_TRACK=pin pins_want lver 0.1.0)" "GFF_PINS_TRACK=pin + latest row with a version => that version"
assert_out "3.13.1" "$(GFF_PINS_TRACK=pin pins_want sops 1.0.0)" "GFF_PINS_TRACK=pin + pin row => its version"
# gff exports the 3-segment key pins.track.mode as GFF_PINS_TRACK_MODE.
assert_out "latest" "$(GFF_PINS_TRACK_MODE=latest pins_want sops 1.0.0)" "GFF_PINS_TRACK_MODE (gff export of pins.track.mode) is honoured"

# --- pins_want: no export => ask gff directly (fleet pins apply, a bare run) ----
GFFSTUB_DIR=$(mktemp -d)
printf '#!/bin/sh\n[ "$1 $2" = "get pins.track.mode" ] && echo "${STUB_MODE:-manifest}"\n' > "$GFFSTUB_DIR/gff"
chmod +x "$GFFSTUB_DIR/gff"
assert_out "latest" "$(PINS_GFF="$GFFSTUB_DIR/gff" STUB_MODE=latest pins_want sops 1.0.0)" "no export: gff get pins.track.mode=latest is honoured"
assert_out "3.13.1" "$(PINS_GFF="$GFFSTUB_DIR/gff" STUB_MODE=bogus pins_want sops 1.0.0)" "no export: an unknown gff answer fails open to manifest"
assert_out "3.13.1" "$(PINS_GFF="$GFFSTUB_DIR/missing-gff" pins_want sops 1.0.0)" "no export: an absent gff fails open to manifest"
assert_out "3.13.1" "$(PINS_GFF="$GFFSTUB_DIR/gff" STUB_MODE=latest GFF_PINS_TRACK_MODE=manifest pins_want sops 1.0.0)" "an export wins over asking gff"
rm -rf "$GFFSTUB_DIR"
assert_out "3.13.1" "$(GFF_PINS_TRACK=manifest GFF_PINS_TRACK_MODE=latest pins_want sops 1.0.0)" "GFF_PINS_TRACK wins over GFF_PINS_TRACK_MODE"

# --- pins_want against the REAL repo manifest -----------------------------------
_real_go=$(tr -d '[:space:]' < "${REPO_ROOT}/.go-version")
_out=$(unset PINS_MANIFEST PINS_ROOT; pins_want go 0.0.0)
assert_out "$_real_go" "$_out" "real manifest: go resolves @.go-version from the repo root"
_out=$(unset PINS_MANIFEST PINS_ROOT; pins_want herdr 0.0.0)
assert_out "latest" "$_out" "real manifest: herdr is tracked latest"
_out=$(unset PINS_MANIFEST PINS_ROOT; pins_want keyd 0.0.0)
assert_out "2.6.0" "$_out" "real manifest: keyd pin has no leading v"

# --- pins_latest_gh via a file:// API base ---------------------------------------
mkdir -p "$TMP/api/repos/o/r/releases" "$TMP/api/repos/o/empty/releases"
cat > "$TMP/api/repos/o/r/releases/latest" <<'EOF'
{
  "url": "https://api.github.com/repos/o/r/releases/1",
  "tag_name": "v1.2.3",
  "name": "Release 1.2.3"
}
EOF
printf '{"message":"Not Found"}\n' > "$TMP/api/repos/o/empty/releases/latest"
if command -v curl >/dev/null 2>&1; then
    _out=$(PINS_GH_API="file://$TMP/api" pins_latest_gh o/r); _rc=$?
    assert_out "1.2.3" "$_out" "pins_latest_gh prints the tag without v"
    assert_rc 0 "$_rc" "pins_latest_gh exits 0 on success"
    PINS_GH_API="file://$TMP/api" pins_latest_gh o/missing >/dev/null 2>&1
    assert_rc 1 $? "pins_latest_gh is non-zero when the API call fails"
    PINS_GH_API="file://$TMP/api" pins_latest_gh o/empty >/dev/null 2>&1
    assert_rc 1 $? "pins_latest_gh is non-zero when there is no tag_name"
fi

# --- pins_latest_gh honours GITHUB_TOKEN (fake curl records its args) -----------
mkdir -p "$TMP/bin"
cat > "$TMP/bin/curl" <<'EOF'
#!/bin/sh
printf '%s\n' "$@" > "$CURL_ARGS"
printf '{"tag_name": "2.0.0"}\n'
EOF
chmod +x "$TMP/bin/curl"
CURL_ARGS="$TMP/curl.args"; export CURL_ARGS
_fake_tok="fake-$$"
_out=$(PATH="$TMP/bin:$PATH" GITHUB_TOKEN="$_fake_tok" pins_latest_gh o/r)
assert_out "2.0.0" "$_out" "pins_latest_gh parses a single-line response"
if grep -q "Authorization: Bearer ${_fake_tok}" "$CURL_ARGS"; then
    echo "PASS: GITHUB_TOKEN is sent as a bearer token"; PASS=$((PASS+1))
else
    echo "FAIL: GITHUB_TOKEN is sent as a bearer token ($(tr '\n' ' ' < "$CURL_ARGS"))"; FAIL=$((FAIL+1))
fi
if grep -q '^https://api.github.com/repos/o/r/releases/latest$' "$CURL_ARGS"; then
    echo "PASS: default API base is api.github.com"; PASS=$((PASS+1))
else
    echo "FAIL: default API base is api.github.com ($(tr '\n' ' ' < "$CURL_ARGS"))"; FAIL=$((FAIL+1))
fi
(PATH="$TMP/bin:$PATH"; unset GITHUB_TOKEN; pins_latest_gh o/r >/dev/null)
if grep -q 'Authorization' "$CURL_ARGS"; then
    echo "FAIL: no Authorization header without GITHUB_TOKEN"; FAIL=$((FAIL+1))
else
    echo "PASS: no Authorization header without GITHUB_TOKEN"; PASS=$((PASS+1))
fi

# --- the seed manifest itself ------------------------------------------------------
REAL="${REPO_ROOT}/opt/profiles/versions.tsv"
_rows=$(awk '$1 !~ /^#/ && NF { n++ } END { print n + 0 }' "$REAL" 2>/dev/null)
assert_out "14" "$_rows" "versions.tsv has the 14 seed rows"
_dups=$(awk '$1 !~ /^#/ && NF { print $1 }' "$REAL" 2>/dev/null | sort | uniq -d)
assert_out "" "$_dups" "versions.tsv has no duplicate tools"
_bad=$(awk '$1 !~ /^#/ && NF && $2 != "pin" && $2 != "floor" && $2 != "latest" { print $1 }' "$REAL" 2>/dev/null)
assert_out "" "$_bad" "every versions.tsv track is pin|floor|latest"
_short=$(awk '$1 !~ /^#/ && NF && NF < 6 { print $1 }' "$REAL" 2>/dev/null)
assert_out "" "$_short" "every versions.tsv row has tool track version upstream installer probe"
_noinst=$(awk '$1 !~ /^#/ && NF && $5 != "-" { print $5 }' "$REAL" 2>/dev/null | while read -r p; do [ -f "${REPO_ROOT}/$p" ] || echo "$p"; done)
assert_out "" "$_noinst" "every versions.tsv installer path exists"

echo "---"
echo "PASS: $PASS  FAIL: $FAIL"
[ "$FAIL" -eq 0 ]
