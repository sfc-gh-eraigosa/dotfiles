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
| T1 spans through join/fit/truncation | todo | | | |
| T2 remote URL + URL builders | todo | | | |
| T3 segment spans + Render delegation | todo | | | |
| T4 flags package (gff, fail-open) | todo | | | |
| T5 wiring: config/cmd/preview/features.yaml/install.sh | todo | | | |
| T6 docs + human click check | todo | | | |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 spans, exact range, balanced | [ ] `TestPaintRuns_*`, `TestJoin_SpansZeroWidth_BothPaths`, parity | [ ] Ctrl+click PR badge opens PR | |
| F2 modes underline/plain/off | [ ] `TestPaintRuns_PlainModeHasNoUnderline`, `TestEffectiveLinks`, golden `links` | [ ] underline visible in herdr pane | |
| F3 repo/dirgit links | [ ] `TestNormalizeRemote`, `TestTreeURL`, `TestRepo_Spans_*`, `TestDirGit_Spans_DirAndBranch` | [ ] label → tree URL; glyph → repo home | |
| F4 file link | [ ] `TestFileURL` | [ ] dir name opens file manager | |
| F5 usage link | [ ] `TestAI_Spans_*`, `TestBuildLinks_Defaults` | [ ] model → usage page (Claude); none under agy | |
| F6 time link | [ ] `TestTimeURL_Placeholders`, `TestTime_Span_WholeTextAfterGlyph` | [ ] time → time.is page | |
| F7 gff gating, fail-open | [ ] `TestResolve_*`, `TestBuildLinks_*`, `gff lint` | [ ] `gff set gsl.links.time false` removes the time link | |
| F8 width safety | [ ] `TestClipSpans`, `TestReanchorSpans_*`, `TestShiftSpans`, `TestTruncateToWidth_ClipsSpans`, fit property test | [ ] narrow pane: no stray escapes | |
| F9 docs | [ ] — | [ ] README/SKILL/design reviewed | |

## 3. Validation done-when — the stop condition

- [ ] T1–T6 rows `done` with SHA + evidence
- [ ] `cd sdk/gsl && go test ./... -race -cover` green, coverage ≥60%
- [ ] `gff lint`, `make lint-shell`, `make lint-portability` clean
- [ ] Human click check recorded (§2 right column) for every family
- [ ] PR flipped from draft; `docs/mbo/index.md` row → `in-review`

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-09-05 | planning | Diagnosed: #249 link already emitted, no affordance, one link per block. Design approved in chat; spec + plan + trio written. |
