# gsl-links — linkable status-line fields — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. The execution trio lives in [`gsl-links/`](./gsl-links/).

- **Slug:** gsl-links
- **Date:** 2026-09-05
- **Status:** Approved
- **Relates to:** spec [`../specs/gsl-links.md`](../specs/gsl-links.md) · issue #(pending) · PR #(pending)

**Goal:** Make every web-addressable fact on the gsl status line a visibly underlined OSC 8 click target (PR, branch/repo, directory, model/context/rate, time), gated by gff flags and one config key.

**Architecture:** Segments record byte-offset `LinkSpan`s while they build their text; the join layer paints unlinked and linked runs separately (OSC 8 + SGR underline around exactly the span); the fit loop re-anchors and clips spans when it strips or truncates. A `render.Links` policy (config + gff + origin URL + usage/time URL templates) is computed once in `cmd` and injected through `Deps`, so `render` stays free of `os/exec`. The legacy `Render` path is collapsed onto `detect`+`formatLinked` so spans exist once.

**Tech Stack:** Go 1.26 (`sdk/gsl`), the `sdk/gff` public SDK (`pkg/gff`), `sdk/gsl/internal/git` fake runner for tests, golden files under `internal/render/testdata`.

**Spec:** `docs/mbo/specs/gsl-links.md`

## Global Constraints
- `render` never imports `os/exec` (package doc contract); all subprocess work goes through `git.Runner` / the new `internal/flags` package called from `cmd`.
- Links are zero display width: `term.DisplayWidth(join(...))` must be identical with and without spans (spec F8).
- Fail-open: any gff error, timeout (>100 ms), unregistered source, or missing binary ⇒ links ON (spec U6 / F7).
- Every OSC 8 open has a matching close; underline SGR (`ESC[4m`/`ESC[24m`) never leaks past a span.
- Stage files by explicit name; check `git status --short -- <path>` for every new path (allowlist `.gitignore`).
- Commit messages end with the session's `Co-Authored-By` / `Claude-Session` trailers (see repo attribution rule).
- Coverage: `sdk/gsl` ≥60% overall (sdk gate); `go test ./... -race` green before every commit.

## 1. Summary & verdict
Builds spec F1–F9 in six tasks. Design approved in chat (2026-09-05). Deviation from #249: the block-level `link` field is replaced by `links []LinkSpan` (the single-link form is the one-span case), and the legacy `Render` path becomes a thin wrapper over `detect`+`formatLinked` — this removes the duplicated text assembly that would otherwise need spans twice.

## 2. File inventory
| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `sdk/gsl/internal/render/segment.go` | `LinkSpan`; `LinkedSegment.RenderLinked` returns `[]LinkSpan`; `Links` policy struct | F1, F7 |
| `sdk/gsl/internal/render/links.go` (new) | `Links`, `spanBuilder`, `validateSpans`, `clipSpans`, `reanchorSpans`, `shiftSpans`, URL builders `TreeURL`/`FileURL`/`TimeURL`, defaults | F1, F3–F6, F8 |
| `sdk/gsl/internal/render/links_test.go` (new) | unit tests for the above | §5 |
| `sdk/gsl/internal/render/glyphs.go` | `segmentBlock.links`, `paintRuns`, both join paths use it | F1, F2 |
| `sdk/gsl/internal/render/link_test.go` | rewritten for spans + modes | §5 |
| `sdk/gsl/internal/render/detect.go` | `linkedFormatter` interface, `formatLinkedOf`, `Format`/`finalTierBlocks` carry spans, glyph-drop shift | F1, F8 |
| `sdk/gsl/internal/render/truncate.go` | `truncateToWidth` re-anchors + clips spans | F8 |
| `sdk/gsl/internal/render/render.go` | `Deps.Links`, `Deps.RemoteURL`; `BuildSegments` injects; `RenderAt` carries spans | F1, F7 |
| `sdk/gsl/internal/render/seg_repo.go`, `seg_repo_data.go` | glyph/label/badge spans; `Render` delegates | F3 |
| `sdk/gsl/internal/render/seg_dirgit.go`, `seg_dirgit_data.go` | dir + branch spans; `Render` delegates | F3, F4 |
| `sdk/gsl/internal/render/seg_ai.go`, `seg_ai_data.go` | model/ctx/5h/7d spans; `Render` delegates | F5 |
| `sdk/gsl/internal/render/seg_time.go`, `seg_time_data.go` | whole-text span; `Render` delegates | F6 |
| `sdk/gsl/internal/git/remote.go` + `remote_test.go` (new) | `RemoteWebURL`, `NormalizeRemote` | F3 |
| `sdk/gsl/internal/flags/flags.go` + `flags_test.go` (new) | gff lookups: concurrent, budgeted, fail-open | F7 |
| `sdk/gsl/go.mod` | `require …/sdk/gff v0.0.0` + `replace => ../gff` | F7 |
| `sdk/gsl/internal/config/config.go` | `Links string` + `EffectiveLinks()` | F2 |
| `sdk/gsl/cmd/config.go` | `config get/set links` | F2 |
| `sdk/gsl/internal/style/style.go` | `Style.Links` (underline vs plain, set by cmd) | F2 |
| `sdk/gsl/cmd/statusline.go` | builds `render.Links` (config + flags + remote URL + options), sets `st.Links` | F2, F7 |
| `sdk/gsl/internal/preview/model.go` | fixture `Links` so `preview` shows links | F9 |
| `.github/gff/features.yaml` | `gsl` area: five bool flags | F7 |
| `install.sh` | `gff install` after the bootstrap export (namespace registration) | F7 |
| `sdk/gsl/README.md`, `sdk/gsl/skill/SKILL.md`, `sdk/gsl/docs/design.md` | options + flags documented | F9 |
| `docs/mbo/plans/gsl-links/evidence/**` | captured gate output per task | §7 |

