package cmd

import (
	"fmt"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/observe"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var renderCmd = &cobra.Command{
	Use:   "render",
	Short: "Render the status line from a Claude JSON payload on stdin",
	Long: `render reads a Claude Code JSON payload from stdin, loads the config,
builds the status-line segments, and prints one rendered line to stdout.

Empty or invalid stdin is handled gracefully — the status line is still
rendered (without AI segment data). If the master enable flag is false,
nothing is printed.`,
	RunE: runRender,
}

func init() {
	rootCmd.AddCommand(renderCmd)
}

func runRender(cmd *cobra.Command, args []string) error {
	// Parse payload from stdin; degrade gracefully on error.
	p, err := payload.ParseReader(os.Stdin)
	if err != nil {
		// Bad JSON on stdin: emit a structured record so the failure is
		// diagnosable (#30 was invisible for months because we only wrote
		// to stderr, which Claude Code discards for status-line commands).
		// Continue with empty payload.
		observe.Default().WithFields(logrus.Fields{
			"event": "payload.parse_error",
			"error": err.Error(),
		}).Warn("stdin parse failed; degrading to empty payload")
		fmt.Fprintf(os.Stderr, "gsl render: stdin parse error (degrading): %v\n", err)
		p = payload.Payload{}
	}

	// Determine cwd hint from payload.
	cwdHint := ""
	if p.Cwd != nil && *p.Cwd != "" {
		cwdHint = *p.Cwd
	}

	return runStatusLine(cmd, p, cwdHint)
}

// configToRawStyles converts config.Styles (map[string]any, raw JSON) to the
// map[string]map[string]any shape that style.ResolveConfig expects.
// Top-level values that are not map[string]any are silently skipped.
func configToRawStyles(raw map[string]any) map[string]map[string]any {
	if raw == nil {
		return nil
	}
	out := make(map[string]map[string]any, len(raw))
	for k, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out[k] = m
		}
	}
	return out
}
