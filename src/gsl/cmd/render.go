package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/wenlock/dotfiles/gsl/internal/config"
	"github.com/wenlock/dotfiles/gsl/internal/gh"
	"github.com/wenlock/dotfiles/gsl/internal/git"
	"github.com/wenlock/dotfiles/gsl/internal/mcp"
	"github.com/wenlock/dotfiles/gsl/internal/payload"
	"github.com/wenlock/dotfiles/gsl/internal/render"
	"github.com/wenlock/dotfiles/gsl/internal/repo"
	"github.com/wenlock/dotfiles/gsl/internal/style"
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
		// Bad JSON on stdin: log to stderr but continue with empty payload.
		fmt.Fprintf(os.Stderr, "gsl render: stdin parse error (degrading): %v\n", err)
		p = payload.Payload{}
	}

	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return fmt.Errorf("gsl render: config load: %w", err)
	}

	if !cfg.Enabled {
		// Master off: print nothing.
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	gitRunner := git.NewSystemRunner()
	ghRunner := gh.NewSystemRunner()
	mcpRunner := mcp.NewSystemRunner()

	// Determine cwd: prefer payload.Cwd, fall back to os.Getwd.
	cwd := ""
	if p.Cwd != nil && *p.Cwd != "" {
		cwd = *p.Cwd
	} else {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}

	// Best-effort branch from git.Status.
	branch := ""
	if cwd != "" {
		if info, err := git.Status(ctx, gitRunner, cwd); err == nil {
			branch = info.Branch
		}
	}

	// Convert raw config.Styles to map[string]map[string]any for ResolveConfig.
	rawStyles := configToRawStyles(cfg.Styles)
	st := style.ResolveConfig(os.Stderr, cfg.Style, rawStyles, false)

	deps := render.Deps{
		Payload:      p,
		Cwd:          cwd,
		Branch:       branch,
		RegistryPath: repo.DefaultRegistryPath(),
		Git:          gitRunner,
		GH:           ghRunner,
		MCP:          mcpRunner,
		MCPOpts:      mcp.ActiveCountOptions{},
		Clock:        time.Now,
	}

	segs := render.BuildSegments(cfg, deps)
	line := render.Render(ctx, cfg, st, segs)
	if line != "" {
		fmt.Println(line)
	}
	return nil
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
