# gff-tui-vim — execution cursor

- **Slug:** `gff-tui-vim`
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../gff-tui-vim.md`](../gff-tui-vim.md) — every task/§ reference points there

> **How to use:** the **first unchecked box is the next action**. Tick a box only after
> you ran the command and read the output. After finishing a `###` task: update
> `TRACKING.md`, commit with the plan's exact message, checkpoint.
>
> **Legend:** `SETUP` prep · `RED` write a failing test · `RUN-RED` run it, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run it, expect PASS · `VERIFY` extra gate ·
> `ALLOWLIST` `.gitignore` check · `DOCS` · `COMMIT` · `LEDGER` update TRACKING.md ·
> `CHECKPOINT` push/PR refresh.

## Preflight (once)

- [ ] SETUP: `cd sdk/gff && go version` → go1.26
- [ ] SETUP: `go test ./... 2>&1 | tail -3` → all `ok` on the base
- [ ] SETUP: `go test -cover ./internal/tui/` → record the baseline (≥ 91.3%) in TRACKING §5
- [ ] SETUP: create the build worker (IMPLEMENTATION §2), paste its `--json` into TRACKING §0, `cd` into its worktree
- [ ] ALLOWLIST: `git status --short -- docs/mbo/plans/gff-tui-vim/evidence/` after creating the first evidence file; `git check-ignore -v` if absent

---

### Task 1 — `lineInput` line editor  (plan Task 1)

- [ ] RED: create `internal/tui/input_test.go` (package `tui`) with `TestLineInputEditing`, `TestLineInputDoesNotConsumeModeKeys`, `TestLineInputRenderAndReset` (plan Task 1 Step 1)
- [ ] RUN-RED: `go test ./internal/tui/ -run 'TestLineInput'` → expect **FAIL** `undefined: lineInput`
- [ ] GREEN: create `internal/tui/input.go` (plan Task 1 Step 3)
- [ ] RUN-GREEN: `mkdir -p $EV/task1 && go test ./internal/tui/ -run 'TestLineInput' -v 2>&1 | tee $EV/task1/go-test.txt | tail -5` → expect **PASS**
- [ ] VERIFY: `go vet ./internal/tui/`
- [ ] COMMIT: `feat(gff/tui): lineInput single-line editor for the / and : prompts`
- [ ] LEDGER + CHECKPOINT

**Done when:** three `TestLineInput*` tests pass; evidence file committed.

### Task 2 — vim motions + help rebind  (plan Task 2)

- [ ] RED: create `internal/tui/vim_test.go` (package `tui_test`) with the 10 tests from plan Task 2 Step 1; edit `round2_test.go:68` `'h'` → `'?'`
- [ ] RUN-RED: `go test ./internal/tui/ -run 'TestVim|TestHelpOpensOn|TestHDoesNot|TestPickerJK'` → expect **FAIL**
- [ ] GREEN: create `internal/tui/keys.go`; apply the `model.go` edits 1–6 and `view.go` edits 1–4 (plan Task 2 Step 3)
- [ ] RUN-GREEN: `mkdir -p $EV/task2 && go test ./internal/tui/ -cover 2>&1 | tee $EV/task2/go-test.txt | tail -3` → expect **ok**, coverage ≥ 91.3%
- [ ] VERIFY: `grep -rn "'h', 'H'" internal/tui/*.go` → no hits (h never opens help)
- [ ] COMMIT: `feat(gff/tui): vim motions (j/k/h/l, gg/G, ^d/^u/^f/^b); help moves to ? and F1`
- [ ] LEDGER + CHECKPOINT

**Done when:** package green incl. the rewired help test; `h` grep clean.

### Task 3 — search primitives  (plan Task 3)

- [ ] RED: create `internal/tui/search_internal_test.go` (package `tui`) with `TestCompilePatternSmartcase`, `TestMatchItemPathOrDescription`, `TestRowKeyDistinguishesAreasAndItems`
- [ ] RUN-RED: `go test ./internal/tui/ -run 'TestCompilePattern|TestMatchItem|TestRowKey'` → expect **FAIL** `undefined: compilePattern`
- [ ] GREEN: create `internal/tui/search.go` (pure half); `buildRows` category branch uses `m.inScope(item)`
- [ ] RUN-GREEN: `mkdir -p $EV/task3 && go test ./internal/tui/ -cover 2>&1 | tee $EV/task3/go-test.txt | tail -3` → expect **ok**
- [ ] COMMIT: `feat(gff/tui): search primitives — smartcase compile, path/description match, row keys, shared inScope`
- [ ] LEDGER + CHECKPOINT

**Done when:** package green after the `inScope` refactor.

### Task 4 — `/` search mode, n/N, :noh, gutter  (plan Task 4)

