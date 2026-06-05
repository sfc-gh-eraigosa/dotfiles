package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

var (
	conflictsFeature string
	conflictsJSON    bool
)

var featureConflictsCmd = &cobra.Command{
	Use:   "conflicts",
	Short: "Report files touched by more than one worker",
	Long: `Cross-worker overlap report: for every active worker in the feature (or all
features), read its working diff and list the paths touched by more than one
worker. Read-only — never resolves conflicts.`,
	Run: func(cmd *cobra.Command, args []string) {
		store, err := newRegistryStore()
		if err != nil {
			fail(err)
		}
		// conflicts needs only the registry + a git runner (per-worktree diff);
		// no origin/NWO resolution.
		svc := &feature.Service{Store: store, Git: git.NewSystemRunner()}
		rep, err := svc.Conflicts(context.Background(), feature.ConflictsOpts{Feature: conflictsFeature, JSON: conflictsJSON})
		if err != nil {
			fail(err)
		}
		if conflictsJSON {
			data, err := rep.MarshalReport()
			if err != nil {
				fail(err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return
		}
		fmt.Fprint(cmd.OutOrStdout(), rep.Text())
	},
}

func init() {
	f := featureConflictsCmd.Flags()
	f.StringVar(&conflictsFeature, "feature", "", "Filter to one feature (default: all)")
	f.BoolVar(&conflictsJSON, "json", false, "JSON output")
	featureCmd.AddCommand(featureConflictsCmd)
}
