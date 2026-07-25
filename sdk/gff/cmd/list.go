package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/spf13/cobra"
)

var listJSON bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all feature flags with their effective values and layer",
	Args:  cobra.NoArgs,
	RunE:  runList,
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "emit JSON array of ResolvedJSON")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, _ []string) error {
	r, err := newResolver()
	if err != nil {
		return err
	}

	all, err := r.All()
	if err != nil {
		return err
	}

	if listJSON {
		out := make([]json.RawMessage, 0, len(all))
		for _, res := range all {
			rj, err := res.JSON()
			if err != nil {
				return fmt.Errorf("list: %w", err)
			}
			b, err := json.Marshal(rj)
			if err != nil {
				return fmt.Errorf("list: marshal: %w", err)
			}
			out = append(out, json.RawMessage(b))
		}
		b, err := json.Marshal(out)
		if err != nil {
			return fmt.Errorf("list: marshal array: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return nil
	}

	// Human-readable table: PATH  TYPE  VALUE  LAYER  DESCRIPTION
	for _, res := range all {
		var typ, value string
		switch v := res.Value.GetKind().(type) {
		case *gffv1.Value_BoolValue:
			typ = "bool"
			value = fmt.Sprintf("%v", v.BoolValue)
		case *gffv1.Value_ChoiceValue:
			typ = "choice"
			value = strings.Join(v.ChoiceValue.GetSelected(), ",")
		default:
			typ = "unknown"
			value = ""
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-40s  %-6s  %-10s  %-20s  %s\n",
			res.Feature.GetPath(),
			typ,
			value,
			res.Layer.String(),
			res.Feature.GetDescription(),
		)
	}
	return nil
}
