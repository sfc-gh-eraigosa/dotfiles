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
// already AND-ed the config mode and the gff master flag into them, so a
// segment only asks "is my family on and do I have a URL?".
type Links struct {
	Repo, DirGit, AI, Time     bool
	RepoURL, UsageURL, TimeURL string
}

const (
	// DefaultClaudeUsageURL is the usage page linked from the ai segment under
	// Claude Code. Antigravity has no known public equivalent, so it gets none.
	DefaultClaudeUsageURL = "https://claude.ai/settings/usage"
	// DefaultTimeURL is the time segment's link template.
	DefaultTimeURL = "https://time.is/{tz_city}"
	sgrUnderline   = "\x1b[4m"
	sgrNoUnderline = "\x1b[24m"
)

// spanBuilder records link spans while a segment builds its raw text, so the
// offsets are exact by construction rather than re-derived by searching.
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

// validateSpans returns the spans that are usable over a text of n bytes:
// non-empty URL, in range, non-empty, sorted, and non-overlapping (a later span
// that overlaps an earlier one is dropped). It never panics on bad input.
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
			continue
		}
		kept = append(kept, sp)
		last = sp.End
	}
	return kept
}

// clipSpans keeps the part of every span that lies within the first keptBytes
// of the text (a truncation never puts the ellipsis inside a link).
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
// stripped = term.StripANSI(orig) by locating each span's visible fragment, left
// to right. A fragment that cannot be found is dropped rather than guessed.
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

// shiftSpans moves every span left by shift bytes (the final tier drops the
// leading glyph), clipping at zero and dropping spans that vanish entirely.
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

// TreeURL is the branch page for base (a normalized origin web URL). The
// /tree/<branch> form is GitHub's; other hosts get no branch link.
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

// FileURL is the file:// URL of an absolute working directory.
func FileURL(cwd string) string {
	if cwd == "" || !strings.HasPrefix(cwd, "/") {
		return ""
	}
	u := url.URL{Scheme: "file", Path: cwd}
	return u.String()
}

// TimeURL expands the time segment's link template. Placeholders: {tz} (IANA
// name), {tz_city} (its last path element), {iso_utc} (20060102T150405Z),
// {unix}. Unknown placeholders are left literal.
func TimeURL(tpl string, t time.Time, loc *time.Location) string {
	if tpl == "" {
		return ""
	}
	if loc == nil {
		loc = time.UTC
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
