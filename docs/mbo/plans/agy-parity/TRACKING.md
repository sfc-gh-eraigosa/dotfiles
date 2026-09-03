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
| T3 forced deny/ask/allow | done | _(T3 commit)_ | installer test → PASS=39 FAIL=0 (RED verified: FAIL=6 — deny/ask absent, host deny not replaced, union missing); `jq -e .` forced ok | evidence/u3-forced-policy/installer_test.txt; apply-forced-settings.sh untouched |
| T4 hooks.json merge | todo | | | |
| T5 adapter sensitive-root ask | todo | | | |
| T6 dotfiles plugin renderer + enable | todo | | | |
| T7 docs, sanity, gates, live evidence | todo | | | |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 launch config | [x] aliases_test.sh (30/30) | [ ] `agy-config status` transcript | |
| F2 settings seed | [x] installer test A | [ ] live settings jq (existing host: no seed expected) | |
| F3 forced policy | [x] installer test A+B | [ ] live settings jq shows deny/ask | |
| F4 hooks merge | [ ] installer test C+D | [ ] live `jq keys hooks.json` after herdr integration | |
| F5 sensitive-root ask | [ ] adapter test | [ ] — | |
| F6 plugin render | [ ] renderer test | [ ] `ls plugins/dotfiles/commands` | |
| F7 plugin enable | [ ] installer test A+B | [ ] live `config.json` | |
| F8 docs + sanity | [ ] make gates | [ ] sanity transcript | |

## 3. Validation done-when — the stop condition

- [ ] T1–T7 rows `done` with SHAs
- [ ] `make shell-test` clean (evidence/u7-docs-gates)
- [ ] `make lint-shell` clean
- [ ] `make lint-portability` clean
- [ ] live-host transcript captured (evidence/live)
- [ ] `docs/mbo/index.md` row `in-review`
- [ ] PR #269 body refreshed to full scope

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |
| 2026-09-03 | plan | `agy agents` prints nothing under `script -qec` pseudo-TTY | `timeout 30 script -qec "agy agents" /dev/null` → empty | out of scope (spec §8); not a gate |
| 2026-09-03 | plan | agy binary has no `{{args}}`/`$ARGUMENTS` substitution token | `strings agy \| grep -oE 'SHORTHAND_ARGS_PLACEHOLDER\|\{\{args\}\}\|\$ARGUMENTS'` → no output | renderer rewrites `$ARGUMENTS` to prose (plan §3) |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-09-02 | analysis | gap analysis of Claude vs agy startup; design doc; issue #268; PR #269 |
| 2026-09-03 | loop-1 | design Approved by user; spec, plan, trio written; index → building; preflight baseline aliases 10/10, installer 24/24; T1 done (RED 18 → GREEN 30/30, 37097be); T2 done (RED 6 → GREEN 32/32, 416d7f9); T3 done (RED 6 → GREEN 39/39) |
