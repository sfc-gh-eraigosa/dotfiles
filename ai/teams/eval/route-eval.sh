#!/usr/bin/env bash
# route-eval.sh — routing-accuracy harness for the AI Teams system.
#
# Builds the candidate roster directly from source (ai/teams/<team>/the_*.md
# frontmatter, the same first-two-'---'-fences extraction install_ai_teams.sh
# uses), asks the selected model to pick the single best "<team>-<role>" (or a
# squad name) for each task in cases.yaml, and scores top-1 team + member
# accuracy against the expectations. A no-model `--check` mode validates the
# fixtures deterministically for CI.
#
# Exit codes:
#   0   gate passed (team & member accuracy >= threshold) OR --check passed
#   1   gate failed OR --check found an invalid expectation
#   77  SKIPPED — selected runner binary/credentials unavailable
#
# Zero hard deps beyond `yq` and the chosen CLI (claude|agy).
set -uo pipefail

# --- locate source tree ---------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEAMS_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"      # ai/teams
CASES="${SCRIPT_DIR}/cases.yaml"
TEAMS_YAML="${TEAMS_DIR}/teams.yaml"

RUNNER="${ROUTE_EVAL_RUNNER:-claude}"
THRESHOLD=90
MODE="run"

usage() {
  cat <<'EOF'
Usage: route-eval.sh [--check] [--runner claude|agy] [--threshold N] [--cases FILE]

  --check            Validate cases.yaml against the source tree only (no model
                     calls): every expect.team is a real folder, every
                     expect.member resolves to a role, every expect.squad exists
                     in teams.yaml. Prints case counts. Exit 0/1. CI-safe.
  --runner <cli>     Model CLI to route with (default: claude; env ROUTE_EVAL_RUNNER).
  --threshold <N>    Min %% accuracy for BOTH team and member to pass (default: 90).
  --cases <file>     Override the cases.yaml path.
  -h, --help         Show this help.

Exit: 0 pass · 1 fail/invalid · 77 SKIPPED (runner unavailable).
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --check) MODE="check" ;;
    --runner) RUNNER="${2:-}"; shift ;;
    --threshold) THRESHOLD="${2:-}"; shift ;;
    --cases) CASES="${2:-}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "route-eval: unknown arg '$1'" >&2; usage; exit 1 ;;
  esac
  shift
done

command -v yq >/dev/null 2>&1 || { echo "route-eval: yq is required" >&2; exit 1; }
[ -f "$CASES" ] || { echo "route-eval: cases file not found: $CASES" >&2; exit 1; }

# --- frontmatter extraction (mirrors install_ai_teams.sh) -----------------------------
_fm_end_line() { grep -n '^---[[:space:]]*$' "$1" | sed -n '2p' | cut -d: -f1; }
get_fm() {
  local f="$1" end; end="$(_fm_end_line "$f")"
  [ -n "$end" ] || return 1
  sed -n "2,$((end - 1))p" "$f"
}
fmq() { get_fm "$1" | yq "$2"; }

# --- build candidate roster from source ------------------------------------------------
# Populates parallel arrays: CAND_ID[], CAND_TEAM[], CAND_ROLE[], CAND_USE[], CAND_AVOID[]
declare -a CAND_ID=() CAND_TEAM=() CAND_ROLE=() CAND_USE=() CAND_AVOID=()
build_roster() {
  local f team role uw aw
  while IFS= read -r f; do
    [ -e "$f" ] || continue
    team="$(fmq "$f" '.team')"; role="$(fmq "$f" '.role')"
    [ -n "$team" ] && [ "$team" != "null" ] || continue
    [ -n "$role" ] && [ "$role" != "null" ] || continue
    uw="$(fmq "$f" '.use_when')"; aw="$(fmq "$f" '.avoid_when')"
    CAND_ID+=("${team}-${role}")
    CAND_TEAM+=("$team")
    CAND_ROLE+=("$role")
    CAND_USE+=("$uw")
    CAND_AVOID+=("$aw")
  done < <(find "$TEAMS_DIR" -mindepth 2 -maxdepth 2 -name 'the_*.md' | sort)
}

