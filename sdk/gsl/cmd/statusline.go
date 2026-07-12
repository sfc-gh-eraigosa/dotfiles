package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/observe"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/render"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/repo"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/theme"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// deriveToolCtx determines the host-tool context string from the payload and
// environment, for use with theme.Resolve.
//
// Rules (in priority order):
//  1. If the Claude payload is populated (Cwd, Model, ContextWindow, or
//     RateLimits is non-nil), the caller is "claude".
//  2. If any recognized Antigravity environment variable — or a legacy
//     Gemini-era variable (Antigravity CLI deliberately reuses the ~/.gemini
//     config tree, and Gemini CLI is EOL) — is set and non-empty, the caller
//     is "antigravity".
//  3. Otherwise "" (unknown / plain shell usage).
//
// env is the env-lookup function (os.Getenv in production; injected in tests).
//
// NOTE: The canonical Antigravity (agy) status-line environment variable is
// unconfirmed at the time of writing. We check ANTIGRAVITY_CLI plus the
// Gemini-era vars as a best-effort heuristic.
// TODO(gsl): confirm canonical Antigravity status-line env var and update this list.
func deriveToolCtx(p payload.Payload, env func(string) string) string {
	// Claude: the render subcommand populates the payload from stdin.
	if p.Cwd != nil || p.Model != nil || p.ContextWindow != nil || p.RateLimits != nil {
		return "claude"
	}
	// Antigravity: best-effort heuristic — check known Antigravity env vars
	// and the legacy Gemini-era vars it inherits.
	// TODO(gsl): confirm canonical Antigravity status-line env var.
	for _, key := range []string{"ANTIGRAVITY_CLI", "ANTIGRAVITY_CLI_CONTEXT", "GEMINI_CLI", "GEMINI_API_KEY", "GEMINI_CLI_CONTEXT"} {
		if env(key) != "" {
			return "antigravity"
		}
	}
	return ""
}

// runStatusLine is the shared wiring for both the render and status commands.
// It loads config, resolves the style, builds deps, runs Detect+Fit, and
// prints the result. It differs between the two callers only in:
//   - p: the payload (populated from stdin by render; empty by status)
//   - cwdHint: a preferred cwd string (render passes payload.Cwd; status passes "")
func runStatusLine(_ *cobra.Command, p payload.Payload, cwdHint string) error {
	// A corrupt or unreadable config must never break the status line. Warn to
	// stderr and fall back to defaults so the line still renders. (Finding #1)
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "gsl: config load failed (using defaults): %v\n", err)
		cfg = config.Default()
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

	// Derive the host-tool context and resolve the auto-theme palette.
	home, _ := os.UserHomeDir()
	toolCtx := deriveToolCtx(p, os.Getenv)
	autoPalette := theme.Resolve(toolCtx, os.Getenv, home)

	rawStyles := configToRawStyles(cfg.Styles)
	st := style.ResolveConfig(os.Stderr, cfg.Style, rawStyles, false, autoPalette)

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

	// Detect-once: ALL subprocess I/O happens here, exactly once.
	datas := render.Detect(ctx, cfg, st, segs)

	// Resolve the terminal width through the single, testable resolver.
	// Precedence: payload.terminal_width → ioctl(stdout) → ioctl(stderr) →
	// $COLUMNS → cfg.FallbackColumns.
	cols, source := resolveColumns(p, os.Getenv, term.StdoutWidthSource(), term.StderrWidthSource())
	if source == sourceDefault {
		cols = cfg.EffectiveFallbackColumns()
	}
	observe.Default().WithFields(logrus.Fields{
		"event":  "width.resolved",
		"cols":   cols,
		"source": source,
	}).Debug("resolved terminal width")

	// Fit: escalate compaction levels until the output fits, or use the most
	// compact form. No additional I/O after Detect.
	line := render.Fit(datas, st, cols)
	if line != "" {
		fmt.Println(line)
	}
	return nil
}
