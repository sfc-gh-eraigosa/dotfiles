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
| build | (create per IMPLEMENTATION §2, `--base` the `sdk-tui` lib branch) | | | | blocked on `sdk-tui` |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 1 adopt libs/tui: keymap + nav.Cursor + help rebind | todo | | | rewires `TestHelpOverlayFromDetail` `h`→`?`; replaces cursor/scrollTop/lastInner |
| 2 `/` search over `search.State` (auto-expand, n/N, :noh, gutter) | todo | | | |
| 3 `:` over `cmdline` (set/unset validation, Tab) | todo | | | adds `resolve.Resolved.WithNamespace` (keep resolve ≥ 95%) |
| 4 docs + key-table pin test + gates + demo | todo | | | Step 5 is human-evidenced (real terminal) |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 vim motions | [ ] `TestVim*`, `TestPickerJKMoveCursor` | [ ] demo `gg`/`G` | |
| F2 help rebind | [ ] `TestHelpOpensOnQuestionAndF1NotH`, `TestHDoesNotOpenHelpInPickerOrDetail` | [ ] | |
| F3 `/` prompt | [ ] `TestSlashSearch*` | [ ] demo `/wispr` | |
| F4 haystack + auto-expand | [ ] `TestSlashSearchExpandsAreaAndJumpsToFirstMatch`, `TestSearchScopeIsTheCurrentPage`, `TestMatchItemPathOrDescription` | [ ] demo (collapsed `install` expands) | |
| F5 n/N + :noh | [ ] `TestSlashSearchEnterCommitsAndNNHop`, `TestEscInListClearsHighlightsButKeepsPatternForN`, `TestNWithoutPatternIsNoop` | [ ] demo `n` | |
| F6 `:` commands | [ ] `TestColon*`, `TestParseCommand`, `TestParseValue*`, `TestFindKeyScopedAndQualified` | [ ] demo `:set`/`:unset` | |
| F7 Tab completion | [ ] `TestColonTab*` | [ ] demo Tab | |
| F8 prompt/gutter rendering | [ ] `TestSearchPromptKeepsFrameWithinHeight` + `gutterLines` asserts | [ ] | |
| F9 docs agree | [ ] `TestTUIHelpListsVimSearchAndCommandKeys`, `TestFooterHintRendersFromTheKeymap` | [ ] README reviewed | |

## 3. Validation done-when — the stop condition

- [ ] `sdk-tui` TRACKING §3 fully ticked (the lib this build consumes)
- [ ] Tasks 1–4 `done` with commit SHAs and evidence files under `evidence/task<N>/`
- [ ] `go test ./... -cover`: module ≥ 90%, `internal/tui` ≥ 91.3%, `internal/resolve` ≥ 95% (`evidence/task7/coverage-gate.txt`)
- [ ] `go vet ./...` clean
- [ ] `evidence/demo/transcript.txt` + `README.md` committed; flipped flag restored
- [ ] `docs/mbo/index.md` row `gff-tui-vim` → `in-review` with the build PR number
- [ ] build draft PR promoted to ready

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-09-05 | planning | Design approved in chat (`:` = ex command line; vim `h/l` win over `h`=help; `/` auto-expands). Spec, plan, trio written on the `design` worker. |
| 2026-09-05 | re-plan | Owner review: shared TUI behaviors → new blocking objective `sdk-tui` (`sdk/libs/tui`). Plan re-cut 7→4 tasks to consume the lib; build worker will stack on `feature/sdk-tui/<user>/lib`. |
