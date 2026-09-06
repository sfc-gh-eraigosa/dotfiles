package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
)

var (
	listFeature string
	listTree    bool
	listJSON    bool
)

var featureListCmd = &cobra.Command{
	Use:   "list",
	Short: "List features and their workers",
	Long: `List features and their workers from the registry. Default output is a flat
table; --tree renders the stack relationships (indented by depth); --json
emits a machine-readable report.`,
	Run: func(cmd *cobra.Command, args []string) {
		store, err := newRegistryStore()
		if err != nil {
			fail(err)
		}
		svc := &feature.Service{Store: store}
		out, err := svc.List(context.Background(), feature.ListOpts{
			Feature: listFeature,
			Tree:    listTree,
			JSON:    listJSON,
		})
		if err != nil {
			fail(err)
		}
		fmt.Fprint(cmd.OutOrStdout(), out)
	},
}

func init() {
	f := featureListCmd.Flags()
	f.StringVar(&listFeature, "feature", "", "Filter to one feature")
	f.BoolVar(&listTree, "tree", false, "Render stack relationships (indented by depth)")
	f.BoolVar(&listJSON, "json", false, "JSON output")
	featureCmd.AddCommand(featureListCmd)
}
