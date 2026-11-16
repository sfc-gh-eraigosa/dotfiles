package hookguard

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

// Discover lists the repos `gss hooks check` scans when given no paths: the
// main checkout behind every registry worktree, plus every git repo directly
// under each of parents (a directory counts when it holds a .git entry, so
// git never walks up from a plain directory into some enclosing repo).
// Repos are de-duplicated by git common dir, since core.hooksPath in a repo's
// own config is shared by all of its worktrees. A linked worktree is listed
// too when the repo turns on extensions.worktreeConfig, since it can then
// carry a value of its own.
func Discover(ctx context.Context, r git.Runner, worktrees, parents []string) []string {
	seen := map[string]bool{}
	var repos []string
	list := func(p string) {
		if !seen[p] {
			seen[p] = true
			repos = append(repos, p)
		}
	}
	add := func(dir string) {
		common, err := revParsePath(ctx, r, dir, "--git-common-dir")
		if err != nil {
			return
		}
		common = canonical(common)
		top, _ := revParsePath(ctx, r, dir, "--show-toplevel")
		if top != "" {
			top = canonical(top)
		}
		if !seen["common:"+common] {
			seen["common:"+common] = true
			main := common
			switch {
			case filepath.Base(common) == ".git":
				main = filepath.Dir(common)
			default:
				// A submodule's git dir is <super>/.git/modules/<name>, whose
				// core.worktree names the main checkout even when dir is one of
				// its linked worktrees. Without one (--separate-git-dir), dir's
				// own toplevel is the best answer; a bare repo keeps its git dir.
				if t, err := revParsePath(ctx, r, common, "--show-toplevel"); err == nil {
					main = canonical(t)
				} else if top != "" {
					main = top
				}
			}
			list(main)
		}
		if top == "" {
			return
		}
		if out, err := r.Run(ctx, "-C", dir, "config", "--bool", "extensions.worktreeConfig"); err != nil || strings.TrimSpace(string(out)) != "true" {
			return
		}
		// Each worktree can carry its own value: list every one git knows,
		// registered with gss or not.
		list(top)
		if out, err := r.Run(ctx, "-C", dir, "worktree", "list", "--porcelain"); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if p, ok := strings.CutPrefix(line, "worktree "); ok {
					if _, err := os.Stat(p); err == nil {
						list(canonical(p))
					}
				}
			}
		}
	}
	for _, wt := range worktrees {
		if wt == "" {
			continue
		}
		if _, err := os.Stat(wt); err != nil {
			continue // stale registry row; audit's job, not ours
		}
		add(wt)
	}
	for _, p := range parents {
		entries, err := os.ReadDir(p)
		if err != nil {
			continue
		}
		for _, e := range entries {
			dir := filepath.Join(p, e.Name())
			if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
				continue // os.Stat follows a symlinked checkout
			}
			if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
				continue
			}
			add(dir)
		}
	}
	sort.Strings(repos)
	return repos
}

// Summary is what CheckAll found.
type Summary struct {
	Checked   int
	Fixed     int // core.hooksPath values removed
	Remaining int // overrides still in place (redundant without --fix, foreign, unfixable)
	Errors    int // paths that could not be inspected or fixed
}

// CheckAll inspects every repo, prints one line per override (and per value
// fixed or error), and with fix unsets the Redundant kind. Foreign values
// are reported with the way to chain them, never changed.
//
// A value in local config is shared by every worktree of the repo. Main
// checkouts are checked before linked worktrees, so a fix of a local value
// happens (and is named) at the main checkout, and a local value that reads
// the same in several worktrees is reported once. A relative value can read
// differently in a linked worktree (.git/hooks there is not the repo's own
// hooks dir); that is its own finding and is reported under that worktree.
func CheckAll(ctx context.Context, r git.Runner, repos []string, fix bool, out io.Writer) Summary {
	var s Summary
	globalMissing := false
	sharedSeen := map[string]bool{}
	ordered := append([]string(nil), repos...)
	sort.SliceStable(ordered, func(i, j int) bool { return isMainCheckout(ordered[i]) && !isMainCheckout(ordered[j]) })
	for _, repo := range ordered {
		res, err := Inspect(ctx, r, repo)
		if err != nil {
			s.Errors++
			fmt.Fprintf(out, "error      %s: %v\n", repo, err)
			continue
		}
		s.Checked++
		if res.Global == "" {
			globalMissing = true
		}
		if res.Kind == OK {
			continue
		}
		if fix && res.Fixable() {
			// A fix can uncover a second value in another scope (worktree, then
			// local). Every value removed gets its own line and counts once, so
			// nothing is changed without saying so, in whatever order repos come.
			var err error
			for n := 0; err == nil && res.Fixable() && n < 4; n++ {
				var after Result
				if after, err = Fix(ctx, r, res); err == nil {
					for _, v := range res.Values { // Fix unsets every value in the scope
						s.Fixed++
						fmt.Fprintf(out, "fixed      %s: removed core.hooksPath = %q (%s config)\n", owner(repo, res), v, res.Scope)
					}
					res = after
				}
			}
			if err != nil {
				s.Errors++
				fmt.Fprintf(out, "error      %s: %v\n", repo, err)
				continue
			}
			if res.Kind == OK {
				continue
			}
		}
		if res.Scope == "local" {
			key := canonical(res.HooksDir) + "\x00" + res.Kind.String() + "\x00" + res.Value
			if sharedSeen[key] {
				continue // already reported under another worktree of this repo
			}
			sharedSeen[key] = true
		}
		s.Remaining++
		switch {
		case res.Kind == Foreign:
			fmt.Fprintf(out, "foreign    %s: core.hooksPath = %q (%s config), so the privacy guard does not run here. Fix by hand: %s\n", repo, res.Value, scopeName(res.Scope), Advice(res))
		default:
			fmt.Fprintf(out, "redundant  %s: core.hooksPath = %q (%s config), so the privacy guard does not run here. Fix: %s\n", repo, res.Value, scopeName(res.Scope), Advice(res))
		}
	}
	if globalMissing {
		fmt.Fprintln(out, "note: the global core.hooksPath is not set, so the privacy git hooks are not installed on this machine (install.sh, gff flag install.git.hooks)")
	}
	fmt.Fprintf(out, "%d repos checked, %d fixed, %d overrides remain\n", s.Checked, s.Fixed, s.Remaining)
	return s
}

// isMainCheckout: dir holds a .git directory (a linked worktree holds a
// .git file pointing into the main one).
func isMainCheckout(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && fi.IsDir()
}

// owner names where a value lives: a local value belongs to the repo, so it
// is named by the main checkout (<main>/.git is the common dir); a worktree
// value, or one in a repo laid out otherwise, by the path that was checked.
func owner(repo string, res Result) string {
	if res.Scope == "local" {
		if common := filepath.Dir(res.HooksDir); filepath.Base(common) == ".git" {
			return filepath.Dir(common)
		}
	}
	return repo
}
