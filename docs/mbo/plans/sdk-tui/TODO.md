# sdk-tui — execution cursor

- **Slug:** `sdk-tui`
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../sdk-tui.md`](../sdk-tui.md) — every task/§ reference points there

> **How to use:** the **first unchecked box is the next action**. Tick a box only after
> you ran the command and read the output. After finishing a `###` task: update
> `TRACKING.md`, commit with the plan's exact message, checkpoint.
>
> **Legend:** `SETUP` prep · `RED` write a failing test · `RUN-RED` run it, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run it, expect PASS · `VERIFY` extra gate ·
> `ALLOWLIST` `.gitignore` check · `DOCS` · `COMMIT` · `LEDGER` update TRACKING.md ·
> `CHECKPOINT` push/PR refresh.

## Preflight (once)

- [x] SETUP: `go version` → go1.26; `golangci-lint version`
- [x] SETUP: from a clean `main` checkout build gss and save `evidence/deps/gss-size-before.txt` (IMPLEMENTATION §1)
- [x] SETUP: create the `lib` worker (IMPLEMENTATION §2), paste its `--json` into TRACKING §0, `cd` into it
- [x] SETUP: `cd sdk/libs && go test ./... | tail -2` → ok on the base
- [x] ALLOWLIST: after the first file under `sdk/libs/tui/` and `evidence/`: `git status --short -- <path>`; `git check-ignore -v` if absent

---

### Task 1 — module wiring + `prompt.Line`  (plan Task 1)

- [x] RED: `tui/prompt/line_test.go` (3 tests, plan Task 1 Step 1)
- [x] RUN-RED: `go get … && go test ./tui/prompt/` → expect **FAIL** `undefined: Line`
- [x] GREEN: `tui/doc.go`, `tui/prompt/line.go`
- [x] RUN-GREEN: `mkdir -p $EV/task1 && go test ./tui/... -cover -v 2>&1 | tee $EV/task1/go-test.txt | grep -E '^(ok|FAIL|---)'` → **ok**, ≥ 90%
- [x] VERIFY: `go vet ./...`; `git diff go.mod` shows only bubbletea + testify
- [x] COMMIT: `feat(libs/tui): module wiring + prompt.Line single-line editor`
- [x] LEDGER + CHECKPOINT

**Done when:** prompt ≥ 90%; go.mod delta is exactly two requirements.

### Task 2 — `keymap`  (plan Task 2)

- [x] RED: `tui/keymap/keymap_test.go` (6 tests)
- [x] RUN-RED: `go test ./tui/keymap/` → expect **FAIL** `undefined: Vim`
- [x] GREEN: `tui/keymap/keymap.go`, `tui/keymap/vim.go`
- [x] RUN-GREEN: `mkdir -p $EV/task2 && go test ./tui/keymap/ -cover -v 2>&1 | tee $EV/task2/go-test.txt | grep -E '^(ok|FAIL|---)'` → **ok**, ≥ 90%
- [x] VERIFY: `Vim.HeaderHint("  ")` equals the GUIDE §7 example (the test pins it)
- [x] COMMIT: `feat(libs/tui): keymap — data-driven bindings, Vim default map, footer/help rows, dispatch`
- [x] LEDGER + CHECKPOINT — **plan §3 is now frozen**

**Done when:** keymap ≥ 90%; TRACKING §5 notes the freeze.

### Task 3 — `nav.Cursor`  (plan Task 3)

- [x] RED: `tui/nav/cursor_test.go` (7 tests)
- [x] RUN-RED: `go test ./tui/nav/` → expect **FAIL** `undefined: Cursor`
- [x] GREEN: `tui/nav/cursor.go`
- [x] RUN-GREEN: `mkdir -p $EV/task3 && go test ./tui/nav/ -cover -v 2>&1 | tee $EV/task3/go-test.txt | grep -E '^(ok|FAIL|---)'` → **ok**, ≥ 90%
- [ ] COMMIT: `feat(libs/tui): nav.Cursor — clamped motion, following viewport, gg chord as state`
- [ ] LEDGER + CHECKPOINT

**Done when:** nav ≥ 90%; `grep -n "^var " tui/nav/cursor.go` → no mutable globals.

