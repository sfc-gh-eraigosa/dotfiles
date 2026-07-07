#!/usr/bin/env bash
# gemini_teardown.sh — consent-based cleanup of retired Gemini CLI leftovers.
#
# Google retired Gemini CLI for individual accounts on 2026-06-18; this repo
# migrated to the Antigravity CLI (agy). References for the turn-down:
#   - https://developers.googleblog.com/an-important-update-transitioning-gemini-cli-to-antigravity-cli/
#   - https://antigravity.google (replacement: Antigravity CLI, binary `agy`)
#   - Migration PR: https://github.com/sfc-gh-eraigosa/dotfiles/pull/153
#
# install.sh runs this after antigravity_install.sh. When leftover Gemini CLI
# artifacts are DISCOVERED it asks before removing them:
#   [y] clean up now
#   [n] not now — ask again on the next install.sh run
#   [k] keep forever — writes a marker; never asks again
#
# Flags (for unattended use):
#   --yes    clean up without prompting
#   --keep   write the keep-forever marker and exit
#   --reset  remove the keep-forever marker (re-enables the prompt)
#
# Non-interactive runs (no TTY, e.g. Docker/CI) never prompt and never
# remove anything — they print a one-line note and exit 0.
#
# What is cleaned up (only what is found):
#   - the @google/gemini-cli npm package / `gemini` binary
#   - ~/.config/gemini/aliases.sh (the gemini()/gemini-yolo() wrappers)
#   - ~/.gemini.profile + its source lines in real (non-symlink) rc files
# NEVER touched: ~/.gemini itself — Antigravity reuses it (settings under
# ~/.gemini/antigravity-cli/, customization root ~/.gemini/config/).

set -u

KEEP_MARKER="${XDG_CONFIG_HOME:-$HOME/.config}/antigravity/gemini-keep"

MODE="prompt"
case "${1:-}" in
    --yes) MODE="yes" ;;
    --keep)
        mkdir -p "$(dirname "$KEEP_MARKER")"
        printf 'Gemini CLI leftovers kept by user choice on %s. Delete this file (or run gemini_teardown.sh --reset) to be asked again.\n' \
            "$(date -u +%Y-%m-%d)" > "$KEEP_MARKER"
        echo "gemini_teardown: keep-forever marker written ($KEEP_MARKER)."
        exit 0 ;;
    --reset)
        rm -f "$KEEP_MARKER"
        echo "gemini_teardown: marker removed; the next install.sh run will ask again."
        exit 0 ;;
    "") ;;
    *) echo "Usage: gemini_teardown.sh [--yes|--keep|--reset]" >&2; exit 2 ;;
esac

# Respect an earlier "keep forever" decision.
if [ -f "$KEEP_MARKER" ] && [ "$MODE" != "yes" ]; then
    exit 0
fi

# --- Discovery -------------------------------------------------------------
FOUND=()

GEMINI_BIN="$(command -v gemini 2>/dev/null || true)"
[ -n "$GEMINI_BIN" ] && FOUND+=("retired Gemini CLI binary on PATH: $GEMINI_BIN")

NPM_HAS_GEMINI=0
if command -v npm >/dev/null 2>&1 && npm ls -g @google/gemini-cli >/dev/null 2>&1; then
    NPM_HAS_GEMINI=1
    [ -z "$GEMINI_BIN" ] && FOUND+=("retired npm package: @google/gemini-cli")
fi

LEGACY_ALIASES="${XDG_CONFIG_HOME:-$HOME/.config}/gemini/aliases.sh"
[ -e "$LEGACY_ALIASES" ] && FOUND+=("legacy shell wrappers (gemini()/gemini-yolo()): $LEGACY_ALIASES")

[ -f "$HOME/.gemini.profile" ] && FOUND+=("legacy environment profile: ~/.gemini.profile")

