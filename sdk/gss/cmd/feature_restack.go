package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
)

var restackOnto string

var featureRestackCmd = &cobra.Command{
	Use:   "restack <worker>",
	Short: "Re-target a worker's branch onto a new base (increments restack_count)",
	Long: `Manual stack edit: re-target the named worker's branch onto --onto, force-push
it, update its PR base, and walk the stack to fix dependents. Every call
increments restack_count on the worker and every descendant whose effective
base moved — permanently excluding them from auto-promote-on-merge.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		svc, err := newFeatureService()
		if err != nil {
			fail(err)
		}
		if err := svc.Restack(context.Background(), feature.RestackOpts{WorkerRef: args[0], Onto: restackOnto}); err != nil {
			fail(err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Restacked %s onto %s\n", args[0], restackOnto)
	},
}

func init() {
	featureRestackCmd.Flags().StringVar(&restackOnto, "onto", "", "New base branch (required)")
	_ = featureRestackCmd.MarkFlagRequired("onto")
	featureCmd.AddCommand(featureRestackCmd)
}
