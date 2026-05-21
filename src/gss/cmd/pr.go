package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/wenlock/dotfiles/gss/internal/approval"
	"github.com/wenlock/dotfiles/gss/internal/classic"
	"github.com/wenlock/dotfiles/gss/internal/config"
	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	"github.com/wenlock/dotfiles/gss/internal/git"
)

var prForceAutonomous bool

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Push current branch and open a pull request (or create a new feature branch if on default)",
	Run: func(cmd *cobra.Command, args []string) {
		path := getRepoPath()
		cwd, _ := os.Getwd()

		// Mode gate (shared with push; PR-26 formalizes via internal/mode):
		// `gss pr` is classic-mode and invalid inside a feature worker
		// worktree, even with --force-autonomous.
		reg, _ := loadRegistry()
		ref, inWorker := isWorkerWorktree(cwd, reg)
		if err := classicAllowed(inWorker, prForceAutonomous); err != nil {
			fmt.Fprintf(os.Stderr, "Error: 'gss pr' is not valid inside feature worker worktree %q; use the gss feature commands.\n", ref)
			os.Exit(errors.ExitCode(err))
		}

		// Wire and run the classic pr orchestrator (PR-24).
		runner := git.NewSystemRunner()
		home, _ := os.UserHomeDir()
		tokenPath := filepath.Join(home, ".config", "gss", "approval.token")
		prer := &classic.PRer{
			Git:      runner,
			GH:       gh.NewSystemClient(),
			Approval: approval.NewVerifier(tokenPath, runner),
			Clock:    config.SystemClock{},
			Out:      os.Stdout,
		}
		if err := prer.PR(context.Background(), classic.PROpts{
			RepoPath:        path,
			DefaultBranch:   getDefaultBranch(),
			ForceAutonomous: prForceAutonomous,
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(errors.ExitCode(err))
		}
	},
}

func init() {
	prCmd.Flags().BoolVar(&prForceAutonomous, "force-autonomous", false, "Skip the approval prompt (Dangerous)")
	rootCmd.AddCommand(prCmd)
}