RC_HITS=()
for rc in "$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.profile"; do
    # Repo-managed symlinked profiles are updated by the repo itself; only
    # real host-owned files carry stale lines the old installer appended.
    if [ -f "$rc" ] && [ ! -L "$rc" ] && grep -q '\.gemini\.profile\|\.config/gemini/aliases\.sh' "$rc" 2>/dev/null; then
        RC_HITS+=("$rc")
        FOUND+=("stale Gemini source line(s) in: $rc")
    fi
done

if [ "${#FOUND[@]}" -eq 0 ]; then
    exit 0
fi

# --- Consent ---------------------------------------------------------------
show_findings() {
    echo ""
    echo "Retired Gemini CLI leftovers detected:"
    for f in "${FOUND[@]}"; do
        echo "  - $f"
    done
    echo ""
    echo "  Gemini CLI stopped serving individual accounts on 2026-06-18 and was"
    echo "  replaced by the Antigravity CLI (agy), which this repo now installs."
    echo "  References:"
    echo "    https://developers.googleblog.com/an-important-update-transitioning-gemini-cli-to-antigravity-cli/"
    echo "    https://github.com/sfc-gh-eraigosa/dotfiles/pull/153"
    echo "  (~/.gemini itself is kept either way — Antigravity reuses it.)"
    echo ""
}

if [ "$MODE" = "prompt" ]; then
    if [ ! -t 0 ]; then
        echo "gemini_teardown: retired Gemini CLI leftovers found; run 'gemini_teardown.sh' interactively (or --yes / --keep) to resolve."
        exit 0
    fi
    show_findings
    printf 'Clean these up? [y] yes  [n] not now (ask again)  [k] keep forever (never ask): '
    read -r answer
    case "$answer" in
        [Yy]*) ;;
        [Kk]*)
            mkdir -p "$(dirname "$KEEP_MARKER")"
            printf 'Gemini CLI leftovers kept by user choice on %s. Delete this file (or run gemini_teardown.sh --reset) to be asked again.\n' \
                "$(date -u +%Y-%m-%d)" > "$KEEP_MARKER"
            echo "Keeping Gemini leftovers; you will not be asked again ($KEEP_MARKER)."
            exit 0 ;;
        *)
            echo "Skipping for now; install.sh will ask again next run."
            exit 0 ;;
    esac
else
    show_findings
    echo "gemini_teardown: --yes given; cleaning up."
fi

# --- Cleanup ---------------------------------------------------------------
if [ "$NPM_HAS_GEMINI" = "1" ]; then
    echo "  Removing @google/gemini-cli (npm)..."
    npm uninstall -g @google/gemini-cli || true
fi
# A gemini binary that survives the npm uninstall (or was never npm-managed)
# only gets removed from user-owned locations; system installs are reported.
GEMINI_BIN="$(command -v gemini 2>/dev/null || true)"
if [ -n "$GEMINI_BIN" ]; then
    case "$GEMINI_BIN" in
        "$HOME"/*)
            echo "  Removing $GEMINI_BIN..."
            rm -f "$GEMINI_BIN" ;;
        *)
            echo "  NOTE: $GEMINI_BIN is outside \$HOME — remove it with your system package manager." ;;
    esac
fi

if [ -e "$LEGACY_ALIASES" ]; then
    echo "  Removing $LEGACY_ALIASES..."
    rm -f "$LEGACY_ALIASES" "$LEGACY_ALIASES.bak"
    rmdir "$(dirname "$LEGACY_ALIASES")" 2>/dev/null || true
fi

if [ -f "$HOME/.gemini.profile" ]; then
    echo "  Removing ~/.gemini.profile..."
    rm -f "$HOME/.gemini.profile"
fi

for rc in "${RC_HITS[@]+"${RC_HITS[@]}"}"; do
    echo "  Removing stale Gemini source lines from $rc..."
    sed -i.bak '/# Load Gemini CLI environment/d;/\.gemini\.profile/d;/# Gemini CLI helpers/d;/\.config\/gemini\/aliases\.sh/d' "$rc"
done

echo "gemini_teardown: done. Restart your shell (e.g. 'exec zsh') so already-defined gemini()/gemini-yolo() functions disappear from open sessions."
