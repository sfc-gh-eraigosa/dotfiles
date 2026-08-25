#!/usr/bin/env bash
# Unified testing entry point for dotfiles
set -e

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IMAGE_NAME="dotfiles-test"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# -----------------------------------------------------------------------------
# Per-module coverage thresholds (issue #46 phase 3)
# -----------------------------------------------------------------------------
# Each Go module under src/ and sdk/ has a minimum line-coverage floor enforced
# by run_unit_tests. Plan defaults (per the issue): gss=70, tmux-mgr=60,
# gsl=60, wol=60. Where a module is currently BELOW the planned floor we
# document the gap in .ci-baseline-issues.md and either lower the gate
# with a TODO or accept red CI as a backlog signal.
#
# A module that does not appear in this map is exempt (a warning is
# printed, not a failure) — this lets new modules land before their test
# suite has stabilised.
# -----------------------------------------------------------------------------
# The thresholds below are the TARGET floors. tmux-mgr and wol are currently
# UNDER their floor (48.8% and 55.6% respectively at the time this gate
# landed); the gap is documented in .ci-baseline-issues.md and tracked by
# backfill issues #50 (tmux-mgr) and #51 (wol).
#
# WARN-ONLY until backfill lands: the coverage gate does not fail the build
# while COVERAGE_ENFORCE=0 (the current default). A module under its floor
# prints a WARN line so the backlog stays visible, but CI stays green so the
# merge queue isn't deadlocked by the pre-existing gap. Once #50/#51 raise
# tmux-mgr and wol above 60%, flip the default below to 1 (or set
# COVERAGE_ENFORCE=1 in the CI job) so the gate becomes strict. Real test
# failures (a failing `go test`) are ALWAYS hard regardless of this flag —
# only the threshold comparison is softened.
# Per-module minimum coverage % — a function (not a bash-4 associative array) so
# it runs under macOS system bash 3.2. Empty output = no threshold configured.
coverage_min() {
    case "$1" in
        gss)      echo 70 ;;
        tmux-mgr) echo 60 ;;
        gsl)      echo 60 ;;
        wol)      echo 60 ;;
        wlink)    echo 60 ;;
        fleet)    echo 60 ;;
        libs)     echo 80 ;;
        *)        echo "" ;;
    esac
}

# 1 = a module under its COVERAGE_MIN floor fails the build; 0 = warn only.
# Default warn-only until the backfill PRs (#50/#51) clear the gap.
COVERAGE_ENFORCE="${COVERAGE_ENFORCE:-0}"

function log() {
    echo -e "${BLUE}[TEST]${NC} $1"
}

# Parse the `total:` line emitted by `go tool cover -func=...`. Returns an
# integer percentage (floor of the float). Empty / malformed input → 0.
# `go tool cover -func` must run from inside the module directory because
# it resolves package paths against the local go.mod — hence the mod_path arg.
function coverage_total_pct() {
    local mod_path="$1"
    local profile="$2"
    [ -s "$profile" ] || { echo "0"; return; }
    (cd "$mod_path" && go tool cover -func="$profile" 2>/dev/null) \
        | awk '/^total:/ { for (i=1; i<=NF; i++) if ($i ~ /%$/) { sub("%", "", $i); printf "%d", $i + 0; exit } }'
}

