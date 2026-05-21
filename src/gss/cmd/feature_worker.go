package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wenlock/dotfiles/gss/internal/feature"
)

// featureWorkerCmd is the nested `gss feature worker` group. `add` lands here
// in PR-45; `update` is deferred — it needs an internal Service.WorkerUpdate
// (design.md → worker update), which Batch G did not implement.
var featureWorkerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Add or update a worker (a worktree) within a feature",
}

var (
	workerFeature string
	workerPurpose string
	workerDesc    string
	workerBase    string
	workerUser    string
	workerForce   bool
	workerGoal    string
)

var featureWorkerAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a worker worktree to a feature",
	Long: `Add a worker: allocate a unique worker_ref, create the worktree on its own
branch (based on the feature's default base unless --base overrides), and seed
WORKER.md. --description is required.`,
	Run: func(cmd *cobra.Command, args []string) {
		svc, err := newFeatureService()
		if err != nil {
			fail(err)
		}
		res, err := svc.WorkerAdd(context.Background(), feature.WorkerAddOpts{
			Feature:     workerFeature,
			Purpose:     workerPurpose,
			Description: workerDesc,
			BaseBranch:  workerBase,
			User:        workerUser,
			ForceSuffix: workerForce,
			Goal:        workerGoal,
		})
		if err != nil {
			fail(err)
		}
		fmt.Fprintf(cmd.OutOrStdout(),
			"Added worker %s\n  branch:   %s\n  worktree: %s\n  base:     %s\n",
			res.Ref.String(), res.Branch, res.Worktree, res.Base)
	},
}

func init() {
	f := featureWorkerAddCmd.Flags()
	f.StringVar(&workerFeature, "feature", "", "Feature name (required)")
	f.StringVar(&workerPurpose, "purpose", "", "Worker purpose, e.g. api/ui (required)")
	f.StringVar(&workerDesc, "description", "", "Worker description (required)")
	f.StringVar(&workerBase, "base", "", "Base branch (default: the feature's default base)")
	f.StringVar(&workerUser, "user", "", "User (default: resolved from git email / $USER)")
	f.BoolVar(&workerForce, "force-suffix", false, "Force a random suffix even when the bare ref is free")
	f.StringVar(&workerGoal, "goal", "", "Worker goal (seeds WORKER.md)")
	_ = featureWorkerAddCmd.MarkFlagRequired("feature")
	_ = featureWorkerAddCmd.MarkFlagRequired("purpose")
	_ = featureWorkerAddCmd.MarkFlagRequired("description")

	featureWorkerCmd.AddCommand(featureWorkerAddCmd)
	featureCmd.AddCommand(featureWorkerCmd)
}
