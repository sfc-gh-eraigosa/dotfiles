package cmd

import (
	"github.com/spf13/cobra"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Render the status line for Gemini/CLI (no stdin payload)",
	Long: `status renders the status line without reading stdin. The AI segment
self-omits because no Claude payload is supplied. The dirgit, repo, and
time segments still render.`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	// No stdin payload; cwd is determined inside runStatusLine via os.Getwd.
	return runStatusLine(cmd, payload.Payload{}, "")
}
