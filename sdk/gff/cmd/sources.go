package cmd

// `gff sources` — owner-approved §3.4 extension (PR #187 review): the CLI
// twin of the TUI help overlay's SOURCES story.

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	lgtable "github.com/charmbracelet/lipgloss/table"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/gitx"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/schema"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/style"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var sourcesJSON bool

var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "List registered sources and discovered origins",
	Long: `List every source flags can resolve from: the registry
(~/.config/gff/sources.yaml, populated by 'gff install') plus the current
repo's discovered flag file when it is not registered.

STATUS explains each row: 'registered' entries serve snapshot layers from any
CWD; a 'discovered' entry is the CWD repo's live flag file only. The current
repo's origin is marked and sorts first.

Any namespace listed here is valid for --source <namespace> on the read verbs.`,
	RunE: runSources,
}

func init() {
	sourcesCmd.Flags().BoolVar(&sourcesJSON, "json", false, "emit the source list as JSON")
	rootCmd.AddCommand(sourcesCmd)
}

// sourceRow is the FROZEN wire shape for `sources --json`.
type sourceRow struct {
	Namespace   string `json:"namespace"`
	URL         string `json:"url"`
	Commit      string `json:"commit"`
	Registered  bool   `json:"registered"`
	CurrentRepo bool   `json:"currentRepo"`
}

func runSources(cmd *cobra.Command, _ []string) error {
	defer func() { sourcesJSON = false }()

	r, err := newResolver()
	if err != nil {
		return err
	}

	// The current repo's declared namespace, when the CWD is inside one.
	curNS := ""
	if root, ok := gitx.RepoRoot(r.P.WorkDir); ok {
		if ff, ferr := schema.LoadFeatureFile(gitx.SourcePath(r.R, root)); ferr == nil {
			curNS = ff.GetNamespace()
		}
	}

	reg := &registry.Registry{P: r.P}
	regSources, err := reg.Sources()
	if err != nil {
		return fmt.Errorf("gff: sources: %w", err)
	}

	var rows []sourceRow
	seen := map[string]bool{}
	for _, s := range regSources {
		rows = append(rows, sourceRow{
			Namespace:   s.GetNamespace(),
			URL:         s.GetUrl(),
			Commit:      s.GetCommit(),
			Registered:  true,
			CurrentRepo: s.GetNamespace() == curNS,
		})
		seen[s.GetNamespace()] = true
	}
	if curNS != "" && !seen[curNS] {
		rows = append(rows, sourceRow{Namespace: curNS, CurrentRepo: true})
	}
	// Current repo first, then registry order.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CurrentRepo && !rows[j].CurrentRepo })

	out := cmd.OutOrStdout()

	if sourcesJSON {
		if rows == nil {
			fmt.Fprintln(out, "[]")
			return nil
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}

	if len(rows) == 0 {
		fmt.Fprintln(out, "no sources — run `gff install` in a repo with a flag file, or cd into one")
		return nil
	}

	// Styled table on a real terminal (same auto behavior as `gff list`).
	if out == os.Stdout && os.Getenv("NO_COLOR") == "" && term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Fprintln(out, renderSourcesTable(rows, terminalWidth()))
		return nil
	}

	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tURL\tCOMMIT\tSTATUS")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			row.Namespace, orDash(row.URL), orDash(row.Commit), status(row))
	}
	return w.Flush()
}

// renderSourcesTable renders the styled sources table; width > 0 constrains
// it to the terminal (same in-cell wrapping contract as renderPrettyTable).
func renderSourcesTable(rows []sourceRow, width int) string {
	pal := style.Active()
	cell := lipgloss.NewStyle().Padding(0, 1)
	tbl := lgtable.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(pal.Border)).
		Headers("NAMESPACE", "URL", "COMMIT", "STATUS").
		StyleFunc(func(rowIdx, _ int) lipgloss.Style {
			if rowIdx == lgtable.HeaderRow {
				return cell.Bold(true).Foreground(pal.Purple)
			}
			if rowIdx >= 0 && rowIdx < len(rows) && rows[rowIdx].CurrentRepo {
				return cell.Foreground(pal.Text)
			}
			return cell
		})
	for _, row := range rows {
		tbl = tbl.Row(row.Namespace, row.URL, row.Commit, status(row))
	}
	if width > 0 {
		tbl = tbl.Width(width)
	}
	return tbl.Render()
}

// status renders a sourceRow's STATUS cell.
func status(row sourceRow) string {
	s := "discovered — not registered"
	if row.Registered {
		s = "registered"
	}
	if row.CurrentRepo {
		s += " · current repo"
	}
	return s
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
