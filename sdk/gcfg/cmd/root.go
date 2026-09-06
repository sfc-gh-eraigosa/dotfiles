// Package cmd wires the gcfg CLI verbs. NewRootCmd builds a fresh command
// tree per call (tests run many in one process); exit-code mapping lives
// only in main.go.
package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Sentinel errors the exit-code mapping keys on (plan §3.5).
var (
	// ErrUsage → exit 2: usage, no credential, or a family unreadable in a
	// way that makes the answer meaningless.
	ErrUsage = errors.New("usage")
	// ErrFindings → exit 1: verify found drift, or apply left findings.
	// The report is already on stdout; main prints nothing more.
	ErrFindings = errors.New("findings")
)

// Globals are the persistent flags every verb reads.
type Globals struct {
	Target  string // -R owner/repo
	Auth    string // env|gh|app|auto
	Org     bool   // operate on the org block
	File    string // -f path to gcfg.yaml
	NoColor bool
}

// parseTarget splits owner/repo, rejecting anything else.
func parseTarget(s string) (owner, repo string, err error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("%w: -R wants owner/repo, got %q", ErrUsage, s)
	}
	return parts[0], parts[1], nil
}

// NewRootCmd returns a fresh root command with every verb attached.
func NewRootCmd() *cobra.Command {
	g := &Globals{}
	root := &cobra.Command{
		Use:   "gcfg",
		Short: "GitHub settings as code — declare .github/gcfg.yaml, then verify and apply it",
		Long: `gcfg keeps a repository's (or org's) GitHub settings in a file:
export what is live, verify the file against it in CI, and apply the
difference on purpose. Settings it does not know are reported, never touched.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	pf := root.PersistentFlags()
	pf.StringVarP(&g.Target, "repo", "R", "", "owner/repo (default: the origin of the current directory)")
	pf.StringVar(&g.Auth, "auth", "auto", "credential source: env|gh|app|auto")
	pf.StringVarP(&g.File, "file", "f", ".github/gcfg.yaml", "path to the settings file")
	pf.BoolVar(&g.Org, "org", false, "operate on the org block (only in the org's .github repo)")
	pf.BoolVar(&g.NoColor, "no-color", false, "plain output")
	root.AddCommand(newVersionCmd())
	return root
}
