package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/configui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Choose which tool runs in which pane, and how tall each pane is",
	Long: `With no subcommand, opens the config screen (needs a terminal).

Keys for get/set:
  prefix                  the mono key, ctrl+<letter> (not c, i, j or m)
  title.size              title bar height, percent of the terminal
  panes.<name>.cmd        command line, run with /bin/sh
  panes.<name>.size       height, percent of the terminal
  panes.<name>.every      re-run interval (e.g. 1s); empty = interactive program
  panes.<name>.fallbacks  comma-separated commands tried when cmd cannot run
  panes.<name>.probe      "" built-in check, "none", or a command (exit 0 = usable)
  panes.<name>.cwd        working directory ("" = where mono started)
  panes.<name>.zoomable   true (part of the middle; z zooms it) or false (fixed, like the gsl footer)
  middle                  split (the middle panes share rows) or single (one deck of all their tools)
  nav_key / nav_esc_ms    nav mode's key after the prefix (space), and the esc esc window (300; 0 = off)
  panes.<p>.tools.<t>.{probe,zoom}                       a deck tool's check, and whether it fills the middle
  panes.<p>.tools.<t>.views.<v>.{cmd,every,fallbacks,probe,cwd}   a deck view (add-tool/add-view to add)

Settings live in ${XDG_CONFIG_HOME:-~/.config}/mono/config.json ($MONO_CONFIG overrides).`,
	Args: cobra.NoArgs,
	RunE: func(c *cobra.Command, _ []string) error {
		path, cfg, err := load()
		if err != nil {
			return err
		}
		if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
			return fmt.Errorf("the config screen needs a terminal; use mono config get|set|add|remove|move")
		}
		_, err = tea.NewProgram(configui.New(path, cfg, nil), tea.WithAltScreen()).Run()
		return err
	},
}

func load() (string, config.Config, error) {
	path, err := config.Path()
	if err != nil {
		return "", config.Config{}, err
	}
	cfg, err := config.Load(path)
	return path, cfg, err
}

// edit loads, applies fn, and saves; nothing is written if fn fails.
func edit(c *cobra.Command, fn func(config.Config) (config.Config, error)) error {
	path, cfg, err := load()
	if err != nil {
		return err
	}
	next, err := fn(cfg)
	if err != nil {
		return err
	}
	if err := config.Save(path, next); err != nil {
		return err
	}
	fmt.Fprintln(c.OutOrStdout(), "saved", path)
	return nil
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Print the whole config as JSON, or one value",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		_, cfg, err := load()
		if err != nil {
			return err
		}
		if len(args) == 0 {
			b, _ := json.MarshalIndent(cfg, "", "  ")
			fmt.Fprintln(c.OutOrStdout(), string(b))
			return nil
		}
		v, err := cfg.Get(args[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(c.OutOrStdout(), v)
		return nil
	},
	ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		_, cfg, _ := load()
		return cfg.Keys(), cobra.ShellCompDirectiveNoFileComp
	},
}

var configSetCmd = &cobra.Command{
	Use:     "set <key> <value...>",
	Short:   "Set one value (rejected values name the field; nothing is written)",
	Example: "  mono config set panes.gpu.cmd nvitop\n  mono config set panes.htop.size 50%\n  mono config set prefix ctrl+g",
	Args:    cobra.MinimumNArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		return edit(c, func(cfg config.Config) (config.Config, error) {
			return cfg.Set(args[0], strings.Join(args[1:], " "))
		})
	},
}

var (
	addSize, addEvery string
	addAt             int
)

var configAddCmd = &cobra.Command{
	Use:     "add <name> <cmd...>",
	Short:   "Add a pane",
	Example: "  mono config add logs --size 15% -- journalctl -f\n  mono config add clock --size 5% --every 1s -- date",
	Args:    cobra.MinimumNArgs(2),
	RunE: func(c *cobra.Command, args []string) error {
		p := config.Pane{Name: args[0], Size: addSize, Cmd: strings.Join(args[1:], " "), Every: addEvery}
		return edit(c, func(cfg config.Config) (config.Config, error) { return cfg.Add(p, addAt) })
	},
}

var configRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a pane",
	Args:  cobra.ExactArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		return edit(c, func(cfg config.Config) (config.Config, error) { return cfg.Remove(args[0]) })
	},
}

var configMoveCmd = &cobra.Command{
	Use:   "move <name> up|down",
	Short: "Move a pane up or down the stack",
	Args:  cobra.ExactArgs(2),
	RunE: func(c *cobra.Command, args []string) error {
		d := map[string]int{"up": -1, "down": 1}[args[1]]
		if d == 0 {
			return fmt.Errorf("direction must be up or down, got %q", args[1])
		}
		return edit(c, func(cfg config.Config) (config.Config, error) { return cfg.Move(args[0], d) })
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the config file path",
	Args:  cobra.NoArgs,
	RunE: func(c *cobra.Command, _ []string) error {
		p, err := config.Path()
		if err != nil {
			return err
		}
		fmt.Fprintln(c.OutOrStdout(), p)
		return nil
	},
}

func init() {
	configAddCmd.Flags().StringVar(&addSize, "size", "10%", "height, percent of the terminal")
	configAddCmd.Flags().StringVar(&addEvery, "every", "", "re-run interval (e.g. 1s); empty = interactive program")
	configAddCmd.Flags().IntVar(&addAt, "at", -1, "position (0 = top; -1 = bottom)")
	configCmd.AddCommand(configGetCmd, configSetCmd, configAddCmd, configRemoveCmd, configMoveCmd, configPathCmd)
	rootCmd.AddCommand(configCmd)
}
