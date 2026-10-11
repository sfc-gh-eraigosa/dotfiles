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
if [ -n "$out" ]; then cp "$src" "$out"; else cat "$src"; fi
SHIM
chmod +x "$STUBS"/*
export STUB_FIXTURES="$FIX"

# Hermetic PATH: symlink only the coreutils the installer needs, so a gitleaks
# the build host has in /usr/bin (apt) can't leak into the "absent" cases.
SYS="$TMP/sys"; mkdir -p "$SYS"
for t in bash sh env cat cp mv rm mkdir chmod ln ls head tail awk sed grep tr cut sort dirname basename mktemp tar gzip uname sha256sum shasum; do
    p="$(command -v "$t" 2>/dev/null)" || continue
    case "$p" in /*) ln -sf "$p" "$SYS/$t" ;; esac
done

# Release fixture: the asset name the script must derive for THIS machine.
case "$(uname -s)" in Darwin) OS=darwin ;; *) OS=linux ;; esac
case "$(uname -m)" in x86_64|amd64) ARCH=x64 ;; arm64|aarch64) ARCH=arm64 ;; armv7l) ARCH=armv7 ;; *) ARCH="$(uname -m)" ;; esac
V="8.21.2"
ASSET="gitleaks_${V}_${OS}_${ARCH}.tar.gz"
mkdir -p "$TMP/tarsrc"; mkfake "$V" "$TMP/tarsrc/gitleaks"
tar -C "$TMP/tarsrc" -czf "$FIX/$ASSET" gitleaks
sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }
printf '%s  %s\n%s  gitleaks_%s_other_arch.tar.gz\n' "$(sha "$FIX/$ASSET")" "$ASSET" "0000000000000000000000000000000000000000000000000000000000000000" "$V" > "$FIX/gitleaks_${V}_checksums.txt"
# A second (newer) release, served as "latest" by the fake GitHub API.
V2="8.22.0"
ASSET2="gitleaks_${V2}_${OS}_${ARCH}.tar.gz"
mkdir -p "$TMP/tarsrc2"; mkfake "$V2" "$TMP/tarsrc2/gitleaks"
tar -C "$TMP/tarsrc2" -czf "$FIX/$ASSET2" gitleaks
printf '%s  %s\n' "$(sha "$FIX/$ASSET2")" "$ASSET2" > "$FIX/gitleaks_${V2}_checksums.txt"
printf '{"tag_name": "v%s"}\n' "$V2" > "$FIX/latest"
# The manifest the default version is read from (pins_want gitleaks ...).
MAN="$TMP/versions.tsv"
printf 'gitleaks pin %s gh:gitleaks/gitleaks opt/scripts/git/install_gitleaks.sh gitleaks version\n' "$V" > "$MAN"

reset() { : > "$STUB_LOG"; rm -f "$STUBS/gitleaks"; rm -rf "$INSTALL_DIR" "$CFG"; }

OUT=""
# usage: run <want-rc> <label> <method> [args...]
run() {
    local want="$1" label="$2" method="$3" rc; shift 3
    # RUN_VERSION="" drops the env override so the manifest decides;
    # RUN_MANIFEST / RUN_TRACK swap the manifest / the gff pins.track.mode value.
    OUT="$(env -u GFF_PINS_TRACK PATH="$STUBS:$SYS" HOME="$TMP/home" GITLEAKS_INSTALL_METHOD="$method" \
          GITLEAKS_INSTALL_DIR="$INSTALL_DIR" GITLEAKS_VERSION="${RUN_VERSION-$V}" GITLEAKS_RELEASE_BASE="https://example.invalid/releases" \
          PINS_MANIFEST="${RUN_MANIFEST:-$MAN}" GFF_PINS_TRACK_MODE="${RUN_TRACK:-manifest}" PINS_GH_API="https://api.example.invalid" \
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

# --- converge (objective `pins`, F3): only a binary in GITLEAKS_INSTALL_DIR ------

# 9. our binary already at WANT => "already at", no download, no package manager.
reset; mkdir -p "$INSTALL_DIR"; mkfake "$V" "$INSTALL_DIR/gitleaks"
run 0 "converge: own binary == want => exit 0" auto
check "converge: says already at $V" 'printf "%s" "$OUT" | grep -q "already at $V"'
check "converge: equal => no calls at all" '[ ! -s "$STUB_LOG" ]'

# 10. our binary older => reinstalled from the release (not apt, even with a candidate).
reset; mkdir -p "$INSTALL_DIR"; mkfake 8.20.0 "$INSTALL_DIR/gitleaks"
run 0 "converge: own binary older => exit 0" auto
check "converge: older => upgraded to $V" '"$INSTALL_DIR/gitleaks" version | grep -q "$V"'
check "converge: older => release download, not apt" 'grep -q "$ASSET" "$STUB_LOG" && ! grep -q "apt-get install" "$STUB_LOG"'
check "converge: reports the upgrade" 'printf "%s" "$OUT" | grep -q "8.20.0 -> $V"'

# 11. a gitleaks outside GITLEAKS_INSTALL_DIR (apt/brew-owned) is left alone.
reset; mkfake 8.0.0 "$STUBS/gitleaks"
run 0 "foreign: apt/brew-owned gitleaks => exit 0" auto
check "foreign: says it is leaving it alone" 'printf "%s" "$OUT" | grep -q "leaving it alone"'
check "foreign: nothing installed or called" '[ ! -s "$STUB_LOG" ] && [ ! -e "$INSTALL_DIR/gitleaks" ]'

# 12. the default WANT comes from the manifest (no GITLEAKS_VERSION in the env).
reset; mkdir -p "$INSTALL_DIR"; mkfake 8.20.0 "$INSTALL_DIR/gitleaks"
RUN_VERSION="" run 0 "manifest: default version from versions.tsv" release
check "manifest: installed the manifest pin" '"$INSTALL_DIR/gitleaks" version | grep -q "$V"'

# 13. the env override wins over the manifest.
printf 'gitleaks pin 8.0.1 - - -\n' > "$TMP/other.tsv"
reset
RUN_MANIFEST="$TMP/other.tsv" run 0 "env override: GITLEAKS_VERSION beats the manifest" release
check "env override: installed GITLEAKS_VERSION" '"$INSTALL_DIR/gitleaks" version | grep -q "$V"'

# 14. pins.track.mode=latest resolves upstream (GitHub API) before downloading.
reset; mkdir -p "$INSTALL_DIR"; mkfake "$V" "$INSTALL_DIR/gitleaks"
RUN_VERSION="" RUN_TRACK=latest run 0 "latest: resolves and converges to upstream" auto
check "latest: queried the releases API" 'grep -q "api.example.invalid/repos/gitleaks/gitleaks/releases/latest" "$STUB_LOG"'
check "latest: installed $V2" '"$INSTALL_DIR/gitleaks" version | grep -q "$V2"'

echo "----"
echo "install_gitleaks_test: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
