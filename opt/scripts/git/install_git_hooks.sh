#!/usr/bin/env bash
# install_git_hooks.sh — make the privacy git hooks GLOBAL on this machine.
#
# Copies ai/githooks/{pre-commit,commit-msg,pre-push} (+ their shared plumbing
# and the rule library from ai/hooks) into ~/.config/git/hooks and points
# core.hooksPath there, so every repo on the box — not only this one — judges
# what it records and publishes. The hooks chain to any repo-local
# .git/hooks/<name>, so a global path does not switch off husky/lefthook.
#
# Copy, not symlink (CLAUDE.md -> Shell & Dotfiles Conventions): a worktree
# checkout must never become the live hook path. Idempotent. Never clobbers a
# core.hooksPath that already points somewhere else — it warns instead, since
# that is a deliberate choice this script cannot see the reason for.
#
# Usage: opt/scripts/git/install_git_hooks.sh
set -u

BASE_DIR="$(cd "$(dirname "$0")/../../.." && pwd)"
SRC_HOOKS="$BASE_DIR/ai/githooks"
SRC_RULES="$BASE_DIR/ai/hooks/privacy_rules.sh"
DEST="${XDG_CONFIG_HOME:-$HOME/.config}/git/hooks"

for f in "$SRC_HOOKS/pre-commit" "$SRC_HOOKS/commit-msg" "$SRC_HOOKS/pre-push" "$SRC_HOOKS/_privacy_common.sh" "$SRC_RULES"; do
    if [ ! -f "$f" ]; then
        echo "install_git_hooks: missing $f" >&2
        exit 1
    fi
done

mkdir -p "$DEST"
for f in "$SRC_HOOKS/pre-commit" "$SRC_HOOKS/commit-msg" "$SRC_HOOKS/pre-push" "$SRC_HOOKS/_privacy_common.sh" "$SRC_RULES"; do
    cp "$f" "$DEST/$(basename "$f")"
done
chmod +x "$DEST/pre-commit" "$DEST/commit-msg" "$DEST/pre-push"

current="$(git config --global core.hooksPath 2>/dev/null || true)"
if [ -z "$current" ]; then
    git config --global core.hooksPath "$DEST"
    echo "  git hooks: installed to $DEST and set as global core.hooksPath"
elif [ "$current" = "$DEST" ]; then
    echo "  git hooks: refreshed in $DEST (core.hooksPath already set)"
else
    echo "  git hooks: copied to $DEST, but global core.hooksPath is '$current' — left untouched." >&2
    echo "  To activate: git config --global core.hooksPath \"$DEST\" (and chain your existing hooks from there)." >&2
fi
