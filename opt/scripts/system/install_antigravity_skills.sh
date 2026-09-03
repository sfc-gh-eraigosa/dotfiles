#!/usr/bin/env bash
# install_antigravity_skills.sh - Idempotently install Antigravity CLI (agy)
# assistant-specific config: lifecycle hooks, shell aliases, and legacy
# Gemini CLI cleanup. Skill links are handled by sync-skills.sh (the
# canonical linker for BOTH assistants); this script only does
# antigravity-specific wiring — mirroring install_claude_skills.sh.
#
# Antigravity CLI layout (verified against agy 1.0.16):
#   ~/.gemini/antigravity-cli/settings.json   CLI-owned settings (not managed here)
#   ~/.gemini/config/                         global customization root
#     ├── skills/                             (populated by sync-skills.sh)
#     ├── hooks/                              guard scripts (copied below)
#     └── hooks.json                          hook wiring (rendered below)

set -e

# Determine the root of the dotfiles repository
BASE_DIR="$(cd "$(dirname "$0")/../../.." && pwd)"

echo "Configuring Antigravity CLI..."

AGY_CONFIG_ROOT="${HOME}/.gemini/config"

# Remove symlinks in $1 (depth 1) that point into this repo checkout.
# Relative link targets are resolved against the directory before matching so
# relative repo-pointing links are cleaned too.
remove_repo_links() {
    local dir="$1"
    [ -d "$dir" ] || return 0
    find "$dir" -maxdepth 1 -type l | while read -r link; do
        target="$(readlink "$link")"
        case "$target" in
            /*) ;;
            *) target="$dir/$target" ;;
        esac
        case "$target" in
            "$BASE_DIR"/*) rm -f "$link" ;;
        esac
    done
}

# --- Hooks (COPIED into ~/.gemini/config/hooks; hooks.json references the
# well-known $HOME path). Copy, not symlink, per the provisioning directive
# (CLAUDE.md -> Shell & Dotfiles Conventions). safety_guard.sh loads
# strip_heredocs.awk from its own dir, so the .awk sibling is copied too.
# antigravity_adapter.sh translates agy's hook dialect to the shared guard
# contract. Test drivers (*_test.sh) stay in the repo and are not copied. ---
if [ -d "$BASE_DIR/ai/hooks" ]; then
    echo "  Copying hooks into ~/.gemini/config/hooks..."
    HOOKS_DEST="${AGY_CONFIG_ROOT}/hooks"
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

# --- hooks.json (repo `guards` entry rendered from the template and MERGED
# over the host file) ---
# agy keeps hook wiring in a dedicated hooks.json keyed by hook NAME. The repo
# owns exactly one name, "guards"; other tools own theirs (herdr's agent
# state-reporting integration adds a "herdr" entry). So the file is merged,
# not overwritten: the rendered "guards" object replaces the host's "guards"
# wholesale (arrays replaced, so a stale matcher list cannot linger) and every
# other named hook is preserved (design: docs/mbo/designs/agy-parity.md unit 4).
# An unparseable host file is set aside as hooks.json.invalid and recreated.
# __HOME__ is substituted because agy does not expand env vars in hook command
# strings; the replacement escapes sed's metacharacters (& | \) so an unusual
# $HOME cannot corrupt the file. Guard scripts are referenced by bare name —
# the adapter resolves them relative to its own directory.
HOOKS_JSON_SRC="$BASE_DIR/ai/antigravity/hooks.json.template"
if [ -f "$HOOKS_JSON_SRC" ]; then
    echo "  Rendering ~/.gemini/config/hooks.json (guards entry, merged)..."
    mkdir -p "$AGY_CONFIG_ROOT"
    HOOKS_JSON_DEST="$AGY_CONFIG_ROOT/hooks.json"
    home_escaped="$(printf '%s' "$HOME" | sed -e 's/[&|\\]/\\&/g')"
    hooks_rendered="$(mktemp "${TMPDIR:-/tmp}/agy-hooks.XXXXXX")"
    sed "s|__HOME__|${home_escaped}|g" "$HOOKS_JSON_SRC" > "$hooks_rendered"
    if command -v jq > /dev/null 2>&1; then
        if [ -f "$HOOKS_JSON_DEST" ] && ! jq -e . "$HOOKS_JSON_DEST" > /dev/null 2>&1; then
            echo "    WARNING: existing hooks.json is not valid JSON; set aside as hooks.json.invalid" >&2
            mv "$HOOKS_JSON_DEST" "$HOOKS_JSON_DEST.invalid"
        fi
        if [ -f "$HOOKS_JSON_DEST" ]; then
            hooks_merged="$(mktemp "${TMPDIR:-/tmp}/agy-hooks-merged.XXXXXX")"
            # `*` deep-merges objects; the rendered right operand wins for
            # "guards" (its arrays replace the host's), other names survive.
            if jq -s '.[0] * .[1]' "$HOOKS_JSON_DEST" "$hooks_rendered" > "$hooks_merged" && [ -s "$hooks_merged" ]; then
                cat "$hooks_merged" > "$HOOKS_JSON_DEST"
            else
                echo "    WARNING: hooks.json merge failed; host file left unchanged" >&2
            fi
            rm -f "$hooks_merged"
        else
            cat "$hooks_rendered" > "$HOOKS_JSON_DEST"
        fi
    else
        # jq-less fallback: overwrite (foreign hooks cannot be preserved).
        echo "    WARNING: jq not installed — hooks.json overwritten, foreign hook entries not preserved" >&2
        cat "$hooks_rendered" > "$HOOKS_JSON_DEST"
    fi
    rm -f "$hooks_rendered"
fi

# --- Shell aliases (~/.config/antigravity/aliases.sh, sourced by .zshrc and
# .bashrc). Provides the agy() wrapper with tmux auto-anchor support.
# COPIED, not symlinked, per the provisioning directive (root AGENTS.md:
# copy is the forward mechanism; a symlink would couple the shell config to
# this checkout's location and break under gss worktrees/CI). ---
AGY_XDG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/antigravity"
if [ -f "$BASE_DIR/ai/antigravity/aliases.sh" ]; then
    echo "  Installing antigravity aliases.sh..."
    mkdir -p "$AGY_XDG_DIR"
    if [ -L "$AGY_XDG_DIR/aliases.sh" ]; then
        rm -f "$AGY_XDG_DIR/aliases.sh"
    elif [ -e "$AGY_XDG_DIR/aliases.sh" ] \
         && ! cmp -s "$BASE_DIR/ai/antigravity/aliases.sh" "$AGY_XDG_DIR/aliases.sh" \
         && [ ! -e "$AGY_XDG_DIR/aliases.sh.bak" ]; then
        echo "    Backing up existing aliases.sh -> aliases.sh.bak"
        cp "$AGY_XDG_DIR/aliases.sh" "$AGY_XDG_DIR/aliases.sh.bak"
    fi
    cp "$BASE_DIR/ai/antigravity/aliases.sh" "$AGY_XDG_DIR/aliases.sh"
fi

# --- statusline-command.sh (shim for gsl status line) ---
# Point agy's statusLine settings to a decoupled shim in ~/.gemini/config/.
if [ -f "$BASE_DIR/ai/claude/statusline-command.sh" ]; then
    echo "  Installing statusline shim for agy..."
    mkdir -p "$AGY_CONFIG_ROOT"
    cp "$BASE_DIR/ai/claude/statusline-command.sh" "$AGY_CONFIG_ROOT/statusline-command.sh"
    chmod +x "$AGY_CONFIG_ROOT/statusline-command.sh"
fi

# --- settings.json (host-owned real file; seeded from the tracked template on
# first run, forced subset merged on every run) ---
# Mirrors install_claude_skills.sh: the host OWNS this file (trustedWorkspaces,
# colorScheme, …). A NEW host is seeded from settings.json.template (its
# self-documenting "_comment" key is dropped at seed time); an EXISTING file is
# never re-seeded. settings.forced.json (statusLine + permissions deny/ask,
# allow unioned) is deep-merged over it every run by apply-forced-settings.sh.
# Design: docs/mbo/designs/agy-parity.md (units 2–3).
AGY_SETTINGS_DEST="${HOME}/.gemini/antigravity-cli/settings.json"
AGY_SETTINGS_TEMPLATE="$BASE_DIR/ai/antigravity/settings.json.template"
AGY_SETTINGS_FORCED="$BASE_DIR/ai/antigravity/settings.forced.json"
APPLY_FORCED="$BASE_DIR/opt/scripts/system/apply-forced-settings.sh"

if [ -f "$AGY_SETTINGS_FORCED" ]; then
    echo "  Configuring statusLine in ~/.gemini/antigravity-cli/settings.json..."
    mkdir -p "$(dirname "$AGY_SETTINGS_DEST")"
    
    # Back up the settings file once if it exists and is a real file
    if [ -f "$AGY_SETTINGS_DEST" ] && [ ! -L "$AGY_SETTINGS_DEST" ] && [ ! -e "$AGY_SETTINGS_DEST.bak" ]; then
        echo "    Backing up existing settings.json -> settings.json.bak"
        cp "$AGY_SETTINGS_DEST" "$AGY_SETTINGS_DEST.bak"
    fi
    
    # Seed a NEW host from the template (empty object if the template or jq is
    # missing, so the forced merge below still has a valid file to work on).
    if [ ! -f "$AGY_SETTINGS_DEST" ]; then
        if [ -f "$AGY_SETTINGS_TEMPLATE" ] && command -v jq > /dev/null 2>&1; then
            echo "    Seeding settings.json from template (first run)"
            jq 'del(._comment)' "$AGY_SETTINGS_TEMPLATE" > "$AGY_SETTINGS_DEST"
        else
            echo '{}' > "$AGY_SETTINGS_DEST"
        fi
    fi
    
    # Deep-merge the forced settings
    if command -v jq > /dev/null 2>&1; then
        bash "$APPLY_FORCED" "$AGY_SETTINGS_DEST" "$AGY_SETTINGS_FORCED"
    else
        echo "    WARNING: jq not installed — cannot merge forced settings; statusLine wiring may be stale" >&2
    fi
fi

# --- Legacy Gemini CLI cleanup (one-time migration, idempotent) ---
# Gemini CLI was retired 2026-06-18. Remove the repo-managed artifacts it
# left behind; leave host-owned files (settings.json, oauth creds, history)
# alone — Antigravity reuses ~/.gemini and they don't conflict.
echo "  Cleaning up retired Gemini CLI artifacts..."
# 1. Policy/command symlinks that pointed into this repo
for legacy_dir in "${HOME}/.gemini/policies" "${HOME}/.gemini/commands"; do
    remove_repo_links "$legacy_dir"
    rmdir "$legacy_dir" 2>/dev/null || true
done
# 2. Gemini's hook copies (agy reads ~/.gemini/config/hooks instead)
if [ -d "${HOME}/.gemini/hooks" ]; then
    rm -rf "${HOME}/.gemini/hooks"
fi
# 3. Gemini aliases dir (the agy() wrapper lives in ~/.config/antigravity now)
LEGACY_GEMINI_XDG="${XDG_CONFIG_HOME:-$HOME/.config}/gemini"
if [ -L "$LEGACY_GEMINI_XDG/aliases.sh" ]; then
    rm -f "$LEGACY_GEMINI_XDG/aliases.sh"
    rmdir "$LEGACY_GEMINI_XDG" 2>/dev/null || true
fi
# 4. Gemini CLI's global skills root (~/.agents/skills): remove only links
#    that point into this repo; sync-skills.sh targets ~/.gemini/config/skills.
remove_repo_links "${HOME}/.agents/skills"
rmdir "${HOME}/.agents/skills" "${HOME}/.agents" 2>/dev/null || true

echo "Antigravity CLI Configuration complete."