function run_unit_tests() {
    log "Running Go Unit Tests with Coverage..."

    mkdir -p "$REPO_ROOT/coverage"

    # Dynamically find Go modules in src/ and sdk/
    modules=()
    while IFS= read -r line; do
        modules+=("$line")
    done < <(find "$REPO_ROOT/sdk" -name "go.mod" -exec dirname {} + 2>/dev/null | xargs -n1 basename | sort)

    if [ ${#modules[@]} -eq 0 ]; then
        log "No Go modules found in src/"
        return
    fi

    # Track coverage failures across the whole run so the developer sees
    # the full picture (don't bail on the first module under threshold).
    local coverage_failures=()

    for mod in "${modules[@]}"; do
        log "Testing module: $mod"
        # Find the full path to the module (search both src/ and sdk/)
        mod_path=$(find "$REPO_ROOT/sdk" -path "*/$mod/go.mod" -exec dirname {} \; 2>/dev/null | head -n 1)
        (cd "$mod_path" && go test -coverprofile="$REPO_ROOT/coverage/$mod.out" ./...)
        (cd "$mod_path" && go tool cover -func="$REPO_ROOT/coverage/$mod.out" | tail -n 1)

        # Emit HTML coverage report alongside the .out profile (CI uploads
        # the whole coverage/ directory as an artifact).
        (cd "$mod_path" && go tool cover \
            -html="$REPO_ROOT/coverage/$mod.out" \
            -o   "$REPO_ROOT/coverage/$mod.html") || true

        # Threshold gate. Modules without an entry in COVERAGE_MIN are
        # warned (not failed) so new modules can land before their suite
        # is mature enough to gate on.
        local pct
        pct=$(coverage_total_pct "$mod_path" "$REPO_ROOT/coverage/$mod.out")
        local min; min="$(coverage_min "$mod")"
        if [ -n "$min" ]; then
            if [ "$pct" -lt "$min" ]; then
                if [ "$COVERAGE_ENFORCE" = "1" ]; then
                    echo -e "${RED}FAIL${NC}: coverage for ${mod} is ${pct}% (minimum: ${min}%)"
                else
                    echo -e "${YELLOW}WARN${NC}: coverage for ${mod} is ${pct}% (below floor ${min}%, warn-only — COVERAGE_ENFORCE=0)"
                fi
                coverage_failures+=("$mod=${pct}%/min=${min}%")
            else
                echo -e "${GREEN}OK${NC}: coverage for ${mod} is ${pct}% (minimum: ${min}%)"
            fi
        else
            echo -e "${YELLOW}WARN${NC}: no coverage threshold configured for module '${mod}' (current: ${pct}%) — add it to coverage_min() in scripts/test.sh"
        fi
    done

    if [ "${#coverage_failures[@]}" -gt 0 ]; then
        echo
        if [ "$COVERAGE_ENFORCE" = "1" ]; then
            echo -e "${RED}Coverage gate FAILED for ${#coverage_failures[@]} module(s):${NC}"
            for entry in "${coverage_failures[@]}"; do
                echo "  - $entry"
            done
            exit 1
        fi
        echo -e "${YELLOW}Coverage below floor for ${#coverage_failures[@]} module(s) (warn-only, COVERAGE_ENFORCE=0):${NC}"
        for entry in "${coverage_failures[@]}"; do
            echo "  - $entry"
        done
        echo -e "${YELLOW}Not failing the build. Set COVERAGE_ENFORCE=1 once #50/#51 land to enforce.${NC}"
    fi

    echo -e "${GREEN}Unit tests passed!${NC}"
}

function run_integration_tests() {
    if ! docker image inspect "$IMAGE_NAME" >/dev/null 2>&1; then
        log "Building Docker image for integration tests..."
        docker build -t "$IMAGE_NAME" "$REPO_ROOT"
    else
        log "Docker image $IMAGE_NAME already exists, skipping build. Use 'docker rmi $IMAGE_NAME' to force rebuild."
    fi
    
    log "Running System Sanity Check inside container..."
    docker run --privileged --rm "$IMAGE_NAME" /home/agent/git/dotfiles/ai/antigravity/scripts/sanity_check.sh
    
    log "Verifying Version Commands..."
    docker run --privileged --rm "$IMAGE_NAME" bash -c "source ~/.profile && gss version && tmux-mgr version && wol version"

    log "Verifying Script PATH Discovery..."
    docker run --privileged --rm "$IMAGE_NAME" bash -c "source ~/.profile && command -v git_add.sh && command -v antigravity_install.sh && command -v claude_install.sh"

    log "Verifying GSS Technical Guardrail..."
    # gss push must refuse without a HEAD-bound approval token. v1.0 reworded
    # the message (internal/approval) and pins exit code 22
    # (ExitApprovalTokenMissing). Assert BOTH the declared stable string and
    # the exit code, so a future reword or exit-code drift is caught.
    # See sdk/gss/docs/plan.md -> "Stable output strings".
    # `gss push` exits non-zero by design here; bracket with `set +e` so the
    # intentional refusal doesn't trip the script-level `set -e` (the original
    # survived only because its failing command sat inside an `if` pipeline).
    # Run INSIDE the repo: v1.0's approval check resolves HEAD before reading
    # the token, so cwd must be a real git repo. (The pre-v1.0 code checked the
    # token first and reported "missing" from anywhere, so running from $HOME
    # passed by accident — it never touched git.)
    set +e
    guard_out=$(docker run --privileged --rm "$IMAGE_NAME" bash -c "source ~/.profile && cd ~/git/dotfiles && gss push" 2>&1)
    guard_rc=$?
    set -e
    if echo "$guard_out" | grep -q "missing or unreadable approval token" && [ "$guard_rc" -eq 22 ]; then
        log "GSS safeguard verified (refused without token, exit ${guard_rc})."
    else
        echo "FAIL: GSS safeguard failed to trigger! (exit=${guard_rc})"
        echo "$guard_out"
        exit 1
    fi

    log "Verifying gss resolves to the binary in interactive zsh (no shadowing alias)..."
    # oh-my-zsh's git plugin defines `alias gss='git status -s'`. Our .zshrc
    # unaliases it so the binary wins. This test catches accidental regression.
    if docker run --privileged --rm "$IMAGE_NAME" zsh -i -c 'gss version' 2>&1 | grep -q "Git Safe Sync"; then
        log "gss binary takes precedence over alias in zsh."
    else
        echo "FAIL: gss in interactive zsh is not resolving to the binary (likely shadowed by oh-my-zsh alias)!"
        exit 1
    fi

    log "Verifying Claude Code installation..."
    docker run --privileged --rm "$IMAGE_NAME" bash -c "source ~/.profile && claude --version"

    log "Running Claude Sanity Check..."
    docker run --privileged --rm "$IMAGE_NAME" bash -c "source ~/.profile && /home/agent/git/dotfiles/ai/claude/scripts/sanity_check.sh"

    log "Verifying Claude safety_guard hook (blocks rm -rf *)..."
    if docker run --privileged --rm "$IMAGE_NAME" bash -c "echo '{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"rm -rf *\"}}' | /home/agent/git/dotfiles/ai/hooks/safety_guard.sh" 2>&1 | grep -q "BLOCKED by safety_guard"; then
        log "Claude safety_guard verified."
    else
        echo "FAIL: Claude safety_guard hook failed to block 'rm -rf *'!"
        exit 1
    fi

    echo -e "${GREEN}Integration tests passed!${NC}"
}

# Parse arguments
MODE=${1:-"all"}

case "$MODE" in
    "unit")
        run_unit_tests
        ;;
    "integration")
        run_integration_tests
        ;;
    "all")
        run_unit_tests
        run_integration_tests
        ;;
    *)
        echo "Usage: ./test.sh [unit|integration|all]"
        exit 1
        ;;
esac

echo -e "\n${GREEN}ALL TESTS PASSED SUCCESSFULLY${NC}"
