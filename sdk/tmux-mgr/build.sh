#!/bin/bash
set -e

DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="${HOME}/opt/bin"
# Shared skill destinations (mirrors opt/scripts/system/sync-skills.sh):
# Antigravity CLI + Claude Code both read their own skills root.
SKILL_INSTALL_DIRS=("${HOME}/.gemini/config/skills" "${HOME}/.claude/skills")

# Drop any inherited GOROOT/GOTOOLCHAIN so the resolved `go` binary uses its
# own matching stdlib (prevents brew-go vs goenv-go version-mismatch).
unset GOROOT GOTOOLCHAIN

# Prefer goenv-managed go (respects repo-root .go-version); fall back to PATH.
if command -v goenv >/dev/null 2>&1; then
    GO_BIN="$(goenv which go 2>/dev/null || command -v go || true)"
else
    GO_BIN="$(command -v go || true)"
fi
if [ -z "$GO_BIN" ]; then
    echo "WARNING: 'go' command not found. Skipping tmux-mgr build."
    echo "Install Go (https://golang.org/doc/install) and run this script again to enable tmux-mgr."
    exit 0
fi

# Version comes from the module's git release tag (sdk/tmux-mgr/vX.Y.Z) —
# the single source of truth. See sdk/version.sh for why the old VERSION
# file was removed (it had to be committed to a protected branch).
# shellcheck source=sdk/version.sh
. "$DIR/../version.sh"
VERSION="$(sdk_version "tmux-mgr" "$DIR")"
COMMIT=$(git -C "$DIR" rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
DIRTY="false"
if ! git -C "$DIR" diff --quiet 2>/dev/null; then
    DIRTY="true"
fi

LDFLAGS="-X github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/cmd.Version=$VERSION \
         -X github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/cmd.Commit=$COMMIT \
         -X github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/cmd.BuildDate=$DATE \
         -X github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/cmd.Dirty=$DIRTY"

mkdir -p "$BIN_DIR"

# Guard: pkg/workspace was deleted in PR-59 (gss owns worktrees). Fail the
# build if it creeps back rather than silently re-coupling tmux-mgr to it.
"$DIR/scripts/no-workspace-guard.sh"

echo "Building tmux-mgr v$VERSION with $($GO_BIN version)..."
cd "$DIR"
"$GO_BIN" build -ldflags "$LDFLAGS" -o "$BIN_DIR/tmux-mgr" main.go

echo "Installing tmux skill..."
for skill_dir in "${SKILL_INSTALL_DIRS[@]}"; do
    mkdir -p "$skill_dir"
    ln -sfn "$DIR/skill" "$skill_dir/tmux"
done

echo "tmux-mgr built and skill installed."
