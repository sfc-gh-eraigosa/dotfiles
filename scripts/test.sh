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
    docker run --rm "$IMAGE_NAME" /home/agent/git/dotfiles/ai/gemini/scripts/sanity_check.sh
    
    log "Verifying GSS Technical Guardrail..."
    if docker run --rm "$IMAGE_NAME" gss push 2>&1 | grep -q "Missing or invalid AI approval token"; then
        log "GSS safeguard verified."
    else
        echo "FAIL: GSS safeguard failed to trigger!"
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
