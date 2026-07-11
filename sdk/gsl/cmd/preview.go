package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/preview"
)

var previewOnce bool

var previewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Preview the status line interactively (or --once for a single frame)",
	Long: `preview launches an interactive bubbletea TUI that lets you:
  - toggle individual segments on/off (keys 1-4)
  - cycle through built-in styles (key s)
  - cycle through fixture payloads (key f)
  - watch the time segment tick live

With --once, a single rendered frame is printed and the command exits immediately.
This is suitable for CI / golden-file checks.`,
	RunE: runPreview,
}

func init() {
	previewCmd.Flags().BoolVar(&previewOnce, "once", false, "Print one rendered frame and exit")
	rootCmd.AddCommand(previewCmd)
}

func runPreview(cmd *cobra.Command, args []string) error {
	if previewOnce {
		line := preview.RenderOnce()
		if line != "" {
			fmt.Println(line)
		} else {
			fmt.Println("(empty — all segments disabled or no data)")
		}
		return nil
	}

	m := preview.NewModel(nil) // nil clock → real time.Now
	p := tea.NewProgram(m, tea.WithOutput(os.Stdout))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("gsl preview: %w", err)
	}
	return nil
}
