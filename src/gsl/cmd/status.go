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
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return fmt.Errorf("gsl status: config load: %w", err)
	}

	if !cfg.Enabled {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	gitRunner := git.NewSystemRunner()
	ghRunner := gh.NewSystemRunner()
	mcpRunner := mcp.NewSystemRunner()

	cwd := ""
	if wd, err := os.Getwd(); err == nil {
		cwd = wd
	}

	branch := ""
	if cwd != "" {
		if info, err := git.Status(ctx, gitRunner, cwd); err == nil {
			branch = info.Branch
		}
	}

	rawStyles := configToRawStyles(cfg.Styles)
	st := style.ResolveConfig(os.Stderr, cfg.Style, rawStyles, false)

	deps := render.Deps{
		Payload:      payload.Payload{}, // no stdin payload
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
