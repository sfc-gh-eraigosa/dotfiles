# sdk-tui — live state ledger

- **Slug:** `sdk-tui`
- **Started:** 2026-09-05 (planning) · build 2026-09-05 (session 1) · **in-review** 2026-09-05
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../sdk-tui.md`](../sdk-tui.md) · spec [`../../specs/sdk-tui.md`](../../specs/sdk-tui.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a
> commit SHA **and** evidence. Never write a result you did not observe.

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| design (docs + GUIDE) | `sdk-tui/edward-raigosa/design` | `feature/sdk-tui/edward-raigosa/design` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/sdk-tui/edward-raigosa/design` | [#286](https://github.com/sfc-gh-eraigosa/dotfiles/pull/286) (draft) | docs written; issue #283 |
| lib | `sdk-tui/edward-raigosa/lib` | `feature/sdk-tui/edward-raigosa/lib` | `$HOME/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/sdk-tui/edward-raigosa/lib` | [#288](https://github.com/sfc-gh-eraigosa/dotfiles/pull/288) (base = design branch; draft — promotion is token-gated, owner runs the `--ready` promote for worker `sdk-tui/edward-raigosa/lib`) | Tasks 1–7 done 2026-09-05; base `feature/sdk-tui/edward-raigosa/design` @ `aa2d78a` |

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
| 6 `overlay` | done | `74b13fc` | RUN-RED `go test ./tui/overlay/` → `undefined: Help` (`evidence/task6/run-red.txt`); RUN-GREEN `go test ./tui/overlay/ -cover -v` → 4/4 PASS, `coverage: 95.7%` (`evidence/task6/go-test.txt`); `go vet` clean; `grep -rln lipgloss --include=*.go tui/` → 0 files (the only two mentions are GUIDE.md prose) | |
| 7 example + docs + gates + demo | done | `256388e` | RUN-RED `go test -tags example ./tui/example/` → `undefined: newModel` (`task7/run-red.txt`); first GREEN run FAILED (`usage: :mark <row>` — see §4); RUN-GREEN → PASS (`task7/example-test.txt`); `go test ./... -cover` → 7/7 packages ok, every `tui/*` ≥ 94.0% (`task7/go-test-all.txt`); `go vet ./...` + `go vet -tags example ./tui/example/` clean; libs module total **94.9%** vs floor 80% (`task7/libs-coverage.txt`); `make lint-go` → 8/8 modules `0 issues`, exit 0 (`task7/lint-go.txt`); gss size raw +336 B, normalized builds byte-identical (`deps/README.md`); real-terminal demo `demo/{run.sh,transcript.txt,README.md}` — 14 steps observed | Repo-wide `COVERAGE_ENFORCE=1 ./scripts/test.sh unit` cannot complete on this host (§4 B1); consumer go.mod tidy (§4 B2) |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 keymap | [x] `TestLookupUsesRealKeyNames` … `TestDispatchCallsTheBoundHandlerOnce` (task2/go-test.txt) | [x] demo `?` overlay lists every binding; footer hint | |
| F2 nav | [x] `TestMoveClampsBothEnds` … `TestKeyWithoutChordInMap` (task3/go-test.txt) | [x] demo `jj`→row 03, `ctrl+d`→row 11, `gg`→row 01 | |
| F3 prompt | [x] `TestLineEditing`, `TestLineDoesNotConsumeModeKeys`, `TestLineRenderResetAtEnd` (task1/go-test.txt) | [x] demo `/25▌`, `:mark row 01▌` | |
| F4 compile | [x] `TestCompileSmartcaseAndErrors` (task4/go-test.txt) | [x] demo `/25` matches `row 25` | |
| F5 search state | [x] `TestTypingRecomputes…`, `TestFirstAndNextWrap`, `TestCommitCancelHideRearmBadge`, `TestModeKeysAreIgnoredNotTyped` (task4/go-test.txt) | [x] demo `/25` parks on row 25, `[1/1]` badge, `n` wraps | |
| F6 parse/registry | [x] `TestParse`, `TestRegistryRunAliasesAndUnknown` (task5/go-test.txt) | [x] demo `:q` exits, `:mark` runs | |
| F7 completion | [x] `TestTabCompletes…` ×2, `TestStateSubmitCancelEmpty` (task5/go-test.txt) | [x] demo `:mark ` Tab → `row 01` | |
| F8 help | [x] `TestHelpRendersEveryBindingSectionsAndHint`, `TestHelpUsesThePalette` (task6/go-test.txt) | [x] demo `?` overlay | |
| F9 confirm | [x] `TestConfirmKeys`, `TestConfirmRender` (task6/go-test.txt) | [x] demo `d` → `delete row 25?`, `x` → `cancelled` | |
| F10 example | [x] `TestExampleComposesAllPackages` (task7/example-test.txt) | [x] `demo/transcript.txt` | |
| F11 docs/gates | [x] libs-coverage.txt (94.9%), lint-go.txt (exit 0); coverage-gate.txt documents the host-side abort | [ ] AGENTS rows reviewed (owner) | |

## 3. Validation done-when — the stop condition

- [x] Tasks 1–7 `done` with commit SHAs and evidence files (`e13455a` `1238136` `65f7cb4` `7fb006b` `dce3f46` `74b13fc` `256388e`)
- [x] every `sdk/libs/tui/*` package ≥ 90% (`evidence/task{1..6}/go-test.txt`; lowest: search 94.0%)
- [x] libs floor: **94.9% ≥ 80%** by the script's own method (`evidence/task7/libs-coverage.txt`); the repo-wide script run aborts on this host at `gss/TestIsDirty`, reproduced on clean `main` (`coverage-gate.txt`, §4 B1) — CI runs it with its own hooks
- [x] `make lint-go` clean (`evidence/task7/lint-go.txt`); `go vet` clean incl. `-tags example`
- [x] `evidence/demo/{README.md,transcript.txt,run.sh}` from a real terminal (tmux pane, foreground)
- [x] `evidence/deps/gss-size-before.txt` vs `gss-size.txt`: raw +336 B explained; normalized builds byte-identical (`deps/README.md`)
- [x] plan §3 unchanged since Task 2 — every signature landed verbatim
- [x] `docs/mbo/index.md` row `sdk-tui` → `in-review` with lib PR #288 · [ ] draft PR promoted — the approval-token gate (safety_guard §10) refused the build agent; owner runs the two-call recipe

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |
| 2026-09-05 | 7 | **B1** repo-wide coverage gate cannot complete on this host | `COVERAGE_ENFORCE=1 ./scripts/test.sh unit` (1) with the env default `GOTOOLCHAIN=local`: `go: go.mod requires go >= 1.26.3 (running go 1.26.1; GOTOOLCHAIN=local)` at `fleet` — `main` already requires 1.26.3 for fleet/gsl/wlink; (2) with `GOTOOLCHAIN=auto`: modules run in alphabetical order and the script stops at `gss`: `--- FAIL: TestIsDirty … status_test.go:54: Failed to commit: exit status 1` — the same command on clean `main` @ `93e0d02` fails identically (the test commits in a temp repo; the host's global `core.hooksPath` pre-commit hook rejects it). `libs` is never reached. | Not caused by this change. libs floor proven with the script's own method: 94.9% (`task7/libs-coverage.txt`). Leave to CI (`make unit-test`), whose hooks are the repo's. |
| 2026-09-05 | 7 | **B2** libs' new deps broke two consumers' read-only builds | `make lint-go`: `golangci-lint run (sdk/tmux-mgr)` and `(sdk/wlink)` → `context loading failed: no go files to analyze: running go mod tidy may solve the problem`; `cd sdk/tmux-mgr && go build ./...` → `go: updates to go.mod needed; to update it: go mod tidy`. Cause: bubbletea pulls `golang.org/x/sys v0.36.0` into `libs/go.mod`; tmux-mgr and wlink (`replace … => ../libs`) pinned `v0.13.0 // indirect`. gss/fleet/gsl already had ≥ v0.36 and were unaffected (gss binary byte-identical, `deps/README.md`). | `go mod tidy` in `sdk/tmux-mgr` and `sdk/wlink` (one line each: `x/sys v0.13.0 → v0.36.0 // indirect` + go.sum) included in the Task 7 commit — outside the plan §6.1 path list, flagged to the owner in the session report. `make lint-go` then 8/8 clean. |
| 2026-09-05 | 7 | (note, not blocking) plan Task 7 example code defects | (a) `:mark row 25` → `usage: :mark <row>`: row names contain a space, `cmdline.Parse` splits on whitespace. Fixed in the example only: `Run` re-joins args, `Complete` completes whole names at argIdx 0 and the number token at argIdx 1 (test unchanged, passes). (b) `SetHeight(msg.Height-4)` but the view has 5 chrome lines → the title scrolls off once a status line shows (demo README). Left as is: the plan's test pins the `-4` arithmetic. | Example-only; `gff-tui-vim` should count its chrome per GUIDE §8. |
| 2026-09-05 | 7 | (follow-up for consumers) multi-rune `KeyMsg` batches | First demo run used `tmux send-keys 'jj'`: bubbletea delivered ONE `KeyMsg{Runes: "jj"}`; `keymap.Lookup` (correctly) found no binding, so `jj`, `/25`, `:mark`, `:q` did nothing — and `gg` moved to the top only because the batch string equalled the chord name. Typing one char per event (`send-keys -l`) reproduces human input and every step works (`demo/transcript.txt`). | Not a lib defect (real keyboards send one rune per event); paste-shaped input is a tool decision. Recommend `gff-tui-vim` splits `len(msg.Runes) > 1` batches in normal mode or documents paste-as-text in prompts. No §3 change. |

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
| 2026-09-05 | build (session 1) | Task 6 pushed to PR #288. Task 7: example green after fixing the plan's `:mark` sample for space-containing row names; AGENTS rows added; gates run (libs 94.9%, lint 8/8, gss normalized byte-identical); B1/B2 filed in §4; real-terminal tmux demo captured (14 steps) after switching the driver to one-char-per-event typing. |
| 2026-09-05 | build (session 1) | Task 7 pushed (`256388e`). `docs/mbo/index.md` → in-review with lib PR #288; PR promotion left to the owner (approval-token gate). Objective gate (IMPLEMENTATION §4) green except the host-only repo-wide coverage script (§4 B1, left to CI). Next: `gss feature start gff-tui-vim` (the feature is not in the registry — the shared registry lost rows to an earlier `audit --repair`) then `gss feature worker add --feature gff-tui-vim --purpose build --base feature/sdk-tui/edward-raigosa/lib`. |
