#!/usr/bin/env bash
# Test driver for opt/scripts/system/google-cli-setup.sh
#
# Focuses on load_node_env() — the regression fixed in PR #43 was
# silently shadowing fnm's current Node by pre-pending nvm-latest.
# Run: bash opt/scripts/system/google-cli-setup_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/google-cli-setup.sh"

# === 1. Syntax check ===
assert_exit_code 0 "google-cli-setup.sh parses with bash -n" \
    bash -n "$SCRIPT"

# === 2. Source-safe ===
# Sourcing must not run setup_all / install_gcloud — load_node_env should
# just be defined. We source in a subshell so this driver doesn't inherit
# `set -e` or other state from the script.
assert_in_subshell "source-safe (no side effects when sourced)" \
    "( . '$SCRIPT' && type load_node_env >/dev/null )"

# === 3. load_node_env() — early-return when npm already on PATH ===
# Regression PR #43 fixed: load_node_env was pre-pending nvm-latest even
# when npm was already on PATH, shadowing fnm's current node. Assert that
# when npm is mockable on PATH, the function does NOT add nvm paths.
TMPDIR_TEST="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_TEST"' EXIT
cat > "${TMPDIR_TEST}/npm" <<'EOF'
#!/usr/bin/env bash
# mock: just need to satisfy command -v npm; npm config get prefix → user-local.
if [ "${1:-}" = "config" ] && [ "${2:-}" = "get" ] && [ "${3:-}" = "prefix" ]; then
    echo "$HOME/.local"
fi
exit 0
EOF
chmod +x "${TMPDIR_TEST}/npm"

OUT=$(
    PATH="${TMPDIR_TEST}:/usr/bin:/bin" \
    HOME="/nonexistent-home-for-test" \
    bash -c "
        set +e
        . '$SCRIPT'
        load_node_env >/dev/null 2>&1
        echo \"PATH=\$PATH\"
    "
)
# When npm is already on PATH the function must NOT add a ~/.nvm/versions/node
# path. We deliberately point HOME at a nonexistent path so any nvm lookup
# would be obvious in the resulting PATH.
case "$OUT" in
    *"/nonexistent-home-for-test/.nvm/versions/node/"*)
        echo "FAIL: load_node_env added nvm-latest even though npm was on PATH"
        FAIL=$((FAIL + 1))
        ;;
    *)
        echo "PASS: load_node_env skips nvm-latest when npm is already on PATH"
        PASS=$((PASS + 1))
        ;;
esac

# === 4. Discovery-order source check ===
# When npm is NOT on PATH, fnm should be tried first, then nodenv, then nvm.
# Verifying the runtime order by mocking three managers gets brittle (each
# returns its own shape of `eval`-able output). Instead, assert the source
# order: fnm-env check appears before the nodenv check, which appears
# before the nvm-latest discovery.
FNM_LINE=$(grep -n 'fnm/fnm' "$SCRIPT" | head -1 | cut -d: -f1)
NODENV_LINE=$(grep -n '\.nodenv' "$SCRIPT" | head -1 | cut -d: -f1)
NVM_LINE=$(grep -n '\.nvm/versions/node' "$SCRIPT" | head -1 | cut -d: -f1)

if [ -n "$FNM_LINE" ] && [ -n "$NODENV_LINE" ] && [ -n "$NVM_LINE" ] \
   && [ "$FNM_LINE" -lt "$NODENV_LINE" ] && [ "$NODENV_LINE" -lt "$NVM_LINE" ]; then
    echo "PASS: discovery order is fnm > nodenv > nvm-latest"
    PASS=$((PASS + 1))
else
    echo "FAIL: discovery order in source is not fnm > nodenv > nvm-latest (fnm=$FNM_LINE nodenv=$NODENV_LINE nvm=$NVM_LINE)"
    FAIL=$((FAIL + 1))
fi

# Guard: the nvm-latest path must be inside the `if ! command -v npm` block
# (PR #43 regression). grep is line-oriented, so check with awk that the
# `.nvm/versions/node` reference sits after the opening guard and before
# the matching `fi` that closes the if-block.
GUARD_OK=$(awk '
    /if[[:space:]]+![[:space:]]+command[[:space:]]+-v[[:space:]]+npm/ { in_guard=1; depth=1; next }
    in_guard && /^[[:space:]]*if[[:space:]]/ { depth++ }
    in_guard && /^[[:space:]]*fi[[:space:]]*$/ { depth--; if (depth==0) { in_guard=0 } }
    in_guard && /\.nvm\/versions\/node/ { print "ok"; exit }
' "$SCRIPT")
assert_eq "$GUARD_OK" "ok" "nvm-latest reference is inside the 'if ! command -v npm' guard"

_test_report
