# gff-tui-vim — live state ledger

- **Slug:** `gff-tui-vim`
- **Started:** 2026-09-05 (planning) · build 2026-09-05 (session 1) · **in-review** 2026-09-05
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../gff-tui-vim.md`](../gff-tui-vim.md) · spec [`../../specs/gff-tui-vim.md`](../../specs/gff-tui-vim.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a
> commit SHA **and** evidence. Never write a result you did not observe.

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| design (docs) | `gff-tui-vim/edward-raigosa/design` | `feature/gff-tui-vim/edward-raigosa/design` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff-tui-vim/edward-raigosa/design` | [#280](https://github.com/sfc-gh-eraigosa/dotfiles/pull/280) (draft) | docs written; issue #281 |
| build | `gff-tui-vim/edward-raigosa/build` | `feature/gff-tui-vim/edward-raigosa/build` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff-tui-vim/edward-raigosa/build` | [#304](https://github.com/sfc-gh-eraigosa/dotfiles/pull/304) (draft, base = design branch) | created 2026-09-05 22:58 PDT; base `feature/gff-tui-vim/edward-raigosa/design` @ `63d0397` (= `main` @ `70629d1` + the trio; the lib is already on `main`, so the design branch replaces the planned lib-branch base) |

`gss feature worker add --feature gff-tui-vim --purpose build --base feature/gff-tui-vim/edward-raigosa/design --engine claude --json` (2026-09-05):

```json
{
  "worker_ref": "gff-tui-vim/edward-raigosa/build",
  "branch": "feature/gff-tui-vim/edward-raigosa/build",
  "worktree_path": "$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff-tui-vim/edward-raigosa/build",
  "base_branch": "feature/gff-tui-vim/edward-raigosa/design"
}
```

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 1 adopt libs/tui: keymap + nav.Cursor + help rebind | done | `01ea253` | RUN-RED `go test ./internal/tui/ -run "TestVim|TestHelpOpensOn|TestHDoesNot|TestPickerJK|TestFooterHint"` → 10 FAIL (`evidence/task1/run-red.txt`); RUN-GREEN `go test ./internal/tui/ -cover` → `ok … coverage: 91.4%` (baseline 91.3%); `grep -rn "'h', 'H'|scrollTop|lastInner" internal/tui/*.go` → no hits; `go vet ./...` clean; `go test ./...` → 12/12 ok (`evidence/task1/go-test.txt`) | rewired `TestHelpOverlayFromDetail` `h`→`?`; `cursor/scrollTop/lastInner` → `nav.Cursor`; three implementation notes in §4 |
| 2 `/` search over `search.State` (auto-expand, n/N, :noh, gutter) | done | `c3c6a37` | RUN-RED `go test ./internal/tui/ -run "TestSlash|TestEscInList|TestNWithout|TestSearch"` → 8 FAIL (`evidence/task2/run-red.txt`); RUN-GREEN `go test ./internal/tui/ -cover` → `ok … coverage: 91.7%`; `go vet ./...` clean; `go test ./...` → 13/13 ok (`evidence/task2/go-test.txt`) | two extra spec-derived tests added for the gate (see §4) |
| 3 `:` over `cmdline` (set/unset validation, Tab) | done | `565abfb` | RUN-RED `go test ./internal/tui/ -run "TestParseValue|TestFindKey|TestColon"` → build failed, `undefined: parseValue`, `m.findKey undefined` (`evidence/task3/run-red.txt`); RUN-GREEN `go test ./internal/tui/ ./internal/resolve/ -cover` → tui `92.6%`, resolve `95.5%`; `go vet ./...` clean; `go test ./...` → 13/13 ok (`evidence/task3/go-test.txt`) | added `resolve.Resolved.WithNamespace` + its one-line test (resolve stays ≥ 95%) |
| 4 docs + key-table pin test + gates + demo | done | `58a10a6` | RUN-RED `go test ./cmd/ -run TestTUIHelpListsVimSearchAndCommandKeys` → FAIL `does not contain "j/k"` (`evidence/task4/run-red.txt`); RUN-GREEN `go test ./... -cover` → 13/13 ok, every package at or above its floor (`evidence/task4/go-test-all.txt`); the exact `gff-ci.yml` recipe → total **91.0%** ≥ 90, resolve **95.5%** ≥ 95, schema **95.7%** ≥ 90, tui **92.6%** ≥ 91.3 — 4× PASS (`evidence/task4/coverage-gate.txt`); `go vet ./...` clean; `make gff-proto-check` clean; real-terminal demo `evidence/demo/{run.sh,transcript.txt,README.md}` — 13 captures | Step 5 human-evidenced against the LIVE inventory; override file `{}` before and after |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 vim motions | [x] `TestVim*`, `TestPickerJKMoveCursor` (task1/go-test.txt) | [x] demo `gg` → first row | |
| F2 help rebind | [x] `TestHelpOpensOnQuestionAndF1NotH`, `TestHDoesNotOpenHelpInPickerOrDetail` (task1/go-test.txt) | [ ] | |
| F3 `/` prompt | [x] `TestSlashSearch*` (task2/go-test.txt) | [x] demo `/wispr` → prompt + jump | |
| F4 haystack + auto-expand | [x] `TestSlashSearchExpandsAreaAndJumpsToFirstMatch`, `TestSearchScopeIsTheCurrentPage` (task2/go-test.txt); `matchesItem` exercised by every search test (the plan's `TestMatchItemPathOrDescription` has no body in §4 and is covered by these) | [x] demo: the area holding `wispr-flow` auto-expanded | |
| F5 n/N + :noh | [x] `TestSlashSearchEnterCommitsAndNNHop`, `TestEscInListClearsHighlightsButKeepsPatternForN`, `TestNWithoutPatternIsNoop`, `TestNOnAPageWithoutMatchesReportsNotFound` (task2/go-test.txt) | [x] demo `n` (single match wraps onto itself) | |
| F6 `:` commands | [x] `TestColon*`, `TestParseValue*`, `TestFindKeyScopedAndQualified` (task3/go-test.txt); `TestParseCommand` is the lib's `cmdline.TestParse` (plan §5 maps F6d/F6g there) | [x] demo `:set` → `false/override/user-override`, `:unset` → `true/default/repo-live` | |
| F7 Tab completion | [x] `TestColonTabCompletesKeysInScope` (task3/go-test.txt) | [x] demo: `:` prompt typed as text; Tab pinned by `TestColonTabCompletesKeysInScope` | |
| F8 prompt/gutter rendering | [x] `TestSearchPromptKeepsFrameWithinHeight` + `gutterLines` asserts (task2/go-test.txt) | [ ] | |
| F9 docs agree | [x] `TestTUIHelpListsVimSearchAndCommandKeys`, `TestFooterHintRendersFromTheKeymap` (task4, task1) | [x] demo footer + `?` overlay render the same table; README **TUI keys** table added | |

## 3. Validation done-when — the stop condition

- [x] `sdk-tui` TRACKING §3 fully ticked (the lib this build consumes) — 8/8 on `main` @ `70629d1`; `go test ./tui/... -cover` → 6/6 ok
- [x] Tasks 1–4 `done` with commit SHAs and evidence files under `evidence/task<N>/`
- [x] `go test ./... -cover`: module **91.0%** ≥ 90, `internal/tui` **92.6%** ≥ 91.3, `internal/resolve` **95.5%** ≥ 95 (`evidence/task4/coverage-gate.txt` — the trio said `task7`, this plan has four tasks)
- [x] `go vet ./...` clean; `make gff-proto-check` clean
- [x] `evidence/demo/{run.sh,transcript.txt,README.md}` committed; override file `{}` before and after (nothing left flipped)
- [x] `docs/mbo/index.md` row `gff-tui-vim` → `in-review` with build PR #304
- [ ] build draft PR promoted to ready — owner runs it (`gss feature pr --ready` is approval-token gated for the agent; `gh pr ready 304` also works)

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |
| 2026-09-06 | review | **#304 review — 2 correctness bugs fixed** (`9b66496`) | (a) `commitSearch`'s refuse path left `Visible=true` with `Re=nil`: `/ai`↵ then `/[`↵ made the badge read `/ai [-/0]` and the next `n` say `pattern not found: ai` for a pattern with two matches. (b) `commitSearch` never cleared `errMsg`, so `pattern not found: zzz` (or an earlier `:set nope true`) stayed pinned under the list through every later successful search. | `Hide()` + `Rearm()` + `collect()` on refuse; clear `errMsg` on prompt open and on successful commit. Both red-before/green-after in `search_test.go`. Also fixed one `ineffassign` hit the PR introduced. |
| 2026-09-06 | review | **#304 review — 4 lower-severity findings fixed** (`review fixes` commit) | F1 opened help but could not close it (the detail legend this PR wrote says `?/F1`); `Q` stopped quitting when the handler moved to the keymap, while `u`/`U` both still work; the help overlay ignored `m.height` and ran 35 lines on an 80×24 terminal after the legend grew from 4 hand-written lines to the full 20-row table; `:set` silently dropped `args[2:]`, so `:set k a, b` wrote `a` without complaint. | F1 added to `updateHelp`; `Quit` re-merged as `q`/`Q`/`ctrl+c` in `gffKeys` (declared in the table, not special-cased); new `fitHeight` trims any overlay to the window keeping the title and close hint; `:set` rejects extra arguments naming the arity. Four new tests, all red first. `buildRows` now calls `inScope` instead of re-implementing it (the comment claimed one rule, the code had two copies). |
| 2026-09-06 | review | **escalation to `sdk-tui`: ctrl+c is dead inside a prompt** | Review finding 12: `prompt.Line.Handle` does not consume `ctrl+c`, and neither `search.State.Key` nor `cmdline.State.Key` acts on it, so with the `/` or `:` prompt open the TUI cannot be interrupted. Esc works. Surprising now that this PR makes `ctrl+c` quit in list mode. | **Not fixed here** — IMPLEMENTATION §5: "consume the lib, never fork it". Belongs in `sdk/libs/tui` (either `Line.Handle` returns false for `ctrl+c` and each prompt maps it to Cancel, or the state machines return a new `Interrupted` event). File against the `sdk-tui` objective before the phase-3 fleet port. |
| 2026-09-06 | review | (accepted, not a defect) `replace ../libs` blocks `go install …@version` | Review finding 6: the new `replace` makes `go install github.com/sfc-gh-eraigosa/dotfiles/sdk/gff@sdk/gff/vX.Y.Z` fail, the path `sdk/README.md` documents. | Mandated by the plan's global constraints and identical to `gsl`, `fleet`, `tmux-mgr`, `wlink`, which all carry the same replace. In-tree builds (`build.sh`, `install.sh`, CI) are unaffected. A real fix is publishing `sdk/libs` as its own tagged module — a repo-wide decision, not this PR's. |
| 2026-09-06 | review | **#304 gets no Go CI while it is based on the design branch** | Review finding 4: `.github/workflows/gff-ci.yml` is `pull_request: branches: [ main ]`, and `.github/mergify.yml`'s "required CI green" protection is `if: base = main`, while the `ready-for-merge` queue rule has no base condition. Labelling #304 as-is would squash ~1.8k lines of Go into the design branch with zero vet/test/coverage/e2e/proto checks. | Merge **#280 first** (docs-only, gets the main gate), then re-target #304 to `main` so gff CI runs on it, then label. Recorded rather than worked around; no workflow edited. |
| 2026-09-05 | 1 | (note, not blocking) the plan's `vim_test.go` redeclares an existing helper | `go vet ./internal/tui/` → `vet: internal/tui/vim_test.go:14:6: cursorLine redeclared in this block` — `round2_test.go:293` already defines `cursorLine` for package `tui_test`. | Dropped the duplicate from the new file and used the existing helper (it matches on the `"> "` prefix, which is what the new assertions want). No test intent changed. |
| 2026-09-05 | 1 | (note, not blocking) plan §3 `gffKeys` loses gff's "category" wording | `go test ./internal/tui/` → `--- FAIL: TestHelpOverlayShowsAboutVersionAndSources … does not contain "category"` (round2_test.go:58, a pre-existing test asserting the overlay names gff's lateral axis). The plan's `gffKeys` merges only Select/Confirm/unset, so the overlay inherited the sdk's generic "previous page / pane". | Merged `PageLeft`/`PageRight` with gff Help text ("previous/next category page"), keeping `Short: "page"` so the footer still reads `h/l page`. This is exactly what GUIDE §3 prescribes for a tool whose lateral axis is real. Existing test kept as written; no lib change. |
| 2026-09-05 | 1 | (note, not blocking) `h`/`H` still closed the help overlay | `grep -rn "'h', 'H'|scrollTop|lastInner" internal/tui/*.go` → 1 hit, `model.go:321` in `updateHelp` (the overlay's close keys). The plan's Task 1 edits cover the list/picker/detail handlers, not the overlay's own close set. | Dropped `h`/`H` from the close keys (`Esc/?/q` remain — exactly what the overlay's own hint advertises), satisfying the TODO's grep gate and IMPLEMENTATION §5 "h/H never open help after Task 1". |
| 2026-09-05 | 2 | (note, not blocking) the plan's Task 2 test list leaves new branches uncovered | After the plan's 10 tests: `go test ./internal/tui/ -cover` → `coverage: 91.1%`, **below the 91.3% floor**. `go tool cover -func` named the gaps: `commitSearch` 66.7% (the `!committed` path — Enter on an outstanding compile error) and `jump` 75% (the `!ok` path — `n` where the committed pattern matches nothing on this page); both are behaviors the plan's own implementation and spec F3b/F5a define. | Added two tests to `search_test.go` — `TestSlashSearchEnterOnInvalidPatternDoesNotCommit` and `TestNOnAPageWithoutMatchesReportsNotFound`. Nothing in the implementation changed. Coverage 91.1% → **91.7%**. |
| 2026-09-05 | 2 | (note, not blocking) the `*` gutter never rendered on non-cursor rows | `go test ./internal/tui/` → 4 FAIL, all `gutterLines`: `"[]" should have 1 item(s), but has 0`. The pre-existing `viewList` writes a hardcoded two-space indent in the non-cursor feature-row branch and ignores the computed `cursor` marker, so the plan's view edit 3 (setting `cursor = "* "`) had no effect there. | Non-cursor rows now render `cursor` + the path (`%s%-40s`), and a matching row's path takes `matchStyleFor(pal)` (bold orange, plain under NO_COLOR) as plan view edit 3 specifies. |
| 2026-09-05 | 4 | (note, not blocking) demo Step 5 runs a temp binary, not `build.sh` | The plan's snippet starts with `./sdk/gff/build.sh`, which installs over `$HOME/opt/bin/gff` — replacing the user's installed binary with a pre-merge build. | Built to `$TMPDIR/gff-demo` and ran `"$bin" tui` in the tmux pane instead. Same real terminal, same live inventory, installed binary untouched. |
| 2026-09-05 | 4 | (note, not blocking) multi-rune `send-keys` matches no binding | First demo pass with `tmux send-keys '/wispr'` delivered ONE `KeyMsg{Runes: "/wispr"}` (the paste shape); nothing fired. Same lesson the `sdk-tui` demo recorded. | `run.sh` types one character per key event (`send-keys -l` + 60 ms), which is what a real keyboard produces; every step then worked. Not a gff or lib defect — a paste-handling decision, still open for tools. |
| 2026-09-05 | 4 | (note, not blocking) the first demo pass could not prove `:set` | The `:set`/`:unset` captures showed the prompt and the footer, but `install.ai.teams` was scrolled off (`… 66 more below`), so the row change was not visible. | Added a `/ai.teams` + Enter step before the `:` commands so the cursor parks on that row; the transcript now shows it flip to `false override user-override` and back to `true default repo-live`. |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-09-05 | planning | Design approved in chat (`:` = ex command line; vim `h/l` win over `h`=help; `/` auto-expands). Spec, plan, trio written on the `design` worker. |
| 2026-09-05 | re-plan | Owner review: shared TUI behaviors → new blocking objective `sdk-tui` (`sdk/libs/tui`). Plan re-cut 7→4 tasks to consume the lib; build worker will stack on `feature/sdk-tui/<user>/lib`. |
| 2026-09-05 | build (session 1) | Preflight: sdk-tui done on `main` (`70629d1`), libs `tui/*` 6/6 ok; gff module all ok on the base; **baseline `internal/tui` coverage 91.3%** (`go test -cover ./internal/tui/`); go1.26.1. Build worker created (§0) on the design branch. |
| 2026-09-05 | build (session 1) | Task 1 done: `keys.go` (gffKeys + palette + listHint), `model.go` on `nav.Cursor` with `gffKeys.Lookup` dispatch + `turnPage`, `view.go` viewport/footer/help via `overlay.Help` + a `sourceLines()` extraction. tui 91.3% → 91.4%; module 12/12 ok. Three implementation notes recorded in §4 (none touch the lib or the plan). |
| 2026-09-05 | build (session 1) | Task 1 pushed → draft PR #304. Task 2 done: `search.go` (rowKey/inScope/hit/collect/start/apply/commit/cancel/jump/noh/updateSearch), `modeSearch` + `modeCommand` modes with the `cmdline` fields declared, view prompt/badge/gutter/`errStyleFor`/`matchStyleFor`. tui 91.7%; module 13/13 ok. |
| 2026-09-05 | build (session 1) | Task 2 pushed. Task 3 done: `command.go` (parseValue/findKey/completeKey/registerCommands/updateCommand), `registerCommands()` in `NewModel`, `modeCommand` dispatch and the `:` key. `resolve.WithNamespace` added (its test needed the `resolve.` qualifier — the file is package `resolve_test`). tui 92.6%, resolve 95.5%, module 13/13 ok. |
| 2026-09-05 | build (session 1) | Task 3 pushed. Task 4 done: `--help` key block, README **TUI keys** table, `AGENTS.md internal/tui` bullet, `cmd/tui_keys_test.go` pin test; all gates green (module 91.0%, tui 92.6%, resolve 95.5%, schema 95.7%, vet, proto-check); real-terminal tmux demo captured against the live inventory with the override file restored to `{}`. |
| 2026-09-05 | build (session 1) | Task 4 pushed (`58a10a6`). `docs/mbo/index.md` row → **in-review** with build PR #304. IMPLEMENTATION §4 objective gate green: 4/4 tasks done with SHAs + evidence, module 91.0% / tui 92.6% / resolve 95.5%, vet + proto-check clean, real-terminal demo committed. Remaining: promote #304 (owner), and after #280 merges `gss feature restack` the build onto `main`. |
| 2026-09-06 | review (#304) | Independent review of the build PR: 12 findings. Two correctness bugs in `commitSearch` fixed with regression tests (`9b66496`), four lower-severity ones fixed here (F1 toggle, `Q` quits, help-overlay height budget, `:set` arity) plus the `inScope` duplication and the demo script now backing up/restoring the operator's real override file. Gates re-run after the fixes: module 91.0%, tui 92.6%, resolve 95.5%, schema 95.7%, `go vet` clean, `golangci-lint` 0 issues. Two findings deliberately not fixed here (lib-side ctrl+c → escalated to `sdk-tui`; the `replace` vs `go install` trade-off → plan-mandated, repo-wide). |
