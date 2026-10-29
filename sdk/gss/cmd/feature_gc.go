package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
)

var (
	gcFeature string
	gcApply   bool
	gcJSON    bool
)

// Seams for tests: how the service is built and how a failed run exits.
var (
	gcServiceFactory = newFeatureService
	gcExit           = os.Exit
)

var featureGCCmd = &cobra.Command{
	Use:   "gc",
	Short: "Remove workers whose PR is merged or closed (dry run by default)",
	Long: `List every worker of the resolved repo (--repo, else cwd) whose PR is merged
or closed on GitHub, whose worktree is clean, whose branch has no unpushed
commits, and that no other worker bases on. The PR state is refreshed from
GitHub first, so a PR that merged on its own is found even though the registry
still caches "open".

Dry run by default: one line per candidate (worker, PR, size on disk, action)
and one per skipped worker with the reason. --apply removes the candidates
through the same path as 'gss feature done' (never --force semantics), and
deletes the local branch of each merged PR. Workers of other repos in the
shared registry are never examined or touched.

--json prints {candidates, removed, skipped, failed, freed_bytes}. Exits
non-zero only if a removal failed.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		svc, err := gcServiceFactory()
		if err != nil {
			fail(err)
		}
		res, err := svc.GC(context.Background(), feature.GCOpts{Feature: gcFeature, Apply: gcApply})
		if err != nil {
			fail(err)
		}
		if gcJSON {
			data, err := res.JSON()
			if err != nil {
				fail(err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
		} else {
			fmt.Fprint(cmd.OutOrStdout(), res.Text(gcApply))
		}
		if len(res.Failed) > 0 {
			gcExit(1)
		}
	},
}

func init() {
	f := featureGCCmd.Flags()
	f.StringVar(&gcFeature, "feature", "", "Restrict to one feature (default: all of this repo's)")
	f.BoolVar(&gcApply, "apply", false, "Remove the candidates (default: dry run)")
	f.BoolVar(&gcJSON, "json", false, "JSON output")
	featureCmd.AddCommand(featureGCCmd)
}
