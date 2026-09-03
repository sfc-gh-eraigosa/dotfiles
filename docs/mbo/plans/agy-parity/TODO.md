# agy-parity — execution cursor

- **Slug:** agy-parity
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../agy-parity.md`](../agy-parity.md) — every task/§ reference points there

> **How to use:** the **first unchecked box is the next action**. Tick a box only after
> you ran the command and read the output. After finishing a `###` task: update
> `TRACKING.md`, commit with the plan's exact message, checkpoint.
>
> **Legend:** `SETUP` prep · `RED` write a failing test · `RUN-RED` run it, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run it, expect PASS · `VERIFY` extra gate ·
> `ALLOWLIST` `.gitignore` check · `DOCS` · `COMMIT` · `LEDGER` update TRACKING.md ·
> `CHECKPOINT` push/PR refresh.

## Preflight (once)

- [x] `git status -sb | head -1` shows `worktree/agy_defaults`
- [x] `jq --version`, `shellcheck --version | head -2`, `python3 -c 'import tomllib'` (optional)
- [x] `gh pr view 269 --json isDraft,state` → draft, OPEN
- [x] Baseline: `bash ai/antigravity/aliases_test.sh | tail -1` → `FAIL=0`; `bash opt/scripts/system/install_antigravity_skills_test.sh | tail -1` → `FAIL=0`
- [x] Read plan §3 and §4

---

### Task 1 — aliases.sh in the Claude shape  (plan Task 1)

- [x] RED: append the F1 cases to `ai/antigravity/aliases_test.sh`; replace the `agy-yolo` assertions with the removal guard
- [x] RUN-RED: `bash ai/antigravity/aliases_test.sh | tail -3` → expect **FAIL** > 0
- [x] GREEN: rewrite `ai/antigravity/aliases.sh` (contract plan §3)
- [x] RUN-GREEN: `bash ai/antigravity/aliases_test.sh | tee -a docs/mbo/plans/agy-parity/evidence/u1-aliases/aliases_test.txt | tail -3` → **FAIL=0**
- [x] VERIFY: `bash -n`, `shellcheck ai/antigravity/aliases.sh`, `make lint-portability | tail -3`
- [x] COMMIT: `feat(agy): agy-config launch config in the claude wrapper shape (#268)`
- [x] LEDGER + CHECKPOINT

**Done when:** aliases test FAIL=0, lint clean, evidence committed.

### Task 2 — settings template seed  (plan Task 2)

- [x] RED: add template assertions (A) + "template NOT applied" (B) to `install_antigravity_skills_test.sh`
- [x] RUN-RED: `bash opt/scripts/system/install_antigravity_skills_test.sh | tail -3` → **FAIL** > 0
- [x] GREEN: create `ai/antigravity/settings.json.template`; installer seeds from it (`jq 'del(._comment)'`)
- [x] RUN-GREEN: installer test | tee `evidence/u2-settings-template/installer_test.txt` → **FAIL=0**
- [x] VERIFY: `jq -e . ai/antigravity/settings.json.template`; shellcheck installer; `git status --short -- ai/antigravity/settings.json.template` (ALLOWLIST)
- [x] COMMIT: `feat(agy): seed agy settings.json from a tracked template on first run (#268)`
- [x] LEDGER + CHECKPOINT

**Done when:** installer test FAIL=0.

### Task 3 — forced deny/ask/allow  (plan Task 3)

- [ ] RED: A deny/ask rows; B pre-seed allow+deny, assert union/replace
- [ ] RUN-RED → **FAIL** > 0
- [ ] GREEN: extend `ai/antigravity/settings.forced.json`
- [ ] RUN-GREEN | tee `evidence/u3-forced-policy/installer_test.txt` → **FAIL=0**
- [ ] VERIFY: `jq -e . ai/antigravity/settings.forced.json`
- [ ] COMMIT: `feat(agy): enforce the repo deny/ask permission policy in agy settings (#268)`
- [ ] LEDGER + CHECKPOINT

