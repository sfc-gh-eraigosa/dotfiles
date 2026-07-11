package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
)

var rebaseWorker string

var featureRebaseCmd = &cobra.Command{
	Use:   "rebase",
	Short: "Rebase the worker on its base branch and update the PR",
	Long: `Convenience: rebase the current worker on its current base branch and update
the PR base — useful when a parent gained commits but a full checkpoint isn't
wanted yet. Resolves the worker from cwd unless --worker is given.`,
	Run: func(cmd *cobra.Command, args []string) {
		svc, err := newFeatureService()
		if err != nil {
			fail(err)
		}
		ref := rebaseWorker
		if ref == "" {
			ref, err = currentWorkerRef()
			if err != nil {
				fail(err)
			}
		}
		if err := svc.Rebase(context.Background(), feature.RebaseOpts{WorkerRef: ref}); err != nil {
			fail(err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Rebased %s on its base branch\n", ref)
	},
}

func init() {
	featureRebaseCmd.Flags().StringVar(&rebaseWorker, "worker", "", "Worker ref (default: resolved from cwd)")
	featureCmd.AddCommand(featureRebaseCmd)
}
