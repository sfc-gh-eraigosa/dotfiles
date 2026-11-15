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
			case top != "":
				main = top // a submodule: its git dir is <super>/.git/modules/<name>
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

// CheckAll inspects every repo, prints one line per repo that has an
// override (and per fix or error), and with fix unsets the Redundant kind.
// Foreign values are reported with the way to chain them, never changed.
func CheckAll(ctx context.Context, r git.Runner, repos []string, fix bool, out io.Writer) Summary {
	var s Summary
	globalMissing := false
	for _, repo := range repos {
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
					s.Fixed++
					fmt.Fprintf(out, "fixed      %s: removed core.hooksPath = %q (%s config)\n", repo, res.Value, res.Scope)
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
