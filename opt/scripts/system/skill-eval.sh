#!/usr/bin/env bash
# skill-eval.sh — fixture validator for agent-skill eval corpora.
#
# Every skill is a folder marked by a `SKILL.md` (the same marker sync-skills.sh
# discovers). The skill-creator convention puts a skill's eval corpus in a sibling
# `evals/evals.json`. This script walks EVERY skill folder, validates the corpora
# that exist, and reports SKIP for the ones that don't — all deterministically with
# NO model calls (the CI-safe analog of `ai/teams/eval/route-eval.sh --check`).
#
# Skill folders are discovered under ai/skills/, src/, and sdk/ (override with
# --skills-dir). A skill's name is the folder name, except a `<tool>/skill/SKILL.md`
# layout names the skill after the parent <tool> (matching sync-skills.sh).
#
# What `--check` enforces for each evals.json that exists:
#   - valid JSON;
#   - `.skill_name` is present and equals the skill name;
#   - `.evals` is a non-empty array;
#   - every case has a unique integer `.id`, a non-empty `.prompt`, and a
#     non-empty `.expected_output`.
# Cases without `assertions` are NOTED (gradable only qualitatively), not failed.
# A skill folder with no evals/evals.json is SKIPPED (reported, not failed).
#
# Exit codes:
#   0   --check passed (every corpus that exists is valid; skips are fine)
#   1   --check found an invalid corpus, or jq is missing
#
# Zero hard deps beyond `jq`.
set -uo pipefail

# --- locate source tree ---------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"   # opt/scripts/system -> repo root
SKILLS_DIR=""                                       # empty => default roots below
MODE="check"

usage() {
  cat <<'EOF'
Usage: skill-eval.sh [--check] [--skills-dir DIR]

  --check            Walk every skill folder (a dir with a SKILL.md) under
                     ai/skills/, src/, and sdk/, then deterministically validate
                     each sibling evals/evals.json (no model calls): valid JSON,
                     skill_name matches the skill, a non-empty evals array, and
                     each case has a unique integer id + non-empty prompt +
                     non-empty expected_output. Cases without `assertions` are
                     noted; skill folders with no eval corpus are SKIPPED (not
                     failed). Default mode. CI-safe.
  --skills-dir DIR   Scan this single root for skill folders instead of the
                     default ai/skills + src + sdk roots.
  -h, --help         Show this help.

Exit: 0 valid (skips ok) · 1 invalid (or jq missing).

Note: behavioral grading (with-skill vs baseline, accuracy %) is the model-driven
skill-creator loop, run on demand — not this deterministic gate.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --check) MODE="check" ;;
    --skills-dir) SKILLS_DIR="${2:-}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "skill-eval: unknown arg '$1'" >&2; usage; exit 1 ;;
  esac
  shift
done

command -v jq >/dev/null 2>&1 || { echo "skill-eval: jq is required" >&2; exit 1; }

# Roots to scan for SKILL.md. A single --skills-dir overrides the defaults.
ROOTS=()
if [ -n "$SKILLS_DIR" ]; then
  [ -d "$SKILLS_DIR" ] || { echo "skill-eval: skills dir not found: $SKILLS_DIR" >&2; exit 1; }
  ROOTS=("$SKILLS_DIR")
else
  for r in "${REPO_ROOT}/ai/skills" "${REPO_ROOT}/src" "${REPO_ROOT}/sdk"; do
    [ -d "$r" ] && ROOTS+=("$r")
  done
fi

# Derive the skill name from a SKILL.md path the way sync-skills.sh does:
# <tool>/skill/SKILL.md -> <tool>; otherwise the SKILL.md's own folder name.
skill_name_for() {
  local md_dir; md_dir="$(dirname "$1")"
  if [ "$(basename "$md_dir")" = "skill" ]; then
    basename "$(dirname "$md_dir")"
  else
    basename "$md_dir"
  fi
}

