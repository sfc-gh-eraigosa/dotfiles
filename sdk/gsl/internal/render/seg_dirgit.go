package render

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git"
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

// Render implements Segment.
//
// Returns raw (unpainted) text plus the colorKey "dirgit". compactLevel is
// accepted but only level 0 (full detail) is implemented; PHASE 2 will add
// branch abbreviation and compaction for levels 1–3.
func (s *DirGitSegment) Render(ctx context.Context, st style.Style, _ int) (text, colorKey string, ok bool) {
	cwd := s.Cwd
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	if cwd == "" {
		return "", "", false
	}

	var b strings.Builder
	if g := glyph(st, "dirgit"); g != "" {
		b.WriteString(g)
		b.WriteString(" ")
	}
	b.WriteString(s.abbrev(cwd))

	// Git detail (best-effort; omit on any error / cancellation).
	if info, ok := s.status(ctx, cwd); ok {
		s.appendGit(&b, st, info)
	}

	return b.String(), "dirgit", true
}

// abbrev returns the basename of dir, with $HOME collapsed to "~" and the
// repo/home root itself shown as "~".
func (s *DirGitSegment) abbrev(dir string) string {
	home := s.home
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = h
		}
	}
	if home != "" {
		if dir == home {
			return "~"
		}
		if strings.HasPrefix(dir, home+string(os.PathSeparator)) {
			// Show ~/<basename> only as the basename to keep the segment short;
			// a leading "~" indicates we're under home.
			base := filepath.Base(dir)
			return base
		}
	}
	base := filepath.Base(dir)
	if base == "" || base == "." {
		return dir
	}
	return base
}

// appendGit writes the git portion (branch + p10k counts) to b. Nothing is
// written when the branch is empty (defensive).
func (s *DirGitSegment) appendGit(b *strings.Builder, st style.Style, info git.Info) {
	if info.Branch != "" {
		b.WriteString(" ")
		if g := glyph(st, "branch"); g != "" {
			b.WriteString(g)
			b.WriteString(" ")
		}
		b.WriteString(info.Branch)
	}

	writeBadge := func(iconKey string, n int) {
		if n > 0 {
			b.WriteString(" ")
			b.WriteString(countBadge(st, iconKey, n))
		}
	}

	writeBadge("staged", info.Staged)
	writeBadge("unstaged", info.Unstaged)
	writeBadge("untracked", info.Untracked)

	if info.Stashes > 0 {
		b.WriteString(" ")
		b.WriteString(countBadge(st, "stash", info.Stashes))
	}

	writeBadge("ahead", info.Ahead)
	writeBadge("behind", info.Behind)
}
