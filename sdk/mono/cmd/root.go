// Package cmd is mono's CLI.
package cmd

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/overlay"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/app"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mono",
	Short: "One screen for the machine: htop, a GPU monitor and gsl, stacked",
	Long: `mono fills the terminal with stacked panes, each running a real program
(htop, nvtop, gsl status, ...) in its own pseudo-terminal. Panes whose tool is
missing or cannot work on this host are left out and the rest grow into the
space. Every key goes to the focused program except the prefix (ctrl+b by
default); see "mono config" to choose tools and sizes.`,
	SilenceUsage: true,
	Args:         cobra.NoArgs,
	RunE: func(c *cobra.Command, _ []string) error {
		path, err := config.Path()
		if err != nil {
			return err
		}
		cfg, err := config.Load(path)
		if err != nil {
			return err
		}
		dir, _ := os.Getwd()
		return app.Run(app.Options{
			Config:     cfg,
			ConfigPath: path,
			Title:      titleText(),
			Dir:        dir,
			ConfigArgv: configArgv,
		}, os.Stdin, os.Stdout)
	},
}

// configArgv is how :config re-runs this binary as `mono config`. It refuses
// inside a Go test binary: re-exec'ing a test binary runs its whole suite
// again (the 2026-10-17 gsl fork bomb).
func configArgv() []string {
	if testing.Testing() {
		return nil
	}
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	return []string{self, "config"}
}

func titleText() string {
	v := version.Get()
	return fmt.Sprintf("mono v%s — %s", v.Version, v.Commit)
}

// Execute runs the CLI.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.SetHelpTemplate(rootCmd.HelpTemplate() + "\n" + keysHelp() + "\n")
}

// keysHelp is the dashboard key table for --help, from the same keymap the
// overlay renders.
func keysHelp() string {
	cfg := config.Default()
	var panes []string
	for _, p := range cfg.Panes {
		panes = append(panes, fmt.Sprintf("%-8s %-5s %s", p.Name, p.Size, p.Cmd))
	}
	return strings.TrimSpace(app.HelpText(app.Keys(cfg.Prefix), cfg.Prefix, panes, overlay.Plain{}))
}
