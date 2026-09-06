#!/usr/bin/env bash
# validate.sh — zero-dependency (yq-only) validator for the ai/teams source of truth.
#
# Asserts the persona frontmatter, model-map.yaml, and teams.yaml are internally
# consistent BEFORE the installer emits anything. Run at install time and in CI.
# Exits non-zero (with a list of problems) on any failure.
set -uo pipefail

TEAMS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODEL_MAP="${TEAMS_DIR}/model-map.yaml"
TEAMS_YAML="${TEAMS_DIR}/teams.yaml"

command -v yq >/dev/null 2>&1 || { echo "validate: yq is required" >&2; exit 1; }

ERRORS=0
err() { echo "  ✗ $*" >&2; ERRORS=$((ERRORS + 1)); }
ok()  { :; }

ALLOWED_TIERS="fast standard think deep-think"
ALLOWED_COLORS="red blue green yellow purple orange pink cyan"
TOOLS="claude antigravity ollama"

# frontmatter helpers (mirror install_ai_teams.sh)
_fm_end() { grep -n '^---[[:space:]]*$' "$1" | sed -n '2p' | cut -d: -f1; }
get_fm() { local end; end="$(_fm_end "$1")"; [ -n "$end" ] && sed -n "2,$((end - 1))p" "$1"; }

in_list() { case " $2 " in *" $1 "*) return 0;; *) return 1;; esac; }

echo "Validating ai/teams source..."

# --- model-map.yaml --------------------------------------------------------------------
if ! yq '.' "$MODEL_MAP" >/dev/null 2>&1; then
  err "model-map.yaml does not parse"
