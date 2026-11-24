package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
)

var (
	featurePRReady  bool
	featurePRForce  bool
	featurePRMerge  bool
	featurePRWorker string
)

var featurePRCmd = &cobra.Command{
	Use:   "pr",
	Short: "Promote the worker's draft PR to ready-for-review (--ready), and queue it (--merge)",
	Long: `Promote a draft PR to ready-for-review with --ready. --merge also labels it
ready-for-merge so the Mergify queue lands it — the one gss step for "ship
it"; never add that label with gh pr edit. On a PR that is already ready,
--ready --merge only adds the label.

Gated on the approval token when the gff flag gss.approval-token.enabled
is on (off by default; GSS_REQUIRE_APPROVAL=on turns it on for one run; only the flag turns it off). The token is the
HUMAN's approval: the person runs
  git rev-parse HEAD > ~/.config/gss/approval.token
and an assistant never writes it. By default gss refuses to promote a
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
		if err := svc.PromoteReady(context.Background(), feature.ReadyOpts{WorkerRef: ref, Force: featurePRForce, Merge: featurePRMerge}); err != nil {
			fail(err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Promoted %s to ready-for-review\n", ref)
		if featurePRMerge {
			fmt.Fprintf(cmd.OutOrStdout(), "Labelled %s %s — the merge queue lands it once checks pass\n", ref, feature.MergeLabel)
		}
	},
}

func init() {
	f := featurePRCmd.Flags()
	f.BoolVar(&featurePRReady, "ready", false, "Promote the draft PR to ready-for-review")
	f.BoolVar(&featurePRMerge, "merge", false, "With --ready: also label the PR ready-for-merge (Mergify queue)")
	f.BoolVar(&featurePRForce, "force", false, "Override the parent-still-draft guard (not the token gate)")
	f.StringVar(&featurePRWorker, "worker", "", "Worker ref (default: resolved from cwd)")
	featureCmd.AddCommand(featurePRCmd)
}
