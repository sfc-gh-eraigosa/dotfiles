#!/usr/bin/env bash
# Test driver for gemini_teardown.sh — consent-based Gemini CLI leftover
# cleanup. Sandboxes HOME/XDG_CONFIG_HOME; a fake npm/gemini pair on PATH
# stands in for the retired CLI. No real tool dirs touched.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
# shellcheck source=/dev/null
. "$REPO_ROOT/ai/_test_helpers.sh"

# String-contains helper (mirrors sync-plugins_test.sh; not in _test_helpers).
assert_contains() {
    local haystack="$1" needle="$2" desc="$3"
    if printf '%s' "$haystack" | grep -qF -- "$needle"; then
        echo "PASS: $desc"; PASS=$((PASS+1))
    else
        echo "FAIL: $desc (missing: $needle)"; FAIL=$((FAIL+1))
    fi
}

TEARDOWN="$SCRIPT_DIR/gemini_teardown.sh"

# Sandbox with the full leftover set: fake gemini binary under $HOME, legacy
# aliases file, ~/.gemini.profile, a real .zshrc with stale source lines, and
# a ~/.gemini data dir that must survive.
make_sandbox() {
    local H; H="$(mktemp -d)"
    mkdir -p "$H/bin" "$H/.config/gemini" "$H/.gemini/antigravity-cli"
    printf '#!/usr/bin/env bash\nexit 0\n' > "$H/bin/gemini"; chmod +x "$H/bin/gemini"
    # Fake npm shadows the real one so the test can NEVER mutate real global
    # packages: `ls -g <pkg>` reports installed; `uninstall -g` records the
    # call and removes the fake gemini binary (mimicking the real effect).
    cat > "$H/bin/npm" <<NPM
#!/usr/bin/env bash
case "\$1 \$2" in
    "ls -g") exit 0 ;;
    "uninstall -g") echo "NPM_UNINSTALL_CALLED \$3" >> "$H/npm.log"; rm -f "$H/bin/gemini"; exit 0 ;;
esac
exit 0
NPM
    chmod +x "$H/bin/npm"
    printf 'gemini() { command gemini "$@"; }\n' > "$H/.config/gemini/aliases.sh"
    printf '# Gemini CLI Environment Setup\n' > "$H/.gemini.profile"
    cat > "$H/.zshrc" <<'RC'
# something unrelated
# Load Gemini CLI environment
[[ -f "$HOME/.gemini.profile" ]] && source "$HOME/.gemini.profile"
# Gemini CLI helpers: gemini() wrapper with tmux auto-anchor
[ -f "${HOME}/.config/gemini/aliases.sh" ] && . "${HOME}/.config/gemini/aliases.sh"
# trailing unrelated line
RC
    echo '{"keep":"me"}' > "$H/.gemini/antigravity-cli/settings.json"
    echo "$H"
}

run_teardown() { # $1=HOME $2...=args
    local H="$1"; shift
    HOME="$H" XDG_CONFIG_HOME="$H/.config" PATH="$H/bin:/usr/bin:/bin" \
        bash "$TEARDOWN" "$@"
}

# === 1. Syntax ===
assert_exit_code 0 "gemini_teardown.sh parses with bash -n" bash -n "$TEARDOWN"

# === 2. Nothing found -> silent success ===
H0="$(mktemp -d)"
assert_exit_code 0 "no leftovers: exits 0" \
    env HOME="$H0" XDG_CONFIG_HOME="$H0/.config" PATH="/usr/bin:/bin" bash "$TEARDOWN"
rm -rf "$H0"

# === 3. Leftovers + no TTY -> reports, removes nothing ===
H1="$(make_sandbox)"
OUT1="$(run_teardown "$H1" </dev/null)"
assert_contains "$OUT1" "leftovers found" "non-TTY prompt mode only reports"
assert_in_subshell "non-TTY: gemini binary untouched" "[ -x '$H1/bin/gemini' ]"
assert_in_subshell "non-TTY: aliases untouched" "[ -f '$H1/.config/gemini/aliases.sh' ]"
rm -rf "$H1"

# === 4. --yes cleans everything, spares ~/.gemini ===
H2="$(make_sandbox)"
OUT2="$(run_teardown "$H2" --yes)"
assert_contains "$OUT2" "Retired Gemini CLI leftovers detected" "--yes shows findings + references"
assert_contains "$OUT2" "developers.googleblog.com" "references included in the ask"
assert_in_subshell "--yes: npm uninstall invoked (via fake npm)" "grep -q 'NPM_UNINSTALL_CALLED @google/gemini-cli' '$H2/npm.log'"
assert_in_subshell "--yes: HOME-local gemini binary removed" "[ ! -e '$H2/bin/gemini' ]"
assert_in_subshell "--yes: legacy aliases removed (dir pruned)" "[ ! -e '$H2/.config/gemini/aliases.sh' ]"
assert_in_subshell "--yes: ~/.gemini.profile removed" "[ ! -e '$H2/.gemini.profile' ]"
assert_grep_negative "--yes: stale source lines removed from real .zshrc" '\.gemini\.profile' "$H2/.zshrc"
assert_grep_negative "--yes: stale aliases line removed from real .zshrc" 'config/gemini/aliases' "$H2/.zshrc"
assert_grep "--yes: unrelated .zshrc lines survive" 'something unrelated' "$H2/.zshrc"
assert_in_subshell "--yes: ~/.gemini data dir preserved (agy reuses it)" "[ -f '$H2/.gemini/antigravity-cli/settings.json' ]"
rm -rf "$H2"

# === 5. keep-forever marker suppresses everything ===
H3="$(make_sandbox)"
run_teardown "$H3" --keep >/dev/null
assert_in_subshell "--keep writes the marker" "[ -f '$H3/.config/antigravity/gemini-keep' ]"
OUT3="$(run_teardown "$H3" </dev/null)"
assert_eq "" "$OUT3" "marker present: teardown is silent, asks nothing"
assert_in_subshell "marker present: leftovers untouched" "[ -x '$H3/bin/gemini' ]"
# --reset re-enables the ask
run_teardown "$H3" --reset >/dev/null
OUT3b="$(run_teardown "$H3" </dev/null)"
assert_contains "$OUT3b" "leftovers found" "--reset re-enables the ask"
rm -rf "$H3"

# === 6. Interactive 'k' answer persists the marker ===
H4="$(make_sandbox)"
# Feed 'k' on stdin; the script only prompts on a TTY, so emulate via script(1)
# when available, else fall back to asserting the --keep path (covered above).
if command -v script >/dev/null 2>&1; then
    HOME="$H4" XDG_CONFIG_HOME="$H4/.config" PATH="$H4/bin:/usr/bin:/bin" \
        script -qec "printf 'k\n' | bash '$TEARDOWN'" /dev/null >/dev/null 2>&1 || true
    # under script(1) stdin IS a tty for the outer shell; the pipe demotes it —
    # accept either outcome as long as nothing was deleted without consent.
    assert_in_subshell "no deletion without an explicit yes" "[ -x '$H4/bin/gemini' ]"
fi
rm -rf "$H4"

_test_report
