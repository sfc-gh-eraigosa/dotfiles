#!/usr/bin/env bash
# install_ai_teams_test.sh — unit tests for install_ai_teams.sh.
# Emits into a throwaway TEAMS_DEST_HOME and asserts tier resolution, emitter validity,
# idempotency, graceful-skip, and compose ordering. No network, no real tool dirs touched.
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALLER="${HERE}/install_ai_teams.sh"

PASS=0; FAIL=0
ok()   { echo "  ✓ $*"; PASS=$((PASS + 1)); }
bad()  { echo "  ✗ $*" >&2; FAIL=$((FAIL + 1)); }
assert_eq()       { [ "$2" = "$3" ] && ok "$1" || bad "$1 (want '$3', got '$2')"; }
assert_file()     { [ -f "$1" ] && ok "file exists: ${1##*/teams/}" || bad "missing file: $1"; }
assert_nofile()   { [ ! -e "$1" ] && ok "$2" || bad "$2 (file unexpectedly present: $1)"; }
assert_contains() { case "$2" in *"$3"*) ok "$1";; *) bad "$1 (missing '$3')";; esac; }

# frontmatter reader (mirrors installer)
fm_end() { grep -n '^---[[:space:]]*$' "$1" | sed -n '2p' | cut -d: -f1; }
fmget()  { local e; e="$(fm_end "$1")"; sed -n "2,$((e-1))p" "$1" | yq "$2"; }

run_install() { TEAMS_DEST_HOME="$1" SKIP_OLLAMA_CREATE=1 bash "$INSTALLER" "${@:2}" >/dev/null 2>&1; }

echo "== install_ai_teams_test =="

# --- full emit into temp HOME ----------------------------------------------------------
H="$(mktemp -d)"
run_install "$H" || bad "installer exited non-zero"

# counts: 22 personas -> 22 files for claude + antigravity; ollama Modelfiles
assert_eq "claude emits 22 agents" \
  "$(find "$H/.claude/agents/teams" -name '*.md' | wc -l | tr -d ' ')" "22"
assert_nofile "$H/.gemini/agents/teams" "retired gemini emitter writes nothing"
assert_eq "antigravity emits 22 agents" \
  "$(find "$H/.config/antigravity/agents" -name '*.yaml' | wc -l | tr -d ' ')" "22"
assert_eq "ollama emits 22 Modelfiles" \
  "$(find "$H/.config/ollama/teams" -name '*.Modelfile' | wc -l | tr -d ' ')" "22"

# grouped layout / naming
assert_file "$H/.claude/agents/teams/web/fe.md"
assert_file "$H/.config/antigravity/agents/web-fe.yaml"

# --- tier resolution -------------------------------------------------------------------
CFE="$H/.claude/agents/teams/web/fe.md"           # standard
CSY="$H/.claude/agents/teams/architecture/sysarch.md"  # deep-think
CWQ="$H/.claude/agents/teams/web/webqa.md"        # fast
ASY="$H/.config/antigravity/agents/architecture-sysarch.yaml"
OGD="$H/.config/ollama/teams/go/godev.Modelfile"  # standard

assert_eq "standard -> claude sonnet"      "$(fmget "$CFE" '.model')"  "sonnet"
assert_eq "standard -> claude effort med"  "$(fmget "$CFE" '.effort')" "medium"
assert_eq "deep-think -> claude opus"      "$(fmget "$CSY" '.model')"  "opus"
assert_eq "deep-think -> claude effort hi" "$(fmget "$CSY" '.effort')" "high"
assert_eq "fast -> claude haiku"           "$(fmget "$CWQ" '.model')"  "haiku"
assert_eq "deep-think -> antigravity o3"   "$(yq '.model' "$ASY")"  "o3"
# The ollama expectations are READ FROM model-map.yaml rather than hardcoded: the
# tiers get re-pointed as the Spark scorecard moves (13a1b90 took standard from
# qwen2.5-coder:7b to qwen3-coder:30b and this test, still hardcoded, turned the
# required teams-eval check red on every PR). What is under test is that the
# emitter honours the map's standard tier, not which tag the map happens to name.
MODEL_MAP="${HERE}/../../../ai/teams/model-map.yaml"
MM_STD_MODEL="$(yq -r '.tiers.standard.ollama.model'   "$MODEL_MAP")"
MM_STD_CTX="$(yq -r '.tiers.standard.ollama.num_ctx'   "$MODEL_MAP")"
assert_contains "ollama standard FROM follows model-map (${MM_STD_MODEL})" \
  "$(cat "$OGD")" "FROM ${MM_STD_MODEL}"
