package cmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/layout"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/probe"
	"github.com/spf13/cobra"
)

// doctorEnv is swapped in tests.
var doctorEnv probe.Env = probe.System{}

var doctorRows int

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Show which tool each pane gets on this machine, and why",
	Args:  cobra.NoArgs,
	RunE: func(c *cobra.Command, _ []string) error {
		path, cfg, err := load()
		if err != nil {
			return err
		}
		out := c.OutOrStdout()
		fmt.Fprintf(out, "config: %s\nprefix: %s\n\n", path, cfg.Prefix)
		choices := probe.Choose(context.Background(), doctorEnv, cfg.Panes)
		title, _ := config.ParseSize(cfg.Title.Size)
		pcts := []int{title}
		total := title
		for _, ch := range choices {
			n, _ := config.ParseSize(ch.Pane.Size)
			total += n
			if ch.Usable() {
				pcts = append(pcts, n)
			}
		}
		rows := layout.Rows(doctorRows, pcts)
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		fmt.Fprintf(tw, "PANE\tSIZE\tROWS@%d\tRUNS\tWHY\n", doctorRows)
		fmt.Fprintf(tw, "title\t%s\t%d\t-\tversion, commit, focused pane, help hint\n", cfg.Title.Size, rows[0])
		i := 1
		for _, ch := range choices {
			if ch.Usable() {
				fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", ch.Pane.Name, ch.Pane.Size, rows[i], ch.Cmd, ch.Reason)
				i++
			} else {
				fmt.Fprintf(tw, "%s\t%s\t-\t(not shown)\t%s\n", ch.Pane.Name, ch.Pane.Size, ch.Reason)
			}
		}
		_ = tw.Flush()
		if total != 100 {
			fmt.Fprintf(out, "\nnote: sizes add up to %d%%; the panes are scaled to fill the screen.\n", total)
		}
		return nil
	},
}

func init() {
	doctorCmd.Flags().IntVar(&doctorRows, "rows", 40, "terminal height to compute rows for")
	rootCmd.AddCommand(doctorCmd)
}
