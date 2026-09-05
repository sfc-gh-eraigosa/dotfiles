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
// It needs NO payload, so it renders in both Claude (live) and Antigravity/CLI
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
	ShowPR    bool // default true
	ShowCount bool // default true
	// LinkPR emits an OSC 8 hyperlink to the PR over the repo segment
	// (default true). Terminals without OSC 8 ignore the sequence, so this
	// exists for the rarer terminal that PRINTS unknown escapes instead of
	// swallowing them.
	LinkPR   bool
	NameMode string // "feature" | "worker" | "branch" | "off"; default "feature"

	// Priority is the DROP priority used by the fit loop (config.Segment.Priority,
	// or the built-in default for this type when unset). It is independent of the
	// segment's position in the line.
	Priority int

	// Links is the link policy (Deps.Links). Repo gates the glyph → repo home,
	// label → branch tree, and PR badge → PR links; LinkPR additionally gates
	// the badge. Zero value ⇒ no links.
	Links Links
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
		LinkPR:       optBool(opts, "link_pr", true),
		NameMode:     optString(opts, "name", nameModeFeature),
	}
	return s
}

// Render implements Segment. It delegates to RenderLinked and discards the
// spans, so the legacy path can never drift from the detect/format path.
func (s *RepoSegment) Render(ctx context.Context, st style.Style, level int) (text, colorKey string, ok bool) {
	text, colorKey, _, ok = s.RenderLinked(ctx, st, level)
	return text, colorKey, ok
}

// RenderLinked implements LinkedSegment: detect once, then format with spans.
func (s *RepoSegment) RenderLinked(ctx context.Context, st style.Style, level int) (text, colorKey string, spans []LinkSpan, ok bool) {
	d, ok := s.detect(ctx)
	if !ok {
		return "", "", nil, false
	}
	text, colorKey, spans = formatLinkedOf(d, st, level)
	return text, colorKey, spans, text != ""
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
	return prBadgeWithPrefix(st, "PR#", number, state)
}

// prBadgeWithPrefix is prBadge with a caller-chosen prefix, so the compaction
// ladder can shrink "PR#157" to "#157" at deeper levels without duplicating the
// state-tinting logic.
func prBadgeWithPrefix(st style.Style, prefix string, number int, state string) string {
	text := prefix + strconv.Itoa(number)
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
	seq := fgSeq(colorKey, true)
	if seq == "" {
		return text
	}
	if st.Fill {
		// Re-emit the segment foreground after the tinted number so that
		// subsequent parts of the same segment retain the powerline background.
		segFG := fgSeq(themeColor(st, "fg"), true)
		if segFG == "" {
			segFG = fgSeq("white", true)
		}
		return seq + text + segFG
	}
	return seq + text + ansiReset
}
