package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
)

var (
	startDesc string
	startBase string
	startGoal string
)

var featureStartCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Start a new feature (creates the registry row + FEATURE.md)",
	Long: `Create a new feature: validate the name, capture the base commit, write the
feature row to the registry, and seed FEATURE.md. Add workers afterward with
'gss feature worker add --feature <name> --purpose <p> --description "…"'.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		svc, err := newFeatureService()
		if err != nil {
			fail(err)
		}
		dir, err := svc.Start(context.Background(), feature.StartOpts{
			Name:        args[0],
			Description: startDesc,
			BaseBranch:  startBase,
			Goal:        startGoal,
		})
		if err != nil {
			fail(err)
		}
		base := startBase
		if base == "" {
			base = "main"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Started feature %q (base %s)\n  %s\n", args[0], base, dir)
	},
}

func init() {
	f := featureStartCmd.Flags()
	f.StringVar(&startDesc, "description", "", "One-line feature description (seeds FEATURE.md)")
	f.StringVar(&startBase, "base", "", "Base branch (default: main)")
	f.StringVar(&startGoal, "goal", "", "High-level goal")
	featureCmd.AddCommand(featureStartCmd)
}
