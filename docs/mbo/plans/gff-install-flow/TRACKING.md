# gff-install-flow — live state ledger

- **Slug:** gff-install-flow
- **Started:** 2026-07-26
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../gff-install-flow.md`](../gff-install-flow.md) · spec [`../../specs/gff-install-flow.md`](../../specs/gff-install-flow.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a
> commit SHA **and** evidence. Never write a result you did not observe.

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| impl (planning) | `gff-install-flow/edward-raigosa/impl` | `feature/gff-install-flow/edward-raigosa/impl` | _(pruned post-merge)_ | [#193](https://github.com/sfc-gh-eraigosa/dotfiles/pull/193) | merged (7a97171) |
| build | `gff-install-flow/edward-raigosa/build` | `feature/gff-install-flow/edward-raigosa/build` | `${HOME}/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff-install-flow/edward-raigosa/build` | _(first checkpoint)_ | building |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| T1 `opt/lib/winsetup.sh` + test driver (TDD) | done | 5bdcd45 | RED: 17 FAIL (`winsetup.sh: No such file`); GREEN: `bash opt/lib/winsetup_test.sh` → 18 passed, 0 failed; `sh …` → 18/0; `make lint-shell` rc=0, `make lint-portability` rc=0 | Plan-snippet deviation (TDD-caught): `[ -r /dev/tty ]` is a false positive without a ctty (proved: `-r` true, open fails) — winsetup_ask now open-probes `( : < /dev/tty )` |
| T2 `install_windows.sh` `--ask`/`--deferred` split | done | _(this commit)_ | `bash -n` clean; non-WSL smoke `--ask`/`--deferred` both rc=0 silent; greps: winsetup sourced l.48 < first call l.228, WSLENV builder ×1 in `run_windows_customization`, zero SENTINEL refs; lint-shell rc=0, lint-portability rc=0 | shebang fixed to `#!/usr/bin/env bash`; `--ask` deliberately keeps `WIN_SETUP_MARKER` (deferred half of the same run writes it) |
| T3 `install.sh` early export + Windows-last | done | _(this commit)_ | `bash -n` clean; order grep exact (source l.22 → early export l.28 → `--ask` l.80 → bootstrap l.368 → `--deferred` l.632 → banner l.639); lint-shell rc=0, lint-portability rc=0 | replaces the "flags take effect … on the next run" caveat comment — the early export IS the next-run mechanism now |
| T4 PowerShell `-GffEnv` + log + loud UAC failure | done | _(this commit)_ | edits per plan Task 4 (param/seeding/self-elevate passthrough in setup-elevated.ps1; $gffPairs + PassThru + warnings in setup-apps.ps1); **pwsh ABSENT on build host — AST parse DEFERRED to T6 human run** (not faked) | log now `$env:USERPROFILE\setup-elevated.log`; seeding regex `^(GFF_INSTALL_WINDOWS_[A-Z_]+)=(true\|false\|[a-z0-9,-]+)$` fail-open on mismatch |
| T5 docs + ledgers | done | _(this commit)_ | root AGENTS.md bullet rewritten (flow, UAC-at-end, gff-owned [s], per-run deploy, log path); scripts AGENTS.md grepped clean (no edit needed); index.md → building + #194; gff §10 row 5 → RESOLVED via #194 | |
| T6 human validation matrix (owner, real WSL) | todo | — | | 4 runs; capstone = elevated-log wispr SKIP |
| **Objective gate** | todo | — | | see §3 |

## 2. Feature → proof matrix (spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| **F1** early export | [x] T3 order-grep (source→export→ask→bootstrap→deferred verified) | [ ] matrix runs 2–4 (no manual eval anywhere) | |
| **F2** prompt-early / Windows-last | [x] winsetup tests 1–2, 9 (T1) + T2 mode dispatch greps/smoke | [x] matrix run 1: prompt in the first minute, deploy + phases + UAC at the very end, banner after | |
| **F3** gff-owned skip state | [x] winsetup tests 3–8 (T1, 5bdcd45) | [ ] matrix run 3 (`gff list` override, no sentinel) | |
| **F4** UAC argument hand-off | [x] T4 parse — proven live by the run (a parse error WAS caught+fixed first, TRACKING §4) | [x] matrix run 1: elevated log shows `SKIP (gff: install.windows.wispr-flow=false)` at [3/4] | the line unreachable on pre-#194 main |
| **F5** readable elevated log | — (path change) | [x] matrix run 1: `cat /mnt/c/.../setup-elevated.log` read with no elevation; "Details:" names the user-profile path | |
| **F6** loud UAC failure | [x] T4 code (PassThru ExitCode check + catch, rerun hints in both paths) | [ ] optional live decline probe | |

