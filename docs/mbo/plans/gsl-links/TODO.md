# gsl-links — execution cursor

- **Slug:** gsl-links
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../gsl-links.md`](../gsl-links.md) — every task/§ reference points there

> **How to use:** the **first unchecked box is the next action**. Tick a box only after
> you ran the command and read the output. After finishing a `###` task: update
> `TRACKING.md`, commit with the plan's exact message, checkpoint.
>
> **Legend:** `SETUP` prep · `RED` write a failing test · `RUN-RED` run it, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run it, expect PASS · `VERIFY` extra gate ·
> `ALLOWLIST` `.gitignore` check · `DOCS` · `COMMIT` · `LEDGER` update TRACKING.md ·
> `CHECKPOINT` push/PR refresh.

## Preflight (once)

- [x] `git rev-parse --abbrev-ref HEAD` → `worktree/gsl`, toplevel ends in `worktree-gsl`
- [x] `cd sdk/gsl && go test ./... 2>&1 | tail -3` → baseline green
- [x] `gff version && gff lint` → clean (gff vdev)
- [x] Draft PR exists: #279

---

### Task 1 — spans through join, fit, truncation  (plan Task 1)

- [x] RED: `internal/render/links_test.go` (plan T1 step 1) + `link_test.go` updates
- [x] RUN-RED: `cd sdk/gsl && go test ./internal/render/ -run 'PaintRuns|ValidateSpans|ClipSpans|Reanchor|ShiftSpans|Join_Spans|TruncateToWidth_Clips'` → expect **FAIL** (compile)
- [x] GREEN: `links.go`, `segment.go` (`LinkSpan`, `RenderLinked` sig), `glyphs.go` (`paintRuns`, `segmentBlock.links`), `style.go` (`Links`), `detect.go` (`linkedFormatter`, `formatLinkedOf`, `Format`, `finalTierBlocks`), `truncate.go`, `render.go`, `seg_repo.go`/`seg_repo_data.go` one-span shim
- [x] RUN-GREEN: `cd sdk/gsl && go test ./internal/render/ -race` → expect **PASS**
- [x] VERIFY: `go vet ./...`; evidence `tee` → `evidence/T1/render-tests.txt`
- [x] COMMIT: `feat(gsl): link spans — per-field OSC 8 hyperlinks with underline through join, fit, and truncation`
- [x] LEDGER + CHECKPOINT (pushed together with T2)

**Done when:** render tests green incl. zero-width and clip tests.

### Task 2 — origin URL + URL builders  (plan Task 2)

- [x] RED: `internal/git/remote_test.go`; `TestTreeURL`/`TestFileURL`/`TestTimeURL_Placeholders` in `links_test.go`
- [x] RUN-RED: `cd sdk/gsl && go test ./internal/git/ ./internal/render/ -run 'Remote|TreeURL|FileURL|TimeURL'` → **FAIL**
- [x] GREEN: `internal/git/remote.go`
- [x] RUN-GREEN: same command → **PASS**
- [x] ALLOWLIST: `git status --short -- sdk/gsl/internal/git/remote.go` lists it
- [x] COMMIT: `feat(gsl): origin web-URL normalization and tree/file/time URL builders`
- [x] LEDGER + CHECKPOINT

**Done when:** 8 remote forms + 3 builder tests pass.

### Task 3 — segments record spans; Render delegates  (plan Task 3)

- [x] RED: `TestRepo_Spans_*`, `TestDirGit_Spans_DirAndBranch`, `TestAI_Spans_*`, `TestTime_Span_WholeTextAfterGlyph`, parity with links, golden `links` case
- [x] RUN-RED: `cd sdk/gsl && go test ./internal/render/ -run 'Spans|Span_|DetectFormat_MatchesRender|Golden'` → **FAIL**
- [x] GREEN: `Deps.Links`/`RemoteURL` + `BuildSegments`; `formatLinked` in the four `*_data.go`; delegation in the four `seg_*.go`
- [x] RUN-GREEN: `cd sdk/gsl && go test ./internal/render/ -race` → **PASS**; `-run Golden -update` adds ONLY `golden_links_*.txt` (check `git diff --stat`)
- [x] VERIFY: `go vet ./...`; evidence → `evidence/T3/segments.txt`
- [x] COMMIT: `feat(gsl): repo, dirgit, ai, and time segments record link spans; legacy Render delegates to detect+format`
- [ ] LEDGER + CHECKPOINT

**Done when:** all four segments implement `LinkedSegment`; parity test green with links on.

### Task 4 — flags package  (plan Task 4)

- [ ] RED: `internal/flags/flags_test.go`
- [ ] RUN-RED: `cd sdk/gsl && go test ./internal/flags/` → **FAIL** (no package)
- [ ] GREEN: `internal/flags/flags.go`; `go.mod` require + replace for `sdk/gff`; `go mod tidy`
- [ ] RUN-GREEN: `cd sdk/gsl && go test ./internal/flags/ -race && go build ./...` → **PASS**
- [ ] ALLOWLIST: `git status --short -- sdk/gsl/internal/flags`
- [ ] COMMIT: `feat(gsl): fail-open gff lookups for the link families`
- [ ] LEDGER + CHECKPOINT

**Done when:** budget test (<100 ms with a 200 ms lookup) passes; build green.

### Task 5 — wiring  (plan Task 5)

- [ ] RED: `TestEffectiveLinks`; `TestBuildLinks_*`; config get/set `links` round-trip + invalid value
- [ ] RUN-RED: `cd sdk/gsl && go test ./internal/config/ ./cmd/ -run 'EffectiveLinks|BuildLinks|Links'` → **FAIL**
- [ ] GREEN: `config.go`, `cmd/config.go`, `cmd/statusline.go` (`buildLinks`, flags goroutine, remote URL, `st.Links`), `preview/model.go` fixture, `features.yaml` `gsl` area, `install.sh` `gff install` line
- [ ] RUN-GREEN: `cd sdk/gsl && go test ./... -race` → **PASS** (review any regenerated golden diff: only `]8;;`/`[4m`)
- [ ] VERIFY: `gff lint`; `make lint-shell`; `make lint-portability`; `bash sdk/gsl/build.sh`; live `gsl status | cat -v | grep -c ']8;;'` ≥ 6 in a PR worktree; `gff set gsl.links.time false` → no `time.is`; `gff unset gsl.links.time`
- [ ] COMMIT: `feat(gsl): links config key, gff-gated link policy, preview fixture, flag schema, install-time namespace registration`
- [ ] LEDGER + CHECKPOINT

**Done when:** all gates clean and the live toggle observed.

### Task 6 — docs + human click check  (plan Task 6)

- [ ] DOCS: `sdk/gsl/README.md`, `sdk/gsl/skill/SKILL.md`, `sdk/gsl/docs/design.md`
- [ ] VERIFY (human): Ctrl+click each family in a herdr pane; record URLs in `TRACKING.md` §2 + `evidence/T6/click-check.md`; agy: no usage link
- [ ] COMMIT: `docs(gsl): document link spans, link options, and the gsl.links.* gff flags`
- [ ] LEDGER: TRACKING §3 all ticked; `docs/mbo/index.md` → `in-review`; flip PR from draft
- [ ] CHECKPOINT

**Done when:** stop condition in TRACKING §3 fully ticked.
