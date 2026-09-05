# sdk-tui — live state ledger

- **Slug:** `sdk-tui`
- **Started:** 2026-09-05 (planning); build not started
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../sdk-tui.md`](../sdk-tui.md) · spec [`../../specs/sdk-tui.md`](../../specs/sdk-tui.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a
> commit SHA **and** evidence. Never write a result you did not observe.

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| design (docs + GUIDE) | `sdk-tui/edward-raigosa/design` | `feature/sdk-tui/edward-raigosa/design` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/sdk-tui/edward-raigosa/design` | (pending checkpoint) | docs written |
| lib | (create per IMPLEMENTATION §2) | | | | not started |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 1 module wiring + `prompt.Line` | todo | | | go.mod: + bubbletea v1.3.10, testify |
| 2 `keymap` | todo | | | **freezes plan §3** |
| 3 `nav.Cursor` | todo | | | extracted from fleet clampViewport/move |
| 4 `search` | todo | | | extracted from fleet compileInto/jumpMatch |
| 5 `cmdline` | todo | | | |
| 6 `overlay` | todo | | | |
| 7 example + docs + gates + demo | todo | | | Step 6 human-evidenced |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 keymap | [ ] `TestLookupUsesRealKeyNames` … `TestDispatchCallsTheBoundHandlerOnce` | [ ] demo `?` overlay | |
| F2 nav | [ ] `TestMoveClampsBothEnds` … `TestKeyWithoutChordInMap` | [ ] demo `jj`, `ctrl+d` | |
| F3 prompt | [ ] `TestLineEditing`, `TestLineDoesNotConsumeModeKeys`, `TestLineRenderResetAtEnd` | [ ] | |
| F4 compile | [ ] `TestCompileSmartcaseAndErrors` | [ ] | |
| F5 search state | [ ] `TestTypingRecomputes…`, `TestFirstAndNextWrap`, `TestCommitCancelHideRearmBadge`, `TestModeKeysAreIgnoredNotTyped` | [ ] demo `/25`, `n` | |
| F6 parse/registry | [ ] `TestParse`, `TestRegistryRunAliasesAndUnknown` | [ ] demo `:q` | |
| F7 completion | [ ] `TestTabCompletes…` ×2, `TestStateSubmitCancelEmpty` | [ ] demo `:mark` Tab | |
| F8 help | [ ] `TestHelpRendersEveryBindingSectionsAndHint`, `TestHelpUsesThePalette` | [ ] demo `?` | |
| F9 confirm | [ ] `TestConfirmKeys`, `TestConfirmRender` | [ ] demo `d`, `x` | |
| F10 example | [ ] `TestExampleComposesAllPackages` | [ ] transcript | |
| F11 docs/gates | [ ] coverage-gate.txt, lint-go.txt | [ ] AGENTS rows reviewed | |

## 3. Validation done-when — the stop condition

- [ ] Tasks 1–7 `done` with commit SHAs and evidence files
- [ ] every `sdk/libs/tui/*` package ≥ 90% (`evidence/task{1..6}/go-test.txt`)
- [ ] `COVERAGE_ENFORCE=1 ./scripts/test.sh unit` green for libs (`evidence/task7/coverage-gate.txt`)
- [ ] `make lint-go` clean (`evidence/task7/lint-go.txt`); `go vet` clean incl. `-tags example`
- [ ] `evidence/demo/{README.md,transcript.txt}` from a real terminal
- [ ] `evidence/deps/gss-size-before.txt` == `gss-size.txt` (or delta explained)
- [ ] plan §3 unchanged since Task 2 (or coordinated `gff-tui-vim` plan edit in this PR)
- [ ] `docs/mbo/index.md` row `sdk-tui` → `in-review` with the lib PR; draft PR promoted

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-09-05 | planning | Owner asked for a shared TUI lib after reviewing the gff plan. Verified fleet already ships the behaviors; extract-and-adopt approved; `:` cmdline in scope; own feature `sdk-tui`. GUIDE, design, spec, plan, trio written on the `design` worker. |