## 3. Interface contracts
```go
// render/segment.go
type LinkSpan struct { Start, End int; URL string } // byte offsets into the RAW text, half-open
type LinkedSegment interface {
    Segment
    RenderLinked(ctx context.Context, st style.Style, level int) (text, colorKey string, spans []LinkSpan, ok bool)
}
// render/links.go — policy computed in cmd, injected via Deps; families already AND-ed with Enabled.
type Links struct {
    Repo, DirGit, AI, Time bool // effective family switches (config "off" and gff master already applied)
    RepoURL  string // normalized origin web URL, "" = none
    UsageURL string // "" = none
    TimeURL  string // template, "" = none
}
const DefaultClaudeUsageURL = "https://claude.ai/settings/usage"
const DefaultTimeURL = "https://time.is/{tz_city}"
func TreeURL(base, branch string) string            // "" unless base starts with https://github.com/
func FileURL(cwd string) string                     // file:///… percent-encoded
func TimeURL(tpl string, t time.Time, loc *time.Location) string
func validateSpans(n int, spans []LinkSpan) []LinkSpan // sorted, in-range, non-overlapping, URL != ""
func clipSpans(spans []LinkSpan, keptBytes int) []LinkSpan
func reanchorSpans(orig, stripped string, spans []LinkSpan) []LinkSpan
func shiftSpans(spans []LinkSpan, shift int) []LinkSpan
// render/detect.go
type linkedFormatter interface { formatLinked(st style.Style, level int) (text, colorKey string, spans []LinkSpan) }
func formatLinkedOf(d segmentData, st style.Style, level int) (string, string, []LinkSpan)
// render/glyphs.go
func paintRuns(st style.Style, text string, spans []LinkSpan) string // OSC8 (+SGR4) around each span
// git/remote.go
func RemoteWebURL(ctx context.Context, r Runner, dir string) (string, error)
func NormalizeRemote(raw string) (string, bool)
// flags/flags.go
type Links struct { Enabled, Repo, DirGit, AI, Time bool }
type Lookup func(key string) (bool, error)
func Resolve(ctx context.Context, look Lookup) Links // concurrent; missing/err/timeout ⇒ true
func GFFLookup(env func(string) string) Lookup     // gff.Bool(key, WithSource(Namespace)) then WithSource($DOTFILES_DIR)
const Namespace = "com.github.sfc-gh-eraigosa.dotfiles"
// config
Config.Links string `json:"links,omitempty"` // "underline" | "plain" | "off"; "" ⇒ underline
func (c Config) EffectiveLinks() string
// style
Style.Links string `json:"links,omitempty"`  // "plain" disables SGR underline; anything else underlines
```
Orchestration in `cmd/statusline.go` (after `git.Status`, before `BuildSegments`):
```
fctx := 100ms child ctx
lf := flags.Resolve(fctx, flags.GFFLookup(os.Getenv))        // parallel with RemoteWebURL below
remote, _ := git.RemoteWebURL(ctx, gitRunner, cwd)           // one exec
mode := cfg.EffectiveLinks(); on := mode != "off" && lf.Enabled
deps.Links = render.Links{Repo: on && lf.Repo, DirGit: on && lf.DirGit, AI: on && lf.AI, Time: on && lf.Time,
    RepoURL: remote, UsageURL: usageDefault(p.IsAntigravity()), TimeURL: render.DefaultTimeURL}
st.Links = mode   // "plain" or "underline"
```
`BuildSegments` overrides `UsageURL` from the `ai` segment option `usage_url` and `TimeURL` from the `time` option `link_url` when set.

## 4. TDD build order

### Task 1: Link spans through join, truncation, and the final tier
**Files:** Create `sdk/gsl/internal/render/links.go`, `links_test.go`; Modify `segment.go`, `glyphs.go` (segmentBlock, `join`, `joinPowerline`), `detect.go` (`linkOf`→spans, `Format`, `finalTierBlocks`), `truncate.go` (`truncateToWidth`), `render.go` (`RenderAt` result), `link_test.go`, `seg_repo.go` (`RenderLinked` signature only), `seg_repo_data.go` (`link()`→`links()` one-span shim).
**Interfaces:** Produces `LinkSpan`, `paintRuns`, `validateSpans`, `clipSpans`, `reanchorSpans`, `shiftSpans`, `segmentBlock.links`, `linkedFormatter`, `formatLinkedOf`. Consumes nothing new.

- [ ] **Step 1: Write the failing tests** (`links_test.go`)
```go
package render

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

const ulOn, ulOff = "\x1b[4m", "\x1b[24m"

func TestPaintRuns_WrapsExactlyTheSpan(t *testing.T) {
	st := asciiStyle() // Links == "" ⇒ underline
	got := paintRuns(st, "root main PR#21", []LinkSpan{{Start: 10, End: 15, URL: wantPRURL}})
	want := "root main " + osc8Open(wantPRURL) + ulOn + "PR#21" + ulOff + osc8Close
	if got != want {
		t.Errorf("paintRuns = %q, want %q", got, want)
	}
}

func TestPaintRuns_PlainModeHasNoUnderline(t *testing.T) {
	st := asciiStyle()
	st.Links = "plain"
	got := paintRuns(st, "PR#21", []LinkSpan{{0, 5, wantPRURL}})
	if strings.Contains(got, ulOn) || !strings.Contains(got, osc8Open(wantPRURL)) {
		t.Errorf("plain mode: got %q", got)
	}
}

func TestPaintRuns_NoSpansIsIdentity(t *testing.T) {
	if got := paintRuns(asciiStyle(), "abc", nil); got != "abc" {
		t.Errorf("got %q", got)
	}
}

func TestValidateSpans_DropsBadOnes(t *testing.T) {
	spans := validateSpans(5, []LinkSpan{
		{3, 5, "u3"}, {0, 2, "u1"}, {1, 3, "overlap"}, {4, 9, "range"}, {0, 1, ""}, {2, 2, "empty"},
	})
	if len(spans) != 2 || spans[0].URL != "u1" || spans[1].URL != "u3" {
		t.Errorf("validateSpans = %+v", spans)
	}
}

func TestClipSpans(t *testing.T) {
	got := clipSpans([]LinkSpan{{0, 4, "a"}, {5, 9, "b"}, {10, 12, "c"}}, 7)
	if len(got) != 2 || got[1].End != 7 {
		t.Errorf("clipSpans = %+v", got)
	}
}

func TestReanchorSpans_SurvivesANSIStrip(t *testing.T) {
	orig := "x \x1b[38;5;5mPR#21\x1b[38;5;7m y"
	spans := []LinkSpan{{2, 2 + len("\x1b[38;5;5mPR#21\x1b[38;5;7m"), wantPRURL}}
	stripped := term.StripANSI(orig)
	got := reanchorSpans(orig, stripped, spans)
	if len(got) != 1 || stripped[got[0].Start:got[0].End] != "PR#21" {
		t.Errorf("reanchor = %+v over %q", got, stripped)
	}
}

func TestShiftSpans(t *testing.T) {
	got := shiftSpans([]LinkSpan{{0, 2, "glyph"}, {3, 7, "label"}}, 3)
	if len(got) != 1 || got[0].Start != 0 || got[0].End != 4 {
		t.Errorf("shiftSpans = %+v", got)
	}
}

func TestJoin_SpansZeroWidth_BothPaths(t *testing.T) {
	plain := []segmentBlock{{text: "main PR#21", colorKey: "repo_root"}}
	linked := []segmentBlock{{text: "main PR#21", colorKey: "repo_root",
		links: []LinkSpan{{0, 4, "https://github.com/o/r/tree/main"}, {5, 10, wantPRURL}}}}
	for _, st := range []style.Style{asciiStyle(), powerlineStyleFixture()} {
		if term.DisplayWidth(join(st, linked)) != term.DisplayWidth(join(st, plain)) {
			t.Errorf("spans changed width for %s", st.Separator)
		}
		out := join(st, linked)
		if strings.Count(out, "\x1b]8;;") != 4 { // 2 opens + 2 closes
			t.Errorf("unbalanced OSC 8 in %q", out)
		}
	}
}

func TestTruncateToWidth_ClipsSpans(t *testing.T) {
	blocks := []segmentBlock{{text: "a-long-repo-label PR#21", colorKey: "repo_root",
		links: []LinkSpan{{0, 17, "https://github.com/o/r/tree/x"}, {18, 23, wantPRURL}}}}
	out := truncateToWidth(blocks, asciiStyle(), 12)
	if len(out) != 1 || len(out[0].links) != 1 || out[0].links[0].End > len(out[0].text)-len(ellipsis) {
		t.Errorf("truncate: %+v", out)
	}
}
```
Also update `link_test.go`: `TestRepo_RenderLinked_ReportsPRURL` reads `spans[len(spans)-1].URL`, `TestJoin_EmitsOSC8` uses `links: []LinkSpan{{0, 5, wantPRURL}}`, `TestTruncate_PreservesLink` checks `out[0].links[0].URL`.

