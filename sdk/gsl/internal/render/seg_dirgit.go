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
	// home overrides $HOME for ~-abbreviation (tests). Empty → os.UserHomeDir.
	home string
}

// NewDirGitSegment builds a DirGitSegment. cwd may be "" to fall back to
// os.Getwd at render time.
func NewDirGitSegment(cwd string, gitRunner git.Runner) *DirGitSegment {
	return &DirGitSegment{Cwd: cwd, Git: gitRunner}
}

// Render implements Segment.
func (s *DirGitSegment) Render(ctx context.Context, st style.Style) (string, bool) {
	cwd := s.Cwd
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	if cwd == "" {
		return "", false
	}

	var b strings.Builder
	if g := glyph(st, "dirgit"); g != "" {
		b.WriteString(g)
		b.WriteString(" ")
	}
	b.WriteString(s.abbrev(cwd))

	// Git detail (best-effort; omit on any error / cancellation).
	if s.Git != nil {
		if info, err := git.Status(ctx, s.Git, cwd); err == nil {
			s.appendGit(&b, st, info)
		}
	}

	return paint(st, "dirgit", b.String()), true
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