# Validate one evals.json for skill $2. Prints failures; returns 0 valid / 1 invalid.
check_corpus() {
  local file="$1" skill_name="$2" bad=0 declared count issues with_assert

  if ! jq empty "$file" >/dev/null 2>&1; then
    echo "  FAIL [${skill_name}] not valid JSON: ${file#"$REPO_ROOT"/}"
    return 1
  fi

  declared="$(jq -r '.skill_name // ""' "$file")"
  if [ -z "$declared" ]; then
    echo "  FAIL [${skill_name}] missing .skill_name"; bad=$((bad + 1))
  elif [ "$declared" != "$skill_name" ]; then
    echo "  FAIL [${skill_name}] .skill_name='${declared}' does not match skill folder '${skill_name}'"; bad=$((bad + 1))
  fi

  count="$(jq '(.evals // []) | length' "$file" 2>/dev/null || echo 0)"
  if [ "$count" -eq 0 ]; then
    echo "  FAIL [${skill_name}] .evals is missing or empty"
    return 1
  fi

  # Per-case structural checks in jq so a malformed case can't crash bash.
  issues="$(jq -r '
    .evals as $e
    | ([ $e[].id ]) as $ids
    | ( $ids | length ) as $n
    | ( $ids | unique | length ) as $u
    | ( if $u < $n then "duplicate or missing .id across cases" else empty end ),
      ( $e[]
        | ( if (.id | type) != "number" then "case id=\(.id // "?"): .id is not an integer" else empty end ),
          ( if ((.prompt // "") | length) == 0 then "case id=\(.id // "?"): empty .prompt" else empty end ),
          ( if ((.expected_output // "") | length) == 0 then "case id=\(.id // "?"): empty .expected_output" else empty end ) )
  ' "$file" 2>/dev/null)"

  if [ -n "$issues" ]; then
    while IFS= read -r line; do
      [ -n "$line" ] && { echo "  FAIL [${skill_name}] ${line}"; bad=$((bad + 1)); }
    done <<< "$issues"
  fi

  with_assert="$(jq '[ .evals[] | select((.assertions // []) | length > 0) ] | length' "$file" 2>/dev/null || echo 0)"

  if [ "$bad" -eq 0 ]; then
    if [ "$with_assert" -eq 0 ]; then
      echo "  PASS [${skill_name}] ${count} case(s) — NOTE: no assertions yet (qualitative grading only)"
    else
      echo "  PASS [${skill_name}] ${count} case(s) — ${with_assert} with assertions"
    fi
    return 0
  fi
  echo "  FAIL [${skill_name}] ${bad} problem(s) in ${file#"$REPO_ROOT"/}"
  return 1
}

if [ "$MODE" != "check" ]; then
  echo "skill-eval: only --check mode is implemented" >&2
  exit 1
fi

echo "skill-eval --check :: scanning for skill folders (SKILL.md) under:"
for r in "${ROOTS[@]}"; do echo "    ${r#"$REPO_ROOT"/}"; done

skills=0
validated=0
invalid=0
skipped=0
while IFS= read -r md; do
  [ -n "$md" ] || continue
  skills=$((skills + 1))
  name="$(skill_name_for "$md")"
  eval_file="$(dirname "$md")/evals/evals.json"
  if [ -f "$eval_file" ]; then
    validated=$((validated + 1))
    check_corpus "$eval_file" "$name" || invalid=$((invalid + 1))
  else
    skipped=$((skipped + 1))
    echo "  SKIP [${name}] no eval corpus (${eval_file#"$REPO_ROOT"/})"
  fi
done < <(
  for r in "${ROOTS[@]}"; do
    find "$r" -name SKILL.md -type f 2>/dev/null
  done | sort -u
)

if [ "$skills" -eq 0 ]; then
  echo "skill-eval --check: no skill folders found (nothing to validate)"
  exit 0
fi

echo "  skill folders     : ${skills}"
echo "  validated         : ${validated} (pass $((validated - invalid)) / fail ${invalid})"
echo "  skipped (no eval) : ${skipped}"
if [ "$invalid" -eq 0 ]; then
  echo "skill-eval --check: OK"
  exit 0
fi
echo "skill-eval --check: FAILED (${invalid} invalid)"
exit 1