# Resolve a "<team> <role>" pair to a real persona file (role: field match).
role_exists() {  # $1=team $2=role -> 0/1
  local team="$1" role="$2" f r
  while IFS= read -r f; do
    [ -e "$f" ] || continue
    r="$(fmq "$f" '.role')"
    [ "$r" = "$role" ] && return 0
  done < <(find "${TEAMS_DIR}/${team}" -maxdepth 1 -name 'the_*.md' 2>/dev/null | sort)
  return 1
}
squad_exists() { yq -e ".squads.\"$1\"" "$TEAMS_YAML" >/dev/null 2>&1; }

# --- runner abstraction ----------------------------------------------------------------
runner_available() {
  command -v "$RUNNER" >/dev/null 2>&1 || return 1
  case "$RUNNER" in
    claude) [ -n "${ANTHROPIC_API_KEY:-}" ] || [ -d "${HOME}/.claude" ] || [ -f "${HOME}/.claude.json" ] ;;
    agy) [ -n "${GEMINI_API_KEY:-}" ] || [ -n "${GOOGLE_API_KEY:-}" ] || [ -d "${HOME}/.gemini" ] ;;
    *) return 1 ;;
  esac
}

# Run the model from a neutral, project-free cwd so a repo CLAUDE.md/AGENTS.md does not
# contaminate the routing decision — the classifier must judge from the prompt alone.
NEUTRAL_CWD="$(mktemp -d 2>/dev/null || echo /tmp)"
trap 'rm -rf "$NEUTRAL_CWD" 2>/dev/null || true' EXIT
call_model() {  # $1=prompt -> raw stdout
  case "$RUNNER" in
    claude) ( cd "$NEUTRAL_CWD" && claude -p "$1" 2>/dev/null ) ;;
    agy) ( cd "$NEUTRAL_CWD" && agy -p "$1" 2>/dev/null ) ;;
    *) echo "route-eval: unknown runner '$RUNNER'" >&2; return 1 ;;
  esac
}

# Build the routing prompt for a single task.
build_prompt() {  # $1=task -> prompt on stdout
  local task="$1" i
  {
    echo "You are a routing classifier for an engineering agent team."
    echo "Pick the SINGLE best candidate for the task below."
    echo
    echo "Candidates (id: use-when | avoid):"
    for i in "${!CAND_ID[@]}"; do
      printf -- '- %s: %s | avoid: %s\n' "${CAND_ID[$i]}" "${CAND_USE[$i]}" "${CAND_AVOID[$i]}"
    done
    echo
    echo "If the task clearly spans multiple teams, you MAY answer with one of these squad names instead:"
    yq -r '.squads | keys | .[]' "$TEAMS_YAML" | sed 's/^/- /'
    echo
    echo "TASK: ${task}"
    echo
    echo 'Output ONLY the single best id as "<team>-<role>" (or a squad name). No prose, no punctuation, no explanation.'
  }
}

# Trim model output to the first valid "<team>-<role>" token or squad name.
parse_pick() {  # stdin -> pick on stdout (empty if none)
  local raw last i sq
  raw="$(cat)"
  # 1) Prefer the model's final non-empty line as an EXACT id/squad (strict answer).
  last="$(printf '%s' "$raw" | grep -v '^[[:space:]]*$' | tail -1 | tr -d '[:space:]')"
  last="${last//\`/}"; last="${last//\"/}"; last="${last//\'/}"; last="${last//./}"; last="${last//\*/}"
  # lowercase once via tr (bash-3.2 safe; ${v,,} is bash 4+)
  local last_lc; last_lc="$(printf '%s' "$last" | tr '[:upper:]' '[:lower:]')"
  for i in "${!CAND_ID[@]}"; do
    [ "$last_lc" = "$(printf '%s' "${CAND_ID[$i]}" | tr '[:upper:]' '[:lower:]')" ] && { echo "${CAND_ID[$i]}"; return; }
  done
  while IFS= read -r sq; do
    [ -n "$sq" ] && [ "$last_lc" = "$(printf '%s' "$sq" | tr '[:upper:]' '[:lower:]')" ] && { echo "$sq"; return; }
  done < <(yq -r '.squads | keys | .[]' "$TEAMS_YAML")
  # 2) Else first roster id / squad that appears as a whole word anywhere in the output.
  for i in "${!CAND_ID[@]}"; do
    grep -qiw -- "${CAND_ID[$i]}" <<<"$raw" && { echo "${CAND_ID[$i]}"; return; }
  done
  while IFS= read -r sq; do
    [ -n "$sq" ] || continue
    grep -qiw -- "$sq" <<<"$raw" && { echo "$sq"; return; }
  done < <(yq -r '.squads | keys | .[]' "$TEAMS_YAML")
  # 3) No valid candidate found -> empty (scored as a miss). No loose-token fallback:
  #    a regex guess would resolve to a non-candidate and silently corrupt the score.
}

