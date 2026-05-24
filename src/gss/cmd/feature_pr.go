package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wenlock/dotfiles/gss/internal/feature"
)

var (
	featurePRReady  bool
	featurePRForce  bool
	featurePRWorker string
)

var featurePRCmd = &cobra.Command{
	Use:   "pr",
	Short: "Promote the worker's draft PR to ready-for-review (--ready)",
	Long: `Promote a draft PR to ready-for-review with --ready. Always gated on the
approval token (refused without it). By default gss refuses to promote a
non-bottom PR while its parent is still draft; --force overrides that guard
(not the token gate). Resolves the worker from cwd unless --worker is given.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !featurePRReady {
			fmt.Fprintln(cmd.ErrOrStderr(), "gss feature pr: pass --ready to promote the draft PR to ready-for-review")
			return
		}
		svc, err := newFeatureService()
		if err != nil {
			fail(err)
		}
		ref := featurePRWorker
		if ref == "" {
			ref, err = currentWorkerRef()
			if err != nil {
				fail(err)
			}
		}
		if err := svc.PromoteReady(context.Background(), feature.ReadyOpts{WorkerRef: ref, Force: featurePRForce}); err != nil {
			fail(err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Promoted %s to ready-for-review\n", ref)
	},
}

func init() {
	f := featurePRCmd.Flags()
	f.BoolVar(&featurePRReady, "ready", false, "Promote the draft PR to ready-for-review")
	f.BoolVar(&featurePRForce, "force", false, "Override the parent-still-draft guard (not the token gate)")
	f.StringVar(&featurePRWorker, "worker", "", "Worker ref (default: resolved from cwd)")
	featureCmd.AddCommand(featurePRCmd)
}
