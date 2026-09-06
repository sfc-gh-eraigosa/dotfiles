package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/schema"
	"github.com/spf13/cobra"
)

// loadFile reads and strictly parses the settings file, mapping any failure
// to ErrUsage: a file gcfg cannot read is a usage problem, not drift.
func loadFile(g *Globals) (*schema.File, []string, error) {
	f, warns, err := schema.Load(g.File)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrUsage, err)
	}
	return f, warns, nil
}

// lintReport is the --json shape.
type lintReport struct {
	OK       bool             `json:"ok"`
	File     string           `json:"file"`
	Warnings []string         `json:"warnings,omitempty"`
	Problems []schema.Problem `json:"problems,omitempty"`
}

func newLintCmd(g *Globals) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "lint",
		Short: "Check the settings file on its own — no network, no credential",
		Long: `Parses the file strictly (an unknown key is an error naming it and its line)
and reports what the types cannot express: values outside their enum,
duplicate names, missing required fields, an org block outside the
organization's .github repository, and any value shaped like a secret.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var owner, repo string
			if g.Target != "" {
				var err error
				if owner, repo, err = parseTarget(g.Target); err != nil {
					return err
				}
			}
			f, warns, err := loadFile(g)
			if err != nil {
				return err
			}
			problems := schema.Lint(f, schema.LintOpts{Owner: owner, Repo: repo})
			out := cmd.OutOrStdout()
			if asJSON {
				rep := lintReport{OK: len(problems) == 0, File: g.File, Warnings: warns, Problems: problems}
				b, err := json.MarshalIndent(rep, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(out, string(b))
			} else {
				for _, w := range warns {
					fmt.Fprintf(out, "warning: %s\n", w)
				}
				for _, p := range problems {
					fmt.Fprintln(out, p)
				}
				if len(problems) == 0 {
					fmt.Fprintf(out, "ok: %s is valid\n", g.File)
				}
			}
			if len(problems) > 0 {
				return fmt.Errorf("%w: %d problem(s) in %s", ErrUsage, len(problems), g.File)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "machine-readable report")
	return c
}
