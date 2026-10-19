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
		fmt.Fprintf(out, "config: %s\nprefix: %s\nmiddle: %s\n\n", path, cfg.Prefix, cfg.MiddleMode())
		choices := probe.Choose(context.Background(), doctorEnv, cfg.Effective())
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
			runs := ch.Cmd
			if ch.Pane.IsDeck() && ch.Usable() {
				runs = "deck"
			}
			if ch.Usable() {
				fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", ch.Pane.Name, ch.Pane.Size, rows[i], runs, ch.Reason)
				i++
			} else {
				fmt.Fprintf(tw, "%s\t%s\t-\t(not shown)\t%s\n", ch.Pane.Name, ch.Pane.Size, ch.Reason)
			}
			if !ch.Pane.IsDeck() {
				continue
			}
			// One row per view: what runs and why each was kept or dropped.
			for _, v := range ch.Views {
				fmt.Fprintf(tw, "  └ %s/%s\t\t\t%s\t%s%s\n", v.Tool, v.View, v.Cmd, v.Reason, zoomNote(v.Zoom))
			}
			for _, v := range ch.Dropped {
				fmt.Fprintf(tw, "  └ %s/%s\t\t\t(not shown)\t%s\n", v.Tool, v.View, v.Reason)
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

func zoomNote(z bool) string {
	if z {
		return " · zoom: fills the middle"
	}
	return ""
}
