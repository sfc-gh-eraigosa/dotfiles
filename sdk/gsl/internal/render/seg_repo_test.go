package render

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"

	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/gh/fake"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/repo"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

const testBranch = "feature/gsl/edward-raigosa/impl"

func registryPath() string { return filepath.Join("testdata", "registry.json") }

// newRepoSeg builds a RepoSegment wired to fakes for the given location.
func newRepoSeg(isWorktree bool, worktreeCount int, opts map[string]any) (*RepoSegment, *gitfake.Runner) {
	r := &gitfake.Runner{Script: locateResponses(isWorktree, "/wt/gsl", worktreeCount)}
	gh := &ghfake.Runner{}
	seg := NewRepoSegment(r, gh, testBranch, registryPath(), opts)
	return seg, r
}

func TestRepo_NotARepo_Omits(t *testing.T) {
	st := asciiStyle()
	r := &gitfake.Runner{Default: gitfake.Response{Err: errors.New("not a repo")}}
	seg := NewRepoSegment(r, &ghfake.Runner{}, testBranch, registryPath(), nil)
	if _, _, ok := seg.Render(context.Background(), st, 0); ok {
		t.Error("repo: outside a git repo should self-omit")
	}
}

func TestRepo_Indicator_Powerline(t *testing.T) {
	pl := powerlineStyleFixture()

	// root: the segment returns raw text + colorKey "repo_root".
	// Painting happens in the join layer, so we verify the raw content + colorKey.
	seg, _ := newRepoSeg(false, 1, nil)
	got, colorKey, ok := seg.Render(context.Background(), pl, 0)
	if !ok {
		t.Fatal("repo root: want ok=true")
	}
	if !strings.Contains(got, pl.Icons["repo_root"]) {
		t.Errorf("repo root powerline: want repo_root glyph in %q", got)
	}
	if colorKey != "repo_root" {
		t.Errorf("repo root: want colorKey=repo_root, got %q", colorKey)
	}
	// raw text must NOT contain bg escape sequences (painting is in the join
	// layer) — except the PR badge's chip, which sets its own background and
	// hands the block's back (TestPRBadge_FillIsAReadableChip).
	if i := strings.Index(got, "PR#"); i >= 0 {
		if chip := strings.LastIndex(got[:i], "\x1b[48;"); chip >= 0 {
			got = got[:chip] // the text before the badge's chip
		}
	}
	if strings.Contains(got, "48;5;") {
		t.Errorf("repo root: raw text should not contain bg escape, got %q", got)
	}

	// worktree
	seg, _ = newRepoSeg(true, 2, nil)
	got, colorKey, ok = seg.Render(context.Background(), pl, 0)
	if !ok {
		t.Fatal("repo worktree: want ok=true")
	}
	if !strings.Contains(got, pl.Icons["repo_worktree"]) {
		t.Errorf("repo worktree powerline: want repo_worktree glyph in %q", got)
	}
	if colorKey != "repo_worktree" {
		t.Errorf("repo worktree: want colorKey=repo_worktree, got %q", colorKey)
	}
}

func TestRepo_Indicator_Emoji(t *testing.T) {
	em := emojiStyleFixture()

	seg, _ := newRepoSeg(false, 1, nil)
	got, _, _ := seg.Render(context.Background(), em, 0)
	if !strings.Contains(got, "🏠") {
		t.Errorf("repo root emoji: want 🏠 in %q", got)
	}

	seg, _ = newRepoSeg(true, 1, nil)
	got, _, _ = seg.Render(context.Background(), em, 0)
	if !strings.Contains(got, "🌳") {
		t.Errorf("repo worktree emoji: want 🌳 in %q", got)
	}
}

func TestRepo_PR_Shown_And_Omitted(t *testing.T) {
	st := asciiStyle()

	// Registry match has PR #21 OPEN → shown by default.
	seg, _ := newRepoSeg(true, 1, nil)
	got, _, _ := seg.Render(context.Background(), st, 0)
	if !strings.Contains(got, "PR#21") {
		t.Errorf("repo: want PR#21, got %q", got)
	}

	// show_pr=false → no PR.
	seg, _ = newRepoSeg(true, 1, map[string]any{"show_pr": false})
	got, _, _ = seg.Render(context.Background(), st, 0)
	if strings.Contains(got, "PR#") {
		t.Errorf("repo: show_pr=false should omit PR, got %q", got)
	}
}

