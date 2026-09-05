package render

// seg_repo_data.go — repoData segmentData + RepoSegment.detect().
//
// Compaction levels for the repo segment:
//
//	level 0: <glyph> <label> PR#157 ⑂4
//	level 1: drop the worktree-count badge   → <glyph> <label> PR#157
//	level 2: shorten the PR badge            → <glyph> <label> #157
//	level 3: ellipsize the repo label        → <glyph> <lab…> #157
//
// Until now repoData's format signature was `format(st style.Style, _ int)` — it
// DISCARDED the level. Its width was therefore flat (~13-44 columns depending on
// the repo name) at every level of the ladder, which meant the repo segment never
// contributed a single column to compaction. Fit had to reach the segment-drop
// tier to shed any width at all, so a long repo name deleted whole segments from
// the line that a few characters of compaction would have saved.

import (
	"context"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/repo"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// repoLabelBudget is the display-width cap applied to the repo label at the
// deepest compaction level.
const repoLabelBudget = 10

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
	// prURL is the PR's web URL. Optional: empty when gh or the gss registry
	// did not report one, which is why link() gates on it rather than deriving
	// a URL from prNumber (the host/owner/repo are not known here).
	prURL string
	// worktreeCount is the number of linked worktrees (0 if not shown).
	worktreeCount int
	// showPR / showCount mirror the segment options.
	showPR    bool
	showCount bool
	// prio is the drop priority (config.Segment.EffectivePriority).
	prio int
}

// priority implements prioritized. The repo/PR you are working in is the last
// thing worth losing from a narrow line — it is the fact that is NOT recoverable
// from anywhere else on screen.
func (d *repoData) priority() int {
	if d.prio != 0 {
		return d.prio
	}
	return config.PriorityRepo
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
		prio:         s.Priority,
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
		if s.LinkPR {
			d.prURL = info.PRURL
		}
	}

	return d, true
}

// format implements segmentData.format for repoData. Pure; no I/O.
//
// Width is monotonically non-increasing in level (spec E5): each level removes
// or shortens exactly one element and never adds one back.
func (d *repoData) format(st style.Style, level int) (text, colorKey string) {
	var b strings.Builder

	if g := glyph(st, d.indicatorKey); g != "" {
		b.WriteString(g)
	}

	// Label. Level 3+ ellipsizes it — grapheme-safely, so a CJK or emoji repo
	// name is cut on a cluster boundary and its true display width is respected.
	if d.label != "" {
		label := d.label
		if level >= 3 {
			label = truncateText(label, repoLabelBudget)
		}
		if label != "" {
			if b.Len() > 0 {
				b.WriteString(" ")
			}
			b.WriteString(label)
		}
	}

	// PR badge. Level 2+ drops the redundant "PR" prefix: in a status line whose
	// segment is already the repo, "#157" is unambiguous.
	if d.showPR && d.prNumber > 0 {
		prefix := "PR#"
		if level >= 2 {
			prefix = "#"
		}
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(prBadgeWithPrefix(st, prefix, d.prNumber, d.prState))
	}

	// Worktree-count badge: the first thing to go. It is a nice-to-have, and the
	// worktree GLYPH already tells you that you are in a linked worktree.
	if level < 1 && d.showCount && d.worktreeCount >= 2 {
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

// formatLinked implements linkedFormatter: the PR badge addresses its PR.
//
// It is gated on the PR badge actually being shown. A hyperlink over a segment
// that displays no PR would be an invisible click target — the whole repo block
// would silently become clickable with nothing on screen explaining why.
func (d *repoData) formatLinked(st style.Style, level int) (string, string, []LinkSpan) {
	text, colorKey := d.format(st, level)
	if text == "" || !d.showPR || d.prNumber <= 0 || d.prURL == "" {
		return text, colorKey, nil
	}
	prefix := "PR#"
	if level >= 2 {
		prefix = "#"
	}
	badge := prBadgeWithPrefix(st, prefix, d.prNumber, d.prState)
	start := strings.LastIndex(text, badge)
	if start < 0 {
		return text, colorKey, nil
	}
	return text, colorKey, []LinkSpan{{Start: start, End: start + len(badge), URL: d.prURL}}
}
