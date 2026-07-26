package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	lgtable "github.com/charmbracelet/lipgloss/table"
	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/style"
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

// layerStyle color-codes the winning layer: overrides pop, definitions stay
// calm. Colors come from the theme-resolved palette (internal/style), so the
// table follows the shell's light/dark theme like the TUI does.
func layerStyle(layer string) lipgloss.Style {
	pal := style.Active()
	switch layer {
	case "user-override":
		return lipgloss.NewStyle().Foreground(pal.Orange)
	case "system-override":
		return lipgloss.NewStyle().Foreground(pal.Red)
	case "repo-live":
		return lipgloss.NewStyle().Foreground(pal.Green)
	case "user-snapshot":
		return lipgloss.NewStyle().Foreground(pal.Blue)
	default: // system-snapshot, none
		return lipgloss.NewStyle().Foreground(pal.Grey)
	}
}

func valueStyle(value string) lipgloss.Style {
	pal := style.Active()
	switch value {
	case "true":
		return lipgloss.NewStyle().Foreground(pal.Green)
	case "false":
		return lipgloss.NewStyle().Foreground(pal.Red)
	}
	return lipgloss.NewStyle()
}

// terminalWidth returns the usable column count: the real terminal size when
// stdout is a TTY, else $COLUMNS, else 0 (unconstrained).
func terminalWidth() int {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
			return w
		}
	}
	if c := os.Getenv("COLUMNS"); c != "" {
		if w, err := strconv.Atoi(c); err == nil && w > 0 {
			return w
		}
	}
	return 0
}

// renderPrettyTable renders the styled table. width > 0 constrains the whole
// table to that many columns (lipgloss wraps cell content within its column)
// so the terminal never hard-wraps mid-cell and mangles the borders.
func renderPrettyTable(rows [][]string, width int) string {
	cell := lipgloss.NewStyle().Padding(0, 1)
	t := lgtable.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(style.Active().Border)).
		Headers("PATH", "TYPE", "VALUE", "LAYER", "DESCRIPTION").
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == lgtable.HeaderRow {
				return cell.Bold(true).Foreground(style.Active().Purple)
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
	if width > 0 {
		t = t.Width(width)
	}
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
		fmt.Fprintln(out, renderPrettyTable(rows, terminalWidth()))
		return nil
	}

	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PATH\tTYPE\tVALUE\tLAYER\tDESCRIPTION")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", row[0], row[1], row[2], row[3], row[4])
	}
	return w.Flush()
}
