package cmd

import (
	"fmt"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/pkg/tmux"
	"github.com/spf13/cobra"
)

var paneCmd = &cobra.Command{
	Use:   "pane",
	Short: "Manage the root/anchor tmux pane for AI-driven operations",
}

func init() {
	paneCmd.AddCommand(&cobra.Command{
		Use:   "anchor [title]",
		Short: "Mark the current tmux pane as the orchestration root",
		Long: `Saves the current $TMUX_PANE as TMUX_MGR_ROOT_PANE in the tmux global
environment and renames the pane. Must be run from inside a tmux pane.

After anchoring, AI processes (Claude, Antigravity — or a legacy Gemini pane) that call 'tmux-mgr window split'
or 'tmux-mgr agent start' will correctly target this pane.`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			title := ""
			if len(args) > 0 {
				title = args[0]
			}
			paneID, err := tmux.AnchorPane(title)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
			if title == "" {
				title = "root"
			}
			fmt.Printf("Anchored pane %s as %q\n", paneID, title)
		},
	})

	paneCmd.AddCommand(&cobra.Command{
		Use:   "adopt",
		Short: "Create a new tmux window and register it as the root anchor",
		Long: `Creates a new tmux window from outside tmux, saves its pane ID as
TMUX_MGR_ROOT_PANE, and prints the pane ID. Requires a running tmux server.

Use this when an AI process (Claude, Antigravity) needs a tmux anchor but was not
started from inside a tmux pane.`,
		Run: func(cmd *cobra.Command, args []string) {
			paneID, err := tmux.AdoptPane()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
			fmt.Printf("Adopted pane %s as root anchor\n", paneID)
		},
	})

	paneCmd.AddCommand(&cobra.Command{
		Use:   "split [horizontal|vertical|left|right|up|down]",
		Short: "Split the root anchor pane",
		Long: `Splits the root anchor pane (from $TMUX_PANE or TMUX_MGR_ROOT_PANE).
Errors with an actionable message if no anchor has been established.`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			dir := "vertical"
			if len(args) > 0 {
				dir = args[0]
			}
			var splitFlag string
			switch dir {
			case "horizontal", "right", "left":
				splitFlag = "-h"
			case "vertical", "up", "down":
				splitFlag = "-v"
			default:
				fmt.Fprintln(os.Stderr, "Invalid direction. Use horizontal|vertical|left|right|up|down")
				os.Exit(1)
			}
			target, err := tmux.RootPaneID()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
			_, splitErr := Tmgr.Run("split-window", splitFlag, "-t", target)
			if splitErr != nil {
				fmt.Fprintln(os.Stderr, "Error splitting pane:", splitErr)
				os.Exit(1)
			}
		},
	})

	rootCmd.AddCommand(paneCmd)
}
