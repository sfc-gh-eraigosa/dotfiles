package render

import (
	"context"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

// testdata/registry.json supplies PR#21 with this URL.
const wantPRURL = "https://github.com/sfc-gh-eraigosa/dotfiles/pull/21"

func osc8Open(url string) string { return "\x1b]8;;" + url + "\x1b\\" }

const osc8Close = "\x1b]8;;\x1b\\"

// TestRepo_RenderLinked_ReportsPRURL: the segment reports a BARE url — the join
// layer, not the segment, owns escape emission.
func TestRepo_RenderLinked_ReportsPRURL(t *testing.T) {
	seg, _ := newRepoSeg(false, 1, nil)
	_, _, spans, ok := seg.RenderLinked(context.Background(), asciiStyle(), 0)
	if !ok {
		t.Fatal("want ok=true")
	}
	if len(spans) == 0 || spans[len(spans)-1].URL != wantPRURL {
		t.Fatalf("spans = %+v, want last span URL %q", spans, wantPRURL)
	}
	if strings.Contains(spans[len(spans)-1].URL, "\x1b") {
		t.Errorf("link must be a bare URL, got an escape sequence: %q", spans[len(spans)-1].URL)
	}
}

// TestRepo_LinkPRDisabled: opting out yields no link but keeps the badge.
func TestRepo_LinkPRDisabled(t *testing.T) {
	seg, _ := newRepoSeg(false, 1, map[string]any{"link_pr": false})
	text, _, spans, ok := seg.RenderLinked(context.Background(), asciiStyle(), 0)
	if !ok {
		t.Fatal("want ok=true")
	}
	if len(spans) != 0 {
		t.Errorf("link_pr=false: want no link, got %+v", spans)
	}
	if !strings.Contains(text, "PR#21") {
		t.Errorf("link_pr=false must not hide the badge; got %q", text)
	}
}

// TestRepo_NoLinkWhenPRHidden: a link with no visible badge would be an
// invisible click target, so show_pr=false must also suppress the link.
func TestRepo_NoLinkWhenPRHidden(t *testing.T) {
	seg, _ := newRepoSeg(false, 1, map[string]any{"show_pr": false})
	_, _, spans, ok := seg.RenderLinked(context.Background(), asciiStyle(), 0)
	if !ok {
		t.Fatal("want ok=true")
	}
	if len(spans) != 0 {
		t.Errorf("show_pr=false: want no link, got %+v", spans)
	}
}

// TestJoin_EmitsOSC8 covers both join paths: the classic separator path and the
// powerline ribbon.
func TestJoin_EmitsOSC8(t *testing.T) {
	blocks := []segmentBlock{
		{text: "left", colorKey: "repo_root"},
		{text: "PR#21", colorKey: "repo_root", links: []LinkSpan{{0, 5, wantPRURL}}},
	}
	for _, st := range []struct {
		name  string
		style style.Style
	}{
		{"classic", asciiStyle()},
		{"powerline", powerlineStyleFixture()},
	} {
		t.Run(st.name, func(t *testing.T) {
			got := join(st.style, blocks)
			if !strings.Contains(got, osc8Open(wantPRURL)) {
				t.Errorf("missing hyperlink opener in %q", got)
			}
			if !strings.Contains(got, osc8Close) {
				t.Errorf("missing hyperlink closer in %q", got)
			}
			// The unlinked block must not be wrapped.
			if strings.Count(got, osc8Open(wantPRURL)) != 1 {
				t.Errorf("want exactly one hyperlink, got %d in %q",
					strings.Count(got, osc8Open(wantPRURL)), got)
			}
		})
	}
}

// TestJoin_OSC8_DoesNotChangeWidth is the invariant the fit loop depends on: a
// hyperlink is decoration, so it must add zero display width no matter how long
// the URL is. Without OSC support in the stripper this fails by ~55 columns.
func TestJoin_OSC8_DoesNotChangeWidth(t *testing.T) {
	plain := []segmentBlock{{text: "PR#21", colorKey: "repo_root"}}
	linked := []segmentBlock{{text: "PR#21", colorKey: "repo_root",
		links: []LinkSpan{{0, 5, "https://github.com/some-org/a-very-long-repository-name/pull/21"}}}}
	for _, st := range []struct {
		name  string
		style style.Style
	}{
		{"classic", asciiStyle()},
		{"powerline", powerlineStyleFixture()},
	} {
		t.Run(st.name, func(t *testing.T) {
			got := term.DisplayWidth(join(st.style, linked))
			want := term.DisplayWidth(join(st.style, plain))
			if got != want {
				t.Errorf("linked width = %d, want %d (hyperlink must be zero-width)", got, want)
			}
		})
	}
}

// TestTruncate_PreservesLink: the truncation path rebuilds blocks, and an
// earlier version of this change silently dropped the link there.
func TestTruncate_PreservesLink(t *testing.T) {
	st := asciiStyle()
	blocks := []segmentBlock{
		{text: "a-long-repo-label PR#21", colorKey: "repo_root", links: []LinkSpan{{0, 17, wantPRURL}}},
	}
	out := truncateToWidth(blocks, st, 12)
	if len(out) == 0 {
		t.Fatal("want at least one surviving block")
	}
	if len(out[0].links) != 1 || out[0].links[0].URL != wantPRURL {
		t.Fatalf("truncation dropped the link: got %+v, want %q", out[0].links, wantPRURL)
	}
	if end := out[0].links[0].End; end > len(out[0].text)-len(ellipsis) {
		t.Errorf("link span (end %d) must stop before the ellipsis in %q", end, out[0].text)
	}
}