- [ ] **Step 2: Run to verify failure**
Run: `cd sdk/gsl && go test ./internal/render/ -run 'PaintRuns|ValidateSpans|ClipSpans|Reanchor|ShiftSpans|Join_Spans|TruncateToWidth_Clips' 2>&1 | head`
Expected: FAIL to compile — `undefined: LinkSpan`, `paintRuns`.

- [ ] **Step 3: Implement** (`links.go`)
```go
package render

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

// Links is the per-render link policy. Families are EFFECTIVE switches: cmd has
// already AND-ed the config mode and the gff master flag into them.
type Links struct {
	Repo, DirGit, AI, Time bool
	RepoURL, UsageURL, TimeURL string
}

const (
	DefaultClaudeUsageURL = "https://claude.ai/settings/usage"
	DefaultTimeURL        = "https://time.is/{tz_city}"
	sgrUnderline          = "\x1b[4m"
	sgrNoUnderline        = "\x1b[24m"
)

// spanBuilder records link spans while a segment builds its raw text.
type spanBuilder struct {
	b     strings.Builder
	spans []LinkSpan
}

func (s *spanBuilder) write(text string) { s.b.WriteString(text) }
func (s *spanBuilder) len() int          { return s.b.Len() }
func (s *spanBuilder) String() string    { return s.b.String() }

// linked writes text and, when url is non-empty, records a span over it.
func (s *spanBuilder) linked(text, url string) {
	start := s.b.Len()
	s.b.WriteString(text)
	if url != "" && text != "" {
		s.spans = append(s.spans, LinkSpan{Start: start, End: s.b.Len(), URL: url})
	}
}

func validateSpans(n int, spans []LinkSpan) []LinkSpan {
	out := make([]LinkSpan, 0, len(spans))
	for _, sp := range spans {
		if sp.URL == "" || sp.Start < 0 || sp.End > n || sp.Start >= sp.End {
			continue
		}
		out = append(out, sp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	kept := out[:0]
	last := 0
	for _, sp := range out {
		if sp.Start < last {
			continue // overlaps the previous span
		}
		kept = append(kept, sp)
		last = sp.End
	}
	return kept
}

func clipSpans(spans []LinkSpan, keptBytes int) []LinkSpan {
	var out []LinkSpan
	for _, sp := range spans {
		if sp.Start >= keptBytes {
			continue
		}
		if sp.End > keptBytes {
			sp.End = keptBytes
		}
		out = append(out, sp)
	}
	return out
}

// reanchorSpans maps spans recorded on orig (which may contain ANSI tints) onto
// stripped = term.StripANSI(orig) by locating each span's visible fragment.
func reanchorSpans(orig, stripped string, spans []LinkSpan) []LinkSpan {
	var out []LinkSpan
	cursor := 0
	for _, sp := range validateSpans(len(orig), spans) {
		frag := term.StripANSI(orig[sp.Start:sp.End])
		if frag == "" {
			continue
		}
		idx := strings.Index(stripped[cursor:], frag)
		if idx < 0 {
			continue
		}
		start := cursor + idx
		out = append(out, LinkSpan{Start: start, End: start + len(frag), URL: sp.URL})
		cursor = start + len(frag)
	}
	return out
}

func shiftSpans(spans []LinkSpan, shift int) []LinkSpan {
	var out []LinkSpan
	for _, sp := range spans {
		sp.Start -= shift
		sp.End -= shift
		if sp.End <= 0 {
			continue
		}
		if sp.Start < 0 {
			sp.Start = 0
		}
		out = append(out, sp)
	}
	return out
}

func TreeURL(base, branch string) string {
	if branch == "" || !strings.HasPrefix(base, "https://github.com/") {
		return ""
	}
	parts := strings.Split(branch, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return base + "/tree/" + strings.Join(parts, "/")
}

func FileURL(cwd string) string {
	if cwd == "" || !strings.HasPrefix(cwd, "/") {
		return ""
	}
	u := url.URL{Scheme: "file", Path: cwd}
	return u.String()
}

func TimeURL(tpl string, t time.Time, loc *time.Location) string {
	if tpl == "" {
		return ""
	}
	tz := loc.String()
	city := tz
	if i := strings.LastIndex(tz, "/"); i >= 0 {
		city = tz[i+1:]
	}
	r := strings.NewReplacer(
		"{tz}", tz,
		"{tz_city}", city,
		"{iso_utc}", t.UTC().Format("20060102T150405Z"),
		"{unix}", strconv.FormatInt(t.Unix(), 10),
	)
	return r.Replace(tpl)
}
```
`segment.go`: add `type LinkSpan struct { Start, End int; URL string }`; change `LinkedSegment.RenderLinked` to return `(text, colorKey string, spans []LinkSpan, ok bool)`.
`glyphs.go`: replace `link string` on `segmentBlock` with `links []LinkSpan`; delete `osc8Wrap`'s callers and add
```go
func paintRuns(st style.Style, text string, spans []LinkSpan) string {
	spans = validateSpans(len(text), spans)
	if len(spans) == 0 {
		return text
	}
	ul, ulOff := sgrUnderline, sgrNoUnderline
	if st.Links == "plain" {
		ul, ulOff = "", ""
	}
	var sb strings.Builder
	pos := 0
	for _, sp := range spans {
		sb.WriteString(text[pos:sp.Start])
		sb.WriteString("\x1b]8;;" + sp.URL + "\x1b\\" + ul + text[sp.Start:sp.End] + ulOff + "\x1b]8;;\x1b\\")
		pos = sp.End
	}
	sb.WriteString(text[pos:])
	return sb.String()
}
```
Classic path: `parts = append(parts, paint(st, b.colorKey, paintRuns(st, b.text, b.links)))`. Powerline path: `sb.WriteString(" " + paintRuns(st, b.text, b.links) + " ")` (padding stays outside the link).
`style.go`: add `Links string \`json:"links,omitempty"\`` to `Style` (doc: `"plain"` = OSC 8 only; anything else underlines).
`detect.go`: replace `linkable`/`linkOf` with
```go
type linkedFormatter interface {
	formatLinked(st style.Style, level int) (text, colorKey string, spans []LinkSpan)
}
func formatLinkedOf(d segmentData, st style.Style, level int) (string, string, []LinkSpan) {
	if lf, ok := d.(linkedFormatter); ok {
		return lf.formatLinked(st, level)
	}
	text, colorKey := d.format(st, level)
	return text, colorKey, nil
}
```
`Format`: `text, colorKey, spans := formatLinkedOf(d, st, level)` → `segmentBlock{text, colorKey, links: spans}`. `finalTierBlocks`: same, then `dropped := dropLeadingGlyph(text); spans = shiftSpans(spans, len(text)-len(dropped)); text = dropped`.
`truncate.go` `truncateToWidth`: `work[i] = segmentBlock{text: stripped, colorKey: b.colorKey, links: reanchorSpans(b.text, stripped, b.links)}`; in the truncating branch: `cut := truncateText(b.text, remaining); kept := len(cut); if cut != b.text { kept -= len(ellipsis) }; out = append(out, segmentBlock{text: cut, colorKey: b.colorKey, links: clipSpans(b.links, kept)})`.
`render.go` `RenderAt`: result field `spans []LinkSpan`; `blocks = append(..., segmentBlock{text: r.text, colorKey: r.colorKey, links: r.spans})`.
`seg_repo.go` `RenderLinked`: return `spans` = `[]LinkSpan{{Start: badgeStart, End: badgeEnd, URL: info.PRURL}}` recorded around `prBadge` (record `badgeStart := b.Len()` before writing it) — the full span set comes in Task 3. `seg_repo_data.go`: rename `link()` to `formatLinked` that calls `format` and returns the badge span (temporary until Task 3 rewrites it; keep the parity test green).