### Task 4 — `search`  (plan Task 4)

- [ ] RED: `tui/search/search_test.go` (5 tests)
- [ ] RUN-RED: `go test ./tui/search/` → expect **FAIL** `undefined: Compile`
- [ ] GREEN: `tui/search/search.go`
- [ ] RUN-GREEN: `mkdir -p $EV/task4 && go test ./tui/search/ -cover -v 2>&1 | tee $EV/task4/go-test.txt | grep -E '^(ok|FAIL|---)'` → **ok**, ≥ 90%
- [ ] COMMIT: `feat(libs/tui): search — smartcase compile, incremental matches, n/N wrap, :noh/re-arm, badge`
- [ ] LEDGER + CHECKPOINT

**Done when:** search ≥ 90%; the invalid-regex test proves `Re` is kept.

### Task 5 — `cmdline`  (plan Task 5)

- [ ] RED: `tui/cmdline/cmdline_test.go` (5 tests)
- [ ] RUN-RED: `go test ./tui/cmdline/` → expect **FAIL** `undefined: Parse`
- [ ] GREEN: `tui/cmdline/cmdline.go`
- [ ] RUN-GREEN: `mkdir -p $EV/task5 && go test ./tui/cmdline/ -cover -v 2>&1 | tee $EV/task5/go-test.txt | grep -E '^(ok|FAIL|---)'` → **ok**, ≥ 90%
- [ ] COMMIT: `feat(libs/tui): cmdline — : parser, command registry, standard verbs, Tab completion`
- [ ] LEDGER + CHECKPOINT

**Done when:** cmdline ≥ 90%; both Tab tests pass (argument + command-name completion).

### Task 6 — `overlay`  (plan Task 6)

- [ ] RED: `tui/overlay/overlay_test.go` (4 tests)
- [ ] RUN-RED: `go test ./tui/overlay/` → expect **FAIL** `undefined: Help`
- [ ] GREEN: `tui/overlay/overlay.go`
- [ ] RUN-GREEN: `mkdir -p $EV/task6 && go test ./tui/overlay/ -cover -v 2>&1 | tee $EV/task6/go-test.txt | grep -E '^(ok|FAIL|---)'` → **ok**, ≥ 90%
- [ ] COMMIT: `feat(libs/tui): overlay — palette interface, help from the keymap, confirm dialog`
- [ ] LEDGER + CHECKPOINT

**Done when:** overlay ≥ 90%; `grep -rn lipgloss tui/` → no hits.

### Task 7 — example, docs, gates, demo  (plan Task 7)

- [ ] RED: `tui/example/model_test.go` (build tag `example`)
- [ ] RUN-RED: `go test -tags example ./tui/example/` → expect **FAIL** `undefined: newModel`
- [ ] GREEN: `tui/example/model.go`, `tui/example/main.go`
- [ ] RUN-GREEN: `go test -tags example ./tui/example/ -v | tee $EV/task7/example-test.txt` → **PASS**
- [ ] DOCS: `sdk/libs/AGENTS.md` table row; `sdk/AGENTS.md` Conventions bullet
- [ ] VERIFY: plan Task 7 Step 5 gates → `go-test-all.txt`, `coverage-gate.txt` (COVERAGE_ENFORCE=1), `lint-go.txt`, `deps/gss-size.txt` vs `gss-size-before.txt`
- [ ] VERIFY (human, real terminal): plan Task 7 Step 6 tmux demo → `evidence/demo/{transcript.txt,README.md}`
- [ ] COMMIT: `feat(libs/tui): composition example, AGENTS docs, module gates + demo evidence`
- [ ] LEDGER: TRACKING §1 all `done`, §2 matrix ticked, §3 stop condition
- [ ] DOCS: `docs/mbo/index.md` row `sdk-tui` → `in-review` (+ lib PR #); COMMIT `docs(mbo): sdk-tui → in-review`
- [ ] CHECKPOINT: `gss feature checkpoint` (confirm first); promote the draft PR; then create `gff-tui-vim/build` `--base` this branch

**Done when:** IMPLEMENTATION §4 objective gate green.