func TestRepo_Count_Threshold_And_Option(t *testing.T) {
	st := asciiStyle()

	// count < 2 → no badge.
	seg, _ := newRepoSeg(false, 1, nil)
	got, _, _ := seg.Render(context.Background(), st, 0)
	if strings.Contains(got, "wt2") || strings.Contains(got, "wt1") {
		t.Errorf("repo: count<2 should omit count badge, got %q", got)
	}

	// count >= 2 → badge "wt3" (ascii worktree_count glyph is "wt").
	seg, _ = newRepoSeg(true, 3, nil)
	got, _, _ = seg.Render(context.Background(), st, 0)
	if !strings.Contains(got, "wt3") {
		t.Errorf("repo: count>=2 should show wt3, got %q", got)
	}

	// show_count=false → no badge even at count 3.
	seg, _ = newRepoSeg(true, 3, map[string]any{"show_count": false})
	got, _, _ = seg.Render(context.Background(), st, 0)
	if strings.Contains(got, "wt3") {
		t.Errorf("repo: show_count=false should omit badge, got %q", got)
	}
}

func TestRepo_NameModes(t *testing.T) {
	st := asciiStyle()

	cases := []struct {
		mode    string
		want    string
		notWant string
	}{
		{mode: "feature", want: "gsl"},
		{mode: "branch", want: testBranch},
		{mode: "worker", want: "impl"},
		{mode: "off", notWant: "gsl"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			seg, _ := newRepoSeg(true, 1, map[string]any{"name": tc.mode})
			got, _, ok := seg.Render(context.Background(), st, 0)
			if !ok {
				t.Fatalf("repo name=%s: want ok=true", tc.mode)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("repo name=%s: want %q in %q", tc.mode, tc.want, got)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Errorf("repo name=%s: should not contain %q, got %q", tc.mode, tc.notWant, got)
			}
		})
	}
}

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
	if n := linkCount(spans); n != 0 {
		t.Errorf("repo family off must yield no links (PR included): %+v", spans)
	}
}

// TestRepo_UsesPrecomputedPR: when cmd threads the PR lookup in (Deps.PR) the
// segment must not look it up again — the gh fallback runner stays untouched.
func TestRepo_UsesPrecomputedPR(t *testing.T) {
	r := &gitfake.Runner{Script: locateResponses(true, "/nowhere", 1)}
	gh := &ghfake.Runner{}
	seg := NewRepoSegment(r, gh, "some/unregistered-branch", "/nonexistent-registry.json", nil)
	seg.Links = Links{Repo: true}
	seg.PR = &repo.RepoInfo{PRNumber: 42, PRState: "OPEN", PRURL: "https://github.com/o/r/pull/42"}
	text, _, spans, ok := seg.RenderLinked(context.Background(), asciiStyle(), 0)
	if !ok || !strings.Contains(text, "PR#42") || len(spans) != 1 || spans[0].URL != "https://github.com/o/r/pull/42" {
		t.Fatalf("precomputed PR not used: text=%q spans=%+v", text, spans)
	}
	if gh.CallCount() != 0 {
		t.Errorf("gh must not be called when the PR is precomputed; got %d calls", gh.CallCount())
	}
}

// The badge's state color is a palette role — OPEN the OK (dirgit) color,
// MERGED Accent (repo_worktree), CLOSED Crit (video) — so it follows the

// Under Fill, a fg tint on a colored block is unreadable (green on the dark
// palette's blue 39). The badge becomes a chip: the state color as its own

// The PR badge is text and a role — never an escape (plan F11/E17).
func TestPRBadge_TextAndRole(t *testing.T) {
	for state, want := range map[string]string{"OPEN": "ok", "merged": "accent", "CLOSED": "err", "WEIRD": ""} {
		text, role := prBadge("PR#", 7, state)
		if text != "PR#7" || role != want {
			t.Errorf("%s → %q %q, want PR#7 %q", state, text, role, want)
		}
	}
}

// Painted by the join layer: under Fill the badge is a chip — the state colour
// behind readable text — and the block's colours return after it; as a tint
// it is the state colour as foreground.
func TestPRBadge_PaintedByJoin(t *testing.T) {
	st := style.Style{Fill: true, Separator: "space", Theme: map[string]string{"fg": "default", "ok": "42", "repo_root": "39", "err": "203"}}
	b := []segmentBlock{{text: "gsl PR#7", colorKey: "repo_root", links: []LinkSpan{{Start: 4, End: 8, Role: "ok"}}}}
	got := join(st, b)
	want := "\x1b[48;5;39m\x1b[38;5;16mgsl \x1b[48;5;42m\x1b[38;5;16mPR#7\x1b[48;5;39m\x1b[38;5;16m" + ansiReset
	if got != want {
		t.Errorf("fill chip:\n got %q\nwant %q", got, want)
	}
	st.Fill = false
	if got := join(st, b); !strings.Contains(got, "\x1b[38;5;42mPR#7\x1b[38;5;39m") {
		t.Errorf("tint: %q", got)
	}
}

// linkCount counts the spans that are links (a span may only carry a role).
func linkCount(spans []LinkSpan) int {
	n := 0
	for _, sp := range spans {
		if sp.URL != "" {
			n++
		}
	}
	return n
}