assert_contains "ollama num_ctx follows model-map (${MM_STD_CTX})" \
  "$(cat "$OGD")" "PARAMETER num_ctx ${MM_STD_CTX}"

# --- emitter validity ------------------------------------------------------------------
fmget "$CFE" '.name' >/dev/null 2>&1 && [ "$(fmget "$CFE" '.name')" = "web-fe" ] \
  && ok "claude frontmatter parses, name=web-fe" || bad "claude frontmatter invalid"
[ "$(yq '.name' "$H/.config/antigravity/agents/web-fe.yaml")" = "web-fe" ] \
  && ok "antigravity yaml parses, name=web-fe" || bad "antigravity yaml invalid"

# description is compiled (non-empty, has negative scoping)
DESC="$(fmget "$CFE" '.description')"
assert_contains "description compiled (PROACTIVELY)" "$DESC" "Use PROACTIVELY for:"
assert_contains "description negative-scoped (Do NOT)" "$DESC" "Do NOT use for:"

# --- compose ordering: safety, then conventions, then body, then handoff footer --------
BODY="$(sed -n "$(( $(fm_end "$CFE") + 1 )),\$p" "$CFE")"
s=$(printf '%s\n' "$BODY" | grep -n 'SAFETY & PRIVACY' | head -1 | cut -d: -f1)
c=$(printf '%s\n' "$BODY" | grep -n 'REPOSITORY CONVENTIONS' | head -1 | cut -d: -f1)
h=$(printf '%s\n' "$BODY" | grep -n 'HANDOFF PROTOCOL (shared)' | head -1 | cut -d: -f1)
{ [ -n "$s" ] && [ -n "$c" ] && [ -n "$h" ] && [ "$s" -lt "$c" ] && [ "$c" -lt "$h" ]; } \
  && ok "compose order: safety < conventions < handoff-footer" \
  || bad "compose order wrong (safety=$s conventions=$c handoff=$h)"

# --- idempotency -----------------------------------------------------------------------
H2="$(mktemp -d)"; run_install "$H2"
diff -r "$H" "$H2" >/dev/null 2>&1 && ok "idempotent: re-run byte-identical" || bad "not idempotent"

# --- graceful skip / tool filter -------------------------------------------------------
H3="$(mktemp -d)"; run_install "$H3" --tool claude
assert_file "$H3/.claude/agents/teams/web/fe.md"
assert_nofile "$H3/.config/antigravity/agents" "--tool claude does not emit antigravity"
assert_nofile "$H3/.config/ollama/teams" "--tool claude does not emit ollama"

# --- dry-run writes nothing ------------------------------------------------------------
H4="$(mktemp -d)"; TEAMS_DEST_HOME="$H4" bash "$INSTALLER" --dry-run >/dev/null 2>&1
assert_nofile "$H4/.claude" "--dry-run writes no files"

# --- prune: a renamed/removed persona leaves no zombie agent ---------------------------
ZOMBIE="$H/.claude/agents/teams/web/zzz_zombie.md"
printf -- '---\nname: web-zzz\n---\nstale\n' > "$ZOMBIE"
run_install "$H"
assert_nofile "$ZOMBIE" "prune removes zombie claude agent on re-run"
assert_eq "prune keeps the real 22 claude agents" \
  "$(find "$H/.claude/agents/teams" -name '*.md' | wc -l | tr -d ' ')" "22"

# --- no claude agent may emit a literal null color (guard the optional-color path) -----
assert_eq "no claude agent emits a literal null color" \
  "$(grep -rlE '^color:[[:space:]]*"?null"?' "$H/.claude/agents/teams" 2>/dev/null | wc -l | tr -d ' ')" "0"

rm -rf "$H" "$H2" "$H3" "$H4"

echo "== result: ${PASS} passed, ${FAIL} failed =="
[ "$FAIL" -eq 0 ]
