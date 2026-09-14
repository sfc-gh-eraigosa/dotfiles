#!/usr/bin/env bash
# Test driver for opt/lib/winpowershell.sh (find_powershell). Fleet runs over ssh
# into WSL have no /mnt/c dirs on PATH, so a bare `command -v powershell.exe`
# misses a powershell.exe that exists and works; find_powershell falls back to
# the System32 path wslpath derives. PATH is fully controlled here and wslpath is
# a stub mapping into a fake Windows tree, so this runs the same on WSL, Linux,
# and macOS.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=/dev/null
. "$REPO_ROOT/ai/_test_helpers.sh"

HELPER="$SCRIPT_DIR/winpowershell.sh"
# Absolute bash: `env PATH=<test PATH> bash` would search the TEST PATH for bash.
BASH_BIN="$(command -v bash)"

T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT

# Fake Windows tree + a stub wslpath that maps ONLY the exact System32 path
# install_windows.sh has always asked for (anything else fails, like a bad path).
SYS32_PS="$T/win/System32/WindowsPowerShell/v1.0/powershell.exe"
mkdir -p "$(dirname "$SYS32_PS")" "$T/stub" "$T/pathps" "$T/empty"
printf '#!/bin/sh\nexit 0\n' > "$SYS32_PS"
printf '#!/bin/sh\nexit 0\n' > "$T/pathps/powershell.exe"
cat > "$T/stub/wslpath" <<'EOF'
#!/bin/sh
[ "$1" = "-u" ] || exit 1
[ "$2" = 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe' ] || exit 1
printf '%s\n' "$FAKE_WIN/System32/WindowsPowerShell/v1.0/powershell.exe"
EOF
chmod +x "$SYS32_PS" "$T/pathps/powershell.exe" "$T/stub/wslpath"

# lookup <PATH> <FAKE_WIN> → "<rc>|<stdout>" of find_powershell in a clean shell.
lookup() {
    local out rc=0
    out="$(env PATH="$1" FAKE_WIN="$2" "$BASH_BIN" -c '. "$0"; find_powershell' "$HELPER")" || rc=$?
    printf '%s|%s' "$rc" "$out"
}

# --- PATH has no powershell.exe → the System32 fallback ---
assert_eq "$(lookup "$T/stub:/usr/bin:/bin" "$T/win")" "0|$SYS32_PS" \
    "no powershell.exe on PATH: finds the System32 fallback via wslpath"

# --- PATH has one → PATH wins (unchanged install_windows.sh behavior) ---
assert_eq "$(lookup "$T/pathps:$T/stub:/usr/bin:/bin" "$T/win")" "0|$T/pathps/powershell.exe" \
    "powershell.exe on PATH: uses the PATH copy"

# --- neither: wslpath maps to a path that does not exist → rc 1, no output ---
assert_eq "$(lookup "$T/stub:/usr/bin:/bin" "$T/nowhere")" "1|" \
    "no PATH copy and no System32 file: returns 1 with no output"

# --- not WSL at all (no wslpath): rc 1, no output ---
assert_eq "$(lookup "$T/empty" "$T/win")" "1|" \
    "no wslpath (not WSL): returns 1 with no output"

# --- sourcing is side-effect free: defines the function, prints nothing ---
assert_eq "$(env PATH="$T/empty" "$BASH_BIN" -c '. "$0"' "$HELPER")" "" \
    "sourcing the helper prints nothing"

# --- both consumers use the shared helper ---
NERD="$REPO_ROOT/sdk/gsl/scripts/install_nerd_font_linux.sh"
WIN="$REPO_ROOT/opt/bin/install_windows.sh"
assert_grep "install_nerd_font_linux.sh resolves powershell via find_powershell" \
    'find_powershell' "$NERD"
assert_grep "install_nerd_font_linux.sh sources opt/lib/winpowershell.sh" \
    'opt/lib/winpowershell\.sh' "$NERD"
# The only permitted bare PATH lookup is the documented helper-absent fallback stub.
bare="$(grep -n 'command -v powershell\.exe' "$NERD" | grep -v 'helper-absent fallback' || true)"
assert_eq "$bare" "" \
    "install_nerd_font_linux.sh no longer gates on bare 'command -v powershell.exe'"
assert_grep_negative "install_nerd_font_linux.sh invokes the resolved path, not bare powershell.exe" \
    '^[[:space:]]*powershell\.exe[[:space:]]' "$NERD"
assert_grep "install_windows.sh resolves powershell via find_powershell" \
    'find_powershell' "$WIN"
assert_grep_negative "install_windows.sh no longer carries its own System32 lookup" \
    'wslpath -u .C:' "$WIN"

_test_report
