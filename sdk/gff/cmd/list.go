package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	lgtable "github.com/charmbracelet/lipgloss/table"
	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	listJSON   bool
	listPretty bool
	listRaw    bool
)

var listCmd = &cobra.Command{
	Use:   "list [pattern]",
	Short: "List feature flags with their effective values and winning layer",
	Long: `List every resolved flag as a table (PATH TYPE VALUE LAYER DESCRIPTION)
or, with --json, as an indented []ResolvedJSON array.

On a terminal the table renders with borders and color-coded layers
(lipgloss); piped output stays plain and greppable. --pretty forces the
styled table, NO_COLOR suppresses the automatic styling.

An optional pattern narrows the output by key: glob characters (*?[) match
the full dotted key via path.Match ("install.ai.*", "*.claude"); a bare
string matches as a segment prefix ("install.ai").`,
	Args: cobra.MaximumNArgs(1),
	RunE: runList,
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "emit an indented JSON array of ResolvedJSON")
	listCmd.Flags().BoolVar(&listPretty, "pretty", false, "force the styled (bordered, colored) table")
	listCmd.Flags().BoolVar(&listRaw, "raw", false, "with --json: compact single-line output (default is indented)")
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

// layerStyle color-codes the winning layer: overrides pop, definitions stay calm.
func layerStyle(layer string) lipgloss.Style {
	switch layer {
	case "user-override":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // orange
	case "system-override":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("203")) // red
	case "repo-live":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")) // green
	case "user-snapshot":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("39")) // blue
	default: // system-snapshot, none
		return lipgloss.NewStyle().Foreground(lipgloss.Color("245")) // grey
	}
}

func valueStyle(value string) lipgloss.Style {
	switch value {
	case "true":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	case "false":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	}
	return lipgloss.NewStyle()
}

func renderPrettyTable(rows [][]string) string {
	cell := lipgloss.NewStyle().Padding(0, 1)
	t := lgtable.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240"))).
		Headers("PATH", "TYPE", "VALUE", "LAYER", "DESCRIPTION").
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == lgtable.HeaderRow {
				return cell.Bold(true).Foreground(lipgloss.Color("63"))
			}
			if row < 0 || row >= len(rows) {
				return cell
			}
			switch col {
			case 2:
				return cell.Inherit(valueStyle(rows[row][2]))
			case 3:
				return cell.Inherit(layerStyle(rows[row][3]))
			}
			return cell
		})
	return t.Render()
}

func runList(cmd *cobra.Command, args []string) error {
	jsonOut, pretty, raw := listJSON, listPretty, listRaw
	// Package-level flag vars persist across Execute calls (tests); reset so
	// each invocation re-derives them from its own args.
	defer func() { listJSON, listPretty, listRaw = false, false, false }()

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

	if jsonOut {
		rows := make([]resolve.ResolvedJSON, 0, len(filtered))
		for _, res := range filtered {
			rj, err := res.JSON()
			if err != nil {
				return fmt.Errorf("list: %w", err)
			}
			rows = append(rows, rj)
		}
		var b []byte
		if raw {
			b, err = json.Marshal(rows)
		} else {
			b, err = json.MarshalIndent(rows, "", "  ")
		}
		if err != nil {
			return fmt.Errorf("list: marshal array: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return nil
	}

	rows := make([][]string, 0, len(filtered))
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
		rows = append(rows, []string{
			res.Feature.GetPath(), typ, value, res.Layer.String(), res.Feature.GetDescription(),
		})
	}

	// Styled table on a real terminal (unless NO_COLOR), or when forced.
	out := cmd.OutOrStdout()
	if !pretty && out == os.Stdout && os.Getenv("NO_COLOR") == "" {
		pretty = term.IsTerminal(int(os.Stdout.Fd()))
	}
	if pretty {
		fmt.Fprintln(out, renderPrettyTable(rows))
		return nil
	}

	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PATH\tTYPE\tVALUE\tLAYER\tDESCRIPTION")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", row[0], row[1], row[2], row[3], row[4])
	}
	return w.Flush()
}
