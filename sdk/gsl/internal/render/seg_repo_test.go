package render

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/gh/fake"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git/fake"
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
	if _, ok := seg.Render(context.Background(), st); ok {
		t.Error("repo: outside a git repo should self-omit")
	}
}

func TestRepo_Indicator_Powerline(t *testing.T) {
	pl := powerlineStyleFixture()

	// root
	seg, _ := newRepoSeg(false, 1, nil)
	got, ok := seg.Render(context.Background(), pl)
	if !ok {
		t.Fatal("repo root: want ok=true")
	}
	if !strings.Contains(got, pl.Icons["repo_root"]) {
		t.Errorf("repo root powerline: want repo_root glyph in %q", got)
	}
	// blue tint (theme repo_root = blue → 38;5;4 fg under fill bg 48;5;4)
	if !strings.Contains(got, "48;5;4") {
		t.Errorf("repo root powerline: want blue bg tint, got %q", got)
	}

	// worktree
	seg, _ = newRepoSeg(true, 2, nil)
	got, ok = seg.Render(context.Background(), pl)
	if !ok {
		t.Fatal("repo worktree: want ok=true")
	}
	if !strings.Contains(got, pl.Icons["repo_worktree"]) {
		t.Errorf("repo worktree powerline: want repo_worktree glyph in %q", got)
	}
	if !strings.Contains(got, "48;5;5") {
		t.Errorf("repo worktree powerline: want magenta bg tint, got %q", got)
	}
}

func TestRepo_Indicator_Emoji(t *testing.T) {
	em := emojiStyleFixture()

	seg, _ := newRepoSeg(false, 1, nil)
	got, _ := seg.Render(context.Background(), em)
	if !strings.Contains(got, "🏠") {
		t.Errorf("repo root emoji: want 🏠 in %q", got)
	}

	seg, _ = newRepoSeg(true, 1, nil)
	got, _ = seg.Render(context.Background(), em)
	if !strings.Contains(got, "🌳") {
		t.Errorf("repo worktree emoji: want 🌳 in %q", got)
	}
}

func TestRepo_PR_Shown_And_Omitted(t *testing.T) {
	st := asciiStyle()

	// Registry match has PR #21 OPEN → shown by default.
	seg, _ := newRepoSeg(true, 1, nil)
	got, _ := seg.Render(context.Background(), st)
	if !strings.Contains(got, "PR#21") {
		t.Errorf("repo: want PR#21, got %q", got)
	}

	// show_pr=false → no PR.
	seg, _ = newRepoSeg(true, 1, map[string]any{"show_pr": false})
	got, _ = seg.Render(context.Background(), st)
	if strings.Contains(got, "PR#") {
		t.Errorf("repo: show_pr=false should omit PR, got %q", got)
	}
}

func TestRepo_Count_Threshold_And_Option(t *testing.T) {
	st := asciiStyle()

	// count < 2 → no badge.
	seg, _ := newRepoSeg(false, 1, nil)
	got, _ := seg.Render(context.Background(), st)
	if strings.Contains(got, "wt2") || strings.Contains(got, "wt1") {
		t.Errorf("repo: count<2 should omit count badge, got %q", got)
	}

	// count >= 2 → badge "wt3" (ascii worktree_count glyph is "wt").
	seg, _ = newRepoSeg(true, 3, nil)
	got, _ = seg.Render(context.Background(), st)
	if !strings.Contains(got, "wt3") {
		t.Errorf("repo: count>=2 should show wt3, got %q", got)
	}

	// show_count=false → no badge even at count 3.
	seg, _ = newRepoSeg(true, 3, map[string]any{"show_count": false})
	got, _ = seg.Render(context.Background(), st)
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
			got, ok := seg.Render(context.Background(), st)
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

func TestRepo_PRBadge_StateTints(t *testing.T) {
	// Direct unit test of prBadge state colouring.
	st := style.Style{}
	if got := prBadge(st, 7, "OPEN"); !strings.Contains(got, "38;5;2") {
		t.Errorf("prBadge OPEN: want green, got %q", got)
	}
	if got := prBadge(st, 7, "MERGED"); !strings.Contains(got, "38;5;5") {
		t.Errorf("prBadge MERGED: want magenta, got %q", got)
	}
	if got := prBadge(st, 7, "CLOSED"); !strings.Contains(got, "38;5;1") {
		t.Errorf("prBadge CLOSED: want red, got %q", got)
	}
	if got := prBadge(st, 7, "WEIRD"); got != "PR#7" {
		t.Errorf("prBadge unknown state: want plain PR#7, got %q", got)
	}
}
