#!/bin/bash
# Unified testing entry point for dotfiles
set -e

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IMAGE_NAME="dotfiles-test"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

function log() {
    echo -e "${BLUE}[TEST]${NC} $1"
}

function run_unit_tests() {
    log "Running Go Unit Tests with Coverage..."
    
    mkdir -p "$REPO_ROOT/coverage"
    
    # Dynamically find Go modules in src/
    modules=()
    while IFS= read -r line; do
        modules+=("$line")
    done < <(find "$REPO_ROOT/src" -name "go.mod" -exec dirname {} + | xargs -n1 basename | sort)
    
    if [ ${#modules[@]} -eq 0 ]; then
        log "No Go modules found in src/"
        return
    fi
    
    for mod in "${modules[@]}"; do
        log "Testing module: $mod"
        # Find the full path to the module
        mod_path=$(find "$REPO_ROOT/src" -name "$mod" -type d | head -n 1)
        (cd "$mod_path" && go test -coverprofile="$REPO_ROOT/coverage/$mod.out" ./...)
        (cd "$mod_path" && go tool cover -func="$REPO_ROOT/coverage/$mod.out" | tail -n 1)
    done
    
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
    docker run --privileged --rm "$IMAGE_NAME" /home/agent/git/dotfiles/ai/gemini/scripts/sanity_check.sh
    
    log "Verifying Version Commands..."
    docker run --privileged --rm "$IMAGE_NAME" bash -c "source ~/.profile && gss version && tmux-mgr version && wol version"

    log "Verifying Script PATH Discovery..."
    docker run --privileged --rm "$IMAGE_NAME" bash -c "source ~/.profile && which git_add.sh && which gemini_install.sh && which claude_install.sh"

    log "Verifying GSS Technical Guardrail..."
    # gss push must refuse without a HEAD-bound approval token. v1.0 reworded
    # the message (internal/approval) and pins exit code 22
    # (ExitApprovalTokenMissing). Assert BOTH the declared stable string and
    # the exit code, so a future reword or exit-code drift is caught.
    # See src/gss/docs/plan.md -> "Stable output strings".
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
    if docker run --privileged --rm "$IMAGE_NAME" bash -c "echo '{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"rm -rf *\"}}' | /home/agent/git/dotfiles/ai/claude/hooks/safety_guard.sh" 2>&1 | grep -q "BLOCKED by safety_guard"; then
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
