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
//  1. In-band `product` key: agy sends "product": "antigravity" in EVERY
//     payload, so it is authoritative and is checked FIRST.
//  2. If the payload is otherwise populated (Cwd, Model, ContextWindow, or
//     RateLimits is non-nil), the caller is "claude".
//  3. If any recognized Antigravity environment variable — or a legacy
//     Gemini-era variable (Antigravity CLI deliberately reuses the ~/.gemini
//     config tree, and Gemini CLI is EOL) — is set and non-empty, the caller
//     is "antigravity". This still covers `gsl status` (no stdin payload).
//  4. Otherwise "" (unknown / plain shell usage).
//
// env is the env-lookup function (os.Getenv in production; injected in tests).
//
// Why rule 1 exists (the bug it fixes): BOTH hosts run the same shim — agy's
// settings.json points statusLine.command at a script that `exec`s `gsl render`
// with the payload on stdin, exactly as Claude Code does — and agy's payload
// carries cwd + model + context_window. So rule 2 matched EVERY agy render and
// deriveToolCtx returned "claude", which made theme.Resolve read
// ~/.claude/settings.json for Antigravity users. The entire "antigravity" branch
// of theme.Resolve (including the colorScheme support) was dead code on the real
// agy render path: an agy user with colorScheme "light" got their Claude theme —
// or a dark line — no matter what their Antigravity settings said. The env-var
// heuristic below never rescued it, because the payload check ran first.
func deriveToolCtx(p payload.Payload, env func(string) string) string {
	// 1. In-band product key — the only reliable discriminator, and the one agy
	//    actually sends on every render.
	if p.IsAntigravity() {
		return "antigravity"
	}
	// 2. Claude: the render subcommand populates the payload from stdin.
	if p.Cwd != nil || p.Model != nil || p.ContextWindow != nil || p.RateLimits != nil {
		return "claude"
	}
	// 3. Antigravity with no payload (`gsl status`): fall back to env vars, incl.
	//    the legacy Gemini-era vars it inherits.
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

	// Git status, computed EXACTLY ONCE (WS3 / F12).
	//
	// The repo segment needs the branch before Detect starts (BuildSegments takes
	// it as a construction arg), so this one call cannot be folded into the
	// concurrent phase. What CAN go is the duplicate: DirGitSegment used to run
	// git.Status a second time inside Detect, for 4 git execs where 2 suffice —
	// and those two extra execs were spent inside the SHARED 1s context, draining
	// the budget the concurrent segments were about to need. Threading the result
	// through Deps.GitInfo makes the segment reuse it.
	branch := ""
	var gitInfo *git.Info
	if cwd != "" {
		if info, err := git.Status(ctx, gitRunner, cwd); err == nil {
			gitInfo = &info
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
		GitInfo:      gitInfo,
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
