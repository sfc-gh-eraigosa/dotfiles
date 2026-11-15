package hookguard

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

// Discover lists the repos `gss hooks check` scans when given no paths: the
// main checkout behind every registry worktree, plus every git repo directly
// under each of parents (a directory counts when it holds a .git entry, so
// git never walks up from a plain directory into some enclosing repo).
// Repos are de-duplicated by git common dir, since core.hooksPath in a repo's
// own config is shared by all of its worktrees.
func Discover(ctx context.Context, r git.Runner, worktrees, parents []string) []string {
	seen := map[string]bool{}
	var repos []string
	add := func(dir string) {
		common, err := revParsePath(ctx, r, dir, "--git-common-dir")
		if err != nil {
			return
		}
		common = canonical(common)
		if seen[common] {
			return
		}
		seen[common] = true
		main := common
		if filepath.Base(common) == ".git" {
			main = filepath.Dir(common)
		}
		repos = append(repos, main)
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
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(p, e.Name())
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
	Fixed     int
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
			after, err := Fix(ctx, r, res)
			if err != nil {
				s.Errors++
				fmt.Fprintf(out, "error      %s: %v\n", repo, err)
				continue
			}
			if after.Kind == OK {
				s.Fixed++
				fmt.Fprintf(out, "fixed      %s: removed core.hooksPath = %q (%s config); the global hooks run here now\n", repo, res.Value, res.Scope)
				continue
			}
			res = after // something else still overrides (e.g. a second scope)
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
