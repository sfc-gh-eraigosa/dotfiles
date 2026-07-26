package cmd

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/gitx"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/schema"
	"github.com/spf13/cobra"
)

// errLintFindings is the exit-1 sentinel returned when lint finds issues.
var errLintFindings = errors.New("lint findings")

var lintCmd = &cobra.Command{
	Use:   "lint [path]",
	Short: "Lint a feature flag file (default: discovered repo file)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runLint,
}

func init() {
	rootCmd.AddCommand(lintCmd)
}

func runLint(cmd *cobra.Command, args []string) error {
	var path string
	if len(args) == 1 {
		path = args[0]
	} else {
		// Discover the repo file from the resolver's working directory.
		r, err := newResolver()
		if err != nil {
			return err
		}
		repoRoot, ok := gitx.RepoRoot(r.P.WorkDir)
		if !ok {
			return fmt.Errorf("lint: not inside a git repository and no path argument given")
		}
		path = gitx.SourcePath(r.R, repoRoot)
	}

	// Resolve to absolute path for cleaner error messages.
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}

	ff, err := schema.LoadFeatureFile(path)
	if err != nil {
		return fmt.Errorf("lint: %w", err)
	}

	findings := schema.Lint(ff)
	for _, f := range findings {
		fmt.Fprintf(cmd.OutOrStdout(), "%s: [%s] %s\n", f.Path, f.Rule, f.Msg)
	}

	if len(findings) > 0 {
		return errLintFindings
	}
	return nil
}
