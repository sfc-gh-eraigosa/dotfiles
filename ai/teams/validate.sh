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
REQUIRED_KEYS="name team role tier domain file_globs keywords use_when avoid_when compose"
TOOLS="claude gemini antigravity ollama"

# frontmatter helpers (mirror install_ai_teams.sh)
_fm_end() { grep -n '^---[[:space:]]*$' "$1" | sed -n '2p' | cut -d: -f1; }
get_fm() { local end; end="$(_fm_end "$1")"; [ -n "$end" ] && sed -n "2,$((end - 1))p" "$1"; }
fmq() { get_fm "$1" | yq "$2" 2>/dev/null; }

in_list() { case " $2 " in *" $1 "*) return 0;; *) return 1;; esac; }

echo "Validating ai/teams source..."

# --- model-map.yaml --------------------------------------------------------------------
if ! yq '.' "$MODEL_MAP" >/dev/null 2>&1; then
  err "model-map.yaml does not parse"
else
  for t in $ALLOWED_TIERS; do
    [ "$(yq ".tiers | has(\"$t\")" "$MODEL_MAP")" = "true" ] || err "model-map: missing tier '$t'"
    for tool in $TOOLS; do
      [ "$(yq ".tiers.\"$t\" | has(\"$tool\")" "$MODEL_MAP")" = "true" ] \
        || err "model-map: tier '$t' missing tool '$tool'"
    done
  done
fi

# --- teams.yaml ------------------------------------------------------------------------
if ! yq '.' "$TEAMS_YAML" >/dev/null 2>&1; then
  err "teams.yaml does not parse"
else
  # each declared team must have a folder with >=1 persona
  while IFS= read -r team; do
    [ -n "$team" ] || continue
    if [ ! -d "${TEAMS_DIR}/${team}" ]; then
      err "teams.yaml: team '$team' has no folder ai/teams/${team}/"
    elif [ -z "$(find "${TEAMS_DIR}/${team}" -maxdepth 1 -name 'the_*.md' -print -quit)" ]; then
      err "teams.yaml: team '$team' folder has no the_*.md personas"
    fi
  done < <(yq '.teams | keys | .[]' "$TEAMS_YAML")

  # squad members must resolve to a real <team>/<role>
  while IFS= read -r ref; do
    [ -n "$ref" ] || continue
    rteam="${ref%%/*}"; rrole="${ref##*/}"
    found=""
    if [ -d "${TEAMS_DIR}/${rteam}" ]; then
      while IFS= read -r pf; do
        [ "$(fmq "$pf" '.role')" = "$rrole" ] && { found=1; break; }
      done < <(find "${TEAMS_DIR}/${rteam}" -maxdepth 1 -name 'the_*.md')
    fi
    [ -n "$found" ] || err "teams.yaml: squad ref '$ref' does not resolve to a real agent"
  done < <(yq '.squads[].members[]' "$TEAMS_YAML" 2>/dev/null)
fi

# --- persona files ---------------------------------------------------------------------
persona_count=0
while IFS= read -r f; do
  [ -e "$f" ] || continue
  persona_count=$((persona_count + 1))
  rel="${f#"$TEAMS_DIR"/}"
  folder_team="$(dirname "$rel")"

  if [ -z "$(_fm_end "$f")" ]; then err "$rel: no YAML frontmatter"; continue; fi
  if ! get_fm "$f" | yq '.' >/dev/null 2>&1; then err "$rel: frontmatter does not parse"; continue; fi

  for k in $REQUIRED_KEYS; do
    v="$(fmq "$f" ".$k")"
    if [ -z "$v" ] || [ "$v" = "null" ]; then err "$rel: missing required key '$k'"; fi
  done

  tier="$(fmq "$f" '.tier')"
  in_list "$tier" "$ALLOWED_TIERS" || err "$rel: tier '$tier' not in {$ALLOWED_TIERS}"
  [ "$(yq ".tiers | has(\"$tier\")" "$MODEL_MAP")" = "true" ] || err "$rel: tier '$tier' absent from model-map"

  team="$(fmq "$f" '.team')"
  [ "$team" = "$folder_team" ] || err "$rel: team '$team' != folder '$folder_team'"

  color="$(fmq "$f" '.color')"
  if [ -n "$color" ] && [ "$color" != "null" ]; then
    in_list "$color" "$ALLOWED_COLORS" || err "$rel: color '$color' not a named Claude color"
  fi

  [ "$(fmq "$f" '.file_globs | length')" -gt 0 ] 2>/dev/null || err "$rel: file_globs is empty"
  [ "$(fmq "$f" '.keywords | length')" -gt 0 ] 2>/dev/null || err "$rel: keywords is empty"

  # compose partials must exist
  while IFS= read -r item; do
    [ -n "$item" ] || continue
    [ "$item" = "__body__" ] && continue
    [ -f "${TEAMS_DIR}/${item}" ] || err "$rel: compose partial missing '${item}'"
  done < <(fmq "$f" '.compose[]')
done < <(find "$TEAMS_DIR" -mindepth 2 -maxdepth 2 -name 'the_*.md' | sort)

# --- role uniqueness within each team --------------------------------------------------
while IFS= read -r team; do
  [ -n "$team" ] || continue
  dups="$(for pf in "${TEAMS_DIR}/${team}"/the_*.md; do [ -e "$pf" ] && fmq "$pf" '.role'; done | sort | uniq -d)"
  [ -z "$dups" ] || err "team '$team': duplicate role(s): $(echo "$dups" | tr '\n' ' ')"
done < <(yq '.teams | keys | .[]' "$TEAMS_YAML" 2>/dev/null)

if [ "$ERRORS" -gt 0 ]; then
  echo "✗ validate: ${ERRORS} problem(s) found." >&2
  exit 1
fi
echo "✓ validate: ${persona_count} personas, model-map + teams.yaml all consistent."
