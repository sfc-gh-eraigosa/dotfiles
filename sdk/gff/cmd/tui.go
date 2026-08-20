package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/tui"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive TUI: browse flags, view provenance, toggle values",
	Long: `Launch an interactive bubbletea TUI that shows all resolved feature flags
in a collapsible area tree with layer provenance.

Navigation: ↑/↓ move  Enter expand/collapse  Space toggle (bool) or pick (choice)  q quit.

Writes go only to the user override file (~/.config/gff/config.yaml, mode 0600).
Quit without any change leaves the file untouched.`,
	RunE: runTUI,
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

func runTUI(_ *cobra.Command, _ []string) error {
	r, err := newResolver()
	if err != nil {
		return err
	}

	items, err := r.All()
	if err != nil {
		return err
	}

	m := tui.NewModel(items, r.P)
	m.Explain = r.Explain
	// &T{...} is never nil, so the old `reg != nil` guard was dead code.
	reg := &registry.Registry{P: r.P}
	if srcs, err := reg.Sources(); err == nil {
		for _, s := range srcs {
			m.Sources = append(m.Sources, tui.SourceInfo{
				Namespace: s.GetNamespace(), URL: s.GetUrl(), Commit: s.GetCommit(),
			})
		}
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
