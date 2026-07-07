#!/usr/bin/env bash
# strip-agy-rc-appends.sh — remove agy's non-portable PATH append from
# repo-managed shell profiles.
#
# The Antigravity CLI bootstrapper (`agy install`, also run by self-updates)
# appends this block to every rc file it finds:
#
#     # Added by Antigravity CLI installer
#     export PATH="/home/<user>/.local/bin:$PATH"
#
# On our hosts ~/.zshrc, ~/.bashrc and ~/.profile are SYMLINKS into the
# dotfiles repo, so the append lands in the repo working tree with a
# hardcoded, machine-specific $HOME — unportable and never committable. The
# repo profiles already export ~/.local/bin portably (opt/profiles/.profile
# and .zshrc), so the appended block is pure redundancy: strip it (plus the
# blank lines the installer adds above it) from any rc file that is a
# symlink. Host-owned REAL rc files are left alone — there the block may be
# the only thing putting agy on PATH, and portability is moot.
#
# Idempotent and safe to run any time (antigravity_install.sh runs it after
# the bootstrapper; rerun it whenever an agy self-update re-appends).

set -u

MARKER='# Added by Antigravity CLI installer'
stripped=0

for rc in "$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.profile"; do
    [ -L "$rc" ] || continue
    grep -qF "$MARKER" "$rc" 2>/dev/null || continue
    echo "  Stripping agy's PATH append from repo-managed $(basename "$rc")..."
    tmp="$(mktemp)"
    # Drop the marker line and the export line after it, then trim blank
    # lines left dangling at EOF (the installer prepends two).
    awk -v marker="$MARKER" '
        $0 == marker { skip = 2 }
        skip > 0     { skip--; next }
        { lines[++n] = $0 }
        END {
            while (n > 0 && lines[n] == "") n--
            for (i = 1; i <= n; i++) print lines[i]
        }
    ' "$rc" > "$tmp"
    # cat-through preserves the symlink (a mv would replace it with a file).
    cat "$tmp" > "$rc"
    rm -f "$tmp"
    stripped=1
done

if [ "$stripped" = "1" ]; then
    echo "  (repo profiles already export ~/.local/bin portably, so nothing is lost)"
fi
exit 0
