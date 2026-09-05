# gsl-links — linkable status-line fields — spec

- **Slug:** gsl-links
- **Date:** 2026-09-05
- **Status:** Approved (design approved in chat 2026-09-05; bounded change, no separate design doc — the shape is §3)
- **Relates to:** issue #(pending) / PR #(pending) / prior art PR #249 (`link_pr`, one OSC 8 link over the whole repo block)

## 1. Goal
Every fact on the gsl status line that has a canonical web home becomes a click target, and the
line *shows* which text is clickable. Today only the PR badge is linked (whole repo block, one URL,
no visual affordance — in gnome-terminal an OSC 8 link only underlines on hover and opens with
Ctrl+click, so users don't discover it). After this: PR badge → PR; feature/worker/branch label and
the root/worktree glyph → the repo on GitHub; dirgit branch → branch tree, directory → `file://`;
model / context / 5h / 7d → the usage page; time → a timezone page. Linked text is underlined.
Every target family can be switched off with a `gff` flag; the affordance with one config key.

## 2. Use cases
| # | Actor / trigger | Flow | Acceptance |
| :-- | :-- | :-- | :-- |
| U1 PR | User in Claude Code or agy, worktree with a PR; status line renders | Ctrl+click `PR#269` → browser opens the PR | OSC 8 wraps exactly the badge text (not the whole block); badge underlined |
| U2 Repo | Same, any git repo with an `origin` remote | Click the label (`feature`/`worker`/`branch` name mode) → `https://<host>/<owner>/<repo>/tree/<branch>`; click the root/worktree glyph → `https://<host>/<owner>/<repo>` | URL derived from `git remote get-url origin` (ssh + https forms normalized, `.git` stripped, URL-escaped branch); no remote → no link, no error, no stderr |
| U3 Dir | Any cwd | Click the dirgit directory name → `file://<absolute cwd>` | Absolute path, percent-encoded; `~` abbreviation stays in the text only |
| U4 Usage | Claude Code payload | Click model name, context %, or a 5h/7d % → `https://claude.ai/settings/usage` | Antigravity payload: no default target (none known); `usage_url` option overrides on either host; MCP badge never linked |
| U5 Time | Always | Click the date/time → template default `https://time.is/{tz_city}` | Placeholders `{tz}` (IANA), `{tz_city}` (IANA tail, e.g. `Los_Angeles`), `{iso_utc}` (`20260905T150000Z`), `{unix}`; template from `time` segment option `link_url` |
| U6 Opt-out | `gff set gsl.links.time false` (or any family flag / master) | Next render omits that family's links | Any gff error (unregistered source, unknown key, gff missing) is **fail-open**: links stay on. `links: "off"` in config disables everything; `links: "plain"` keeps OSC 8 without underline |
| U7 Width safety | Narrow terminal | Fit loop sheds/truncates as today | Links add zero display width; a truncated segment clips its spans; every emitted OSC 8 open has a matching close; underline SGR never leaks past the span |

## 3. Architecture
Components (each independently testable; `render` still never imports `os/exec`):

- **`render/segment.go` — `LinkSpan{Start, End int; URL string}`** (byte offsets into the segment's
  raw text, half-open). `LinkedSegment.RenderLinked` returns `spans []LinkSpan` instead of one
  `link string`; detect.go's twin `linkable.links() []LinkSpan`. Parity test extended.
- **`render/glyphs.go` — join.** `segmentBlock.links []LinkSpan`. A new `paintRuns(text, spans,
  mode)` splits text into unlinked / linked runs; a linked run is
  `OSC8(url) + [SGR 4] + run + [SGR 24] + OSC8()`. Chevrons and padding stay outside every link.
  Applies to the powerline and classic paths alike.
- **`render/truncate.go` — `clipSpans(spans, keptBytes) []LinkSpan`** drops spans past the cut,
  clips one that straddles it (the ellipsis is never inside a link).
- **`render/links.go` — `LinkPolicy{Mode string; Repo, DirGit, AI, Time bool}`** built once per
  render from config (`links`) + gff, injected into every segment; pure URL builders
  `TreeURL(base, branch)`, `FileURL(cwd)`, `UsageURL(isAntigravity, override)`, `TimeURL(tpl, t, loc)`.
- **`git/remote.go` — `RemoteWebURL(ctx, Runner, dir)`** wraps `git remote get-url origin`;
  normalizes `git@host:o/r.git`, `ssh://git@host/o/r.git`, `https://host/o/r.git` → `https://host/o/r`.
  Called once per render alongside the existing status call (same 800 ms budget).
- **`config`** — top-level `links` (`"underline"` default | `"plain"` | `"off"`; `config get/set`
  keys extended); segment options: repo `link_pr` (kept, back-compat), ai `usage_url`, time `link_url`.
- **gff gating** — `.github/gff/features.yaml` gains an `gsl` area: `gsl.links.enabled` (master),
  `gsl.links.repo` (repo + dirgit branch/glyph), `gsl.links.dirgit` (directory `file://`),
  `gsl.links.ai`, `gsl.links.time` — all bool, default true. gsl resolves them through the public
  SDK `gff.Bool(key, gff.WithSource(ns))` with `ns = com.github.sfc-gh-eraigosa.dotfiles`, falling
  back to `WithSource($DOTFILES_DIR)` when set; the five lookups run concurrently with segment
  detection under a 100 ms budget; every error → true (fail-open, mirrors install.sh's `gff_on`).
  `install.sh`'s gff block runs `gff install` from the checkout so the namespace is registered on
  a fresh host (today nothing registers it, so cross-repo resolution fails → fail-open → links on).
- Data flow: `RenderAt` → build `LinkPolicy` (config + gff, concurrent) → segments render
  `(text, colorKey, spans)` → fit loop (drop / truncate + `clipSpans`) → `join` paints runs.

## 4. Behavior / features
- **F1** Link spans through the whole render path (segment → block → join), replacing the single block link.
- **F2** Affordance modes `underline` / `plain` / `off` via config `links`.
- **F3** Repo links: PR badge (span only), label → tree URL, glyph → repo home; dirgit branch → tree URL.
- **F4** Directory `file://` link on the dirgit directory name.
- **F5** AI usage link on model, context %, 5h %, 7d % (Claude default; `usage_url` override).
- **F6** Time link from `link_url` template with the four placeholders.
- **F7** gff gating: five flags, master + four families, fail-open; `install.sh` registers the source.
- **F8** Width safety: zero-width links, span clipping on truncation, balanced escapes.
- **F9** Docs: README, `skill/SKILL.md`, `docs/design.md` link section, `opt/etc`/gff README row for the new flags.

## 5. Evaluation criteria (per feature)
| Feature | Trigger predicate | Fires | Must-not-fire | Edge | Pass |
| :-- | :-- | :-- | :-- | :-- | :-- |
| F1 | segment returns ≥1 span | join emits one OSC 8 open/close pair per span, around exactly `text[Start:End]` | empty spans → byte-identical output to today | overlapping / out-of-range spans are rejected (dropped, never panic) | `TestJoin_Spans_*`, parity test |
| F2 | `links` config value | `underline` adds `ESC[4m`…`ESC[24m` inside the link; `plain` none; `off` no OSC 8 at all | `off` never emits `]8;;` | unknown value → `underline` + no error | golden files per mode |
| F3 | origin remote present | label → `/tree/<branch>`, glyph → repo home, PR badge → PR URL | no remote → no repo spans; PR link absent when `show_pr=false` or no PR (unchanged rule) | detached HEAD → no tree link; branch with `/` and `#` percent-encoded | `TestRemoteWebURL` table (6 forms), `TestRepoSegment_Spans` |
| F4 | `gsl.links.dirgit` on | dir name → `file://` absolute cwd | cwd unresolvable → no span | spaces / unicode in path percent-encoded | `TestDirGit_FileSpan` |
| F5 | Claude payload with model/context/rate | four spans, same URL | Antigravity payload w/o `usage_url` → no spans; MCP badge never in a span | `usage_url` set → used verbatim on both hosts | `TestAI_UsageSpans_{Claude,Agy,Override}` |
| F6 | time renders | one span over the whole date+time text with expanded template | empty `link_url` → no span | bad placeholder left literal; `{tz_city}` of `UTC` = `UTC` | `TestTimeURL_Placeholders`, `TestTime_Span` |
| F7 | flag resolves false | that family's spans absent | any gff error → family on; `gsl.links.enabled=false` → all off | gff lookup > 100 ms → treated as on, render not delayed | `TestLinkPolicy_*` with a fake resolver; `gff lint` clean |
| F8 | fit loop truncates / sheds | spans clipped to kept bytes; StripANSI width unchanged | ellipsis never inside a link; no unmatched `]8;;` | span ending exactly at cut | `TestClipSpans`, fit property test extended (balanced-OSC invariant) |
| F9 | docs | README + SKILL.md option tables list `links`, `usage_url`, `link_url`, the five flags | — | — | review checklist in TRACKING |

## 6. Verification harness
- `cd sdk/gsl && go test ./... -race` — unit + golden + property tests; sdk coverage gate ≥60% (`make sdk-test` / gsl CI).
- `gsl preview --once` goldens regenerated deliberately (`-update`) and reviewed in the diff.
- `gff lint .github/gff/features.yaml` clean; `make lint-shell` + `make lint-portability` for the `install.sh` line.
- Human-evidenced: in a herdr pane inside gnome-terminal, Ctrl+click each target family in a worktree with a PR (PR, label, glyph, dir, model, time) and record the opened URL in `plans/gsl-links/TRACKING.md`; repeat under agy for the no-usage-link case.

## 7. Prerequisites / dependencies
`sdk/gff` public SDK (`pkg/gff`, `Bool` + `WithSource`) — gsl gains a `require` on it (workspace-local, like `libs`); `gh` for PR URLs (unchanged); no new external deps.

## 8. Out of scope (and why)
- GitLab / Bitbucket tree-URL forms (`/-/tree/`): only the repo-home link is emitted for non-GitHub hosts; the branch form is GitHub-only. Not needed by any repo here.
- An Antigravity usage-page default: no public URL known; `usage_url` covers it.
- MCP badge link (nothing canonical to open), OSC 8 `id=` grouping, click handling itself (herdr / the terminal own that).
- A converter site that pre-fills the instant: timeanddate.com blocks non-browser fetches so its query format could not be verified; the template lets a user set it.

## 9. Rollback
No persisted state. `gsl config set links off` or `gff set gsl.links.enabled false` disables everything at runtime; reverting the PR restores #249 behaviour (one block link, no underline).

> Produced via `superpowers:brainstorming`. The matching plan goes in `../plans/gsl-links.md`.
> Registered in `../index.md`.
