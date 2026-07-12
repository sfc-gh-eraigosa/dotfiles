#!/bin/bash
# install_claude_skills.sh - Idempotently install Claude Code's assistant-specific
# config: settings.json, slash commands, hooks, and shell aliases.
#
# Mirrors install_antigravity_skills.sh (which handles Antigravity's hooks/aliases).
# Skill linking is NOT done here — sync-skills.sh is the single canonical linker that
# discovers every SKILL.md and links it into BOTH ~/.gemini/config/skills (Antigravity) and
# ~/.claude/skills (Claude). Run sync-skills.sh (or the `sync-skills` alias) to refresh
# skills; this script only owns the Claude-specific config below.

set -e

BASE_DIR="$(cd "$(dirname "$0")/../../.." && pwd)"

echo "Configuring Claude Code..."

CLAUDE_HOME="${HOME}/.claude"
mkdir -p "$CLAUDE_HOME/commands"

cleanup_broken_links() {
    local dir="$1"
    if [ -d "$dir" ]; then
        find "$dir" -type l ! -exec test -e {} \; -delete 2>/dev/null || true
    fi
}

# --- settings.json (host-owned real file; forced subset merged on every run) ---
# The host OWNS ~/.claude/settings.json: it keeps host-local fields
# (apiKeyHelper, ANTHROPIC_BASE_URL, enabledPlugins, theme, ...). On first run
# we seed it from the tracked template; on EVERY run we deep-merge the immutable
# subset (settings.forced.json: hooks, statusLine, security deny/ask) over it,
# so the security wiring stays current without clobbering host customizations.
# No symlink and no repo-internal host copy (provisioning directive — CLAUDE.md).
# See docs/mbo/designs/2026-06-02-ai-config-home-provisioning.md (D2) and
# apply-forced-settings.sh.
SETTINGS_DEST="$CLAUDE_HOME/settings.json"
SETTINGS_TEMPLATE="$BASE_DIR/ai/claude/settings.json.template"
SETTINGS_FORCED="$BASE_DIR/ai/claude/settings.forced.json"
APPLY_FORCED="$BASE_DIR/opt/scripts/system/apply-forced-settings.sh"
# Back up a pre-existing real host settings.json ONCE before we first mutate it,
# so a host's pre-migration config is always recoverable (.bak is not churned on
# re-run). A legacy symlink needs no backup — its target lives in the repo.
if [ -f "$SETTINGS_DEST" ] && [ ! -L "$SETTINGS_DEST" ] && [ ! -e "$SETTINGS_DEST.bak" ]; then
    echo "  Backing up existing settings.json -> settings.json.bak"
    cp "$SETTINGS_DEST" "$SETTINGS_DEST.bak"
