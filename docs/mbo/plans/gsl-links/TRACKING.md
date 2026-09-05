# gsl-links — live state ledger

- **Slug:** gsl-links
- **Started:** 2026-09-05
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../gsl-links.md`](../gsl-links.md) · spec [`../../specs/gsl-links.md`](../../specs/gsl-links.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a
> commit SHA **and** evidence. Never write a result you did not observe.

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| gsl-links (single) | classic lane (no gss feature) | `worktree/gsl` | `~/.herdr/worktrees/dotfiles/worktree-gsl` | #279 | planning |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| T1 spans through join/fit/truncation | done | f4162ea | `go test ./internal/render/ -race -cover` → `ok … coverage` (evidence/T1/render-tests.txt); 6 goldens regenerated, visible text byte-identical (escape placement only) | RED: compile failure on `LinkSpan`; old `TestTruncate_PreservesLink` re-targeted to the surviving label |
| T2 remote URL + URL builders | done | ae0765f | `go test ./internal/git/ ./internal/render/ -race -cover` → ok, render 90.5% (evidence/T2/url-tests.txt); mutation checks TreeURL/FileURL/TimeURL each FAIL when broken | builders were written in T1 (`links.go`), so their tests were proven by mutation rather than by a RED run |
| T3 segment spans + Render delegation | done | ce1ff1a | `go test ./internal/render/ -race -cover` ok; `TestDetectFormat_MatchesRender_Linked` + `TestGolden_Links` pass; `make lint-go` 0 issues (evidence/T3/segments.txt); linked golden targets: file://, usage, repo home, 2× tree, PR, time.is | Decision: zero-value `Links` links nothing (PR badge is in the Repo family + `link_pr`), so default goldens lost the badge link — visible text byte-identical |
| T4 flags package (gff, fail-open) | done | 34b3aa6 | `go test ./internal/flags/ -race -cover` → ok, 90.3% incl. the 20 ms-budget test; `go build ./...` with the gff dependency; `make lint-go` 0 issues (evidence/T4/flags.txt) | `go mod tidy` also bumped x/sys 0.36→0.47 and colorprofile (gff transitive requirements); full module tests green |
| T5 wiring: config/cmd/preview/features.yaml/install.sh | done | 485329d | `go test ./... -race -cover` green; `gff lint` clean, `gff list` shows the 5 `gsl.links.*` flags; `make lint-shell` / `lint-portability` / `lint-go` clean; live: 5 targets in a PR worktree, `gff set gsl.links.time false` → 0 time.is, unset → 1, `links off` → 0 OSC 8, render 28–35 ms (evidence/T5/{all-tests,lints,live-check}.txt) | Preview narrow-width tests measured width with a CSI-only stripper (counted URLs); switched the helper to `term.DisplayWidth`. Live flags resolved via the `$DOTFILES_DIR` path-source fallback — the namespace is registered by `install.sh` (`gff install`) on a normal host |
| T6 docs + human click check | done | 72066b1 (docs) + this commit (evidence) | README `## Links` + option rows; SKILL.md `### Links (OSC 8 hyperlinks)` + options table; design.md `## Link spans`; human Ctrl+click check: all six targets open (evidence/T6/click-check.md) | Chain proven gsl → Claude Code → herdr → gnome-terminal |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 spans, exact range, balanced | [x] `TestPaintRuns_*`, `TestJoin_SpansZeroWidth_BothPaths`, parity | [x] Ctrl+click PR badge opens PR | |
| F2 modes underline/plain/off | [x] `TestPaintRuns_PlainModeHasNoUnderline`, `TestEffectiveLinks`, golden `links` | [x] underline visible in herdr pane | |
| F3 repo/dirgit links | [x] `TestNormalizeRemote`, `TestTreeURL`, `TestRepo_Spans_*`, `TestDirGit_Spans_DirAndBranch` | [x] label → tree URL; glyph → repo home | |
| F4 file link | [x] `TestFileURL` | [x] dir name opens file manager | |
| F5 usage link | [x] `TestAI_Spans_*`, `TestBuildLinks_Defaults` | [x] model → usage page (Claude); none under agy | |
| F6 time link | [x] `TestTimeURL_Placeholders`, `TestTime_Span_WholeTextAfterGlyph` | [x] time → time.is page | |
| F7 gff gating, fail-open | [x] `TestResolve_*`, `TestBuildLinks_*`, `gff lint` | [x] `gff set gsl.links.time false` removes the time link | |
| F8 width safety | [x] `TestClipSpans`, `TestReanchorSpans_*`, `TestShiftSpans`, `TestTruncateToWidth_ClipsSpans`, fit property test | [x] narrow pane: no stray escapes | |
| F9 docs | [x] — | [x] README/SKILL/design reviewed | |

## 3. Validation done-when — the stop condition

- [x] T1–T6 rows `done` with SHA + evidence
- [x] `cd sdk/gsl && go test ./... -race -cover` green, coverage ≥60% (render 90.5%, flags 90.3%)
- [x] `gff lint`, `make lint-shell`, `make lint-portability` clean (+ `make lint-go` 0 issues)
- [x] Human click check recorded (§2 right column) for every family
- [x] PR flipped from draft; `docs/mbo/index.md` row → `in-review`

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-09-05 | planning | Diagnosed: #249 link already emitted, no affordance, one link per block. Design approved in chat; spec + plan + trio written. |
| 2026-09-05 | build-1 | T1 done: link spans through join/fit/truncation; goldens refreshed. |
| 2026-09-05 | build-1 | T2 done: `git.RemoteWebURL`/`NormalizeRemote`; URL builder tests + mutation proof. First `gss push` refused on unstaged T2 files — pushing T1+T2 together. |
| 2026-09-05 | build-1 | T3 done: all four segments emit spans via `formatLinked`; legacy `Render` delegates to detect+format; linked goldens + parity test. |
| 2026-09-05 | build-1 | T4 done: `internal/flags` (concurrent, budgeted, fail-open gff lookups) + gff SDK dependency. |
| 2026-09-05 | build-1 | T5 done: `links` config key, `buildLinks` policy in cmd (flags ∥ origin lookup, 100 ms budget), preview fixture links, 5 gff flags, `gff install` in install.sh. |
| 2026-09-05 | build-1 | T6 docs written (README, SKILL.md, design.md); awaiting the human click check. |
| 2026-09-05 | build-1 | T6 done: human click check passed for all six targets; objective → in-review; PR #279 ready for review. |
