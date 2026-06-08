#!/usr/bin/env bash
# install_ai_teams.sh — transform ai/teams/<team>/the_*.md persona files into native
# agent artifacts for Claude Code, Gemini CLI, Antigravity, and Ollama.
#
# Personas declare an abstract `tier:` in their YAML frontmatter; this script resolves
# the tier through ai/teams/model-map.yaml into each tool's concrete model + effort/
# temperature, assembles the system prompt from the `compose:` partial list, compiles a
# routing-grade `description`, and emits each tool's native format.
#
# Design: docs/mbo/specs/2026-06-01-ai-teams-install-design.md
# Mirrors install_gemini_skills.sh / install_claude_skills.sh. Idempotent; each tool
# emit is independent and degrades gracefully (warn + continue) when a tool is absent.
set -euo pipefail

# --- locate repo + sources -------------------------------------------------------------
BASE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TEAMS_DIR="${BASE_DIR}/ai/teams"
MODEL_MAP="${TEAMS_DIR}/model-map.yaml"
TEAMS_YAML="${TEAMS_DIR}/teams.yaml"
VALIDATE="${TEAMS_DIR}/validate.sh"

# Output roots. DEST_HOME lets tests redirect every tool dir under a temp root.
DEST_HOME="${TEAMS_DEST_HOME:-$HOME}"
CLAUDE_AGENTS="${DEST_HOME}/.claude/agents/teams"
GEMINI_AGENTS="${DEST_HOME}/.gemini/agents/teams"
ANTIGRAVITY_AGENTS="${DEST_HOME}/.config/antigravity/agents"
OLLAMA_DIR="${DEST_HOME}/.config/ollama/teams"

DRY_RUN=0
SKIP_OLLAMA_CREATE="${SKIP_OLLAMA_CREATE:-0}"
ONLY_TOOL=""

usage() {
  cat <<'EOF'
Usage: install_ai_teams.sh [--dry-run] [--tool claude|gemini|antigravity|ollama] [--skip-ollama-create]

Transforms ai/teams personas into native agents for all four tools.
  --dry-run             Print the plan; write nothing, run no ollama create.
  --tool <name>         Emit for only one tool.
  --skip-ollama-create  Generate Modelfiles but do not run `ollama create`.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=1 ;;
    --tool) ONLY_TOOL="${2:-}"; shift ;;
    --skip-ollama-create) SKIP_OLLAMA_CREATE=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "install_ai_teams: unknown arg '$1'" >&2; usage; exit 2 ;;
  esac
  shift
done

command -v yq >/dev/null 2>&1 || { echo "install_ai_teams: yq is required (run install_yq.sh)" >&2; exit 1; }

want_tool() { [ -z "$ONLY_TOOL" ] || [ "$ONLY_TOOL" = "$1" ]; }
log() { echo "  $*"; }
warn() { echo "  WARNING: $*" >&2; }

# --- frontmatter / body helpers --------------------------------------------------------
# Markdown files carry YAML frontmatter between the first two '---' fences, then the body.
_fm_end_line() { grep -n '^---[[:space:]]*$' "$1" | sed -n '2p' | cut -d: -f1; }

get_fm() {  # print the frontmatter YAML (without fences)
  local f="$1" end; end="$(_fm_end_line "$f")"
  [ -n "$end" ] || { warn "no frontmatter in $f"; return 1; }
  sed -n "2,$((end - 1))p" "$f"
}
get_body() {  # print everything after the closing fence, verbatim
  local f="$1" end; end="$(_fm_end_line "$f")"
  [ -n "$end" ] || return 1
  tail -n +"$((end + 1))" "$f"
}

# Resolve a model-map cell: tier, tool, field -> value
mm() { yq ".tiers.\"$1\".\"$2\".\"$3\"" "$MODEL_MAP"; }

# Assemble the composed system prompt from the persona's compose: list.
# $2 is the persona metadata as JSON (already extracted).
compose_prompt() {
  local f="$1" fm_data="$2" item body
  body="$(get_body "$f")"
  local out="" first=1
  while IFS= read -r item; do
    [ -n "$item" ] || continue
    [ $first -eq 1 ] || out+=$'\n\n'
    first=0
    if [ "$item" = "__body__" ]; then
      out+="$body"
    elif [ -f "${TEAMS_DIR}/${item}" ]; then
      out+="$(cat "${TEAMS_DIR}/${item}")"
    else
      warn "compose partial not found: ${item} (in $f)"
    fi
  done < <(echo "$fm_data" | yq -r '.compose[]' 2>/dev/null || true)
  # Default: body only, if compose was empty/absent.
  [ -n "$out" ] && printf '%s\n' "$out" || printf '%s\n' "$body"
}

