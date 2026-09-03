# agy-parity — live state ledger

- **Slug:** agy-parity
- **Started:** 2026-09-03
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../agy-parity.md`](../agy-parity.md) · spec [`../../specs/agy-parity.md`](../../specs/agy-parity.md)
- **Objective anchors:** issue #268 · PR #269 · `docs/mbo/index.md` row `agy-parity`

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a
> commit SHA **and** evidence. Never write a result you did not observe.

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| all tasks | — (classic lane, no gss feature worker) | `worktree/agy_defaults` | `${HOME}/.herdr/worktrees/dotfiles/worktree-agy-defaults` | [#269](https://github.com/sfc-gh-eraigosa/dotfiles/pull/269) | building |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| T0 spec + plan + trio | done | 1f4a236 | files present under docs/mbo; index row → building | design marked Approved |
| T1 aliases.sh in the Claude shape | done | 37097be | `bash ai/antigravity/aliases_test.sh` → PASS=30 FAIL=0 (RED verified: FAIL=18 before the rewrite); `shellcheck` clean (one pre-existing SC2015 info on the tmux-anchor line); `make lint-portability` rc=0 | evidence/u1-aliases/{aliases_test,lint_gates}.txt; `agy-yolo` removed |
| T2 settings.json.template seed | done | 416d7f9 | installer test → PASS=32 FAIL=0 (RED verified: FAIL=6, all template keys null); `jq -e .` template ok; shellcheck installer clean; `git status --short -- ai/antigravity/settings.json.template` → `??` (allowlisted) | evidence/u2-settings-template/installer_test.txt |
| T3 forced deny/ask/allow | done | c873c35 | installer test → PASS=39 FAIL=0 (RED verified: FAIL=6 — deny/ask absent, host deny not replaced, union missing); `jq -e .` forced ok | evidence/u3-forced-policy/installer_test.txt; apply-forced-settings.sh untouched |
| T4 hooks.json merge | done | 09f5067 | installer test → PASS=47 FAIL=0 (RED verified: FAIL=3 — herdr key dropped, herdr command null, no .invalid); validate_hooks.sh on a merged host (guards+herdr) → rc=0; shellcheck installer + install_herdr clean; `bash -n install.sh` ok | evidence/u4-hooks-merge/{installer_test,validate_hooks}.txt; install.sh + install_herdr.sh comments corrected (ordering no longer load-bearing) |
| T5 adapter sensitive-root ask | done | 73c70d6 | `bash ai/hooks/antigravity_adapter_test.sh` → PASS=11 FAIL=0 (RED verified: FAIL=5, every ask case answered allow); shellcheck adapter + driver clean; installer test still 47/47 | evidence/u4-hooks-merge/adapter_test.txt; new driver allowlisted (`??` in git status) |
| T6 dotfiles plugin renderer + enable | done | 5e62ef0 | renderer test → PASS=24 FAIL=0 (RED: script absent; one wrong assertion fixed — sync.md has a legit `---` rule, leak check moved to frontmatter keys); installer test → PASS=54 FAIL=0 (RED: FAIL=5 — plugin files + config.json enable missing); shellcheck clean; `make lint-shell` rc=0; `make lint-portability` rc=0 | evidence/u5-u6-plugin/{renderer_test,installer_test}.txt; both new files allowlisted |
| T7 docs, sanity, gates, live evidence | done | 717a0da | `make lint-shell` rc=0; `make lint-portability` rc=0; `make shell-test` on this branch → 39 passed, 1 failed (install_ai_teams_test — PRE-EXISTING at the branch base 13a1b90, fixed on origin/main by #263; see §4) and on the branch rebased onto origin/main in a throwaway worktree → **40 passed, 0 failed**; live host: installer run, forced deny/ask in the real settings, hooks.json validates, dotfiles plugin rendered (8 commands, 5 memory sections) + enabled, `agy-config` status/doctor from the copied aliases | evidence/u7-docs-gates/{shell-test,lint}.txt, evidence/live/{install_antigravity_skills,agy-config,settings,hooks,plugin,sanity_check,agy-p-probe}.txt |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 launch config | [x] aliases_test.sh (30/30) | [x] evidence/live/agy-config.txt (yolo OFF, doctor 1 binary) | |
| F2 settings seed | [x] installer test A | [x] evidence/live/settings.txt: existing host, toolPermission absent as expected | |
| F3 forced policy | [x] installer test A+B | [x] evidence/live/settings.txt: deny 11 / ask 7 entries present | agy -p probe confirms agy reads permissions.* (headless auto-denies non-allowlisted commands) |
| F4 hooks merge | [x] installer test C+D | [x] evidence/live/hooks.txt: guards rendered, validate_hooks rc=0 | live file had only `guards` before (no herdr entry present on this host to preserve; unit test C covers it) |
| F5 sensitive-root ask | [x] adapter test (11/11) | [x] — (n/a) | |
| F6 plugin render | [x] renderer test (24/24) | [x] evidence/live/plugin.txt: 8 TOMLs, 5 rule sections | |
| F7 plugin enable | [x] installer test A+B | [x] evidence/live/plugin.txt: plugins.dotfiles.enabled true beside 6 existing plugins | |
| F8 docs + sanity | [x] make gates (lint-shell, lint-portability, shell-test on merged result) | [x] evidence/live/sanity_check.txt: agy steps 7–9 pass standalone | full sanity aborts earlier on a pre-existing Claude-side item (§4) |

## 3. Validation done-when — the stop condition

- [x] T1–T7 rows `done` with SHAs (T7 SHA recorded on the checkpoint commit)
- [x] `make shell-test` clean — on the merged result (branch ⊕ origin/main): 40/40; on the raw branch 39/40, the 1 being pre-existing (§4)
- [x] `make lint-shell` clean
- [x] `make lint-portability` clean
- [x] live-host transcript captured (evidence/live)
- [x] `docs/mbo/index.md` row `in-review`
- [x] PR #269 body refreshed to full scope

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |
| 2026-09-03 | plan | `agy agents` prints nothing under `script -qec` pseudo-TTY | `timeout 30 script -qec "agy agents" /dev/null` → empty | out of scope (spec §8); not a gate |
| 2026-09-03 | T7 | `make shell-test` 39/40 on the branch: `install_ai_teams_test.sh` fails (`ollama standard FROM` expects `qwen2.5-coder:7b`) | rc=1 at the branch base 13a1b90 in a temp worktree; rc=0 on origin/main 2f17b59 (fix landed via #263); this branch never touched `ai/teams` or the teams scripts (`git diff --stat 13a1b90..HEAD -- ai/teams …` empty) | out of scope; proven fixed on the merged result (40/40 in a throwaway worktree rebased onto origin/main); Mergify's in-place update brings the fix in before merge |
| 2026-09-03 | T7 | full `ai/antigravity/scripts/sanity_check.sh` aborts before its agy steps | it delegates to `ai/claude/scripts/sanity_check.sh`, whose `validate_hooks.sh` cannot parse the quoted path in herdr's SessionStart hook command (`bash '~/.claude/hooks/herdr-agent-state.sh' session`) in the live `~/.claude/settings.json` → "configured hook command not executable: 'bash …'" | pre-existing, Claude-side, herdr-integration-vs-validator; out of scope. agy steps 7–9 run standalone: all PASS (evidence/live/sanity_check.txt). Follow-up: teach validate_hooks.sh to shell-split quoted commands |
| 2026-09-03 | T7 | `command(...)` prefix-match semantics still not observable | bounded `agy -p` probes (evidence/live/agy-p-probe.txt): headless mode auto-denies ANY command needing the `command` permission that is not allowlisted, so both the benign `echo` and the deny-listed `rm -rf /tmp/…` were refused before deny rules could be distinguished. The message does confirm agy reads `permissions.allow` from the live settings | unresolved by design (deny rules are additive to the hook); verify interactively when convenient: `agy` → ask it to run `rm -rf /tmp/agy-parity-probe` → expect a deny |
| 2026-09-03 | plan | agy binary has no `{{args}}`/`$ARGUMENTS` substitution token | `strings agy \| grep -oE 'SHORTHAND_ARGS_PLACEHOLDER\|\{\{args\}\}\|\$ARGUMENTS'` → no output | renderer rewrites `$ARGUMENTS` to prose (plan §3) |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-09-02 | analysis | gap analysis of Claude vs agy startup; design doc; issue #268; PR #269 |
| 2026-09-03 | loop-1 | design Approved by user; spec, plan, trio written; index → building; preflight baseline aliases 10/10, installer 24/24; T1 done (RED 18 → GREEN 30/30, 37097be); T2 done (RED 6 → GREEN 32/32, 416d7f9); T3 done (RED 6 → GREEN 39/39, c873c35); T4 done (RED 3 → GREEN 47/47, 09f5067); T5 done (RED 5 → GREEN 11/11, 73c70d6); T6 done (RED 5 → GREEN 24/24 + 54/54, 5e62ef0); T7 done (docs, sanity steps, gates, live evidence; 3 blockers recorded, none in scope); index → in-review |
| 2026-09-03 | loop-2 | CI on PR #269 green: Run Shell Test Drivers, Run Unit Tests, binary e2e, Go Lint, Lint, Shell Lint ×2, teams validation, coverage gate all `pass` (build job + Mergify queue `skipping` — draft-gated). Loop stopped; awaiting review |
