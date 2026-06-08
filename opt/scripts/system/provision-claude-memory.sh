#!/usr/bin/env bash
# provision-claude-memory.sh — seed the repo's account-scoped Claude memories
# into THIS machine's live project-memory store (issue #134).
#
# Claude Code memory is PROJECT-scoped at ~/.claude/projects/<slug>/memory/,
# where <slug> is the absolute repo path with '/'->'-' (it embeds the username,
# so it differs per machine). There is no account-level memory, so "account
# scope" is SYNTHESIZED here: copy the canonical scope:account memories from the
# repo into the per-machine COMPUTED slug dir.
#
# Design: docs/mbo/designs/memory-provisioning.md. Mirrors apply-forced-settings.sh
# (complex provisioning in its own testable script, invoked by install_claude_skills.sh).
#
# Contract (pure function of env, no args):
#   BASE_DIR  repo root (default: derived from this script's location)
#   HOME      target home; CLAUDE_HOME defaults to $HOME/.claude
# Behaviour: seed-and-preserve — copy scope:account files into the live dir,
#   NEVER delete host-local files, skip a collision with a host-local file, and
#   regenerate MEMORY.md from the union (account index + host-local lines).
set -u

BASE_DIR="${BASE_DIR:-$(cd "$(dirname "$0")/../../.." && pwd -P)}"
CLAUDE_HOME="${CLAUDE_HOME:-${HOME}/.claude}"
REPO_MEM="$BASE_DIR/ai/claude/memory"

[ -d "$REPO_MEM" ] || exit 0   # nothing to provision

# --- F1: compute the live slug from the RESOLVED checkout (pwd -P), the way the
# session cwd resolves it. Never hardcode the slug or bake in $USER/$HOME. ---
REPO_ABS="$(cd "$BASE_DIR" && pwd -P)"
SLUG="$(printf '%s' "$REPO_ABS" | sed 's#/#-#g')"
PROJECTS="$CLAUDE_HOME/projects"
LIVE_DIR="$PROJECTS/$SLUG/memory"

# is_account FILE → true if the front-matter declares scope: account
is_account() { grep -qE '^[[:space:]]*scope:[[:space:]]*account([[:space:]]|$)' "$1" 2>/dev/null; }
# fm KEY FILE → value of a leading-block front-matter key (first match)
fm() { sed -n "s/^${1}:[[:space:]]*//p" "$2" 2>/dev/null | head -1; }

# --- UC-5: slug-mismatch signal. Pre-seeding a fresh machine is legitimate, so
# we always provision into the computed slug; we only WARN if the slug dir is
# absent yet a sibling project dir plausibly maps to this same repo basename
# (e.g. a symlinked/relocated checkout), so the user can verify. ---
if [ ! -d "$PROJECTS/$SLUG" ] && [ -d "$PROJECTS" ]; then
    base_seg="$(printf '%s' "$SLUG" | sed 's/.*-//')"
    for d in "$PROJECTS"/*-"$base_seg"; do
        [ -d "$d" ] || continue
        echo "  WARN: memory slug '$SLUG' not found under $PROJECTS, but '$(basename "$d")'" >&2
        echo "        shares this repo's name — if this checkout is reached via a symlink, verify" >&2
        echo "        memory landed where Claude reads it. Provisioning into the computed slug anyway." >&2
        break
    done
fi

mkdir -p "$LIVE_DIR"

# --- F2/F3/F4: copy scope:account files (seed-and-preserve). ---
for f in "$REPO_MEM"/*.md; do
    [ -e "$f" ] || continue
    b="$(basename "$f")"
    [ "$b" = "MEMORY.md" ] && continue        # the index is regenerated, not copied as a topic
    is_account "$f" || continue               # F2: only account-scoped files
    if [ -e "$LIVE_DIR/$b" ] && ! is_account "$LIVE_DIR/$b"; then
        # F4: a host-local file already owns this name — never clobber it.
        echo "  WARN: skipping '$b' — a host-local memory of the same name exists in $LIVE_DIR" >&2
        continue
    fi
    cp "$f" "$LIVE_DIR/$b"                     # repo wins for account files it ships (UC-3)
done

# --- F5: regenerate MEMORY.md from the UNION (repo account index + a line per
# surviving host-local file). Never blind-copy the repo index — that would drop
# host-only entries from session-start context. Deterministic => idempotent (F6). ---
tmp_index="$LIVE_DIR/.MEMORY.md.tmp.$$"
{
    [ -f "$REPO_MEM/MEMORY.md" ] && cat "$REPO_MEM/MEMORY.md"
    for f in "$LIVE_DIR"/*.md; do
        [ -e "$f" ] || continue
        b="$(basename "$f")"
        [ "$b" = "MEMORY.md" ] && continue
        [ -f "$REPO_MEM/$b" ] && continue      # an account file: already in the repo index above
        name="$(fm name "$f")"; [ -n "$name" ] || name="${b%.md}"
        desc="$(fm description "$f")"
        echo "- [$name]($b) — $desc  <!-- host-local -->"
    done
} > "$tmp_index" && mv "$tmp_index" "$LIVE_DIR/MEMORY.md"

exit 0
