#!/usr/bin/env bash
# Test driver for opt/scripts/system/skill-eval.sh
#
# skill-eval.sh --check walks every skill folder (a dir with a SKILL.md) and
# deterministically validates each sibling evals/evals.json, reporting SKIP for
# skill folders that have none. We exercise it against the real repo AND against
# synthetic --skills-dir fixtures (valid, each invalid shape, and a no-eval skip)
# so the pass path, every fail path, and the skip path are all proven. No HOME
# side effects.
#
# Run: bash opt/scripts/system/skill-eval_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/skill-eval.sh"

# --- helpers -------------------------------------------------------------------------
# A skill folder is marked by a SKILL.md. make_corpus writes a discoverable skill
# ($1/$2/SKILL.md) WITH an eval corpus ($1/$2/evals/evals.json from body $3).
make_corpus() {
  local root="$1" name="$2" body="$3"
  mkdir -p "${root}/${name}/evals"
  printf '# %s\n' "$name" > "${root}/${name}/SKILL.md"
  printf '%s\n' "$body" > "${root}/${name}/evals/evals.json"
}

# make_skill_no_eval writes a discoverable skill folder WITHOUT any eval corpus.
make_skill_no_eval() {
  local root="$1" name="$2"
  mkdir -p "${root}/${name}"
  printf '# %s\n' "$name" > "${root}/${name}/SKILL.md"
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# === 1. Syntax + help ===
assert_exit_code 0 "skill-eval.sh parses with bash -n" \
    bash -n "$SCRIPT"
assert_exit_code 0 "skill-eval.sh --help exits 0" \
    bash "$SCRIPT" --help
assert_exit_code 1 "unknown arg exits 1" \
    bash "$SCRIPT" --bogus

# === 2. Real repo: passes, and SKIPs skills without evals ===
OUT="${TMP}/real.out"
bash "$SCRIPT" --check > "$OUT" 2>&1 || true
assert_exit_code 0 "real ai/skills corpus passes --check" \
    bash "$SCRIPT" --check
assert_grep "real run validates mbo-plan" "PASS \[mbo-plan\]" "$OUT"
assert_grep "real run skips an eval-less skill" "SKIP \[.*\] no eval corpus" "$OUT"

# === 3. A valid synthetic corpus passes (exit 0, prints PASS) ===
VALID="${TMP}/valid"
make_corpus "$VALID" "demo-skill" '{
  "skill_name": "demo-skill",
  "evals": [
    { "id": 1, "prompt": "do a thing", "expected_output": "does the thing" },
    { "id": 2, "prompt": "do another", "expected_output": "does another", "assertions": [{"text":"x"}] }
  ]
}'
OUT="${TMP}/valid.out"
bash "$SCRIPT" --check --skills-dir "$VALID" > "$OUT" 2>&1 || true
assert_exit_code 0 "valid corpus exits 0" \
    bash "$SCRIPT" --check --skills-dir "$VALID"
assert_grep "valid corpus reports PASS" "PASS \[demo-skill\]" "$OUT"
assert_grep "overall OK" "skill-eval --check: OK" "$OUT"

# === 4. A skill folder with NO eval corpus is SKIPPED (exit 0) ===
SKIPD="${TMP}/skipd"
make_skill_no_eval "$SKIPD" "lonely-skill"
OUT="${TMP}/skip.out"
bash "$SCRIPT" --check --skills-dir "$SKIPD" > "$OUT" 2>&1 || true
assert_exit_code 0 "skill with no eval exits 0 (skip is ok)" \
    bash "$SCRIPT" --check --skills-dir "$SKIPD"
assert_grep "skip line printed" "SKIP \[lonely-skill\] no eval corpus" "$OUT"

# === 5. Mixed root: one valid + one skipped, exit 0, both surfaced ===
MIX="${TMP}/mix"
make_corpus "$MIX" "has-eval" '{ "skill_name": "has-eval", "evals": [ { "id": 1, "prompt": "p", "expected_output": "e" } ] }'
make_skill_no_eval "$MIX" "no-eval"
OUT="${TMP}/mix.out"
bash "$SCRIPT" --check --skills-dir "$MIX" > "$OUT" 2>&1 || true
assert_exit_code 0 "mixed root exits 0" \
    bash "$SCRIPT" --check --skills-dir "$MIX"
assert_grep "mixed: PASS surfaced" "PASS \[has-eval\]" "$OUT"
assert_grep "mixed: SKIP surfaced" "SKIP \[no-eval\] no eval corpus" "$OUT"

