package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/flags"
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
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/video"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// videoSpawn starts the detached GPU refresher (video.SpawnRefresher). It is a
// variable so the cmd tests can replace it with a no-op (see TestMain): a real
// re-exec from inside `go test` would run the suite again, which renders,
// which spawns, with nothing reaping the detached children.
var videoSpawn = video.SpawnRefresher

// mcpSpawn starts the detached MCP refresh (mcp.SpawnRefresh) when the cached
// status is due one; a variable so the cmd tests can stub it (TestMain).
var mcpSpawn = mcp.SpawnRefresh

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

// linkFlagsBudget bounds the gff flag resolution. It runs concurrently with the
// origin lookup, so the render only ever waits this long for the flags, and a
// slow or absent gff simply leaves every link family on.
const linkFlagsBudget = 100 * time.Millisecond

// buildLinks folds the config mode and the gff flags into the effective link
// policy. "off" and gsl.links.enabled=false both disable every family; each
// family flag disables its own. Antigravity has no known usage page, so it gets
// no usage link unless the ai segment's usage_url option supplies one.
func buildLinks(mode string, lf flags.Links, remoteURL string, antigravity bool) render.Links {
	on := mode != config.LinksOff && lf.Enabled
	l := render.Links{
		Repo:    on && lf.Repo,
		DirGit:  on && lf.DirGit,
		AI:      on && lf.AI,
		Time:    on && lf.Time,
		RepoURL: remoteURL,
		TimeURL: render.DefaultTimeURL,
	}
	if !antigravity {
		l.UsageURL = render.DefaultClaudeUsageURL
	}
	return l
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

	deps, st, err := prepare(ctx, cfg, p, cwdHint)
	if err != nil {
		return err
	}
	deps.MCPSpawn = mcpSpawn
	if dir, derr := video.Dir(); derr == nil {
		deps.Video = &video.Cache{Dir: dir}
		deps.VideoSpawn = videoSpawn
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

// paletteFor is the palette for a render: GSL_THEME / SDK_THEME (a session
// override) first, then config.json's palette, then the shared rule with the
// host tool's theme as its hint (theme.Resolve).
func paletteFor(cfg config.Config, toolCtx string, env func(string) string, home string) string {
	if cfg.Palette != "" && env("GSL_THEME") == "" && env("SDK_THEME") == "" {
		if _, ok := style.Palette(cfg.Palette); ok {
			return cfg.Palette
		}
	}
	return theme.Resolve(toolCtx, env, home)
}

// prepare is everything a render needs before Detect: the cwd, git status
// (once), the PR and link policy, and the style — shared by the status line
// and the config studio so they draw the same line. The caller adds the
// refresher hooks (MCPSpawn, Video).
func prepare(ctx context.Context, cfg config.Config, p payload.Payload, cwdHint string) (render.Deps, style.Style, error) {
	gitRunner := git.NewSystemRunner()
	ghRunner := gh.NewSystemRunner()

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

	// Link policy. The gff flags resolve concurrently (fail-open, bounded by
	// linkFlagsBudget) while the one-exec origin lookup runs; both are threaded
	// through Deps so no segment shells out for them.
	flagCh := make(chan flags.Links, 1)
	go func() {
		fctx, fcancel := context.WithTimeout(ctx, linkFlagsBudget)
		defer fcancel()
		flagCh <- flags.Resolve(fctx, flags.GFFLookup(os.Getenv))
	}()
	// The PR lookup (gss registry, then gh) is pre-computed here too, so both
	// the repo badge and the directory → vscode.dev changes link share it.
	prCh := make(chan *repo.RepoInfo, 1)
	go func() {
		if cwd == "" || branch == "" {
			prCh <- nil
			return
		}
		top, terr := git.Toplevel(ctx, gitRunner, cwd)
		if terr != nil {
			prCh <- nil
			return
		}
		info, perr := repo.PR(ctx, ghRunner, branch, top, repo.DefaultRegistryPath())
		if perr != nil {
			prCh <- nil
			return
		}
		prCh <- info
	}()
	remoteURL := ""
	if cwd != "" {
		if u, rerr := git.RemoteWebURL(ctx, gitRunner, cwd); rerr == nil {
			remoteURL = u
		} else {
			observe.Default().WithFields(logrus.Fields{"event": "links.remote_url", "error": rerr.Error()}).Debug("no origin web URL; repo links omitted")
		}
	}
	linkFlags := <-flagCh
	prInfo := <-prCh

	st, err := styleFor(cfg, deriveToolCtx(p, os.Getenv))
	if err != nil {
		return render.Deps{}, style.Style{}, err
	}

	deps := render.Deps{
		Payload:      p,
		Cwd:          cwd,
		Branch:       branch,
		GitInfo:      gitInfo,
		RegistryPath: repo.DefaultRegistryPath(),
		Git:          gitRunner,
		GH:           ghRunner,
		MCPOpts:      mcp.Options{},
		Clock:        time.Now,
		Links:        buildLinks(cfg.EffectiveLinks(), linkFlags, remoteURL, p.IsAntigravity()),
		PR:           prInfo,
		SectionOff:   linkFlags.SectionOff(),
	}
	return deps, st, nil
}

// styleFor resolves cfg's style for a host tool: the palette (paletteFor),
// --color / --glyphs, and the links mode.
func styleFor(cfg config.Config, toolCtx string) (style.Style, error) {
	home, _ := os.UserHomeDir()
	mode, ascii, err := renderFlags(os.Getenv)
	if err != nil {
		return style.Style{}, err
	}
	st := style.ResolveConfig(os.Stderr, cfg.Style, configToRawStyles(cfg.Styles), ascii, paletteFor(cfg, toolCtx, os.Getenv, home))
	st.Mode = mode
	st.Links = cfg.EffectiveLinks()
	return st, nil
}
