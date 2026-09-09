#!/usr/bin/env bash
# Test driver for install_gitleaks.sh — hermetic: package managers, sudo and curl
# are stubs on PATH; the release path is served from a local fixture tarball.
# exit 0 = all pass, 1 = a failure.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/install_gitleaks.sh"
PASS=0
FAIL=0
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

STUBS="$TMP/stubs"; mkdir -p "$STUBS"
export STUB_LOG="$TMP/calls.log"
export STUB_APT_CANDIDATE="8.16.0-1build2"
export STUB_FAKE_BIN_DIR="$STUBS"          # where stub installers drop a fake gitleaks
export CFG="$TMP/cfg"
export INSTALL_DIR="$TMP/opt/bin"
FIX="$TMP/fixtures"; mkdir -p "$FIX"

mkfake() { printf '#!/usr/bin/env bash\necho "gitleaks version %s"\n' "$1" > "$2"; chmod +x "$2"; }

cat > "$STUBS/sudo" <<'SHIM'
#!/usr/bin/env bash
printf 'sudo %s\n' "$*" >> "$STUB_LOG"
# drop VAR=val prefixes the way sudo does, then run the command
while [ $# -gt 0 ] && [[ "$1" == *=* ]]; do shift; done
exec "$@"
SHIM
cat > "$STUBS/apt-get" <<'SHIM'
#!/usr/bin/env bash
printf 'apt-get %s\n' "$*" >> "$STUB_LOG"
case " $* " in *" install "*) printf '#!/usr/bin/env bash\necho "gitleaks version apt"\n' > "$STUB_FAKE_BIN_DIR/gitleaks"; chmod +x "$STUB_FAKE_BIN_DIR/gitleaks" ;; esac
SHIM
cat > "$STUBS/apt-cache" <<'SHIM'
#!/usr/bin/env bash
printf 'apt-cache %s\n' "$*" >> "$STUB_LOG"
printf 'gitleaks:\n  Installed: (none)\n  Candidate: %s\n' "$STUB_APT_CANDIDATE"
SHIM
cat > "$STUBS/brew" <<'SHIM'
#!/usr/bin/env bash
printf 'brew %s\n' "$*" >> "$STUB_LOG"
case " $* " in *" install "*) printf '#!/usr/bin/env bash\necho "gitleaks version brew"\n' > "$STUB_FAKE_BIN_DIR/gitleaks"; chmod +x "$STUB_FAKE_BIN_DIR/gitleaks" ;; esac
SHIM
cat > "$STUBS/curl" <<'SHIM'
#!/usr/bin/env bash
printf 'curl %s\n' "$*" >> "$STUB_LOG"
url=""; out=""; prev=""
for a in "$@"; do [ "$prev" = "-o" ] && out="$a"; case "$a" in http*|file*) url="$a" ;; esac; prev="$a"; done
src="$STUB_FIXTURES/$(basename "$url")"
[ -f "$src" ] || { echo "curl: (22) not found: $url" >&2; exit 22; }
cp "$src" "$out"
SHIM
chmod +x "$STUBS"/*
export STUB_FIXTURES="$FIX"

# Release fixture: the asset name the script must derive for THIS machine.
case "$(uname -s)" in Darwin) OS=darwin ;; *) OS=linux ;; esac
case "$(uname -m)" in x86_64|amd64) ARCH=x64 ;; arm64|aarch64) ARCH=arm64 ;; armv7l) ARCH=armv7 ;; *) ARCH="$(uname -m)" ;; esac
V="8.21.2"
ASSET="gitleaks_${V}_${OS}_${ARCH}.tar.gz"
mkdir -p "$TMP/tarsrc"; mkfake "$V" "$TMP/tarsrc/gitleaks"
tar -C "$TMP/tarsrc" -czf "$FIX/$ASSET" gitleaks
sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }
printf '%s  %s\n%s  gitleaks_%s_other_arch.tar.gz\n' "$(sha "$FIX/$ASSET")" "$ASSET" "0000000000000000000000000000000000000000000000000000000000000000" "$V" > "$FIX/gitleaks_${V}_checksums.txt"

reset() { : > "$STUB_LOG"; rm -f "$STUBS/gitleaks"; rm -rf "$INSTALL_DIR" "$CFG"; }

OUT=""
# usage: run <want-rc> <label> <method> [args...]
run() {
    local want="$1" label="$2" method="$3" rc; shift 3
    OUT="$(env PATH="$STUBS:/usr/bin:/bin" HOME="$TMP/home" GITLEAKS_INSTALL_METHOD="$method" \
          GITLEAKS_INSTALL_DIR="$INSTALL_DIR" GITLEAKS_VERSION="$V" GITLEAKS_RELEASE_BASE="https://example.invalid/releases" \
          PRIVACY_GUARD_CONFIG_DIR="$CFG" STUB_LOG="$STUB_LOG" STUB_FAKE_BIN_DIR="$STUB_FAKE_BIN_DIR" STUB_FIXTURES="$FIX" \
          STUB_APT_CANDIDATE="$STUB_APT_CANDIDATE" bash "$SCRIPT" "$@" 2>&1)"; rc=$?
    if [ "$rc" -eq "$want" ]; then echo "PASS: $label"; PASS=$((PASS+1))
    else echo "FAIL: $label (want $want, got $rc) :: $OUT"; FAIL=$((FAIL+1)); fi
}
check() { if eval "$2"; then echo "PASS: $1"; PASS=$((PASS+1)); else echo "FAIL: $1 :: $(cat "$STUB_LOG" 2>/dev/null | tr '\n' '|') :: $OUT"; FAIL=$((FAIL+1)); fi; }

# 1. already installed
reset; mkfake present "$STUBS/gitleaks"
run 0 "already installed => no-op" auto
check "already: says already" 'printf "%s" "$OUT" | grep -qi already'
check "already: no package-manager calls" '[ ! -s "$STUB_LOG" ]'

# 2. apt
reset
run 0 "apt: installs via sudo apt-get (no apt-get update — one package, keep it fast)" apt
check "apt: install -y gitleaks logged" 'grep -q "apt-get install.*gitleaks" "$STUB_LOG"'
check "apt: went through sudo" 'grep -q "^sudo .*apt-get" "$STUB_LOG"'
check "apt: no apt-get update" '! grep -q "apt-get update" "$STUB_LOG"'

# 3. brew
reset
run 0 "brew: installs via brew install" brew
check "brew: install gitleaks logged" 'grep -q "^brew install gitleaks" "$STUB_LOG"'

# 4. auto picks apt when apt has a candidate
reset
run 0 "auto: apt candidate present => apt" auto
check "auto: used apt-get" 'grep -q "apt-get install" "$STUB_LOG"'

# 5. auto falls to release when apt has no candidate and brew is absent
reset; mv "$STUBS/brew" "$TMP/brew.off"
STUB_APT_CANDIDATE="(none)" run 0 "auto: no apt candidate, no brew => release download" auto
check "auto/release: fetched the checksums file" 'grep -q "checksums.txt" "$STUB_LOG"'
check "auto/release: binary installed and runs" '[ -x "$INSTALL_DIR/gitleaks" ] && "$INSTALL_DIR/gitleaks" version | grep -q "$V"'
mv "$TMP/brew.off" "$STUBS/brew"; STUB_APT_CANDIDATE="8.16.0-1build2"

# 6. release: verified download
reset
run 0 "release: pinned version, sha256-verified" release
check "release: fetched $ASSET" 'grep -q "$ASSET" "$STUB_LOG"'
check "release: binary in GITLEAKS_INSTALL_DIR" '[ -x "$INSTALL_DIR/gitleaks" ]'

# 7. release: checksum mismatch refuses to install
reset; cp "$FIX/gitleaks_${V}_checksums.txt" "$TMP/sums.bak"
sed "s/^[0-9a-f]\{64\}  $ASSET/deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef  $ASSET/" "$TMP/sums.bak" > "$FIX/gitleaks_${V}_checksums.txt"
run 1 "release: checksum mismatch => exit 1" release
check "release: nothing installed on mismatch" '[ ! -e "$INSTALL_DIR/gitleaks" ]'
check "release: says checksum" 'printf "%s" "$OUT" | grep -qi checksum'
cp "$TMP/sums.bak" "$FIX/gitleaks_${V}_checksums.txt"

# 8. --off: the flag is false => no install, marker tells the hooks to skip gitleaks
reset
run 0 "--off: exit 0" auto --off
check "--off: writes off marker" '[ -f "$CFG/gitleaks" ] && grep -qx off "$CFG/gitleaks"'
check "--off: installs nothing" '! grep -q "install" "$STUB_LOG"'
# ensure again (flag back on) removes the marker
mkfake present "$STUBS/gitleaks"
run 0 "ensure after --off: clears the marker" auto
check "ensure: marker removed" '[ ! -f "$CFG/gitleaks" ]'

echo "----"
echo "install_gitleaks_test: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