- [ ] RED: create `internal/tui/search_test.go` (package `tui_test`) with the 10 tests from plan Task 4 Step 1 (`typeKeys`, `gutterLines` helpers)
- [ ] RUN-RED: `go test ./internal/tui/ -run 'TestSlash|TestEscInList|TestNWithout|TestSearch'` → expect **FAIL**
- [ ] GREEN: add the mode methods to `search.go`; `model.go` edits 1–5 (incl. `cmd lineInput` field); `view.go` edits 1–4
- [ ] RUN-GREEN: `mkdir -p $EV/task4 && go test ./internal/tui/ -cover 2>&1 | tee $EV/task4/go-test.txt | tail -3` → expect **ok**, ≥ 91.3%
- [ ] VERIFY: `go vet ./...`
- [ ] COMMIT: `feat(gff/tui): / incremental regex search with auto-expand, n/N, :noh, match gutter`
- [ ] LEDGER + CHECKPOINT

**Done when:** all search tests pass; frame-height test passes at height 8.

### Task 5 — `:` parser + value validation  (plan Task 5)

- [ ] RED: create `internal/tui/command_internal_test.go` (package `tui`) with `TestParseCommand`, `TestParseValueBool`, `TestParseValueChoice`, `TestFindKeyScopedAndQualified`; add `WithNamespace` + its one-line test to `internal/resolve`
- [ ] RUN-RED: `go test ./internal/tui/ -run 'TestParseCommand|TestParseValue|TestFindKey'` → expect **FAIL** `undefined: parseCommand`
- [ ] GREEN: create `internal/tui/command.go` (parser half) (plan Task 5 Step 3)
- [ ] RUN-GREEN: `mkdir -p $EV/task5 && go test ./internal/tui/ ./internal/resolve/ -cover 2>&1 | tee $EV/task5/go-test.txt | tail -3` → expect **ok** ×2; resolve ≥ 95%
- [ ] COMMIT: `feat(gff/tui): :-line parser, typed value validation, scoped key lookup`
- [ ] LEDGER + CHECKPOINT

**Done when:** parser/value/findKey tables pass; resolve coverage bar intact.

### Task 6 — `:` command mode, exec, Tab completion  (plan Task 6)

- [ ] RED: create `internal/tui/command_test.go` (package `tui_test`) with the 11 `TestColon*` tests (`newCmdModel`, `enter` helpers)
- [ ] RUN-RED: `go test ./internal/tui/ -run 'TestColon'` → expect **FAIL**
- [ ] GREEN: add `execCommand/cmdSet/cmdUnset/completion/completeKey/completeCommand/updateCommand` to `command.go`; `model.go` (`comp completion`, `:` key, dispatch); `view.go` command footer branch
- [ ] RUN-GREEN: `mkdir -p $EV/task6 && go test ./internal/tui/ -cover 2>&1 | tee $EV/task6/go-test.txt | tail -3 && go vet ./...` → expect **ok**, ≥ 91.3%
- [ ] COMMIT: `feat(gff/tui): : command line — set/unset/q/help/:/re with Tab key completion`
- [ ] LEDGER + CHECKPOINT

**Done when:** all `TestColon*` pass; `:/re` equivalence test passes.

### Task 7 — docs, key-table test, gates, demo  (plan Task 7)

- [ ] RED: create `cmd/tui_keys_test.go` with `TestTUIHelpListsVimSearchAndCommandKeys`
- [ ] RUN-RED: `go test ./cmd/ -run TestTUIHelpListsVimSearchAndCommandKeys` → expect **FAIL** on `j/k`
- [ ] DOCS: `cmd/tui.go` Long; `README.md` **TUI keys** section; `AGENTS.md` `internal/tui` bullet (plan Task 7 Step 3)
- [ ] RUN-GREEN: `mkdir -p $EV/task7 && go test ./... -cover 2>&1 | tee $EV/task7/go-test-all.txt | grep -E 'coverage|FAIL'` → every package **ok**
- [ ] VERIFY: run the `gff-ci.yml` coverage recipe, `tee $EV/task7/coverage-gate.txt` → module ≥ 90%; `go vet ./...`; `make gff-proto-check` from the repo root
- [ ] VERIFY (human, real terminal): plan Task 7 Step 5 tmux demo → `$EV/demo/transcript.txt` + `README.md`; restore the flipped flag (`gff unset …`)
- [ ] COMMIT: `docs(gff/tui): key table in --help, README, AGENTS; pin with a test; demo evidence`
- [ ] LEDGER: TRACKING §1 all `done`, §2 matrix ticked, §3 stop condition
- [ ] DOCS: `docs/mbo/index.md` row `gff-tui-vim` → `in-review` (+ build PR #); COMMIT `docs(mbo): gff-tui-vim → in-review`
- [ ] CHECKPOINT: `gss feature checkpoint` (confirm first); promote the draft PR when §3 is fully ticked

**Done when:** IMPLEMENTATION §4 objective gate green.
