package cmd

import (
	"context"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/hookguard"
)

var hooksFix bool

var hooksCmd = &cobra.Command{
	Use:   "hooks",
	Short: "Check that the global privacy git hooks run in your repos",
}

var hooksCheckCmd = &cobra.Command{
	Use:   "check [--fix] [paths...]",
	Short: "Find repos whose own core.hooksPath switches off the global privacy hooks",
	Long: `The privacy git hooks are installed globally (global core.hooksPath). A repo
that sets core.hooksPath in its own config replaces them, and nothing says so.
check prints one line per repo with such an override:

  redundant  the repo points at its own .git/hooks, which the global hooks
             already run after their check, so removing the value loses nothing
  foreign    the repo points somewhere else (.husky, /dev/null, …); fix it by
             hand by chaining those hooks from <git dir>/hooks/<name>

With no paths it scans the main checkout behind every gss registry worktree
and the git repos one level under ~/github/*/, ~/GitHub/*/ and ~/git/, plus
each linked worktree of a repo that turns on extensions.worktreeConfig.
--fix unsets the redundant kind (git config --local or --worktree
--unset-all) and only warns about foreign ones; it never touches global
config or hook files. The exit code is non-zero while any override remains.

gss refuses to push from a repo with an override for the same reason.`,
	Run: func(cmd *cobra.Command, args []string) {
		runner := git.NewSystemRunner()
		ctx := context.Background()
		repos := args
		if len(repos) == 0 {
			repos = hookguard.Discover(ctx, runner, registryWorktrees(), scanParents())
		}
		s := hookguard.CheckAll(ctx, runner, repos, hooksFix, cmd.OutOrStdout())
		if s.Remaining > 0 || s.Errors > 0 {
			os.Exit(1)
		}
	},
}

// registryWorktrees lists every worker worktree in the gss registry (all
// repos share one registry).
func registryWorktrees() []string {
	reg, ok := loadRegistry()
	if !ok {
		return nil
	}
	var out []string
	for _, f := range reg.Features {
		for _, w := range f.Workers {
			out = append(out, w.Worktree)
		}
	}
	return out
}

// scanParents is where checkouts live by convention: ~/github/<org>/<repo>,
// ~/GitHub/<org>/<repo> and ~/git/<repo>.
func scanParents() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var parents []string
	for _, root := range []string{"github", "GitHub"} {
		orgs, _ := filepath.Glob(filepath.Join(home, root, "*"))
		parents = append(parents, orgs...)
	}
	return append(parents, filepath.Join(home, "git"))
}

func init() {
	hooksCheckCmd.Flags().BoolVar(&hooksFix, "fix", false, "Unset redundant overrides (the repo's own .git/hooks); warn about foreign ones")
	hooksCmd.AddCommand(hooksCheckCmd)
	rootCmd.AddCommand(hooksCmd)
}
