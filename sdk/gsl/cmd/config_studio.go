package cmd

import (
	"context"
	"errors"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	xterm "github.com/charmbracelet/x/term"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/preview"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/render"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/tui"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/video"
	"github.com/spf13/cobra"
)

// The studio seams: tests replace them (no terminal, no bubbletea program).
var (
	studioTTY = func() bool { return xterm.IsTerminal(os.Stdin.Fd()) && xterm.IsTerminal(os.Stdout.Fd()) }
	runStudio = func(m tui.Model) error {
		_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
		return err
	}
)

// errNoTerminal is bare `gsl config` without a terminal.
var errNoTerminal = errors.New("gsl config: the studio needs a terminal — use gsl config get|set|enable|disable|toggle|style|validate")

// runConfigStudio is bare `gsl config` (plan F16/E23): the bubbletea studio.
// Subcommands keep their CLI behaviour.
func runConfigStudio(_ *cobra.Command, _ []string) error {
	if !studioTTY() {
		return errNoTerminal
	}
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return err
	}
	env, err := studioEnv(cfg)
	if err != nil {
		return err
	}
	return runStudio(tui.New(cfg, env))
}

// studioEnv detects every segment ONCE against the real repo (a sample payload
// stands in for Claude's, so the ai segment shows), then re-fits that
// snapshot for every edit — the preview is the line the status line draws.
func studioEnv(cfg config.Config) (tui.Env, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	deps, st, err := prepare(ctx, cfg, preview.CleanRepoPayload(), "")
	if err != nil {
		return tui.Env{}, err
	}
	// The GPU reading is a cache the status line attaches after prepare; the
	// studio reads it too (no refresher — it only shows), or the video row
	// would toggle a segment the preview can never draw.
	if dir, derr := video.Dir(); derr == nil {
		deps.Video = &video.Cache{Dir: dir}
	}
	snap := render.DetectAll(ctx, cfg, st, deps)
	cwd := deps.Cwd
	return tui.Env{
		Path: config.DefaultPath(),
		Save: config.Save,
		Line: func(c config.Config, cols int) string {
			s, err := styleFor(c, "")
			if err != nil {
				return ""
			}
			return snap.Line(c, s, cols)
		},
		MCP: func() mcp.Status {
			s, _ := mcp.Get(context.Background(), cwd, mcp.Options{})
			return s
		},
		Refresh: func() error {
			return mcp.Refresh(context.Background(), mcpRunner, cwd, mcp.Options{Force: true})
		},
		Styles: tui.StyleNames(cfg),
	}, nil
}
