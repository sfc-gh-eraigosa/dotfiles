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

# --- ollama create: existence check, real errors, gff gate ------------------------------
# Fleet runs (2026-09-11) on 7 GB boards showed `ollama create` AUTO-PULLING a missing
# base model (18 GB qwen3-coder:30b) and a guessed warning text hiding the real failure.
# A stub `ollama` on PATH records every call; `show` answers from OLLAMA_STUB_PRESENT.
STUB="$(mktemp -d)"
cat > "$STUB/ollama" <<'EOF'
#!/usr/bin/env bash
echo "$*" >> "$OLLAMA_STUB_LOG"
case "$1" in
  show)
    if [ -n "${OLLAMA_STUB_SHOW_ERR:-}" ]; then echo "Error: ${OLLAMA_STUB_SHOW_ERR}" >&2; exit 1; fi
    case " ${OLLAMA_STUB_PRESENT:-} " in *" $2 "*) echo "  Model"; exit 0 ;; esac
    echo "Error: model '$2' not found" >&2; exit 1 ;;
  create)
    if [ -n "${OLLAMA_STUB_CREATE_ERR:-}" ]; then
      echo "gathering model components" >&2
      echo "Error: ${OLLAMA_STUB_CREATE_ERR}" >&2; exit 1
    fi
    echo "success"; exit 0 ;;
esac
exit 0
EOF
chmod +x "$STUB/ollama"

# Every distinct base model the map names, and the deep-think one (the model the
# fleet boards could not create).
ALL_MODELS="$(yq -r '.tiers[].ollama.model' "$MODEL_MAP" | sort -u | tr '\n' ' ')"
DT_MODEL="$(yq -r '.tiers."deep-think".ollama.model' "$MODEL_MAP")"
NOT_DT=""
for m in $ALL_MODELS; do [ "$m" = "$DT_MODEL" ] || NOT_DT="$NOT_DT $m"; done

# run_ollama <home> [VAR=value ...] — ollama-only emit with the stub first on PATH.
run_ollama() {
  local h="$1"; shift
  env PATH="$STUB:$PATH" TEAMS_DEST_HOME="$h" OLLAMA_STUB_LOG="$h/ollama.log" \
    SKIP_OLLAMA_CREATE=0 GFF_INSTALL_AI_TEAMS_OLLAMA_CREATE= "$@" \
    bash "$INSTALLER" --tool ollama >"$h/out" 2>"$h/err"
}
calls() { grep -c "^$2" "$1/ollama.log" 2>/dev/null | tr -d ' ' || true; }
n_modelfiles() { find "$1/.config/ollama/teams" -name '*.Modelfile' | wc -l | tr -d ' '; }

# (a) every base model present -> one create per persona, no skip summary.
O1="$(mktemp -d)"
run_ollama "$O1" OLLAMA_STUB_PRESENT="$ALL_MODELS" || bad "ollama present: installer exited non-zero"
assert_eq "present base models: ollama create runs for all 22 agents" "$(calls "$O1" 'create ')" "22"
assert_eq "present base models: no 'not pulled' summary" \
  "$(grep -c 'not pulled' "$O1/out" | tr -d ' ')" "0"

# (b) deep-think base model missing -> no create for those agents (no auto-pull),
#     ONE summary line for the model, Modelfiles still written.
O2="$(mktemp -d)"
run_ollama "$O2" OLLAMA_STUB_PRESENT="$NOT_DT" || bad "ollama missing: installer exited non-zero"
DT_N="$(grep -lxF "FROM ${DT_MODEL}" "$O2/.config/ollama/teams"/*/*.Modelfile 2>/dev/null | wc -l | tr -d ' ')"
[ "${DT_N:-0}" -gt 0 ] && ok "fixture: ${DT_N} deep-think Modelfiles use ${DT_MODEL}" \
  || bad "fixture: no Modelfile uses the deep-think model ${DT_MODEL}"
assert_eq "missing base model: Modelfiles still written for all 22" "$(n_modelfiles "$O2")" "22"
assert_eq "missing base model: create skipped for its ${DT_N} agents" \
  "$(calls "$O2" 'create ')" "$((22 - DT_N))"
assert_eq "missing base model: no create names a deep-think agent's Modelfile" \
  "$(grep '^create ' "$O2/ollama.log" | while read -r _ _ _ mf; do grep -lxF "FROM ${DT_MODEL}" "$mf"; done | wc -l | tr -d ' ')" "0"
SUMMARY="ollama: base model ${DT_MODEL} not pulled; skipped ${DT_N} teams-* agents (run: ollama pull ${DT_MODEL})"
assert_eq "missing base model: exactly one summary line for ${DT_MODEL}" \
  "$(grep -cF -- "$SUMMARY" "$O2/out" | tr -d ' ')" "1"
assert_eq "missing base model: summary printed once per missing model (1 total)" \
  "$(grep -c 'not pulled' "$O2/out" | tr -d ' ')" "1"
assert_eq "missing base model: ollama show probes each base model once" \
  "$(calls "$O2" "show ${DT_MODEL}")" "1"
assert_eq "missing base model: nothing on stderr" "$(wc -c < "$O2/err" | tr -d ' ')" "0"

# (c) create fails -> the REAL error is surfaced, not a guess.
O3="$(mktemp -d)"
REAL_ERR="pull model manifest: 412: requires a newer version of Ollama"
run_ollama "$O3" OLLAMA_STUB_PRESENT="$ALL_MODELS" OLLAMA_STUB_CREATE_ERR="$REAL_ERR" \
  && ok "create failure: installer still exits 0" || bad "create failure: installer exited non-zero"
assert_contains "create failure: real ollama error surfaced" "$(cat "$O3/err")" "Error: ${REAL_ERR}"
case "$(cat "$O3/err")" in
  *"likely not pulled"*) bad "create failure: stale guessed warning text still printed" ;;
  *) ok "create failure: no guessed 'likely not pulled' text" ;;
esac

# (d) ollama present but unreachable -> one warning, no create attempts.
O4="$(mktemp -d)"
run_ollama "$O4" OLLAMA_STUB_SHOW_ERR="could not connect to ollama server" \
  || bad "ollama unreachable: installer exited non-zero"
assert_eq "ollama unreachable: no create attempted" "$(calls "$O4" 'create ')" "0"
assert_eq "ollama unreachable: exactly one warning line" \
  "$(grep -c 'could not connect' "$O4/err" | tr -d ' ')" "1"
assert_eq "ollama unreachable: Modelfiles still written" "$(n_modelfiles "$O4")" "22"

# (e) gff flag off -> no ollama calls at all, Modelfiles still written.
O5="$(mktemp -d)"
run_ollama "$O5" OLLAMA_STUB_PRESENT="$ALL_MODELS" GFF_INSTALL_AI_TEAMS_OLLAMA_CREATE=false \
  || bad "flag off: installer exited non-zero"
assert_nofile "$O5/ollama.log" "flag off: ollama never invoked"
assert_eq "flag off: Modelfiles still written" "$(n_modelfiles "$O5")" "22"
assert_contains "flag off: says why create was skipped" "$(cat "$O5/out")" \
  "SKIP (gff: install.ai.teams-ollama-create=false)"

rm -rf "$STUB" "$O1" "$O2" "$O3" "$O4" "$O5"

echo "== result: ${PASS} passed, ${FAIL} failed =="
[ "$FAIL" -eq 0 ]
