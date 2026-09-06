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
| design (docs + GUIDE) | `sdk-tui/edward-raigosa/design` | `feature/sdk-tui/edward-raigosa/design` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/sdk-tui/edward-raigosa/design` | [#286](https://github.com/sfc-gh-eraigosa/dotfiles/pull/286) (draft) | docs written; issue #283 |
| lib | `sdk-tui/edward-raigosa/lib` | `feature/sdk-tui/edward-raigosa/lib` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/sdk-tui/edward-raigosa/lib` | [#288](https://github.com/sfc-gh-eraigosa/dotfiles/pull/288) (draft, base = design branch) | created 2026-09-05; base `feature/sdk-tui/edward-raigosa/design` @ `aa2d78a`; first checkpoint after Task 2 |

`gss feature worker add --feature sdk-tui --purpose lib --base feature/sdk-tui/edward-raigosa/design --engine claude --json` (2026-09-05):

```json
{
  "worker_ref": "sdk-tui/edward-raigosa/lib",
  "branch": "feature/sdk-tui/edward-raigosa/lib",
  "worktree_path": "$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/sdk-tui/edward-raigosa/lib",
  "base_branch": "feature/sdk-tui/edward-raigosa/design"
}
```

Notes: the `sdk-tui` feature row (and the design worker's row) had been dropped from the shared
gss registry by an earlier `gss feature audit --repair` run from another repo, so the feature was
re-created with `gss feature start sdk-tui --base main` before `worker add`. `worker add` checked the
new worktree out on the **base** branch instead of the lib branch; fixed with
`git checkout -b feature/sdk-tui/edward-raigosa/lib` at the same commit (`aa2d78a`), which matches
the registry row.

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 1 module wiring + `prompt.Line` | done | `e13455a` | RUN-RED `go test ./tui/prompt/` → `undefined: Line` (`evidence/task1/run-red.txt`); RUN-GREEN `go test ./tui/... -cover -v` → `ok … tui/prompt … coverage: 97.3%` (`evidence/task1/go-test.txt`); `go vet ./...` clean | go.mod direct delta = bubbletea v1.3.10 + testify v1.12.1 exactly; `go mod tidy` (go1.26) canonicalizes the directive to `go 1.24.0` and pulls lipgloss only as `// indirect` via bubbletea — nothing in `tui/` imports it |
| 2 `keymap` | done | `1238136` | RUN-RED `go test ./tui/keymap/` → `undefined: Action/Down/Up/PageLeft…` (`evidence/task2/run-red.txt`); RUN-GREEN `go test ./tui/keymap/ -cover -v` → 6/6 PASS, `coverage: 98.5%` (`evidence/task2/go-test.txt`); `go vet` clean; `Vim.HeaderHint("  ")` pinned to the GUIDE §7 footer by `TestHeaderHintGroupsAndOrders` | **freezes plan §3** (see §5 log). `line_test.go` gofmt-aligned in this commit (lint gate) |
| 3 `nav.Cursor` | done | `65f7cb4` | RUN-RED `go test ./tui/nav/` → `undefined: Cursor` (`evidence/task3/run-red.txt`); RUN-GREEN `go test ./tui/nav/ -cover -v` → 7/7 PASS, `coverage: 98.4%` (`evidence/task3/go-test.txt`); `go vet` clean; `grep -c "^var " tui/nav/cursor.go` → 0 | extracted from fleet clampViewport/move; `gg` pending state lives on the value |
| 4 `search` | done | `7fb006b` | RUN-RED `go test ./tui/search/` → `undefined: State` / `undefined: Compile` (`evidence/task4/run-red.txt`); RUN-GREEN `go test ./tui/search/ -cover -v` → 5/5 PASS, `coverage: 94.0%` (`evidence/task4/go-test.txt`); `go vet` clean | extracted from fleet compileInto/jumpMatch; `TestTypingRecomputesAndInvalidKeepsPreviousRe` proves `Re` survives an invalid keystroke |
| 5 `cmdline` | done | `dce3f46` | RUN-RED `go test ./tui/cmdline/` → `undefined: Command` (`evidence/task5/run-red.txt`); first GREEN run FAILED `TestParse/q` (`expected: []string(nil)`, `actual: []string{}`); RUN-GREEN after the fix → 5/5 PASS, `coverage: 96.2%` (`evidence/task5/go-test.txt`); `go vet` clean | **Deviation from the plan's Task 5 Step 3 code (not from §3):** `Parse` returns `Args == nil` when the line has no arguments (`c.Args = f[1:]` only when `len(f) > 1`) — the plan's own `TestParse` expects `Command{Name: "q"}` with nil Args and `assert.Equal` distinguishes nil from empty. Signature unchanged. |
| 6 `overlay` | done | (SHA recorded at the next commit) | RUN-RED `go test ./tui/overlay/` → `undefined: Help` (`evidence/task6/run-red.txt`); RUN-GREEN `go test ./tui/overlay/ -cover -v` → 4/4 PASS, `coverage: 95.7%` (`evidence/task6/go-test.txt`); `go vet` clean; `grep -rln lipgloss --include=*.go tui/` → 0 files (the only two mentions are GUIDE.md prose) | |
| 7 example + docs + gates + demo | todo | | | Step 6 human-evidenced |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 keymap | [x] `TestLookupUsesRealKeyNames` … `TestDispatchCallsTheBoundHandlerOnce` (task2/go-test.txt) | [ ] demo `?` overlay | |
| F2 nav | [x] `TestMoveClampsBothEnds` … `TestKeyWithoutChordInMap` (task3/go-test.txt) | [ ] demo `jj`, `ctrl+d` | |
| F3 prompt | [x] `TestLineEditing`, `TestLineDoesNotConsumeModeKeys`, `TestLineRenderResetAtEnd` (task1/go-test.txt) | [ ] | |
| F4 compile | [x] `TestCompileSmartcaseAndErrors` (task4/go-test.txt) | [ ] | |
| F5 search state | [x] `TestTypingRecomputes…`, `TestFirstAndNextWrap`, `TestCommitCancelHideRearmBadge`, `TestModeKeysAreIgnoredNotTyped` (task4/go-test.txt) | [ ] demo `/25`, `n` | |
| F6 parse/registry | [x] `TestParse`, `TestRegistryRunAliasesAndUnknown` (task5/go-test.txt) | [ ] demo `:q` | |
| F7 completion | [x] `TestTabCompletes…` ×2, `TestStateSubmitCancelEmpty` (task5/go-test.txt) | [ ] demo `:mark` Tab | |
| F8 help | [x] `TestHelpRendersEveryBindingSectionsAndHint`, `TestHelpUsesThePalette` (task6/go-test.txt) | [ ] demo `?` | |
| F9 confirm | [x] `TestConfirmKeys`, `TestConfirmRender` (task6/go-test.txt) | [ ] demo `d`, `x` | |
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
| 2026-09-05 | build (session 1) | Preflight: go1.26.1, golangci-lint v2.0.2; baseline gss size 7675540 B (`evidence/deps/gss-size-before.txt`); feature row re-created, `lib` worker created (§0); libs green on the base (`ok libs/log`). |
| 2026-09-05 | build (session 1) | Task 1 done (prompt.Line 97.3%). Ledger convention: each task commit carries its own TODO/TRACKING update; the task's SHA is written into its row by the following commit. |
| 2026-09-05 | build (session 1) | Task 2 done (keymap 98.5%). **Plan §3 interfaces are now frozen** — `prompt.Line` and `keymap` land exactly as written in the plan; every later change to a §3 signature is a §4 blocker + coordinated `plans/gff-tui-vim.md` edit. Task 1 checkpoint not yet pushed: the session's playground `demo-guard` hook blocks `gss feature checkpoint` for the dotfiles worker and the auto-mode classifier refused the hook's `DEMO_GUARD=skip` escape; owner runs the checkpoint. |
| 2026-09-05 | build (session 1) | Checkpoint after Task 2 succeeded (`--auto`, playground hook skipped, owner-approved) → draft PR #288. Task 3 done (nav 98.4%, zero globals). |
| 2026-09-05 | build (session 1) | Task 3 pushed to PR #288. Task 4 done (search 94.0%). |
| 2026-09-05 | build (session 1) | Task 4 pushed to PR #288. Task 5 done (cmdline 96.2%) with one implementation-level deviation (Parse nil Args) recorded in the task row; plan text untouched. |
| 2026-09-05 | build (session 1) | Task 5 pushed to PR #288. Task 6 done (overlay 95.7%, no lipgloss in Go). |
