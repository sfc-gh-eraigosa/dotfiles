package cmd

import (
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
	"github.com/spf13/cobra"
)

// Deck verbs (mono-gdash spec F8): add and remove tools and views of a deck
// pane. Fields are set with `mono config set panes.<p>.tools.<t>…`.

var (
	toolView, toolProbe string
	toolZoom            bool
	deckAt              int
	viewEvery           string
)

var configAddToolCmd = &cobra.Command{
	Use:     "add-tool <pane> <tool> [--view <name>] [--zoom] [--probe <cmd>] -- <cmd...>",
	Short:   "Add a tool (with its first view) to a deck pane",
	Example: "  mono config add-tool mid logs --view journal -- journalctl -f\n  mono config add-tool mid gdash --zoom --probe 'gdash doctor --quiet' --view conductor -- gdash conductor",
	Args:    cobra.MinimumNArgs(3),
	RunE: func(c *cobra.Command, args []string) error {
		view := toolView
		if view == "" {
			view = args[1]
		}
		t := config.Tool{Name: args[1], Probe: toolProbe, Zoom: toolZoom,
			Views: []config.View{{Name: view, Cmd: strings.Join(args[2:], " "), Every: viewEvery}}}
		return edit(c, func(cfg config.Config) (config.Config, error) { return cfg.AddTool(args[0], t, deckAt) })
	},
}

var configAddViewCmd = &cobra.Command{
	Use:     "add-view <pane> <tool> <view> -- <cmd...>",
	Short:   "Add a view to a tool of a deck pane",
	Example: "  mono config add-view mid gdash nodes -- gdash nodes",
	Args:    cobra.MinimumNArgs(4),
	RunE: func(c *cobra.Command, args []string) error {
		v := config.View{Name: args[2], Cmd: strings.Join(args[3:], " "), Every: viewEvery}
		return edit(c, func(cfg config.Config) (config.Config, error) { return cfg.AddView(args[0], args[1], v, deckAt) })
	},
}

var configRemoveToolCmd = &cobra.Command{
	Use:   "remove-tool <pane> <tool>",
	Short: "Remove a tool from a deck pane",
	Args:  cobra.ExactArgs(2),
	RunE: func(c *cobra.Command, args []string) error {
		return edit(c, func(cfg config.Config) (config.Config, error) { return cfg.RemoveTool(args[0], args[1]) })
	},
}

var configRemoveViewCmd = &cobra.Command{
	Use:   "remove-view <pane> <tool> <view>",
	Short: "Remove a view from a tool of a deck pane",
	Args:  cobra.ExactArgs(3),
	RunE: func(c *cobra.Command, args []string) error {
		return edit(c, func(cfg config.Config) (config.Config, error) { return cfg.RemoveView(args[0], args[1], args[2]) })
	},
}

func init() {
	for _, c := range []*cobra.Command{configAddToolCmd, configAddViewCmd} {
		c.Flags().IntVar(&deckAt, "at", -1, "position (0 = first; default last)")
		c.Flags().StringVar(&viewEvery, "every", "", "re-run interval for the view (e.g. 1s); empty = interactive")
	}
	configAddToolCmd.Flags().StringVar(&toolView, "view", "", "the first view's name (default: the tool's)")
	configAddToolCmd.Flags().StringVar(&toolProbe, "probe", "", "command that must exit 0 for the tool to show")
	configAddToolCmd.Flags().BoolVar(&toolZoom, "zoom", false, "fill the middle whenever this tool is on screen")
	configCmd.AddCommand(configAddToolCmd, configAddViewCmd, configRemoveToolCmd, configRemoveViewCmd)
}