## 3. Validation done-when — the stop condition

- [ ] Tasks 1–5 done with evidence; winsetup driver green under bash AND dash
- [ ] Both lint gates clean on the final tree (`make lint-shell && make lint-portability`)
- [ ] Four-run human matrix captured → `../gff/evidence/F09-gating/gff-install-flow-matrix.txt`
- [ ] Run 1 shows `SKIP (gff: install.windows.wispr-flow=false)` in the **elevated** log
- [ ] gff TRACKING §10 row 5 (UAC env-strip) resolution updated; root AGENTS.md flow text updated
- [ ] `docs/mbo/index.md` state advanced (building → in-review → merged)
- [ ] PR #193 ready (owner-confirmed) and merged (owner-gated)

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |
| 2026-07-26 | T6 run 3 | `[s]` recording failed gff-first: `gff: resolve: unknown flag key: install.windows.desktop-deploy`, sentinel fallback fired. Root cause: install.sh does `cd "${HOME}"` at line ~610, so the deferred-phase `gff set` ran with CWD=$HOME — no repo-live layer, key unknown (the dotfiles repo isn't gff-registered on that host). The bootstrap eval already guards this with a subshell cd; the winsetup calls didn't | owner run 3 tail: `gff: resolve: unknown flag key: install.windows.desktop-deploy` / `gff unavailable; wrote legacy sentinel …` | Fixed TDD: `winsetup_run_gff` subshell-cd's to `WINSETUP_REPO_DIR` (exported = BASE_DIR by install_windows.sh); new test 8b asserts gff's CWD (RED observed, then 20/20 bash+dash). Bonus proof: the sentinel fallback branch held in production (fail-open). Leftover sentinel exercises live migration on the next run |
| 2026-07-26 | T6 run 1 | First deferred run died silently right after "Starting Windows customization...": the T4 edit put a Unicode em-dash INSIDE a `Write-Warning` string; PS 5.1 reads BOM-less .ps1 as ANSI, so the UTF-8 em-dash's 3rd byte (0x94) decodes to a curly right-quote `”` — which PowerShell accepts as a string TERMINATOR → whole-file ParserError cascade; `set -e` then killed install_windows.sh before the `cat /tmp/setup_apps.log`, hiding the error | owner: `cat /tmp/setup_apps.log` → `The string is missing the terminator: "` at setup-apps.ps1:524 + cascade (513/509/500/491), `FullyQualifiedErrorId: TerminatorExpectedAtEndOfString` | Two fixes: (1) all non-ASCII introduced by T4 replaced with ASCII (`--`, `sec`, `...`) in BOTH .ps1 files — strings-in-.ps1 must stay ASCII while the files are BOM-less; (2) the setup-apps invocation now captures rc, ALWAYS cats the log, and warns loud with a rerun hint (the silent-death mode is gone). Prompt-early/early-export/Windows-last/deferred-choice all validated working by the same failed run |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-07-26 | planning | Spec + plan authored and approved (owner); worker + draft PR #193 created; execution trio (this file, IMPLEMENTATION, TODO) scaffolded; templates added to `docs/mbo/templates/` and the mbo-plan skill now requires the trio for build objectives |
| 2026-07-26 | build-1 | Owner merged #193 (planning artifacts on main @ 7a97171) — kickoff's "work in the impl worktree" corrected to the new `build` worker branched from the merged tip (`gss feature merged` run on impl first). T1 done TDD: RED 17-fail observed, GREEN 18/18 under bash AND dash; `[ -r /dev/tty ]` false-positive found empirically and fixed with an open-probe (plan-snippet deviation, recorded on the T1 row). |
