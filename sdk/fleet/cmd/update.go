package cmd

import (
	"fmt"
	"io"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
	"github.com/spf13/cobra"
)

// validRef is kept as a thin alias to updplan.ValidRef, which is the same
// charset rule PLUS the git check-ref-format hardening (no leading '-', no
// "..", no "@{", no ".lock" suffix — a leading '-' is a git option once
// interpolated bare).
func validRef(ref string) bool { return updplan.ValidRef(ref) }

// resolveLocalPolicy folds --local and --force into one updplan.Local
// override: "" means "use each repo's own policy". --force is an alias for
// --local rescue; giving both with conflicting values is an error rather
// than one silently winning.
func resolveLocalPolicy(local string, force bool) (updplan.Local, error) {
	if local != "" {
		switch updplan.Local(local) {
		case updplan.LocalSkip, updplan.LocalRescue, updplan.LocalCarry:
		default:
			return "", fmt.Errorf("invalid --local %q: must be skip, rescue, or carry", local)
		}
	}
	if force {
		if local != "" && local != string(updplan.LocalRescue) {
			return "", fmt.Errorf("--force conflicts with --local %s (--force implies --local rescue)", local)
		}
		return updplan.LocalRescue, nil
	}
	return updplan.Local(local), nil
}

var (
	flagUpdateLocal     string
	flagUpdateForce     bool
	flagUpdateNoRestore bool
	flagUpdateReset     bool
	flagUpdateTimeout   time.Duration
	flagUpdateNoRetry   bool
	flagUpdateRefs      []string
	flagUpdateFile      string
	flagUpdateDryRun    bool
)

// buildExecutor assembles the Executor a live (non-dry-run) update runs
// through, from the resolved CLI flags. out is the headless capture (task
// 23's newRunLogOutput); nil is a valid Discard.
func buildExecutor(r runner.Runner, out updexec.Output, local updplan.Local) updexec.Executor {
	return updexec.Executor{
		IO:        updexec.Console{R: r, Preamble: localAnswerPreamble},
		Out:       out,
		Local:     local,
		NoRestore: flagUpdateNoRestore,
		Reset:     flagUpdateReset,
		NoRetry:   flagUpdateNoRetry,
		Timeout:   flagUpdateTimeout,
	}
}

// runUpdate resolves the plan and flags, then runs every host serially
// (interactive steps cannot share a terminal) through the plan executor.
func runUpdate(cmd *cobra.Command, hosts []string) error {
	return runUpdateWith(cmd.OutOrStdout(), hosts, runner.Exec{})
}

// runUpdateWith is runUpdate with its output writer and runner injected, so
// a test can drive the whole CLI path — plan resolution, the executor, the
// headless capture, the report — without a cobra.Command or a real ssh.
func runUpdateWith(out io.Writer, hosts []string, r runner.Runner) error {
	local, err := resolveLocalPolicy(flagUpdateLocal, flagUpdateForce)
	if err != nil {
		return err
	}
	if flagUpdateReset && local == updplan.LocalCarry {
		return fmt.Errorf("--reset is incompatible with --local carry")
	}

	plan, err := loadPlan(flagUpdateFile, &featflag.GFF{Repo: flagRepo}, flagRepo)
	if err != nil {
		return err
	}
	if len(flagUpdateRefs) > 0 {
		if plan, err = plan.WithRefs(flagUpdateRefs); err != nil {
			return err
		}
	}

	if flagUpdateDryRun {
		return printDryRun(out, plan, local, flagUpdateReset)
	}

	reports := make([]updexec.HostReport, 0, len(hosts))
	for _, host := range hosts {
		ex := buildExecutor(r, newRunLogOutput(host), local)
		reports = append(reports, ex.RunHost(host, plan))
	}

	if flagJSON {
		return printJSONReport(out, plan, reports)
	}

	fmt.Fprintf(out, "plan: %s\n", plan.Source)
	for _, rep := range reports {
		printHostReport(out, plan, rep)
	}
	return exitErrorForReports(reports)
}

var updateCmd = &cobra.Command{
	Use: "update <host>...",
	Short: "Update hosts from a fleet.yaml plan (today's dotfiles fetch+ff+" +
		"install.sh when none is configured)",
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUpdate(cmd, args)
	},
}

func init() {
	updateCmd.Flags().StringVar(&flagUpdateLocal, "local", "", "local-changes policy override: skip|rescue|carry")
	updateCmd.Flags().BoolVar(&flagUpdateForce, "force", false, "alias for --local rescue")
	updateCmd.Flags().BoolVar(&flagUpdateNoRestore, "no-restore", false, "never restore a repo's original branch/stash")
	updateCmd.Flags().BoolVar(&flagUpdateReset, "reset", false, "force the clone onto the fetched commit instead of fast-forwarding (incompatible with --local carry)")
	updateCmd.Flags().DurationVar(&flagUpdateTimeout, "timeout", 0, "override every batch step's per-attempt timeout")
	updateCmd.Flags().BoolVar(&flagUpdateNoRetry, "no-retry", false, "run every step at most once")
	updateCmd.Flags().StringArrayVar(&flagUpdateRefs, "ref", nil, "git ref (branch or tag) to target: B or repo=B; repeatable; default = the plan's own branches")
	updateCmd.Flags().StringVar(&flagUpdateFile, "file", "", "explicit fleet.yaml plan path (skips gff resolution)")
	updateCmd.Flags().BoolVar(&flagUpdateDryRun, "dry-run", false, "print every effective script and send nothing")
	rootCmd.AddCommand(updateCmd)
}
