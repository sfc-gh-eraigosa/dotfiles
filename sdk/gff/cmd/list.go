package cmd

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"text/tabwriter"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/spf13/cobra"
)

var listJSON bool

var listCmd = &cobra.Command{
	Use:   "list [pattern]",
	Short: "List feature flags with their effective values and winning layer",
	Long: `List every resolved flag as an aligned table (PATH TYPE VALUE LAYER
DESCRIPTION) or, with --json, as an indented []ResolvedJSON array.

An optional pattern narrows the output by key: glob characters (*?[) match
the full dotted key via path.Match ("install.ai.*", "*.claude"); a bare
string matches as a segment prefix ("install.ai").`,
	Args: cobra.MaximumNArgs(1),
	RunE: runList,
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "emit an indented JSON array of ResolvedJSON")
	rootCmd.AddCommand(listCmd)
}

// matchKey reports whether key matches pattern: glob when pattern carries
// glob metacharacters, else exact key or dotted-segment prefix.
func matchKey(pattern, key string) bool {
	if pattern == "" {
		return true
	}
	if strings.ContainsAny(pattern, "*?[") {
		ok, err := path.Match(pattern, key)
		return err == nil && ok
	}
	return key == pattern || strings.HasPrefix(key, pattern+".")
}

func runList(cmd *cobra.Command, args []string) error {
	pattern := ""
	if len(args) == 1 {
		pattern = args[0]
	}

	r, err := newResolver()
	if err != nil {
		return err
	}

	all, err := r.All()
	if err != nil {
		return err
	}

	filtered := all[:0:0]
	for _, res := range all {
		if matchKey(pattern, res.Feature.GetPath()) {
			filtered = append(filtered, res)
		}
	}

	if listJSON {
		rows := make([]resolve.ResolvedJSON, 0, len(filtered))
		for _, res := range filtered {
			rj, err := res.JSON()
			if err != nil {
				return fmt.Errorf("list: %w", err)
			}
			rows = append(rows, rj)
		}
		b, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return fmt.Errorf("list: marshal array: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PATH\tTYPE\tVALUE\tLAYER\tDESCRIPTION")
	for _, res := range filtered {
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
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			res.Feature.GetPath(), typ, value, res.Layer.String(), res.Feature.GetDescription())
	}
	return w.Flush()
}