else
  # Use a single yq pass to check tiers and tools
  tiers_json="$(yq -o=json '.tiers' "$MODEL_MAP")"
  for t in $ALLOWED_TIERS; do
    if [ "$(echo "$tiers_json" | yq "has(\"$t\")")" != "true" ]; then
      err "model-map: missing tier '$t'"
      continue
    fi
    tier_data="$(echo "$tiers_json" | yq ".\"$t\"")"
    for tool in $TOOLS; do
      if [ "$(echo "$tier_data" | yq "has(\"$tool\")")" = "true" ]; then
        [ "$(echo "$tier_data" | yq ".\"$tool\".model")" != "null" ] \
          || err "model-map: tier '$t' tool '$tool' missing 'model'"
      else
        err "model-map: tier '$t' missing tool '$tool'"
      fi
    done
  done
fi

# --- teams.yaml ------------------------------------------------------------------------
if ! yq '.' "$TEAMS_YAML" >/dev/null 2>&1; then
  err "teams.yaml does not parse"
else
  # Pre-cache teams and squads to avoid repeated yq calls
  teams_list="$(yq -r '.teams | keys | .[]' "$TEAMS_YAML")"
  squad_members="$(yq -r '.squads[].members[]' "$TEAMS_YAML" 2>/dev/null)"

  while IFS= read -r team; do
    [ -n "$team" ] || continue
    if [ ! -d "${TEAMS_DIR}/${team}" ]; then
      err "teams.yaml: team '$team' has no folder ai/teams/${team}/"
    elif [ -z "$(find "${TEAMS_DIR}/${team}" -maxdepth 1 -name 'the_*.md' -print -quit)" ]; then
      err "teams.yaml: team '$team' folder has no the_*.md personas"
    fi
  done <<< "$teams_list"
fi

# --- persona files ---------------------------------------------------------------------
# Pre-calculate all valid team/role pairs for squad validation
ALL_AGENTS=""
persona_count=0

while IFS= read -r -d '' f; do
  [ -e "$f" ] || continue
  persona_count=$((persona_count + 1))
  rel="${f#"$TEAMS_DIR"/}"
  folder_team="$(dirname "$rel")"

  if [ -z "$(_fm_end "$f")" ]; then err "$rel: no YAML frontmatter"; continue; fi

  # Extract all needed fields in ONE yq call to avoid ~10 subprocesses per file
  fm_data="$(get_fm "$f" | yq -o=json '{
    "name": .name,
    "team": .team,
    "role": .role,
    "tier": .tier,
    "domain": .domain,
    "file_globs_len": (.file_globs | length),
    "keywords_len": (.keywords | length),
    "use_when": .use_when,
    "avoid_when": .avoid_when,
    "compose": .compose,
    "color": .color
  }')"

  if [ -z "$fm_data" ]; then err "$rel: frontmatter does not parse"; continue; fi
  grep -q '"""' "$f" && err "$rel: contains '\"\"\"' which would corrupt the Ollama Modelfile"

  team="$(echo "$fm_data" | yq -r '.team')"
  role="$(echo "$fm_data" | yq -r '.role')"
  tier="$(echo "$fm_data" | yq -r '.tier')"
  color="$(echo "$fm_data" | yq -r '.color')"

  # Only record a team/role pair once both are present, so a malformed persona
  # cannot inject a literal "null/null" token into the squad/uniqueness pools.
  if [ -n "$team" ] && [ "$team" != "null" ] && [ -n "$role" ] && [ "$role" != "null" ]; then
    ALL_AGENTS="${ALL_AGENTS}${team}/${role} "
  fi

  # Required keys. The *_len projections carry array lengths, so an empty array
  # (length 0) must be reported as "is empty"; the "0" sentinel applies ONLY to
  # those, never to scalar fields whose literal value could legitimately be "0".
  for k in name team role tier domain file_globs_len keywords_len use_when avoid_when compose; do
    v="$(echo "$fm_data" | yq -r ".$k")"
    case $k in
      *_len) { [ -z "$v" ] || [ "$v" = "0" ] || [ "$v" = "null" ]; } && err "$rel: ${k%_len} is empty" ;;
      *)     { [ -z "$v" ] || [ "$v" = "null" ]; }                  && err "$rel: missing required key '$k'" ;;
    esac
  done

  in_list "$tier" "$ALLOWED_TIERS" || err "$rel: tier '$tier' not in {$ALLOWED_TIERS}"
  [ "$(yq ".tiers | has(\"$tier\")" "$MODEL_MAP")" = "true" ] || err "$rel: tier '$tier' absent from model-map"
  [ "$team" = "$folder_team" ] || err "$rel: team '$team' != folder '$folder_team'"

  if [ -n "$color" ] && [ "$color" != "null" ]; then
    in_list "$color" "$ALLOWED_COLORS" || err "$rel: color '$color' not a named Claude color"
  fi

  # compose partials must exist
  while IFS= read -r item; do
    [ -n "$item" ] || continue
    [ "$item" = "__body__" ] && continue
    [ -f "${TEAMS_DIR}/${item}" ] || err "$rel: compose partial missing '${item}'"
  done < <(echo "$fm_data" | yq -r '.compose[]')
done < <(find "$TEAMS_DIR" -mindepth 2 -maxdepth 2 -name 'the_*.md' -print0 | sort -z)

# --- squad validation (using pre-cached agents) ----------------------------------------
if [ -n "$squad_members" ]; then
  while IFS= read -r ref; do
    [ -n "$ref" ] || continue
    in_list "$ref" "$ALL_AGENTS" || err "teams.yaml: squad ref '$ref' does not resolve to a real agent"
  done <<< "$squad_members"
fi

# --- role uniqueness within each team --------------------------------------------------
# Already validated via ALL_AGENTS by checking for duplicates
dups="$(echo "$ALL_AGENTS" | tr ' ' '\n' | sort | uniq -d)"
[ -z "$dups" ] || err "duplicate role(s) detected across teams: $(echo "$dups" | tr '\n' ' ')"

if [ "$ERRORS" -gt 0 ]; then
  echo "✗ validate: ${ERRORS} problem(s) found." >&2
  exit 1
fi
echo "✓ validate: ${persona_count} personas, model-map + teams.yaml all consistent."
