package render

// seg_dirgit_data.go — dirGitData segmentData + DirGitSegment.detect().
//
// Compaction levels for the dirgit segment:
//   level 0: <glyph> <basename> <branch-glyph> <branch> +N !N ?N <stash> ⇡N ⇣N
//   level 1: truncate branch to last component (feature/a/b → b)
//   level 2: show only first letter of branch last component (b → b, "impl" → "i")
//   level 3: omit the branch name entirely (show only the directory)

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/repo"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// dirGitData is the detect-once intermediate for the dirgit segment.
type dirGitData struct {
	// cwd is the resolved working directory.
	cwd string
	// home is the resolved home directory (for ~ abbreviation).
	home string
	// gitInfo is the git status (nil if outside a repo).
	gitInfo *git.Info
	// hasGit is true when gitInfo was populated.
	hasGit bool
	// prio is the drop priority (config.Segment.EffectivePriority).
	prio int
	// links is the policy the spans are gated on.
	links Links
	// pr / dirLink feed the directory link (see DirGitSegment).
	pr      *repo.RepoInfo
	dirLink string
}

// priority implements prioritized.
func (d *dirGitData) priority() int {
	if d.prio != 0 {
		return d.prio
	}
	return config.PriorityDirGit
}

// detect implements detectable for DirGitSegment. Runs git.Status once.
func (s *DirGitSegment) detect(ctx context.Context) (segmentData, bool) {
	cwd := s.Cwd
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	if cwd == "" {
		return nil, false
	}

	home := s.home
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = h
		}
	}

	d := &dirGitData{cwd: cwd, home: home, prio: s.Priority, links: s.Links, pr: s.PR, dirLink: s.DirLink}

	// Reuse the pre-threaded status when the caller already computed it
	// (Deps.GitInfo); shell out only when it is nil. This is the OTHER half of
	// the 4-execs-where-2-suffice fix — cmd used to run git.Status serially for
	// the branch and then this segment ran it AGAIN inside Detect.
	if info, ok := s.status(ctx, cwd); ok {
		d.gitInfo = &info
		d.hasGit = true
	}

	return d, true
}

// format implements segmentData.format for dirGitData. Pure; no I/O.
func (d *dirGitData) format(st style.Style, level int) (text, colorKey string) {
	text, colorKey, _ = d.formatLinked(st, level)
	return text, colorKey
}

// formatLinked implements linkedFormatter. Links: the directory name → the
// vscode.dev view of the PR changes, else of the branch, else file:// (DirGit
// family; dir_link=file forces file://); the branch → the branch on GitHub
// (Repo family).
func (d *dirGitData) formatLinked(st style.Style, level int) (string, string, []LinkSpan) {
	var sb spanBuilder

	if g := glyph(st, "dirgit"); g != "" {
		sb.write(g)
		sb.write(" ")
	}

	// Directory display: depends on compaction level.
	sb.linked(d.formatDir(level), d.dirURL())

	// Git detail: omit entirely at level 3.
	if level < 3 && d.hasGit && d.gitInfo != nil {
		d.appendGitFormatted(&sb, st, level)
	}

	return sb.String(), "dirgit", sb.spans
}

// dirURL is the directory link: with the DirGit family on and dir_link not
// "file", the vscode.dev changes view of the PR when one is known, else the
// vscode.dev view of the branch (GitHub remotes only); otherwise file://.
func (d *dirGitData) dirURL() string {
	if !d.links.DirGit {
		return ""
	}
	if d.dirLink != "file" && d.links.RepoURL != "" {
		if d.pr != nil && d.pr.PRNumber > 0 {
			if u := VSCodeDevPRURL(d.links.RepoURL, d.pr.PRNumber); u != "" {
				return u
			}
		}
		if d.hasGit && d.gitInfo != nil && !d.gitInfo.Detached {
			if u := VSCodeDevTreeURL(d.links.RepoURL, d.gitInfo.Branch); u != "" {
				return u
			}
		}
	}
	return FileURL(d.cwd)
}

// formatDir returns the directory label for the given compaction level.
//
//	All levels: basename only (same as existing abbrev behavior: ~ for home, or last component).
//	The directory is not compacted further; branch compaction provides the width reduction.
func (d *dirGitData) formatDir(_ int) string {
	return abbrevDir(d.cwd, d.home)
}

// abbrevDir returns the basename of dir with $HOME collapsed to "~".
// This matches the original DirGitSegment.abbrev logic.
func abbrevDir(dir, home string) string {
	if home != "" {
		if dir == home {
			return "~"
		}
		if strings.HasPrefix(dir, home+string(os.PathSeparator)) {
			return filepath.Base(dir)
		}
	}
	base := filepath.Base(dir)
	if base == "" || base == "." {
		return dir
	}
	return base
}

// appendGitFormatted writes the git portion of the segment at the given level.
//
//	level 0: full branch name + all badges
//	level 1: last branch component (feature/gsl/impl → impl) + badges
//	level 2: first letter of last component (impl → i) + badges
//	level 3: no branch at all (caller already skips calling this at level 3)
func (d *dirGitData) appendGitFormatted(b *spanBuilder, st style.Style, level int) {
	info := *d.gitInfo
	if info.Branch == "" {
		return
	}

	b.write(" ")
	if g := glyph(st, "branch"); g != "" {
		b.write(g)
		b.write(" ")
	}
	// The link targets the FULL branch even when the text is abbreviated below;
	// a detached HEAD has no branch page.
	treeURL := ""
	if d.links.Repo && !info.Detached {
		treeURL = TreeURL(d.links.RepoURL, info.Branch)
	}
	branch := info.Branch
	switch level {
	case 1:
		// Show only the last slash-delimited segment.
		if idx := strings.LastIndex(branch, "/"); idx >= 0 {
			branch = branch[idx+1:]
		}
	case 2:
		// Show only the first letter of the last slash-delimited segment.
		if idx := strings.LastIndex(branch, "/"); idx >= 0 {
			branch = branch[idx+1:]
		}
		if runes := []rune(branch); len(runes) > 0 {
			branch = string(runes[:1])
		}
	}
	b.linked(branch, treeURL)

	// Status badges: shown at all levels (even compacted ones).
	writeBadge := func(iconKey string, n int) {
		if n > 0 {
			b.write(" ")
			b.write(countBadge(st, iconKey, n))
		}
	}

	writeBadge("staged", info.Staged)
	writeBadge("unstaged", info.Unstaged)
	writeBadge("untracked", info.Untracked)

	if info.Stashes > 0 {
		b.write(" ")
		b.write(countBadge(st, "stash", info.Stashes))
	}

	writeBadge("ahead", info.Ahead)
	writeBadge("behind", info.Behind)
}