- [ ] **Step 4: Run to verify pass**
Run: `cd sdk/gsl && go test ./internal/render/ -race 2>&1 | tail -5`
Expected: `ok  …/internal/render`

- [ ] **Step 5: Evidence + commit**
```bash
mkdir -p docs/mbo/plans/gsl-links/evidence/T1 && (cd sdk/gsl && go test ./internal/render/ -race -cover) 2>&1 | tee docs/mbo/plans/gsl-links/evidence/T1/render-tests.txt
git add sdk/gsl/internal/render/links.go sdk/gsl/internal/render/links_test.go sdk/gsl/internal/render/segment.go sdk/gsl/internal/render/glyphs.go sdk/gsl/internal/render/detect.go sdk/gsl/internal/render/truncate.go sdk/gsl/internal/render/render.go sdk/gsl/internal/render/link_test.go sdk/gsl/internal/render/seg_repo.go sdk/gsl/internal/render/seg_repo_data.go sdk/gsl/internal/style/style.go docs/mbo/plans/gsl-links/evidence/T1
git commit -m "feat(gsl): link spans — per-field OSC 8 hyperlinks with underline through join, fit, and truncation"
```

### Task 2: Origin URL normalization and URL builders
**Files:** Create `sdk/gsl/internal/git/remote.go`, `remote_test.go`; extend `links_test.go`.
**Interfaces:** Produces `git.RemoteWebURL`, `git.NormalizeRemote`; tests `TreeURL`/`FileURL`/`TimeURL` from Task 1.

- [ ] **Step 1: Failing tests** (`remote_test.go`)
```go
package git

import (
	"context"
	"testing"

	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git/fake"
)

func TestNormalizeRemote(t *testing.T) {
	cases := map[string]string{
		"git@github.com:o/r.git":            "https://github.com/o/r",
		"ssh://git@github.com/o/r.git":      "https://github.com/o/r",
		"ssh://git@gitlab.com:2222/o/r.git": "https://gitlab.com/o/r",
		"https://github.com/o/r.git":        "https://github.com/o/r",
		"https://user@github.com/o/r":       "https://github.com/o/r",
		"git://github.com/o/r.git":          "https://github.com/o/r",
		"/srv/git/r.git":                    "",
		"":                                  "",
	}
	for in, want := range cases {
		got, ok := NormalizeRemote(in)
		if got != want || ok != (want != "") {
			t.Errorf("NormalizeRemote(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
}

func TestRemoteWebURL_UsesOriginAndDir(t *testing.T) {
	r := &gitfake.Runner{Script: []gitfake.Response{{Stdout: []byte("git@github.com:o/r.git\n")}}}
	got, err := RemoteWebURL(context.Background(), r, "/repo")
	if err != nil || got != "https://github.com/o/r" {
		t.Fatalf("got %q, %v", got, err)
	}
	c := r.Calls[0]
	if c.Name != "-C" || c.Args[0] != "/repo" || c.Args[1] != "remote" || c.Args[2] != "get-url" || c.Args[3] != "origin" {
		t.Errorf("unexpected call %+v", c)
	}
}

func TestRemoteWebURL_NoOriginIsError(t *testing.T) {
	r := &gitfake.Runner{Script: []gitfake.Response{{Stdout: []byte("error: No such remote 'origin'\n"), ExitCode: 2, Err: context.Canceled}}}
	if _, err := RemoteWebURL(context.Background(), r, "/repo"); err == nil {
		t.Fatal("want error")
	}
}
```
Add to `links_test.go`:
```go
func TestTreeURL(t *testing.T) {
	if got := TreeURL("https://github.com/o/r", "feature/a b#1"); got != "https://github.com/o/r/tree/feature/a%20b%231" {
		t.Errorf("TreeURL = %q", got)
	}
	if TreeURL("https://gitlab.com/o/r", "main") != "" || TreeURL("https://github.com/o/r", "") != "" {
		t.Error("tree URL must be GitHub-only and need a branch")
	}
}

func TestFileURL(t *testing.T) {
	if got := FileURL("/home/u/my dir"); got != "file:///home/u/my%20dir" {
		t.Errorf("FileURL = %q", got)
	}
	if FileURL("") != "" || FileURL("rel") != "" {
		t.Error("relative or empty cwd must yield no URL")
	}
}

func TestTimeURL_Placeholders(t *testing.T) {
	loc, _ := time.LoadLocation("America/Los_Angeles")
	ts := time.Date(2026, 9, 5, 8, 0, 0, 0, loc)
	got := TimeURL("x?tz={tz}&c={tz_city}&i={iso_utc}&u={unix}&k={keep}", ts, loc)
	want := "x?tz=America/Los_Angeles&c=Los_Angeles&i=20260905T150000Z&u=" + strconv.FormatInt(ts.Unix(), 10) + "&k={keep}"
	if got != want {
		t.Errorf("TimeURL = %q want %q", got, want)
	}
	if TimeURL("", ts, loc) != "" || TimeURL(DefaultTimeURL, ts, time.UTC) != "https://time.is/UTC" {
		t.Error("empty template ⇒ no URL; UTC city = UTC")
	}
}
```
- [ ] **Step 2: Run** `cd sdk/gsl && go test ./internal/git/ ./internal/render/ -run 'Remote|TreeURL|FileURL|TimeURL' 2>&1 | head` → FAIL (`undefined: NormalizeRemote`).
- [ ] **Step 3: Implement** (`remote.go`)
```go
package git

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// RemoteWebURL returns the https web URL of the origin remote of dir ("" when
// dir is empty), normalized by NormalizeRemote. One git exec.
func RemoteWebURL(ctx context.Context, r Runner, dir string) (string, error) {
	args := buildArgs(dir, "remote", "get-url", "origin")
	out, err := r.Run(ctx, args[0], args[1:]...)
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin: %w", err)
	}
	web, ok := NormalizeRemote(strings.TrimSpace(string(out)))
	if !ok {
		return "", fmt.Errorf("git remote get-url origin: unrecognized remote %q", strings.TrimSpace(string(out)))
	}
	return web, nil
}

// NormalizeRemote maps the common remote spellings onto https://host/owner/repo.
// Local paths and anything without a host yield ok=false.
func NormalizeRemote(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, ".") {
		return "", false
	}
	var host, path string
	switch {
	case strings.Contains(raw, "://"):
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "", false
		}
		host, path = u.Hostname(), u.Path
	case strings.Contains(raw, ":") && strings.Contains(raw, "@"):
		// scp-like: user@host:owner/repo.git
		hp := raw[strings.Index(raw, "@")+1:]
		i := strings.Index(hp, ":")
		host, path = hp[:i], hp[i+1:]
	default:
		return "", false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if host == "" || path == "" {
		return "", false
	}
	return "https://" + host + "/" + path, true
}
```
- [ ] **Step 4: Run** the same command → PASS.
- [ ] **Step 5: Evidence + commit**
```bash
(cd sdk/gsl && go test ./internal/git/ ./internal/render/ -race -cover) 2>&1 | tee docs/mbo/plans/gsl-links/evidence/T2/url-tests.txt
git add sdk/gsl/internal/git/remote.go sdk/gsl/internal/git/remote_test.go sdk/gsl/internal/render/links_test.go docs/mbo/plans/gsl-links/evidence/T2
git commit -m "feat(gsl): origin web-URL normalization and tree/file/time URL builders"
```