# Compile the routing-grade description (unless the author set one explicitly).
# $2 is the persona metadata as JSON (already extracted).
compile_description() {
  local f="$1" fm_data="$2" team role domain globs kw uw aw desc
  desc="$(echo "$fm_data" | yq -r '.description // ""')"
  if [ -n "$desc" ] && [ "$desc" != "null" ]; then printf '%s' "$desc"; return; fi
  team="$(echo "$fm_data" | yq -r '.team')"; role="$(echo "$fm_data" | yq -r '.role')"
  domain="$(echo "$fm_data" | yq -r '.domain')"
  globs="$(echo "$fm_data" | yq -r '.file_globs | join(", ")')"
  kw="$(echo "$fm_data" | yq -r '.keywords | join(", ")')"
  uw="$(echo "$fm_data" | yq -r '.use_when')"; aw="$(echo "$fm_data" | yq -r '.avoid_when')"
  printf '[%s team · %s] %s. Use PROACTIVELY for: %s. Matches files: %s. Keywords: %s. Do NOT use for: %s.' \
    "$team" "$role" "$domain" "$uw" "$globs" "$kw" "$aw"
}

# Atomically write $2 to file $1 (deterministic content => idempotent).
atomic_write() {
  local dest="$1" content="$2"
  if [ "$DRY_RUN" -eq 1 ]; then log "[dry-run] would write ${dest/#$DEST_HOME/\~}"; return; fi
  mkdir -p "$(dirname "$dest")"
  local tmp; tmp="$(mktemp)"
  printf '%s' "$content" > "$tmp"
  mv "$tmp" "$dest"
}

# --- per-tool emitters -----------------------------------------------------------------
emit_claude() {
  local f="$1" team="$2" role="$3" tier="$4" name="${2}-${3}" desc="$5" prompt="$6" color="$7"
  local model effort
  model="$(mm "$tier" claude model)"; effort="$(mm "$tier" claude effort)"
  local expr='.name=strenv(name) | .description=strenv(desc) | .model=strenv(model) | .effort=strenv(effort)'
  # Only emit color when set — strenv would otherwise write a literal `color: "null"`.
  [ -n "$color" ] && [ "$color" != "null" ] && expr="${expr} | .color=strenv(color)"
  local fm
  fm="$(name="$name" desc="$desc" model="$model" effort="$effort" color="$color" \
        yq -n "$expr")"
  atomic_write "${CLAUDE_AGENTS}/${team}/${role}.md" "$(printf -- '---\n%s\n---\n\n%s\n' "$fm" "$prompt")"
}

emit_gemini() {
  local f="$1" team="$2" role="$3" tier="$4" name="${2}-${3}" desc="$5" prompt="$6"
  local model temp
  model="$(mm "$tier" gemini model)"; temp="$(mm "$tier" gemini temperature)"
  local fm
  fm="$(name="$name" desc="$desc" model="$model" temp="$temp" \
        yq -n '.name=strenv(name) | .description=strenv(desc) | .model=strenv(model) | .temperature=(strenv(temp)|from_yaml)')"
  atomic_write "${GEMINI_AGENTS}/${team}/${role}.md" "$(printf -- '---\n%s\n---\n\n%s\n' "$fm" "$prompt")"
}

emit_antigravity() {
  local team="$2" role="$3" tier="$4" name="${2}-${3}" desc="$5" prompt="$6"
  local model; model="$(mm "$tier" antigravity model)"
  # Pure YAML; instructions carries the full prompt as a block scalar (yq handles escaping).
  local doc
  doc="$(name="$name" desc="$desc" model="$model" instr="$prompt" \
        yq -n '.name=strenv(name) | .description=strenv(desc) | .model=strenv(model) | .instructions=strenv(instr)')"
  atomic_write "${ANTIGRAVITY_AGENTS}/${name}.yaml" "$doc"$'\n'
}

