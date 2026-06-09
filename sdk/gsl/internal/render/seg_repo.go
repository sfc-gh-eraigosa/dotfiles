package render

import (
	"context"
	"strconv"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/repo"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// Repo name modes (the "name" option of the repo segment).
const (
	nameModeFeature = "feature" // gss registry feature name (default)
	nameModeWorker  = "worker"  // trailing branch segment (gss worker purpose)
	nameModeBranch  = "branch"  // raw branch name
	nameModeOff     = "off"     // no name label
)

// RepoSegment renders a root/worktree indicator, an optional feature/worker/
// branch name label, an optional PR number (tinted by state), and an optional
// worktree count badge.
//
// It needs NO payload, so it renders in both Claude (live) and Gemini/CLI
// (on-demand) modes. Outside a git repository the whole segment self-omits
// (ok == false).
type RepoSegment struct {
	// Git is the injected git.Runner used by repo.Locate.
	Git git.Runner
	// GH is the injected gh.Runner used by repo.PR's fallback path.
	GH gh.Runner
	// Branch is the current branch name (from git.Status, supplied by the
	// caller). Used for registry/gh PR lookup and the "branch"/"worker" name
	// modes. May be empty.
	Branch string
	// RegistryPath is the gss registry path passed to repo.PR.
	RegistryPath string

	// Options (from config Segment.Options), already coerced.
	ShowPR    bool   // default true
	ShowCount bool   // default true
	NameMode  string // "feature" | "worker" | "branch" | "off"; default "feature"
}

// NewRepoSegment builds a RepoSegment, applying option defaults from opts.
func NewRepoSegment(gitRunner git.Runner, ghRunner gh.Runner, branch, registryPath string, opts map[string]any) *RepoSegment {
	s := &RepoSegment{
		Git:          gitRunner,
		GH:           ghRunner,
		Branch:       branch,
		RegistryPath: registryPath,
		ShowPR:       optBool(opts, "show_pr", true),
		ShowCount:    optBool(opts, "show_count", true),
		NameMode:     optString(opts, "name", nameModeFeature),
	}
	return s
}

// Render implements Segment.
//
// Returns raw (unpainted) text plus a dynamic colorKey: "repo_root" for the
// main worktree, "repo_worktree" for a linked worktree. compactLevel is
// accepted but only level 0 (full detail) is implemented; PHASE 2 will pass
// the level through for future compaction.
func (s *RepoSegment) Render(ctx context.Context, st style.Style, _ int) (text, colorKey string, ok bool) {
	if s.Git == nil {
		return "", "", false
	}
	loc, err := repo.Locate(ctx, s.Git, "")
	if err != nil {
		// Not a git repo (or git unavailable) → omit the whole segment.
		return "", "", false
	}

	// PR / feature lookup (best-effort; nil or error ⇒ omit those parts).
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

	var b strings.Builder
	if g := glyph(st, indicatorKey); g != "" {
		b.WriteString(g)
	}

	if label := s.nameLabel(info); label != "" {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(label)
	}

	if s.ShowPR && info != nil && info.PRNumber > 0 {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		// prBadge inlines ANSI tints for the PR state color, but these are
		// sub-field tints within the segment text — they are NOT the segment's
		// primary colorKey and are acceptable as embedded sequences here because
		// they restore the segment's own fg color before returning. This is a
		// known exception: prBadge's tinting is scoped to the number only and
		// does not embed a full reset that would interfere with the join layer.
		b.WriteString(prBadge(st, info.PRNumber, info.PRState))
	}

	if s.ShowCount && loc.WorktreeCount >= 2 {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(countBadge(st, "worktree_count", loc.WorktreeCount))
	}

	if b.Len() == 0 {
		return "", "", false
	}
	return b.String(), themeKey, true
}

// nameLabel resolves the segment's name label per NameMode.
func (s *RepoSegment) nameLabel(info *repo.RepoInfo) string {
	switch s.NameMode {
	case nameModeOff:
		return ""
	case nameModeBranch:
		return s.Branch
	case nameModeWorker:
		return workerLabel(s.Branch)
	case nameModeFeature:
		fallthrough
	default:
		if info != nil && info.FeatureName != "" {
			return info.FeatureName
		}
		return ""
	}
}

// workerLabel returns the trailing path segment of a gss-style branch name
// (e.g. "feature/gsl/edward-raigosa/impl" → "impl"). For a branch with no
// slash the whole branch is returned. Empty branch → "".
func workerLabel(branch string) string {
	if branch == "" {
		return ""
	}
	idx := strings.LastIndex(branch, "/")
	if idx < 0 {
		return branch
	}
	return branch[idx+1:]
}

// prBadge renders "PR#<n>" tinted by PR state. OPEN → green, MERGED → magenta,
// CLOSED → red; any other/empty state is left untinted. The "#" prefix keeps
// the badge readable across glyph modes (no dedicated PR glyph in the style
// table).
//
// When st.Fill is true the badge tint colour is applied for the number only,
// then the segment's own fg is restored so that any content appended after the
// badge (e.g. the worktree-count badge) stays inside the same powerline
// background block. The single trailing ansiReset for the whole segment is
// owned by paint(), NOT by this helper.
func prBadge(st style.Style, number int, state string) string {
	text := "PR#" + strconv.Itoa(number)
	var colorKey string
	switch strings.ToUpper(state) {
	case "OPEN":
		colorKey = "green"
	case "MERGED":
		colorKey = "magenta"
	case "CLOSED":
		colorKey = "red"
	default:
		return text
	}
	seq := fgSeq(colorKey)
	if seq == "" {
		return text
	}
	if st.Fill {
		// Re-emit the segment foreground after the tinted number so that
		// subsequent parts of the same segment retain the powerline background.
		segFG := fgSeq(themeColor(st, "fg"))
		if segFG == "" {
			segFG = fgSeq("white")
		}
		return seq + text + segFG
	}
	return seq + text + ansiReset
}