### Task 3: Segments record spans; legacy Render delegates to detect+formatLinked
**Files:** Modify `render.go` (`Deps.Links`, `BuildSegments`), `seg_repo.go`/`seg_repo_data.go`, `seg_dirgit.go`/`seg_dirgit_data.go`, `seg_ai.go`/`seg_ai_data.go`, `seg_time.go`/`seg_time_data.go`; tests in `seg_repo_test.go`, `seg_dirgit_test.go`, `seg_ai_test.go`, `seg_time_test.go`, `detect_test.go` (parity), new golden `golden_test.go` case `links`.
**Interfaces:** Consumes `Links`, `spanBuilder`, `TreeURL`/`FileURL`/`TimeURL`. Produces each segment's `formatLinked` and `RenderLinked`; every segment now implements `LinkedSegment`.

- [ ] **Step 1: Failing tests** — one per family (pattern; write all four):
```go
// seg_repo_test.go
func TestRepo_Spans_GlyphLabelBadge(t *testing.T) {
	seg, _ := newRepoSeg(true, 1, nil)
	seg.Links = Links{Repo: true, RepoURL: "https://github.com/o/r"}
	seg.Branch = "feature/gsl/x"
	text, _, spans, ok := seg.RenderLinked(context.Background(), asciiStyle(), 0)
	if !ok || len(spans) != 3 {
		t.Fatalf("spans = %+v text=%q", spans, text)
	}
	if spans[0].URL != "https://github.com/o/r" || spans[1].URL != "https://github.com/o/r/tree/feature/gsl/x" || spans[2].URL != wantPRURL {
		t.Errorf("urls = %+v", spans)
	}
	if !strings.Contains(term.StripANSI(text[spans[2].Start:spans[2].End]), "PR#21") {
		t.Errorf("badge span mismatch: %q", text[spans[2].Start:spans[2].End])
	}
}

func TestRepo_Spans_FamilyOff(t *testing.T) {
	seg, _ := newRepoSeg(true, 1, nil)
	seg.Links = Links{Repo: false, RepoURL: "https://github.com/o/r"}
	_, _, spans, _ := seg.RenderLinked(context.Background(), asciiStyle(), 0)
	if len(spans) != 0 {
		t.Errorf("repo family off must yield no spans (PR included): %+v", spans)
	}
}

// seg_dirgit_test.go
func TestDirGit_Spans_DirAndBranch(t *testing.T) {
	seg := NewDirGitSegment("/home/u/proj", &gitfake.Runner{Script: gitStatusResponses("main")})
	seg.Links = Links{DirGit: true, Repo: true, RepoURL: "https://github.com/o/r"}
	_, _, spans, ok := seg.RenderLinked(context.Background(), asciiStyle(), 0)
	if !ok || len(spans) != 2 || spans[0].URL != "file:///home/u/proj" || spans[1].URL != "https://github.com/o/r/tree/main" {
		t.Fatalf("spans = %+v", spans)
	}
}

// seg_ai_test.go
func TestAI_Spans_UsageFourFields(t *testing.T) {
	seg := NewAISegment(samplePayload(), "", nil, mcp.ActiveCountOptions{})
	seg.Links = Links{AI: true, UsageURL: DefaultClaudeUsageURL}
	text, _, spans, _ := seg.RenderLinked(context.Background(), asciiStyle(), 0)
	if len(spans) != 4 {
		t.Fatalf("want model/ctx/5h/7d spans, got %+v in %q", spans, text)
	}
	for _, sp := range spans {
		if sp.URL != DefaultClaudeUsageURL || strings.Contains(text[sp.Start:sp.End], "MCP") {
			t.Errorf("bad span %+v", sp)
		}
	}
}

func TestAI_Spans_NoUsageURL(t *testing.T) {
	seg := NewAISegment(samplePayload(), "", nil, mcp.ActiveCountOptions{})
	seg.Links = Links{AI: true}
	if _, _, spans, _ := seg.RenderLinked(context.Background(), asciiStyle(), 0); len(spans) != 0 {
		t.Errorf("no usage URL ⇒ no spans: %+v", spans)
	}
}

// seg_time_test.go
func TestTime_Span_WholeTextAfterGlyph(t *testing.T) {
	seg := NewTimeSegment(fixedClock(), "America/Los_Angeles", "15:04:05", "2006-01-02")
	seg.Links = Links{Time: true, TimeURL: DefaultTimeURL}
	text, _, spans, _ := seg.RenderLinked(context.Background(), asciiStyle(), 0)
	if len(spans) != 1 || spans[0].URL != "https://time.is/Los_Angeles" || spans[0].End != len(text) {
		t.Errorf("spans = %+v text=%q", spans, text)
	}
}
```
Parity: in `detect_test.go` `TestDetectFormat_MatchesRender`, set `deps.Links` on both segment sets (via a `buildGoldenSegmentsLinked` helper that fills `Links{Repo: true, DirGit: true, AI: true, Time: true, RepoURL: "https://github.com/o/r", UsageURL: DefaultClaudeUsageURL, TimeURL: DefaultTimeURL}`) and assert equality — spans must be identical on both paths. Golden: add case `links` (powerline + emoji) using the same helper; `TestGolden_SanityMarkers` for it requires `"\x1b]8;;"` and `"\x1b[4m"`.