**Done when:** installer test FAIL=0.

### Task 4 — hooks.json merge  (plan Task 4)

- [ ] RED: hosts C (herdr + stale guards) and D (invalid JSON) in the installer test
- [ ] RUN-RED → **FAIL** > 0
- [ ] GREEN: render to temp + `jq -s '.[0] * .[1]'`; `.invalid` rename; jq-less fallback; fix comments in `install.sh` + `install_herdr.sh`
- [ ] RUN-GREEN | tee `evidence/u4-hooks-merge/installer_test.txt` → **FAIL=0**
- [ ] VERIFY: `bash ai/claude/scripts/validate_hooks.sh <C>/.gemini/config/hooks.json` exit 0; shellcheck installer
- [ ] COMMIT: `fix(agy): merge hooks.json so foreign named hooks (herdr) survive an install (#268)`
- [ ] LEDGER + CHECKPOINT

**Done when:** installer test FAIL=0; validator exit 0.

### Task 5 — adapter sensitive-root ask  (plan Task 5)

- [ ] RED: create `ai/hooks/antigravity_adapter_test.sh` (plan §4 T5 step 1)
- [ ] RUN-RED: `bash ai/hooks/antigravity_adapter_test.sh | tail -3` → **FAIL** (ask cases)
- [ ] GREEN: sensitive-root block in `ai/hooks/antigravity_adapter.sh`
- [ ] RUN-GREEN | tee `evidence/u4-hooks-merge/adapter_test.txt` → **FAIL=0**
- [ ] VERIFY: shellcheck adapter; `git status --short -- ai/hooks/antigravity_adapter_test.sh` (ALLOWLIST)
- [ ] COMMIT: `feat(agy): ask before file tools touch credential paths (dir_added_guard parity) (#268)`
- [ ] LEDGER + CHECKPOINT

**Done when:** adapter test FAIL=0.

### Task 6 — dotfiles plugin renderer + enable  (plan Task 6)

- [ ] RED: create `opt/scripts/system/render-agy-plugin_test.sh`; add plugin + config.json rows (A, B) to the installer test
- [ ] RUN-RED: both drivers → **FAIL** > 0
- [ ] GREEN: create `opt/scripts/system/render-agy-plugin.sh`; installer calls it + enables in `config.json`
- [ ] RUN-GREEN: both drivers | tee `evidence/u5-u6-plugin/{renderer_test,installer_test}.txt` → **FAIL=0**
- [ ] VERIFY: shellcheck renderer + installer; `make lint-portability | tail -3`; ALLOWLIST both new files
- [ ] COMMIT: `feat(agy): render repo slash commands + account memories as a local agy plugin (#268)`
- [ ] LEDGER + CHECKPOINT

**Done when:** renderer + installer tests FAIL=0; lint clean.

### Task 7 — docs, sanity, gates, live evidence  (plan Task 7)

- [ ] GREEN: sanity_check.sh asserts seeded keys + plugin dir
- [ ] DOCS: `ai/antigravity/AGENTS.md`, `ai/AGENTS.md`, `docs/machine-local-overrides.md`
- [ ] VERIFY: `make shell-test`, `make lint-shell`, `make lint-portability` | tee `evidence/u7-docs-gates/`
- [ ] LIVE: `bash opt/scripts/system/install_antigravity_skills.sh`; capture `agy-config status`, settings jq, hooks keys, plugin ls, bounded `agy -p` probe → `evidence/live/`
- [ ] DOCS: `docs/mbo/index.md` → `in-review`; TRACKING §2/§3 ticked
- [ ] COMMIT: `docs(agy): agy-parity docs, sanity check, and evidence (#268)`
- [ ] LEDGER + CHECKPOINT + PR body refresh

**Done when:** three make gates clean, live transcript committed, PR body current.