# === 6. skill_name mismatch fails ===
MISMATCH="${TMP}/mismatch"
make_corpus "$MISMATCH" "alpha" '{ "skill_name": "beta", "evals": [ { "id": 1, "prompt": "p", "expected_output": "e" } ] }'
OUT="${TMP}/mismatch.out"
bash "$SCRIPT" --check --skills-dir "$MISMATCH" > "$OUT" 2>&1 || true
assert_exit_code 1 "skill_name mismatch exits 1" \
    bash "$SCRIPT" --check --skills-dir "$MISMATCH"
assert_grep "mismatch reported" "does not match skill folder" "$OUT"

# === 7. Empty prompt fails ===
EMPTYP="${TMP}/emptyprompt"
make_corpus "$EMPTYP" "gamma" '{ "skill_name": "gamma", "evals": [ { "id": 1, "prompt": "", "expected_output": "e" } ] }'
assert_exit_code 1 "empty prompt exits 1" \
    bash "$SCRIPT" --check --skills-dir "$EMPTYP"

# === 8. Missing expected_output fails ===
NOEXP="${TMP}/noexp"
make_corpus "$NOEXP" "delta" '{ "skill_name": "delta", "evals": [ { "id": 1, "prompt": "p" } ] }'
assert_exit_code 1 "missing expected_output exits 1" \
    bash "$SCRIPT" --check --skills-dir "$NOEXP"

# === 9. Duplicate ids fail ===
DUP="${TMP}/dup"
make_corpus "$DUP" "epsilon" '{ "skill_name": "epsilon", "evals": [ { "id": 1, "prompt": "p", "expected_output": "e" }, { "id": 1, "prompt": "q", "expected_output": "f" } ] }'
OUT="${TMP}/dup.out"
bash "$SCRIPT" --check --skills-dir "$DUP" > "$OUT" 2>&1 || true
assert_exit_code 1 "duplicate ids exit 1" \
    bash "$SCRIPT" --check --skills-dir "$DUP"
assert_grep "duplicate id reported" "duplicate or missing .id" "$OUT"

# === 10. Empty evals array fails ===
EMPTYE="${TMP}/emptyevals"
make_corpus "$EMPTYE" "zeta" '{ "skill_name": "zeta", "evals": [] }'
assert_exit_code 1 "empty evals array exits 1" \
    bash "$SCRIPT" --check --skills-dir "$EMPTYE"

# === 11. Malformed JSON fails ===
BADJSON="${TMP}/badjson"
make_corpus "$BADJSON" "eta" '{ "skill_name": "eta", "evals": [ { "id": 1, '
assert_exit_code 1 "malformed JSON exits 1" \
    bash "$SCRIPT" --check --skills-dir "$BADJSON"

# === 12. No skill folders at all is OK (nothing to validate) ===
EMPTY="${TMP}/none"
mkdir -p "$EMPTY"
OUT="${TMP}/none.out"
bash "$SCRIPT" --check --skills-dir "$EMPTY" > "$OUT" 2>&1 || true
assert_exit_code 0 "no skill folders exits 0" \
    bash "$SCRIPT" --check --skills-dir "$EMPTY"
assert_grep "no-skill-folders message" "no skill folders found" "$OUT"

# === 13. assertions-present count surfaced ===
OUT="${TMP}/valid2.out"
bash "$SCRIPT" --check --skills-dir "$VALID" > "$OUT" 2>&1 || true
assert_grep "with-assertions count surfaced" "with assertions" "$OUT"

# === 14. <tool>/skill/SKILL.md layout names the skill after <tool> ===
TOOLROOT="${TMP}/toolroot"
mkdir -p "${TOOLROOT}/mytool/skill/evals"
printf '# mytool\n' > "${TOOLROOT}/mytool/skill/SKILL.md"
printf '%s\n' '{ "skill_name": "mytool", "evals": [ { "id": 1, "prompt": "p", "expected_output": "e" } ] }' \
  > "${TOOLROOT}/mytool/skill/evals/evals.json"
OUT="${TMP}/tool.out"
bash "$SCRIPT" --check --skills-dir "$TOOLROOT" > "$OUT" 2>&1 || true
assert_exit_code 0 "tool/skill layout validates" \
    bash "$SCRIPT" --check --skills-dir "$TOOLROOT"
assert_grep "tool/skill named after parent tool" "PASS \[mytool\]" "$OUT"

_test_report