# =====================================================================================
# --check : deterministic fixture validation (no model)
# =====================================================================================
run_check() {
  local n total ok=0 bad=0 team member squad
  total="$(yq '.cases | length' "$CASES")"
  echo "route-eval --check :: validating ${total} case(s) in ${CASES##*/}"
  local n_member=0 n_squad=0
  for ((n=0; n<total; n++)); do
    team="$(yq -r ".cases[$n].expect.team // \"\"" "$CASES")"
    member="$(yq -r ".cases[$n].expect.member // \"\"" "$CASES")"
    squad="$(yq -r ".cases[$n].expect.squad // \"\"" "$CASES")"
    local task; task="$(yq -r ".cases[$n].task" "$CASES")"

    if [ -n "$squad" ]; then
      n_squad=$((n_squad + 1))
      if squad_exists "$squad"; then ok=$((ok + 1)); else
        bad=$((bad + 1)); echo "  FAIL [#$n] squad '$squad' not in teams.yaml :: ${task}"
      fi
      continue
    fi

    # member case: team folder must exist AND member role must resolve.
    n_member=$((n_member + 1))
    if [ -z "$team" ]; then
      bad=$((bad + 1)); echo "  FAIL [#$n] missing expect.team and expect.squad :: ${task}"; continue
    fi
    if [ ! -d "${TEAMS_DIR}/${team}" ]; then
      bad=$((bad + 1)); echo "  FAIL [#$n] team folder '${team}' does not exist :: ${task}"; continue
    fi
    if [ -z "$member" ]; then
      bad=$((bad + 1)); echo "  FAIL [#$n] missing expect.member :: ${task}"; continue
    fi
    if role_exists "$team" "$member"; then ok=$((ok + 1)); else
      bad=$((bad + 1)); echo "  FAIL [#$n] role '${member}' not found in '${team}/the_*.md' :: ${task}"
    fi
  done

  echo "  candidates discovered : ${#CAND_ID[@]}"
  echo "  member cases          : ${n_member}"
  echo "  squad/ambiguous cases : ${n_squad}"
  echo "  valid / invalid       : ${ok} / ${bad}"
  if [ "$bad" -eq 0 ]; then echo "route-eval --check: OK"; return 0; fi
  echo "route-eval --check: FAILED (${bad} invalid)"; return 1
}

