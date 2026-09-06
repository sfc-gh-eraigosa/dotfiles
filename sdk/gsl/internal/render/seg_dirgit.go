package render

import (
	"context"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/repo"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// DirGitSegment renders the current directory plus p10k-style git status:
//
//	<dir-glyph> <dir> <branch-glyph> <branch> +N !N ?N <stash> ⇡N ⇣N
//
// The directory is the basename of the working directory, with $HOME
// abbreviated to "~". Git detail is appended only when the injected git.Runner
// reports a repository; outside a repo (Status errors) only the directory is
// shown (still ok=true).
type DirGitSegment struct {
	// Cwd is the working directory to display. When empty, os.Getwd() is used.
	Cwd string
	// Git is the injected git.Runner used for git.Status.
	Git git.Runner
	// Priority is the DROP priority used by the fit loop (config.Segment.Priority,
	// or the built-in default for this type when unset). It is independent of the
	// segment's position in the line.
	Priority int

	// Info is a PRE-COMPUTED git status (from Deps.GitInfo, threaded through
	// BuildSegments). When non-nil the segment MUST NOT shell out: the caller
	// already paid for those two execs. Nil means "not pre-computed; detect it
	// yourself", which keeps every other caller (tests, internal/preview)
	// working unchanged.
	Info *git.Info
	// home overrides $HOME for ~-abbreviation (tests). Empty → os.UserHomeDir.
	home string

	// Links is the link policy (Deps.Links): DirGit gates the directory link,
	// Repo the branch → GitHub tree link. Zero value ⇒ no links.
	Links Links
	// PR is the PRE-COMPUTED pull-request lookup (Deps.PR), used for the
	// directory → vscode.dev "changes" link. Nil ⇒ no PR known.
	PR *repo.RepoInfo
	// DirLink selects the directory link target: "vscode" (default — the
	// vscode.dev view of the PR changes, else of the branch, falling back to
	// file:// off GitHub) or "file" (always file://).
	DirLink string
}

// NewDirGitSegment builds a DirGitSegment. cwd may be "" to fall back to
// os.Getwd at render time.
//
// The signature is deliberately unchanged (out-of-package callers depend on it);
// a pre-computed status is supplied by setting the Info field, which
// BuildSegments does from Deps.GitInfo.
func NewDirGitSegment(cwd string, gitRunner git.Runner) *DirGitSegment {
	return &DirGitSegment{Cwd: cwd, Git: gitRunner}
}

// status returns the git status for dir, reusing the pre-computed Info when the
// caller threaded one in and shelling out only otherwise.
func (s *DirGitSegment) status(ctx context.Context, dir string) (git.Info, bool) {
	if s.Info != nil {
		return *s.Info, true
	}
	if s.Git == nil {
		return git.Info{}, false
	}
	info, err := git.Status(ctx, s.Git, dir)
	if err != nil {
		return git.Info{}, false
	}
	return info, true
}

// Render implements Segment. It delegates to RenderLinked and discards the
// spans, so the legacy path can never drift from the detect/format path.
func (s *DirGitSegment) Render(ctx context.Context, st style.Style, level int) (text, colorKey string, ok bool) {
	text, colorKey, _, ok = s.RenderLinked(ctx, st, level)
	return text, colorKey, ok
}

// RenderLinked implements LinkedSegment: detect once, then format with spans.
func (s *DirGitSegment) RenderLinked(ctx context.Context, st style.Style, level int) (text, colorKey string, spans []LinkSpan, ok bool) {
	d, ok := s.detect(ctx)
	if !ok {
		return "", "", nil, false
	}
	text, colorKey, spans = formatLinkedOf(d, st, level)
	return text, colorKey, spans, text != ""
}
