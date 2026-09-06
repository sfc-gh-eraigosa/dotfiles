# gff-tui-vim — execution cursor

- **Slug:** `gff-tui-vim`
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../gff-tui-vim.md`](../gff-tui-vim.md) — every task/§ reference points there
- **Depends on:** `sdk-tui` ([`../sdk-tui/TODO.md`](../sdk-tui/TODO.md) must be fully ticked first)

> **How to use:** the **first unchecked box is the next action**. Tick a box only after
> you ran the command and read the output. After finishing a `###` task: update
> `TRACKING.md`, commit with the plan's exact message, checkpoint.
>
> **Legend:** `SETUP` prep · `RED` write a failing test · `RUN-RED` run it, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run it, expect PASS · `VERIFY` extra gate ·
> `ALLOWLIST` `.gitignore` check · `DOCS` · `COMMIT` · `LEDGER` update TRACKING.md ·
> `CHECKPOINT` push/PR refresh.

## Preflight (once)

- [x] SETUP: `sdk-tui` TRACKING §3 fully ticked; `cd sdk/libs && go test ./tui/... -cover` → all ok ≥ 90%
- [x] SETUP: `cd sdk/gff && go version` → go1.26; `go test ./... 2>&1 | tail -3` → all ok on the base
- [x] SETUP: `go test -cover ./internal/tui/` → record the baseline (≥ 91.3%) in TRACKING §5
- [x] SETUP: create the build worker `--base feature/sdk-tui/<user>/lib` (IMPLEMENTATION §2), paste its `--json` into TRACKING §0, `cd` into it
- [x] ALLOWLIST: `git status --short -- docs/mbo/plans/gff-tui-vim/evidence/` after the first evidence file

---

### Task 1 — adopt libs/tui: keymap + nav.Cursor + help rebind  (plan Task 1)

- [x] RED: `internal/tui/vim_test.go` (11 tests, plan Task 1 Step 1); `round2_test.go:68` `'h'` → `'?'`
- [x] RUN-RED: `go mod edit … && go mod tidy && go test ./internal/tui/ -run 'TestVim|TestHelpOpensOn|TestHDoesNot|TestPickerJK|TestFooterHint'` → expect **FAIL**
- [x] GREEN: `internal/tui/keys.go`; `model.go` edits 1–4 (`cur nav.Cursor`, Lookup switch, `turnPage`, picker/detail `j/k` + `?`/F1); `view.go` edits 1–4 (viewport via `cur.Visible`, footer `listHint()`, `overlay.Help` + SOURCES)
- [x] RUN-GREEN: `mkdir -p $EV/task1 && go test ./internal/tui/ -cover 2>&1 | tee $EV/task1/go-test.txt | tail -3 && go vet ./...` → **ok**, ≥ 91.3%
- [x] VERIFY: `grep -rn "'h', 'H'\|scrollTop\|lastInner" internal/tui/*.go` → no hits
- [x] COMMIT: `feat(gff/tui): adopt libs/tui — keymap + nav.Cursor vim motions; help moves to ? and F1`
- [x] LEDGER + CHECKPOINT

**Done when:** package green incl. the rewired help test; grep clean; go.mod delta is only `sdk/libs`.

### Task 2 — `/` search over `search.State`  (plan Task 2)

- [x] RED: `internal/tui/search_test.go` (10 tests; `typeKeys`, `gutterLines` helpers)
- [x] RUN-RED: `go test ./internal/tui/ -run 'TestSlash|TestEscInList|TestNWithout|TestSearch'` → expect **FAIL**
- [x] GREEN: `internal/tui/search.go`; `model.go` edits 1–4; `view.go` edits 1–4
- [x] RUN-GREEN: `mkdir -p $EV/task2 && go test ./internal/tui/ -cover 2>&1 | tee $EV/task2/go-test.txt | tail -3 && go vet ./...` → **ok**, ≥ 91.3%
- [x] COMMIT: `feat(gff/tui): / search via libs/tui/search — auto-expand, n/N, :noh, match gutter`
- [x] LEDGER + CHECKPOINT

**Done when:** all search tests pass; frame-height test passes at height 8.

### Task 3 — `:` over `cmdline`  (plan Task 3)

- [x] RED: `internal/tui/command_internal_test.go` (3 tables) + `internal/tui/command_test.go` (8 tests); `resolve.WithNamespace` + its one-line test
- [x] RUN-RED: `go test ./internal/tui/ -run 'TestParseValue|TestFindKey|TestColon'` → expect **FAIL**
- [x] GREEN: `internal/tui/command.go`; `model.go` (`registerCommands` in `NewModel`, dispatch, `:` key)
- [x] RUN-GREEN: `mkdir -p $EV/task3 && go test ./internal/tui/ ./internal/resolve/ -cover 2>&1 | tee $EV/task3/go-test.txt | tail -3 && go vet ./...` → **ok** ×2; tui ≥ 91.3%, resolve ≥ 95%
- [ ] COMMIT: `feat(gff/tui): : command line via libs/tui/cmdline — set/unset with typed validation, Tab completion`
- [ ] LEDGER + CHECKPOINT

**Done when:** all `TestColon*` pass; `:/re` equivalence test passes.

### Task 4 — docs, pin test, gates, demo  (plan Task 4)

- [ ] RED: `cmd/tui_keys_test.go`
- [ ] RUN-RED: `go test ./cmd/ -run TestTUIHelpListsVimSearchAndCommandKeys` → expect **FAIL** on `j/k`
- [ ] DOCS: `cmd/tui.go` Long; `README.md` **TUI keys**; `AGENTS.md` `internal/tui` bullet
- [ ] RUN-GREEN: `mkdir -p $EV/task4 && go test ./... -cover 2>&1 | tee $EV/task4/go-test-all.txt | grep -E 'coverage|FAIL'` → every package **ok**
- [ ] VERIFY: the `gff-ci.yml` coverage recipe → `$EV/task4/coverage-gate.txt` ≥ 90%; `go vet ./...`; `make gff-proto-check`
- [ ] VERIFY (human, real terminal): plan Task 4 Step 5 tmux demo → `$EV/demo/{transcript.txt,README.md}`; restore the flipped flag
- [ ] COMMIT: `docs(gff/tui): key table in --help, README, AGENTS from the shared keymap; pin test; demo evidence`
- [ ] LEDGER: TRACKING §1 all `done`, §2 matrix ticked, §3 stop condition
- [ ] DOCS: `docs/mbo/index.md` row `gff-tui-vim` → `in-review` (+ build PR #); COMMIT `docs(mbo): gff-tui-vim → in-review`
- [ ] CHECKPOINT: `gss feature checkpoint` (confirm first); after the lib merges, `gss feature restack <build> --onto main`; promote the draft PR

**Done when:** IMPLEMENTATION §4 objective gate green.