fi
# Migrate a legacy symlinked settings.json into a real host-owned file,
# preserving whatever the host had configured (the merge re-applies wiring).
if [ -L "$SETTINGS_DEST" ]; then
    echo "  Migrating legacy settings.json symlink to a host-owned file"
    # Portable replacement for `readlink -f` (GNU-only; absent on macOS/BSD):
    # plain `readlink` (BSD+GNU) reads the link, then anchor a relative target
    # to the symlink's physical dir via `cd ... && pwd -P`.
    legacy_link="$(readlink "$SETTINGS_DEST" 2>/dev/null || true)"
    case "$legacy_link" in
        "") legacy_target="" ;;
        /*) legacy_target="$legacy_link" ;;
        *)  legacy_target="$(cd -- "$(dirname -- "$SETTINGS_DEST")" >/dev/null 2>&1 && pwd -P)/$legacy_link" ;;
    esac
    rm -f "$SETTINGS_DEST"
    if [ -n "$legacy_target" ] && [ -f "$legacy_target" ]; then
        cp "$legacy_target" "$SETTINGS_DEST"
    fi
fi
if [ ! -f "$SETTINGS_DEST" ] && [ -f "$SETTINGS_TEMPLATE" ]; then
    echo "  Seeding ~/.claude/settings.json from template (first run)"
    cp "$SETTINGS_TEMPLATE" "$SETTINGS_DEST"
fi
if [ -f "$SETTINGS_DEST" ] && [ -f "$SETTINGS_FORCED" ]; then
    if command -v jq > /dev/null 2>&1; then
        echo "  Applying forced settings subset (hooks, statusLine, deny/ask)"
        bash "$APPLY_FORCED" "$SETTINGS_DEST" "$SETTINGS_FORCED"
    else
        echo "  WARNING: jq not installed — cannot merge forced settings; hook wiring may be stale" >&2
    fi
fi

# --- Commands (.md slash commands) ---
if [ -d "$BASE_DIR/ai/claude/commands" ]; then
    echo "  Setting up custom slash commands..."
    for cmd in "$BASE_DIR/ai/claude/commands"/*.md; do
        [ -e "$cmd" ] || continue
        cmd_name=$(basename "$cmd")
        echo "    Linking $cmd_name"
        ln -sf "$cmd" "$CLAUDE_HOME/commands/$cmd_name"
    done
fi

cleanup_broken_links "$CLAUDE_HOME/commands"

# --- Skills ---
# Handled by sync-skills.sh, which links every discovered SKILL.md into
# ~/.claude/skills (and ~/.gemini/config/skills for Antigravity). Nothing to do here.

# --- Hooks (COPIED into ~/.claude/hooks; settings.json references the
# well-known $HOME path). Copy, not symlink, per the provisioning directive
# (CLAUDE.md -> Shell & Dotfiles Conventions). safety_guard.sh loads
# strip_heredocs.awk from its own directory, so the .awk sibling is copied too.
# Test drivers (*_test.sh) stay in the repo and are not copied. ---
if [ -d "$BASE_DIR/ai/hooks" ]; then
    HOOKS_DEST="$CLAUDE_HOME/hooks"
    mkdir -p "$HOOKS_DEST"
    for f in "$BASE_DIR/ai/hooks"/*.sh "$BASE_DIR/ai/hooks"/*.awk; do
        [ -e "$f" ] || continue
        case "$(basename "$f")" in
            *_test.sh) continue ;;
        esac
        cp "$f" "$HOOKS_DEST/$(basename "$f")"
    done
    chmod +x "$HOOKS_DEST"/*.sh 2>/dev/null || true
fi

# --- Account memories (issue #134): seed the repo's scope:account Claude
# memories into this machine's live project-memory store
# (~/.claude/projects/<computed-slug>/memory). Delegated to a standalone,
# tested provisioner (mirrors apply-forced-settings.sh): seed-and-preserve,
# never clobbers host-local memories, regenerates the index from the union. ---
if [ -f "$BASE_DIR/opt/scripts/system/provision-claude-memory.sh" ]; then
    echo "  Provisioning Claude account memories (~/.claude/projects/<slug>/memory)"
    env BASE_DIR="$BASE_DIR" CLAUDE_HOME="$CLAUDE_HOME" \
        bash "$BASE_DIR/opt/scripts/system/provision-claude-memory.sh" || true
fi

# --- Shell aliases (~/.config/claude/aliases.sh, sourced by .zshrc and .bashrc) ---
# Mirrors the tmux-mgr convention of ~/.config/<tool>/aliases.sh. The state
# file (yolo.enabled) lives in the same directory.
if [ -f "$BASE_DIR/ai/claude/aliases.sh" ]; then
    CLAUDE_XDG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/claude"
    mkdir -p "$CLAUDE_XDG_DIR"
    if [ -e "$CLAUDE_XDG_DIR/aliases.sh" ] && [ ! -L "$CLAUDE_XDG_DIR/aliases.sh" ]; then
        echo "    Backing up existing aliases.sh -> aliases.sh.bak"
        mv "$CLAUDE_XDG_DIR/aliases.sh" "$CLAUDE_XDG_DIR/aliases.sh.bak"
    fi
    ln -sf "$BASE_DIR/ai/claude/aliases.sh" "$CLAUDE_XDG_DIR/aliases.sh"
fi

# --- statusline-command.sh (shim for gsl status line) ---
# The settings.json template points statusLine.command at ~/.claude/statusline-command.sh.
# COPY the repo shim into that well-known $HOME path — do not symlink it.
#
# Per CLAUDE.md ("AI-tool config provisioning"), copy is the forward mechanism and
# new repo-pointing symlinks must not be introduced. The old `ln -sf` was actively
# dangerous here: running the installer from a `gss feature` worktree pointed
# ~/.claude/statusline-command.sh at a TRANSIENT worktree path, so the user's global
# status line broke the moment that worktree was cleaned up. install_antigravity_skills.sh
# already copies this same shim (cp + chmod +x); this brings Claude to parity.
if [ -f "$BASE_DIR/ai/claude/statusline-command.sh" ]; then
    if [ -L "$CLAUDE_HOME/statusline-command.sh" ]; then
        # Legacy symlink (possibly into a stale checkout). Drop it so `install`
        # writes a fresh regular file instead of following it back into the repo.
        echo "  Replacing legacy statusline-command.sh symlink with a copy"
        rm -f "$CLAUDE_HOME/statusline-command.sh"
    elif [ -e "$CLAUDE_HOME/statusline-command.sh" ] &&
        ! cmp -s "$BASE_DIR/ai/claude/statusline-command.sh" "$CLAUDE_HOME/statusline-command.sh"; then
        # A different real file is present (hand-edited, or from an older revision).
        # Back it up once; an identical copy is left alone so re-runs stay idempotent.
        echo "  Backing up existing statusline-command.sh -> statusline-command.sh.bak"
        mv "$CLAUDE_HOME/statusline-command.sh" "$CLAUDE_HOME/statusline-command.sh.bak"
    fi
    install -m 0755 "$BASE_DIR/ai/claude/statusline-command.sh" "$CLAUDE_HOME/statusline-command.sh"
fi

echo "Claude Code configuration complete."
