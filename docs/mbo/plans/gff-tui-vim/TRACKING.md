# gff-tui-vim — live state ledger

- **Slug:** `gff-tui-vim`
- **Started:** 2026-09-05 (planning); build not started
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../gff-tui-vim.md`](../gff-tui-vim.md) · spec [`../../specs/gff-tui-vim.md`](../../specs/gff-tui-vim.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a
> commit SHA **and** evidence. Never write a result you did not observe.

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| design (docs) | `gff-tui-vim/edward-raigosa/design` | `feature/gff-tui-vim/edward-raigosa/design` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff-tui-vim/edward-raigosa/design` | [#280](https://github.com/sfc-gh-eraigosa/dotfiles/pull/280) (draft) | docs written; issue #281 |
| build | `gff-tui-vim/edward-raigosa/build` | `feature/gff-tui-vim/edward-raigosa/build` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff-tui-vim/edward-raigosa/build` | (pending checkpoint) | created 2026-09-05 22:58 PDT; base `feature/gff-tui-vim/edward-raigosa/design` @ `63d0397` (= `main` @ `70629d1` + the trio; the lib is already on `main`, so the design branch replaces the planned lib-branch base) |

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
| 1 adopt libs/tui: keymap + nav.Cursor + help rebind | done | (SHA at the next commit) | RUN-RED `go test ./internal/tui/ -run "TestVim|TestHelpOpensOn|TestHDoesNot|TestPickerJK|TestFooterHint"` → 10 FAIL (`evidence/task1/run-red.txt`); RUN-GREEN `go test ./internal/tui/ -cover` → `ok … coverage: 91.4%` (baseline 91.3%); `grep -rn "'h', 'H'|scrollTop|lastInner" internal/tui/*.go` → no hits; `go vet ./...` clean; `go test ./...` → 12/12 ok (`evidence/task1/go-test.txt`) | rewired `TestHelpOverlayFromDetail` `h`→`?`; `cursor/scrollTop/lastInner` → `nav.Cursor`; three implementation notes in §4 |
| 2 `/` search over `search.State` (auto-expand, n/N, :noh, gutter) | todo | | | |
| 3 `:` over `cmdline` (set/unset validation, Tab) | todo | | | adds `resolve.Resolved.WithNamespace` (keep resolve ≥ 95%) |
| 4 docs + key-table pin test + gates + demo | todo | | | Step 5 is human-evidenced (real terminal) |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 vim motions | [x] `TestVim*`, `TestPickerJKMoveCursor` (task1/go-test.txt) | [ ] demo `gg`/`G` | |
| F2 help rebind | [x] `TestHelpOpensOnQuestionAndF1NotH`, `TestHDoesNotOpenHelpInPickerOrDetail` (task1/go-test.txt) | [ ] | |
| F3 `/` prompt | [ ] `TestSlashSearch*` | [ ] demo `/wispr` | |
| F4 haystack + auto-expand | [ ] `TestSlashSearchExpandsAreaAndJumpsToFirstMatch`, `TestSearchScopeIsTheCurrentPage`, `TestMatchItemPathOrDescription` | [ ] demo (collapsed `install` expands) | |
| F5 n/N + :noh | [ ] `TestSlashSearchEnterCommitsAndNNHop`, `TestEscInListClearsHighlightsButKeepsPatternForN`, `TestNWithoutPatternIsNoop` | [ ] demo `n` | |
| F6 `:` commands | [ ] `TestColon*`, `TestParseCommand`, `TestParseValue*`, `TestFindKeyScopedAndQualified` | [ ] demo `:set`/`:unset` | |
| F7 Tab completion | [ ] `TestColonTab*` | [ ] demo Tab | |
| F8 prompt/gutter rendering | [ ] `TestSearchPromptKeepsFrameWithinHeight` + `gutterLines` asserts | [ ] | |
| F9 docs agree | [ ] `TestTUIHelpListsVimSearchAndCommandKeys`, `TestFooterHintRendersFromTheKeymap` | [ ] README reviewed | |

## 3. Validation done-when — the stop condition

- [x] `sdk-tui` TRACKING §3 fully ticked (the lib this build consumes) — 8/8 on `main` @ `70629d1`; `go test ./tui/... -cover` → 6/6 ok
- [ ] Tasks 1–4 `done` with commit SHAs and evidence files under `evidence/task<N>/`
- [ ] `go test ./... -cover`: module ≥ 90%, `internal/tui` ≥ 91.3%, `internal/resolve` ≥ 95% (`evidence/task7/coverage-gate.txt`)
- [ ] `go vet ./...` clean
- [ ] `evidence/demo/transcript.txt` + `README.md` committed; flipped flag restored
- [ ] `docs/mbo/index.md` row `gff-tui-vim` → `in-review` with the build PR number
- [ ] build draft PR promoted to ready

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |
| 2026-09-05 | 1 | (note, not blocking) the plan's `vim_test.go` redeclares an existing helper | `go vet ./internal/tui/` → `vet: internal/tui/vim_test.go:14:6: cursorLine redeclared in this block` — `round2_test.go:293` already defines `cursorLine` for package `tui_test`. | Dropped the duplicate from the new file and used the existing helper (it matches on the `"> "` prefix, which is what the new assertions want). No test intent changed. |
| 2026-09-05 | 1 | (note, not blocking) plan §3 `gffKeys` loses gff's "category" wording | `go test ./internal/tui/` → `--- FAIL: TestHelpOverlayShowsAboutVersionAndSources … does not contain "category"` (round2_test.go:58, a pre-existing test asserting the overlay names gff's lateral axis). The plan's `gffKeys` merges only Select/Confirm/unset, so the overlay inherited the sdk's generic "previous page / pane". | Merged `PageLeft`/`PageRight` with gff Help text ("previous/next category page"), keeping `Short: "page"` so the footer still reads `h/l page`. This is exactly what GUIDE §3 prescribes for a tool whose lateral axis is real. Existing test kept as written; no lib change. |
| 2026-09-05 | 1 | (note, not blocking) `h`/`H` still closed the help overlay | `grep -rn "'h', 'H'|scrollTop|lastInner" internal/tui/*.go` → 1 hit, `model.go:321` in `updateHelp` (the overlay's close keys). The plan's Task 1 edits cover the list/picker/detail handlers, not the overlay's own close set. | Dropped `h`/`H` from the close keys (`Esc/?/q` remain — exactly what the overlay's own hint advertises), satisfying the TODO's grep gate and IMPLEMENTATION §5 "h/H never open help after Task 1". |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-09-05 | planning | Design approved in chat (`:` = ex command line; vim `h/l` win over `h`=help; `/` auto-expands). Spec, plan, trio written on the `design` worker. |
| 2026-09-05 | re-plan | Owner review: shared TUI behaviors → new blocking objective `sdk-tui` (`sdk/libs/tui`). Plan re-cut 7→4 tasks to consume the lib; build worker will stack on `feature/sdk-tui/<user>/lib`. |
| 2026-09-05 | build (session 1) | Preflight: sdk-tui done on `main` (`70629d1`), libs `tui/*` 6/6 ok; gff module all ok on the base; **baseline `internal/tui` coverage 91.3%** (`go test -cover ./internal/tui/`); go1.26.1. Build worker created (§0) on the design branch. |
| 2026-09-05 | build (session 1) | Task 1 done: `keys.go` (gffKeys + palette + listHint), `model.go` on `nav.Cursor` with `gffKeys.Lookup` dispatch + `turnPage`, `view.go` viewport/footer/help via `overlay.Help` + a `sourceLines()` extraction. tui 91.3% → 91.4%; module 12/12 ok. Three implementation notes recorded in §4 (none touch the lib or the plan). |