- [ ] **Step 2: Run** `cd sdk/gsl && go test ./internal/render/ -run 'Spans|Span_|DetectFormat_MatchesRender|Golden' 2>&1 | head` → FAIL (`seg.Links undefined`).
- [ ] **Step 3: Implement**
`render.go`: add `Links Links` and `RemoteURL string` to `Deps` (doc: `RemoteURL` is the one-exec `git.RemoteWebURL` result, threaded like `GitInfo`); in `BuildSegments` set `s.Links = deps.Links` on every segment; for `ai`: `s.Links.UsageURL = optString(sc.Options, "usage_url", deps.Links.UsageURL)`; for `time`: `s.Links.TimeURL = optString(sc.Options, "link_url", deps.Links.TimeURL)`; for `repo`: `if !s.LinkPR { s.Links.Repo = false }` is WRONG (that would also drop label links) — instead keep `LinkPR` gating only the badge URL as today.
Every segment struct gains `Links Links`; every `*Data` gains `links Links` (+ `repoURL`, `branch` where needed) copied in `detect`.
`seg_repo_data.go` `formatLinked` (rename the current `format` body; `format` becomes `text, key, _ := d.formatLinked(st, level); return text, key`):
```go
func (d *repoData) formatLinked(st style.Style, level int) (string, string, []LinkSpan) {
	var sb spanBuilder
	repoURL := ""
	if d.links.Repo {
		repoURL = d.links.RepoURL
	}
	if g := glyph(st, d.indicatorKey); g != "" {
		sb.linked(g, repoURL)
	}
	if d.label != "" {
		label := d.label
		if level >= 3 {
			label = truncateText(label, repoLabelBudget)
		}
		if label != "" {
			if sb.len() > 0 {
				sb.write(" ")
			}
			sb.linked(label, TreeURL(repoURL, d.branch))
		}
	}
	if d.showPR && d.prNumber > 0 {
		prefix := "PR#"
		if level >= 2 {
			prefix = "#"
		}
		if sb.len() > 0 {
			sb.write(" ")
		}
		prURL := ""
		if d.links.Repo {
			prURL = d.prURL
		}
		sb.linked(prBadgeWithPrefix(st, prefix, d.prNumber, d.prState), prURL)
	}
	if level < 1 && d.showCount && d.worktreeCount >= 2 {
		if sb.len() > 0 {
			sb.write(" ")
		}
		sb.write(countBadge(st, "worktree_count", d.worktreeCount))
	}
	if sb.len() == 0 {
		return "", "", nil
	}
	return sb.String(), d.themeKey, sb.spans
}
```
`seg_repo.go`: delete the hand-built text in `RenderLinked`; new bodies:
```go
func (s *RepoSegment) Render(ctx context.Context, st style.Style, level int) (string, string, bool) {
	text, key, _, ok := s.RenderLinked(ctx, st, level)
	return text, key, ok
}
func (s *RepoSegment) RenderLinked(ctx context.Context, st style.Style, level int) (string, string, []LinkSpan, bool) {
	d, ok := s.detect(ctx)
	if !ok {
		return "", "", nil, false
	}
	text, key, spans := formatLinkedOf(d, st, level)
	return text, key, spans, text != ""
}
```
Apply the identical two-function delegation to `DirGitSegment`, `AISegment`, `TimeSegment` (their old `Render` bodies are deleted; `appendGit`/`abbrev` helpers that become unused are deleted too).
`seg_dirgit_data.go` `formatLinked`: `sb.linked(d.formatDir(level), fileURL)` where `fileURL = FileURL(d.cwd)` when `d.links.DirGit`; `appendGitFormatted` takes `*spanBuilder` and writes the branch via `sb.linked(info.Branch, TreeURL(repoURL, info.Branch))` (`repoURL` = `d.links.RepoURL` when `d.links.Repo`, else ""; detached HEAD ⇒ `TreeURL` gets `"(detached)"` — guard: pass `""` when `d.gitInfo.Detached`).
`seg_ai_data.go` `formatLinked`: replace the `parts` slice with a `spanBuilder` and a `sep()` helper (`if sb.len() > 0 { sb.write(" ") }`); `usage := ""; if d.links.AI { usage = d.links.UsageURL }`; model: write glyph+" " then `sb.linked(name, usage)`; context: glyph+" " then `sb.linked(pct(...)+tokenRatio, usage)`; MCP: plain `sb.write`; rates: `sb.linked("5h "+pct(*d.rate5h), usage)` and `sb.linked("7d "+pct(*d.rate7d), usage)`.
`seg_time_data.go` `formatLinked`: write glyph+" " plain, then build the date/time/tz string exactly as today into `body`, and `sb.linked(body, TimeURL(tpl, d.t, d.loc))` with `tpl = d.links.TimeURL` when `d.links.Time` (store `loc *time.Location` on `timeData` in `detect`).
- [ ] **Step 4: Run** `cd sdk/gsl && go test ./internal/render/ -race 2>&1 | tail -3` → PASS; then `go test ./internal/render/ -run Golden -update` ONLY for the new `links` case (verify `git diff --stat sdk/gsl/internal/render/testdata` shows only `golden_links_*.txt` added).
- [ ] **Step 5: Evidence + commit**
```bash
(cd sdk/gsl && go test ./internal/render/ -race -cover && go vet ./...) 2>&1 | tee docs/mbo/plans/gsl-links/evidence/T3/segments.txt
git add sdk/gsl/internal/render/render.go sdk/gsl/internal/render/seg_repo.go sdk/gsl/internal/render/seg_repo_data.go sdk/gsl/internal/render/seg_dirgit.go sdk/gsl/internal/render/seg_dirgit_data.go sdk/gsl/internal/render/seg_ai.go sdk/gsl/internal/render/seg_ai_data.go sdk/gsl/internal/render/seg_time.go sdk/gsl/internal/render/seg_time_data.go sdk/gsl/internal/render/seg_repo_test.go sdk/gsl/internal/render/seg_dirgit_test.go sdk/gsl/internal/render/seg_ai_test.go sdk/gsl/internal/render/seg_time_test.go sdk/gsl/internal/render/detect_test.go sdk/gsl/internal/render/golden_test.go sdk/gsl/internal/render/testdata/golden_links_powerline.txt sdk/gsl/internal/render/testdata/golden_links_emoji.txt docs/mbo/plans/gsl-links/evidence/T3
git commit -m "feat(gsl): repo, dirgit, ai, and time segments record link spans; legacy Render delegates to detect+format"
```

### Task 4: gff-backed link flags (fail-open, budgeted)
**Files:** Create `sdk/gsl/internal/flags/flags.go`, `flags_test.go`; Modify `sdk/gsl/go.mod` (+`go.sum`).
**Interfaces:** Produces `flags.Links`, `flags.Resolve`, `flags.GFFLookup`, `flags.Namespace`.

- [ ] **Step 1: Failing tests**
```go
package flags

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestResolve_AllTrueByDefault(t *testing.T) {
	got := Resolve(context.Background(), func(string) (bool, error) { return true, nil })
	if got != (Links{true, true, true, true, true}) {
		t.Errorf("got %+v", got)
	}
}

func TestResolve_FalseFlagWins(t *testing.T) {
	look := func(k string) (bool, error) { return k != "gsl.links.time", nil }
	got := Resolve(context.Background(), look)
	if got.Time || !got.Enabled || !got.Repo {
		t.Errorf("got %+v", got)
	}
}

func TestResolve_ErrorIsFailOpen(t *testing.T) {
	look := func(string) (bool, error) { return false, errors.New("unknown source") }
	if got := Resolve(context.Background(), look); got != (Links{true, true, true, true, true}) {
		t.Errorf("errors must fail open: %+v", got)
	}
}

func TestResolve_SlowLookupIsFailOpenAndBounded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	look := func(string) (bool, error) { time.Sleep(200 * time.Millisecond); return false, nil }
	start := time.Now()
	got := Resolve(ctx, look)
	if time.Since(start) > 100*time.Millisecond || got != (Links{true, true, true, true, true}) {
		t.Errorf("slow lookup: took %v, got %+v", time.Since(start), got)
	}
}

func TestResolve_NilLookupIsAllTrue(t *testing.T) {
	if got := Resolve(context.Background(), nil); !got.Enabled {
		t.Errorf("nil lookup must fail open: %+v", got)
	}
}
```
- [ ] **Step 2: Run** `cd sdk/gsl && go test ./internal/flags/` → FAIL (no package).
- [ ] **Step 3: Implement**
```go
// Package flags resolves the gff feature flags gsl consults at render time.
// Every lookup is fail-open: an error, an unregistered source, a missing gff
// schema, or a slow answer all mean "on" — flags only ever turn links OFF.
package flags

import (
	"context"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/pkg/gff"
)

// Namespace is the dotfiles repo's gff namespace (.github/gff/features.yaml).
const Namespace = "com.github.sfc-gh-eraigosa.dotfiles"

const (
	KeyEnabled = "gsl.links.enabled"
	KeyRepo    = "gsl.links.repo"
	KeyDirGit  = "gsl.links.dirgit"
	KeyAI      = "gsl.links.ai"
	KeyTime    = "gsl.links.time"
)

type Links struct{ Enabled, Repo, DirGit, AI, Time bool }

type Lookup func(key string) (bool, error)

// Resolve evaluates the five keys concurrently and returns when all have
// answered or ctx is done; unanswered keys read true.
func Resolve(ctx context.Context, look Lookup) Links {
	out := Links{true, true, true, true, true}
	if look == nil {
		return out
	}
	type ans struct {
		key string
		val bool
	}
	ch := make(chan ans, 5)
	keys := []string{KeyEnabled, KeyRepo, KeyDirGit, KeyAI, KeyTime}
	for _, k := range keys {
		go func(k string) {
			v, err := look(k)
			if err != nil {
				v = true
			}
			ch <- ans{k, v}
		}(k)
	}
	for range keys {
		select {
		case a := <-ch:
			switch a.key {
			case KeyEnabled:
				out.Enabled = a.val
			case KeyRepo:
				out.Repo = a.val
			case KeyDirGit:
				out.DirGit = a.val
			case KeyAI:
				out.AI = a.val
			case KeyTime:
				out.Time = a.val
			}
		case <-ctx.Done():
			return out
		}
	}
	return out
}

// GFFLookup resolves through the registered namespace, then through the
// checkout named by $DOTFILES_DIR (a path source) when the namespace is not
// registered on this host.
func GFFLookup(env func(string) string) Lookup {
	if env == nil {
		env = os.Getenv
	}
	return func(key string) (bool, error) {
		v, err := gff.Bool(key, gff.WithSource(Namespace))
		if err == nil {
			return v, nil
		}
		if dir := env("DOTFILES_DIR"); dir != "" {
			return gff.Bool(key, gff.WithSource(dir))
		}
		return true, err
	}
}
```
`go.mod`: add `github.com/sfc-gh-eraigosa/dotfiles/sdk/gff v0.0.0` to the first `require` block and `replace github.com/sfc-gh-eraigosa/dotfiles/sdk/gff => ../gff`; run `cd sdk/gsl && go mod tidy`.
- [ ] **Step 4: Run** `cd sdk/gsl && go test ./internal/flags/ -race && go build ./...` → PASS. Sanity: `cd sdk/gsl && go run . version >/dev/null && echo build-ok`.
- [ ] **Step 5: Evidence + commit**
```bash
(cd sdk/gsl && go test ./internal/flags/ -race -cover) 2>&1 | tee docs/mbo/plans/gsl-links/evidence/T4/flags.txt
git status --short -- sdk/gsl/internal/flags   # must list the new files (allowlist)
git add sdk/gsl/internal/flags/flags.go sdk/gsl/internal/flags/flags_test.go sdk/gsl/go.mod sdk/gsl/go.sum docs/mbo/plans/gsl-links/evidence/T4
git commit -m "feat(gsl): fail-open gff lookups for the link families"
```