emit_ollama() {
  local team="$2" role="$3" tier="$4" name="teams-${2}-${3}" prompt="$6"
  local model num_ctx
  model="$(mm "$tier" ollama model)"; num_ctx="$(mm "$tier" ollama num_ctx)"
  local mf="${OLLAMA_DIR}/${team}/${role}.Modelfile"
  local content
  content="$(printf 'FROM %s\nPARAMETER num_ctx %s\nSYSTEM """\n%s\n"""\n' "$model" "$num_ctx" "$prompt")"
  atomic_write "$mf" "$content"
  if [ "$DRY_RUN" -eq 1 ] || [ "$SKIP_OLLAMA_CREATE" -eq 1 ]; then return; fi
  if command -v ollama >/dev/null 2>&1; then
    # Best-effort: base model may not be pulled. Never fail the install on this.
    if ! ollama create "$name" -f "$mf" >/dev/null 2>&1; then
      warn "ollama create $name failed (base model '$model' likely not pulled) — Modelfile still written"
    fi
  else
    log "ollama not installed — wrote Modelfile only ($name)"
  fi
}

# --- main ------------------------------------------------------------------------------
echo "Installing AI teams (claude/gemini/antigravity/ollama)..."

# Validate source before emitting anything (a failure aborts teams install only).
if [ -x "$VALIDATE" ]; then
  if ! "$VALIDATE"; then warn "validation failed — aborting teams install"; exit 1; fi
elif [ -f "$VALIDATE" ]; then
  if ! bash "$VALIDATE"; then warn "validation failed — aborting teams install"; exit 1; fi
fi

# Prune managed subtrees first so renamed/removed personas don't leave zombie agents.
# Only the dedicated teams/ dirs are cleared (wholly ours). Antigravity shares its agents/
# dir with possible user agents, so it is additive (overwrite-by-name, no prune); likewise
# `ollama` models created in the registry are not removed here.
if [ "$DRY_RUN" -ne 1 ]; then
  if want_tool claude; then rm -rf "${CLAUDE_AGENTS}"; fi
  if want_tool gemini; then rm -rf "${GEMINI_AGENTS}"; fi
  if want_tool ollama; then rm -rf "${OLLAMA_DIR}"; fi
fi

count=0
while IFS= read -r -d '' f; do
  [ -e "$f" ] || continue

  # Extract all needed fields in ONE yq call to avoid ~10 subprocesses per file
  fm_data="$(get_fm "$f" | yq -o=json '{
    "team": .team,
    "role": .role,
    "tier": .tier,
    "color": .color,
    "description": .description,
    "domain": .domain,
    "file_globs": .file_globs,
    "keywords": .keywords,
    "use_when": .use_when,
    "avoid_when": .avoid_when,
    "compose": .compose
  }' 2>/dev/null)"

  if [ -z "$fm_data" ]; then warn "skipping $f (frontmatter does not parse)"; continue; fi

  team="$(echo "$fm_data" | yq -r '.team')"; role="$(echo "$fm_data" | yq -r '.role')"
  tier="$(echo "$fm_data" | yq -r '.tier')"
  color="$(echo "$fm_data" | yq -r '.color')"

  if [ -z "$team" ] || [ "$team" = "null" ] || [ -z "$role" ] || [ "$role" = "null" ]; then
    warn "skipping $f (missing team/role)"; continue
  fi

  desc="$(compile_description "$f" "$fm_data")"
  prompt="$(compose_prompt "$f" "$fm_data")"

  want_tool claude      && emit_claude      "$f" "$team" "$role" "$tier" "$desc" "$prompt" "$color"
  want_tool gemini      && emit_gemini      "$f" "$team" "$role" "$tier" "$desc" "$prompt"
  want_tool antigravity && emit_antigravity "$f" "$team" "$role" "$tier" "$desc" "$prompt"
  want_tool ollama      && emit_ollama      "$f" "$team" "$role" "$tier" "$desc" "$prompt"
  count=$((count + 1))
done < <(find "$TEAMS_DIR" -mindepth 2 -maxdepth 2 -name 'the_*.md' -print0 | sort -z)

suffix=""; [ "$DRY_RUN" -eq 1 ] && suffix=" (dry-run)"
echo "AI teams install complete: ${count} personas processed${suffix}."
