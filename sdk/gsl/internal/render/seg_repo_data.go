package render

// seg_repo_data.go — repoData segmentData + RepoSegment.detect().
//
// The repo segment has no per-level compaction in Phase 2 (it's a "repo extra"
// that is dropped whole in the final tier). detect() captures all the data;
// format() renders the same content at all levels 0-3.

import (
	"context"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/repo"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// repoData is the detect-once intermediate for the repo segment.
type repoData struct {
	// themeKey is "repo_root" or "repo_worktree".
	themeKey string
	// indicatorKey is the glyph key (same value as themeKey in current code).
	indicatorKey string
	// label is the optional feature/worker/branch name label.
	label string
	// prNumber / prState are the PR details (0 / "" if absent).
	prNumber int
	prState  string
	// worktreeCount is the number of linked worktrees (0 if not shown).
	worktreeCount int
	// showPR / showCount mirror the segment options.
	showPR    bool
	showCount bool
}

// detect implements detectable for RepoSegment. Runs repo.Locate + repo.PR once.
func (s *RepoSegment) detect(ctx context.Context) (segmentData, bool) {
	if s.Git == nil {
		return nil, false
	}
	loc, err := repo.Locate(ctx, s.Git, "")
	if err != nil {
		return nil, false
	}

	var info *repo.RepoInfo
	if s.NameMode == nameModeFeature || s.ShowPR {
		if pr, perr := repo.PR(ctx, s.GH, s.Branch, loc.Toplevel, s.RegistryPath); perr == nil {
			info = pr
		}
	}

	themeKey := "repo_root"
	indicatorKey := "repo_root"
	if loc.IsWorktree {
		themeKey = "repo_worktree"
		indicatorKey = "repo_worktree"
	}

	d := &repoData{
		themeKey:     themeKey,
		indicatorKey: indicatorKey,
		label:        s.nameLabel(info),
		showPR:       s.ShowPR,
		showCount:    s.ShowCount,
		worktreeCount: func() int {
			if s.ShowCount && loc.WorktreeCount >= 2 {
				return loc.WorktreeCount
			}
			return 0
		}(),
	}

	if s.ShowPR && info != nil && info.PRNumber > 0 {
		d.prNumber = info.PRNumber
		d.prState = info.PRState
	}

	return d, true
}

// format implements segmentData.format for repoData. Pure; no I/O.
// The repo segment has no per-level text compaction in Phase 2 (it is dropped
// whole by the final tier if needed).
func (d *repoData) format(st style.Style, _ int) (text, colorKey string) {
	var b strings.Builder

	if g := glyph(st, d.indicatorKey); g != "" {
		b.WriteString(g)
	}

	if d.label != "" {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(d.label)
	}

	if d.showPR && d.prNumber > 0 {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(prBadge(st, d.prNumber, d.prState))
	}

	if d.showCount && d.worktreeCount >= 2 {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(countBadge(st, "worktree_count", d.worktreeCount))
	}

	if b.Len() == 0 {
		return "", ""
	}
	return b.String(), d.themeKey
}