# =====================================================================================
# run : score against a live model
# =====================================================================================
run_eval() {
  if ! runner_available; then
    echo "SKIPPED: no runner available (runner='${RUNNER}')"
    exit 77
  fi

  local total team_hit=0 member_hit=0 scored=0
  total="$(yq '.cases | length' "$CASES")"
  declare -a MISROUTES=()
  # bash-3.2 portable counters: append each occurrence as a line, count later
  # with grep -cxF (associative arrays are bash 4+).
  local EXP_TEAM_LIST="" GOT_TEAM_LIST=""

  local n
  for ((n=0; n<total; n++)); do
    local task etea emem esq prompt raw pick gteam gmem expect_label
    task="$(yq -r ".cases[$n].task" "$CASES")"
    etea="$(yq -r ".cases[$n].expect.team // \"\"" "$CASES")"
    emem="$(yq -r ".cases[$n].expect.member // \"\"" "$CASES")"
    esq="$(yq -r ".cases[$n].expect.squad // \"\"" "$CASES")"

    prompt="$(build_prompt "$task")"
    raw="$(call_model "$prompt")"
    pick="$(printf '%s' "$raw" | parse_pick)"

    scored=$((scored + 1))

    if [ -n "$esq" ]; then
      # Squad case: a correct squad pick counts for BOTH team and member metrics.
      expect_label="$esq"
      EXP_TEAM_LIST="${EXP_TEAM_LIST}${esq}"$'\n'
      if [ "$pick" = "$esq" ]; then
        team_hit=$((team_hit + 1)); member_hit=$((member_hit + 1))
        GOT_TEAM_LIST="${GOT_TEAM_LIST}${esq}"$'\n'
      else
        GOT_TEAM_LIST="${GOT_TEAM_LIST}${pick:-<none>}"$'\n'
        MISROUTES+=("${task} :: expected=${esq} got=${pick:-<none>}")
      fi
      continue
    fi

    # Member case: pick is "<team>-<role>". Resolve against the roster rather than
    # cutting at the first hyphen — team names can themselves contain hyphens
    # (terraform-aws, ai-ci), so a naive split would mis-score those teams.
    expect_label="${etea}-${emem}"
    gteam=""; gmem=""
    if [ -n "$pick" ]; then
      for i in "${!CAND_ID[@]}"; do
        if [ "${CAND_ID[$i]}" = "$pick" ]; then
          gteam="${CAND_TEAM[$i]}"; gmem="${CAND_ROLE[$i]}"; break
        fi
      done
    fi
    EXP_TEAM_LIST="${EXP_TEAM_LIST}${etea}"$'\n'
    GOT_TEAM_LIST="${GOT_TEAM_LIST}${gteam:-<none>}"$'\n'

    local th=0
    if [ "$gteam" = "$etea" ]; then team_hit=$((team_hit + 1)); th=1; fi
    if [ "$th" -eq 1 ] && [ "$gmem" = "$emem" ]; then
      member_hit=$((member_hit + 1))
    else
      MISROUTES+=("${task} :: expected=${expect_label} got=${pick:-<none>}")
    fi
  done

  # --- report ---
  local team_pct member_pct
  team_pct=$(( scored > 0 ? team_hit * 100 / scored : 0 ))
  member_pct=$(( scored > 0 ? member_hit * 100 / scored : 0 ))

  echo "==================== route-eval (runner=${RUNNER}) ===================="
  echo "cases scored      : ${scored}"
  echo "team  top-1       : ${team_hit}/${scored}  (${team_pct}%)"
  echo "member top-1      : ${member_hit}/${scored}  (${member_pct}%)"
  echo
  echo "--- per-team counts (expected vs predicted) ---"
  printf '%s%s' "$EXP_TEAM_LIST" "$GOT_TEAM_LIST" | grep -v '^[[:space:]]*$' | sort -u | while IFS= read -r key; do
    [ -n "$key" ] || continue
    exp="$(printf '%s' "$EXP_TEAM_LIST" | grep -cxF -- "$key")"
    got="$(printf '%s' "$GOT_TEAM_LIST" | grep -cxF -- "$key")"
    printf '  %-22s expected=%-3s predicted=%-3s\n' "$key" "$exp" "$got"
  done

  if [ "${#MISROUTES[@]}" -gt 0 ]; then
    echo
    echo "--- misroutes (${#MISROUTES[@]}) ---"
    local m
    for m in "${MISROUTES[@]}"; do echo "  - ${m}"; done
  fi

  echo
  echo "gate threshold    : ${THRESHOLD}% (team AND member)"
  if [ "$team_pct" -ge "$THRESHOLD" ] && [ "$member_pct" -ge "$THRESHOLD" ]; then
    echo "RESULT: PASS"
    return 0
  fi
  echo "RESULT: FAIL"
  return 1
}

# --- dispatch --------------------------------------------------------------------------
build_roster
if [ "${#CAND_ID[@]}" -eq 0 ]; then
  echo "route-eval: no candidates discovered under ${TEAMS_DIR}" >&2
  exit 1
fi

if [ "$MODE" = "check" ]; then
  run_check; exit $?
fi
run_eval; exit $?
