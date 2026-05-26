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

// runStatusLine is the shared wiring for both the render and status commands.
// It loads config, resolves the style, builds deps, runs BuildSegments+Render,
// and prints the result. It differs between the two callers only in:
//   - p: the payload (populated from stdin by render; empty by status)
//   - cwdHint: a preferred cwd string (render passes payload.Cwd; status passes "")
func runStatusLine(_ *cobra.Command, p payload.Payload, cwdHint string) error {
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return fmt.Errorf("config load: %w", err)
	}

	if !cfg.Enabled {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	gitRunner := git.NewSystemRunner()
	ghRunner := gh.NewSystemRunner()
	mcpRunner := mcp.NewSystemRunner()

	// Determine cwd: prefer the caller-supplied hint, fall back to os.Getwd.
	cwd := cwdHint
	if cwd == "" {
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
