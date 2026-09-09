#!/usr/bin/env bash
# render-agy-plugin.sh <repo-root> <plugin-dir>
#
# Render the repo's Claude Code slash commands (ai/claude/commands/*.md) and
# scope:account memories (ai/claude/memory/*.md) into a LOCAL Antigravity CLI
# plugin so agy gets the same /commands and always-on notes Claude gets:
#
#   <plugin-dir>/plugin.json            manifest (name "dotfiles")
#   <plugin-dir>/commands/<name>.toml   one per command: description + prompt
#   <plugin-dir>/rules/AGENTS.md        one "## <title>" section per account memory
#
# The Markdown stays the single source; this is a pure function of the two
# repo dirs (deterministic, idempotent, stale TOMLs removed on re-render).
# Invoked by install_antigravity_skills.sh, which also enables the plugin in
# ~/.gemini/config/config.json. Design: docs/mbo/designs/agy-parity.md (5–6).
#
# Body transforms (Claude-only syntax agy does not understand):
#   `!`cmd``  (Claude shell injection line)  ->  "Run `cmd` first and use its output."
#   $ARGUMENTS                                ->  prose (agy substitutes no token;
#                                                 it appends the user's input)
#   TOML basic-string escaping for the prompt: \ -> \\, """ -> ""\"
#
# bash 3.2 + POSIX awk/sed only (macOS-safe); no jq needed.
set -u

REPO="${1:-}"
DEST="${2:-}"
if [ -z "$REPO" ] || [ -z "$DEST" ]; then
    echo "usage: render-agy-plugin.sh <repo-root> <plugin-dir>" >&2
    exit 1
fi
CMDS="$REPO/ai/claude/commands"
MEM="$REPO/ai/claude/memory"
if [ ! -d "$CMDS" ]; then
    echo "render-agy-plugin: commands dir not found: $CMDS" >&2
    exit 1
fi

mkdir -p "$DEST/commands" "$DEST/rules"
rm -f "$DEST"/commands/*.toml

cat > "$DEST/plugin.json" <<'JSON'
{
  "name": "dotfiles",
  "version": "1",
  "description": "dotfiles repo slash commands + account memories (rendered by install_antigravity_skills.sh from ai/claude/commands and ai/claude/memory; do not edit by hand)"
}
JSON

# fm_value KEY FILE -> value of a top-of-file YAML frontmatter key ("" if none).
fm_value() {
    awk -v key="$1" '
        NR == 1 && $0 != "---" { exit }
        /^---$/ { fence++; if (fence == 2) exit; next }
        fence == 1 && index($0, key ":") == 1 {
            sub("^" key ":[[:space:]]*", ""); print; exit
        }' "$2"
}

# body FILE -> the file without its leading frontmatter block.
body() {
    awk '
        NR == 1 && $0 != "---" { keep = 1 }
        !keep && /^---$/ { fence++; if (fence == 2) { keep = 1 }; next }
        keep' "$1"
}

# toml_basic STRING -> escaped for a TOML basic string ("...").
toml_basic() { printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }

# --- commands ---
for f in "$CMDS"/*.md; do
    [ -e "$f" ] || continue
    name="$(basename "$f" .md)"
    desc="$(fm_value description "$f")"
    [ -n "$desc" ] || desc="dotfiles /$name command"
    {
        printf 'description = "%s"\n' "$(toml_basic "$desc")"
        printf 'prompt = """\n'
        # shellcheck disable=SC2016  # sed script: single quotes are deliberate, no expansion wanted
        body "$f" \
            | sed -e 's/\\/\\\\/g' \
                  -e 's/"""/""\\"/g' \
                  -e 's/^!`\(.*\)`[[:space:]]*$/Run `\1` first and use its output./' \
                  -e 's/\$ARGUMENTS/the arguments the user passed to this command/g'
        printf '"""\n'
    } > "$DEST/commands/$name.toml"
done

# --- rules (account memories) ---
{
    echo "# dotfiles account memories"
    echo
    echo "Always-on notes rendered from the dotfiles repo's account-scoped Claude memories"
    echo "(ai/claude/memory, scope: account). Treat them as standing guidance for this workspace."
    if [ -d "$MEM" ]; then
        for f in "$MEM"/*.md; do
            [ -e "$f" ] || continue
            case "$(basename "$f")" in MEMORY.md) continue ;; esac
            grep -qE '^[[:space:]]*scope:[[:space:]]*account([[:space:]]|$)' "$f" || continue
            title="$(fm_value title "$f")"
            [ -n "$title" ] || title="$(body "$f" | sed -n 's/^# //p' | head -1)"
            [ -n "$title" ] || title="$(fm_value name "$f" | tr '-' ' ')"
            [ -n "$title" ] || title="$(basename "$f" .md | tr '-' ' ')"
            echo
            echo "## $title"
            echo
            body "$f" | sed '/^# /d'
        done
    fi
} > "$DEST/rules/AGENTS.md"

exit 0