### Task 5: Wiring — config key, CLI, preview, flag declarations, install registration
**Files:** Modify `sdk/gsl/internal/config/config.go`, `config_test.go`, `sdk/gsl/cmd/config.go`, `cmd/config_test.go` (or the existing cmd test file), `cmd/statusline.go`, `cmd/statusline_test.go`, `sdk/gsl/internal/preview/model.go`, `.github/gff/features.yaml`, `install.sh`.
**Interfaces:** Consumes `flags.*`, `git.RemoteWebURL`, `render.Links`, `Style.Links`. Produces config key `links`, flags in the schema.

- [ ] **Step 1: Failing tests**
```go
// internal/config/config_test.go
func TestEffectiveLinks(t *testing.T) {
	for in, want := range map[string]string{"": "underline", "underline": "underline", "plain": "plain", "off": "off", "bogus": "underline"} {
		if got := (Config{Links: in}).EffectiveLinks(); got != want {
			t.Errorf("EffectiveLinks(%q) = %q want %q", in, got, want)
		}
	}
}

// cmd/statusline_test.go
func TestBuildLinks_ConfigOffBeatsFlags(t *testing.T) {
	l := buildLinks("off", flags.Links{true, true, true, true, true}, "https://github.com/o/r", false)
	if l.Repo || l.DirGit || l.AI || l.Time {
		t.Errorf("config off must disable every family: %+v", l)
	}
}

func TestBuildLinks_MasterFlagOff(t *testing.T) {
	l := buildLinks("underline", flags.Links{Enabled: false, Repo: true, DirGit: true, AI: true, Time: true}, "", false)
	if l.Repo || l.Time {
		t.Errorf("gsl.links.enabled=false must disable every family: %+v", l)
	}
}

func TestBuildLinks_Defaults(t *testing.T) {
	l := buildLinks("underline", flags.Links{true, true, true, true, true}, "https://github.com/o/r", false)
	if l.UsageURL != render.DefaultClaudeUsageURL || l.TimeURL != render.DefaultTimeURL || l.RepoURL != "https://github.com/o/r" {
		t.Errorf("defaults: %+v", l)
	}
	if a := buildLinks("underline", flags.Links{true, true, true, true, true}, "", true); a.UsageURL != "" {
		t.Errorf("antigravity must have no usage URL by default: %q", a.UsageURL)
	}
}
```
Plus a `config set links plain` / `config get links` round-trip test in the cmd config test file, and `config set links bogus` returning an error.
- [ ] **Step 2: Run** `cd sdk/gsl && go test ./internal/config/ ./cmd/ -run 'EffectiveLinks|BuildLinks|Links' 2>&1 | head` → FAIL.
- [ ] **Step 3: Implement**
`config.go`: field `Links string \`json:"links,omitempty"\`` after `Style`; 
```go
// EffectiveLinks normalizes Links: "" or an unknown value means "underline".
func (c Config) EffectiveLinks() string {
	switch c.Links {
	case "plain", "off":
		return c.Links
	}
	return "underline"
}
```
`cmd/config.go`: `case "links": fmt.Println(cfg.EffectiveLinks())` in `printConfigKey`; in set: `case "links": if value != "underline" && value != "plain" && value != "off" { return fmt.Errorf("gsl config set links: want underline|plain|off, got %q", value) }; cfg.Links = value`; update the `Use`/help strings listing valid keys.
`cmd/statusline.go`, new helper + call site:
```go
func buildLinks(mode string, lf flags.Links, remoteURL string, antigravity bool) render.Links {
	on := mode != "off" && lf.Enabled
	l := render.Links{
		Repo: on && lf.Repo, DirGit: on && lf.DirGit, AI: on && lf.AI, Time: on && lf.Time,
		RepoURL: remoteURL, TimeURL: render.DefaultTimeURL,
	}
	if !antigravity {
		l.UsageURL = render.DefaultClaudeUsageURL
	}
	return l
}
```
Call site (after `git.Status`): start `flags.Resolve` in a goroutine with `fctx, fcancel := context.WithTimeout(ctx, 100*time.Millisecond)`; run `git.RemoteWebURL(ctx, gitRunner, cwd)` inline (error ⇒ `""`, logged at debug); wait for the flags result; `deps.Links = buildLinks(cfg.EffectiveLinks(), lf, remote, p.IsAntigravity())`; after `style.ResolveConfig`: `st.Links = cfg.EffectiveLinks()`.
`preview/model.go`: in both fixture `Deps` add `Links: render.Links{Repo: true, DirGit: true, AI: true, Time: true, RepoURL: "https://github.com/example/myproject", UsageURL: render.DefaultClaudeUsageURL, TimeURL: render.DefaultTimeURL}`, and set `st.Links = cfg.EffectiveLinks()` where the preview resolves its style.
`.github/gff/features.yaml` (append):
```yaml
  # gsl (Go Status Line) runtime flags — read by `gsl render` on every turn via
  # the gff Go SDK (fail-open: any resolution error leaves the links ON).
  - area: gsl
    features:
      - path: gsl.links.enabled
        description: Master switch for clickable (OSC 8) links on the gsl status line. false removes every link family below; the `links` key in ~/.config/gsl/config.json still controls underline vs plain.
        boolDefault: true
      - path: gsl.links.repo
        description: Repo links — the PR badge to its pull request, the feature/worker/branch label and the dirgit branch to the branch on GitHub, and the root/worktree glyph to the repository home.
        boolDefault: true
      - path: gsl.links.dirgit
        description: Directory link — the dirgit directory name opens the working directory as a file:// URL (file manager in VTE terminals).
        boolDefault: true
      - path: gsl.links.ai
        description: Usage links — model name, context %, and the 5h/7d rate fields open the host's usage page (Claude Code default claude.ai/settings/usage; `usage_url` segment option overrides).
        boolDefault: true
      - path: gsl.links.time
        description: Time link — the date/time opens a timezone page built from the time segment's `link_url` template (default https://time.is/{tz_city}).
        boolDefault: true
```
`install.sh`, inside the `if [ "$gff_bootstrap_ok" = "true" ] …` block after `set +a`:
```sh
  # Register this checkout's namespace so cross-repo consumers (gsl render, from
  # ANY cwd) can resolve the flags. Fail-open: a failure only warns.
  (cd "${BASE_DIR}" && "${HOME}/opt/bin/gff" install >/dev/null 2>&1) \
    || echo "WARNING: gff install (namespace registration) failed; gsl link flags fail open (links stay on)."
```
- [ ] **Step 4: Run**
`cd sdk/gsl && go test ./... -race 2>&1 | tail -8` → all PASS (regenerate only cmd/preview goldens that legitimately changed with `-update`, and review `git diff` of each: the only differences are `]8;;` / `[4m` sequences).
`gff lint` (repo root) → clean. `make lint-shell && make lint-portability` → clean.
Live: `bash sdk/gsl/build.sh` then in the agy_defaults worktree `gsl status | cat -v | grep -c ']8;;'` ≥ 6; `gff set gsl.links.time false && gsl status | cat -v | grep -c 'time.is'` = 0; `gff unset gsl.links.time`.
- [ ] **Step 5: Evidence + commit**
```bash
(cd sdk/gsl && go test ./... -race -cover) 2>&1 | tee docs/mbo/plans/gsl-links/evidence/T5/all-tests.txt
(gff lint && make lint-shell && make lint-portability) 2>&1 | tee docs/mbo/plans/gsl-links/evidence/T5/lints.txt
git add sdk/gsl/internal/config/config.go sdk/gsl/internal/config/config_test.go sdk/gsl/cmd/config.go sdk/gsl/cmd/statusline.go sdk/gsl/cmd/statusline_test.go sdk/gsl/internal/preview/model.go .github/gff/features.yaml install.sh docs/mbo/plans/gsl-links/evidence/T5   # + the cmd config test file and any regenerated golden
git commit -m "feat(gsl): links config key, gff-gated link policy, preview fixture, flag schema, install-time namespace registration"
```

