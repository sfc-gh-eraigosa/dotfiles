package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

var (
	auditFeature string
	auditRepair  bool
	auditJSON    bool
)

var featureAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Report (and optionally repair) registry drift vs observed state",
	Long: `Walk the registry and surface drift between recorded state and observed
reality (missing worktrees/branches, 404 PRs, diverged PR bases, duplicate
branches, stale pr_state). Read-only by default; --repair applies the
deterministic registry-local fixes (never force-pushes, renames a branch, or
calls a mutating gh verb). Exits non-zero if any error-severity finding
exists. "Observable state wins over the registry."`,
	Run: func(cmd *cobra.Command, args []string) {
		store, err := newRegistryStore()
		if err != nil {
			fail(err)
		}
		// audit is a recovery tool — give it only the registry + read-only
		// git/gh observers + the repo path; no origin/NWO resolution.
		svc := &feature.Service{Store: store, Git: git.NewSystemRunner(), GH: gh.NewSystemClientInDir(getRepoPath())}
		rep, err := svc.Audit(context.Background(), feature.AuditOpts{
			Feature:  auditFeature,
			Repair:   auditRepair,
			RepoPath: getRepoPath(),
		})
		if err != nil {
			fail(err)
		}
		if auditJSON {
			data, err := rep.JSON()
			if err != nil {
				fail(err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
		} else {
			fmt.Fprint(cmd.OutOrStdout(), rep.Text())
		}
		if rep.HasErrors() {
			os.Exit(1) // problems found (lint-style); the report is already printed
		}
	},
}

func init() {
	f := featureAuditCmd.Flags()
	f.StringVar(&auditFeature, "feature", "", "Restrict to one feature (default: all)")
	f.BoolVar(&auditRepair, "repair", false, "Apply deterministic registry-local fixes")
	f.BoolVar(&auditJSON, "json", false, "JSON output")
	featureCmd.AddCommand(featureAuditCmd)
}
