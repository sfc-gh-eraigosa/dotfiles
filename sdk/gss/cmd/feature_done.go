package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
)

var doneForce bool

var featureDoneCmd = &cobra.Command{
	Use:   "done [<worker-ref>]",
	Short: "Tear down a worker (remove worktree + registry row)",
	Long: `Remove a worker: refuses on a dirty worktree, remaining dependents, or an
open/unmerged PR unless --force; then removes the worktree and drops the
registry row. When that empties the feature and FEATURE.md is unedited, the
feature row + file are deleted too. Resolves the worker from cwd when no
<worker-ref> is given.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		svc, err := newFeatureService()
		if err != nil {
			fail(err)
		}
		ref, err := workerRefArgOrCwd(args)
		if err != nil {
			fail(err)
		}
		res, err := svc.Done(context.Background(), feature.DoneOpts{WorkerRef: ref, Force: doneForce})
		if err != nil {
			fail(err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed worker %s\n", res.Removed)
		if res.FeatureDeleted {
			fmt.Fprintln(cmd.OutOrStdout(), "  feature was empty and template-clean → deleted")
		}
		if res.RetainedNotice != "" {
			fmt.Fprintln(cmd.ErrOrStderr(), res.RetainedNotice)
		}
	},
}

func init() {
	featureDoneCmd.Flags().BoolVar(&doneForce, "force", false, "Remove despite a dirty worktree, dependents, or an open PR")
	featureCmd.AddCommand(featureDoneCmd)
}

// workerRefArgOrCwd returns the positional worker-ref if given, else resolves
// it from cwd. Shared by done and merged (both accept an optional ref).
func workerRefArgOrCwd(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	return currentWorkerRef()
}
