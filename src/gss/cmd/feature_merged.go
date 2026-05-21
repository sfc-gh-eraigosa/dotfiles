package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wenlock/dotfiles/gss/internal/feature"
)

var mergedNoAutoReady bool

var featureMergedCmd = &cobra.Command{
	Use:   "merged [<worker-ref>]",
	Short: "Re-target children after a worker's PR merged (+ gated auto-promote)",
	Long: `After a worker's PR lands, re-target every direct child onto the merged
worker's former base. When the merged worker was the stack bottom, the stack
is linear, and the single child has restack_count 0, auto-promote that child's
draft to ready (re-target before promote). --no-auto-ready disables the
promote leg. Resolves the worker from cwd when no <worker-ref> is given.`,
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
		res, err := svc.Merged(context.Background(), feature.MergedOpts{WorkerRef: ref, NoAutoReady: mergedNoAutoReady})
		if err != nil {
			fail(err)
		}
		if len(res.Retargeted) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Re-targeted: %s\n", strings.Join(res.Retargeted, ", "))
		}
		if res.Promoted != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Promoted %s to ready-for-review\n", res.Promoted)
		}
		if res.Notice != "" {
			fmt.Fprintln(cmd.ErrOrStderr(), res.Notice)
		}
	},
}

func init() {
	featureMergedCmd.Flags().BoolVar(&mergedNoAutoReady, "no-auto-ready", false, "Re-target only; never auto-promote a child to ready")
	featureCmd.AddCommand(featureMergedCmd)
}
