package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/engine"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/family"
	_ "github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/family/general"
	_ "github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/family/security"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/report"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/schema"
	"github.com/spf13/cobra"
)

// newEngine is the seam for the engine; tests over a custom registry swap it.
var newEngine = func() *engine.Engine { return engine.New(family.Default) }

// outputFormat is how a report is rendered.
type outputFormat struct {
	json     bool
	markdown bool
}

func (f *outputFormat) bind(c *cobra.Command) {
	c.Flags().BoolVar(&f.json, "json", false, "machine-readable report")
	c.Flags().BoolVar(&f.markdown, "markdown", false, "markdown table, for a GitHub step summary")
}

// render writes a report in the requested shape.
func (f outputFormat) render(w io.Writer, rep engine.Report, g *Globals) error {
	switch {
	case f.json:
		return report.JSON(w, rep)
	case f.markdown:
		return report.Markdown(w, rep)
	default:
		return report.TTY(w, rep, report.Options{NoColor: g.NoColor})
	}
}

// findings is the error a verb returns when a report is not clean; the
// report has already been printed, so main stays quiet (exit 1).
func findings(rep engine.Report) error {
	if rep.Clean() {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrFindings, rep.Headline())
}

// loadForRun reads the settings file, turning "not there" into advice.
func loadForRun(g *Globals) (*schema.File, error) {
	f, _, err := schema.Load(g.File)
	if err != nil {
		if os.IsNotExist(underlying(err)) {
			return nil, fmt.Errorf("%w: no %s — run `gcfg init` to write one, or `gcfg export` to capture what is live", ErrUsage, g.File)
		}
		return nil, fmt.Errorf("%w: %v", ErrUsage, err)
	}
	return f, nil
}

// underlying unwraps to the deepest error, for os.IsNotExist.
func underlying(err error) error {
	for {
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return err
		}
		next := u.Unwrap()
		if next == nil {
			return err
		}
		err = next
	}
}

// options builds the engine options from the flags.
func (g *Globals) options(only []string, dryRun bool) engine.Options {
	return engine.Options{Only: only, Org: g.Org, DryRun: dryRun}
}

// usage wraps an engine error as a usage error when it is one.
func engineErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrUsage, err)
}

func newVerifyCmd(g *Globals) *cobra.Command {
	var only []string
	var format outputFormat
	c := &cobra.Command{
		Use:   "verify",
		Short: "Check the live settings against the file (exit 1 on drift)",
		Long: `Reads every family the file declares and reports what disagrees. Exit 0 when
the live settings match, 1 when anything needs attention, 2 when the file or
the credential is the problem.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, client, err := g.resolve(cmd.Context())
			if err != nil {
				return err
			}
			file, err := loadForRun(g)
			if err != nil {
				return err
			}
			rep, err := newEngine().Verify(cmd.Context(), client, famTarget(target), file, g.options(only, false))
			if err != nil {
				return engineErr(err)
			}
			if err := format.render(cmd.OutOrStdout(), rep, g); err != nil {
				return err
			}
			return findings(rep)
		},
	}
	c.Flags().StringSliceVar(&only, "only", nil, "limit to these families")
	format.bind(c)
	return c
}

func newPlanCmd(g *Globals) *cobra.Command {
	var only []string
	var format outputFormat
	c := &cobra.Command{
		Use:   "plan",
		Short: "Show what apply would change, without changing anything",
		RunE: func(cmd *cobra.Command, args []string) error {
			target, client, err := g.resolve(cmd.Context())
			if err != nil {
				return err
			}
			file, err := loadForRun(g)
			if err != nil {
				return err
			}
			changes, rep, err := newEngine().Plan(cmd.Context(), client, famTarget(target), file, g.options(only, false))
			if err != nil {
				return engineErr(err)
			}
			out := cmd.OutOrStdout()
			if format.json {
				if err := report.JSON(out, rep); err != nil {
					return err
				}
			} else {
				for _, ch := range changes {
					fmt.Fprintf(out, "%s\n", ch)
				}
				if err := format.render(out, rep, g); err != nil {
					return err
				}
			}
			return findings(rep)
		},
	}
	c.Flags().StringSliceVar(&only, "only", nil, "limit to these families")
	format.bind(c)
	return c
}

// isTTY reports whether stdout is a terminal; apply refuses to guess
// otherwise. It is a var so a test can be explicit about which it is.
var isTTY = func() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func newApplyCmd(g *Globals) *cobra.Command {
	var only []string
	var yes, dryRun bool
	var format outputFormat
	c := &cobra.Command{
		Use:   "apply",
		Short: "Make the live settings match the file, then read them back",
		Long: `Plans the difference, applies it, and re-reads every family — a write GitHub
accepted is not the same as a setting that changed. Requires --yes when
stdout is not a terminal, so nothing is written by a script that did not ask.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, client, err := g.resolve(cmd.Context())
			if err != nil {
				return err
			}
			file, err := loadForRun(g)
			if err != nil {
				return err
			}
			e := newEngine()
			opts := g.options(only, dryRun)
			changes, rep, err := e.Plan(cmd.Context(), client, famTarget(target), file, opts)
			if err != nil {
				return engineErr(err)
			}
			out := cmd.OutOrStdout()
			if len(changes) == 0 {
				if err := format.render(out, rep, g); err != nil {
					return err
				}
				return findings(rep)
			}
			for _, ch := range changes {
				fmt.Fprintf(out, "%s\n", ch)
			}
			if dryRun {
				fmt.Fprintf(out, "\n--dry-run: nothing was written\n")
				return findings(rep)
			}
			if !yes && !isTTY() {
				return fmt.Errorf("%w: refusing to apply %d change(s) without a terminal to confirm at — re-run with --yes", ErrUsage, len(changes))
			}
			if !yes {
				fmt.Fprintf(out, "\napply %d change(s) to %s? [y/N] ", len(changes), target)
				var answer string
				fmt.Fscanln(cmd.InOrStdin(), &answer)
				if answer != "y" && answer != "Y" {
					return fmt.Errorf("%w: cancelled", ErrUsage)
				}
			}
			after, err := e.Apply(cmd.Context(), client, famTarget(target), file, changes, opts)
			if err != nil {
				return err
			}
			if err := format.render(out, after, g); err != nil {
				return err
			}
			return findings(after)
		},
	}
	c.Flags().StringSliceVar(&only, "only", nil, "limit to these families")
	c.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change and stop")
	format.bind(c)
	return c
}

// famTarget converts the cmd target to the family one.
func famTarget(t Target) family.Target {
	ft := family.Target{Owner: t.Owner, Repo: t.Repo}
	return ft
}