### Task 6: Docs, index, and human-evidenced click check
**Files:** Modify `sdk/gsl/README.md` (repo/ai/time option tables + a "Links" subsection under Configuration), `sdk/gsl/skill/SKILL.md` (options table + flags), `sdk/gsl/docs/design.md` (link-span section replacing the block-link note), `docs/mbo/index.md` (state), `docs/mbo/plans/gsl-links/TRACKING.md`.
- [ ] **Step 1: README** — add rows: repo `link_pr` (kept), ai `usage_url` (string, host default), time `link_url` (template + placeholders); a `links` row in the schema table (`underline|plain|off`); a "Links" subsection: what is linked, Ctrl+click in VTE terminals, the five `gff set gsl.links.* false` switches, fail-open note, `gff install` requirement for cross-repo resolution.
- [ ] **Step 2: SKILL.md** — same options + flags in the "Repo segment options" area (rename to "Link options"), and replace the "About `link_pr`" paragraph with the span model (links are per field; the join layer emits them; zero width).
- [ ] **Step 3: design.md** — short section "Link spans" (offsets, validate/clip/reanchor/shift, why underline SGR, why policy lives in cmd).
- [ ] **Step 4: Human evidence** — in a herdr pane (gnome-terminal), worktree with a PR: Ctrl+click PR badge, label, glyph, dirgit dir, branch, model, time; record each opened URL in `TRACKING.md` §2 and `evidence/T6/click-check.md`. Under `agy`: confirm model has no link.
- [ ] **Step 5: Commit + checkpoint**
```bash
git add sdk/gsl/README.md sdk/gsl/skill/SKILL.md sdk/gsl/docs/design.md docs/mbo/index.md docs/mbo/plans/gsl-links/TRACKING.md docs/mbo/plans/gsl-links/TODO.md docs/mbo/plans/gsl-links/evidence/T6
git commit -m "docs(gsl): document link spans, link options, and the gsl.links.* gff flags"
```
Then push via `gss push` (confirm first) and flip the PR from draft when the TRACKING stop condition is fully ticked.

## 5. Verification mapping
| Spec rule | Test(s) |
| :-- | :-- |
| F1 spans, exact range, balanced | `TestPaintRuns_WrapsExactlyTheSpan`, `TestValidateSpans_DropsBadOnes`, `TestJoin_SpansZeroWidth_BothPaths`, parity `TestDetectFormat_MatchesRender` |
| F2 modes | `TestPaintRuns_PlainModeHasNoUnderline`, `TestEffectiveLinks`, golden `links` |
| F3 repo/dirgit links | `TestNormalizeRemote`, `TestRemoteWebURL_*`, `TestTreeURL`, `TestRepo_Spans_*`, `TestDirGit_Spans_DirAndBranch` |
| F4 file link | `TestFileURL`, `TestDirGit_Spans_DirAndBranch` |
| F5 usage link | `TestAI_Spans_UsageFourFields`, `TestAI_Spans_NoUsageURL`, `TestBuildLinks_Defaults` |
| F6 time link | `TestTimeURL_Placeholders`, `TestTime_Span_WholeTextAfterGlyph` |
| F7 gff gating | `TestResolve_*`, `TestBuildLinks_ConfigOffBeatsFlags`, `TestBuildLinks_MasterFlagOff`, `gff lint` |
| F8 width safety | `TestJoin_SpansZeroWidth_BothPaths`, `TestClipSpans`, `TestReanchorSpans_SurvivesANSIStrip`, `TestShiftSpans`, `TestTruncateToWidth_ClipsSpans`, existing fit property test |
| F9 docs | Task 6 checklist, TRACKING §2 |

## 6. Integration & rollout
Single PR on branch `worktree/gsl` (this herdr worktree; classic gss lane — `gss pr` draft after the first commit, `gss push` per task). Install path unchanged: `bash sdk/gsl/build.sh` refreshes `~/opt/bin/gsl`; `install.sh` gains one `gff install` line. No breakout (sequential; every task touches `render`).

### 6.1 Build leaves / DAG
Not broken out — one worker, tasks strictly ordered T1 → T2 → T3 → T4 → T5 → T6.

## 7. Validation & evidence (show the work)
Coverage: `go test ./... -cover` in `sdk/gsl` ≥60% overall (sdk gate), and `internal/flags` + `internal/git/remote.go` at 100% of their branches. Evidence tree `docs/mbo/plans/gsl-links/evidence/T<n>/` gets every gate's `tee`'d output, committed with its task. Human click check in T6 is the only non-automated gate and is recorded in TRACKING with the observed URLs.

> Produced via `superpowers:writing-plans`. Execute with `superpowers:executing-plans` /
> `subagent-driven-development`, TDD throughout. Update `../index.md` state as it moves.
