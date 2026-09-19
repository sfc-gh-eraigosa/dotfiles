package cmd

import (
	"fmt"
	"io"
	"strings"
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
	flagUpdateInputs    []string
	flagUpdateListIn    bool
)

// buildExecutor assembles the Executor a live (non-dry-run) update runs
// through, from the resolved CLI flags. out is the headless capture (task
// 23's newRunLogOutput); nil is a valid Discard.
func buildExecutor(r runner.Runner, out updexec.Output, local updplan.Local, plan updplan.Plan, host string, vals inputValues) updexec.Executor {
	// The plan's declared inputs ride in on the same two channels the sudo
	// answers already use: plain values in the export preamble, confidential
	// ones on stdin.
	preamble := func(st updplan.Step) string {
		return localAnswerPreamble(st) + inputPreambleOrFail(plan, st, host, vals)
	}
	io := updexec.Console{R: r, Preamble: preamble}
	// Stdin is attached ONLY when the plan actually has a confidential value
	// to send. The CLI lane has no sudo secret of its own, and a non-nil
	// Stdin that always returns "" would still change what the lane looks
	// like to anything inspecting it.
	if planHasConfidentialInput(plan) {
		io.Stdin = func(st updplan.Step) string { return inputStdin(plan, st, host, vals) }
	}
	return updexec.Executor{
		IO:        io,
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
	return runUpdateWith(cmd.OutOrStdout(), hosts, runner.Exec{}, newRunLogOutput())
}

// inputPreambleOrFail is inputPreamble for a lane that has already started:
// validateInputDelivery refuses every undeliverable combination up front, so
// an error here means a plan the checks did not foresee — and the step must
// then FAIL, loudly, with the reason. Dropping the preamble and running the
// script anyway would be the silent "feature stayed off" outcome rcInputFlag
// exists to remove.
func inputPreambleOrFail(plan updplan.Plan, st updplan.Step, host string, vals inputValues) string {
	pre, err := inputPreamble(plan, st, host, vals)
	if err != nil {
		return fmt.Sprintf("echo %s >&2; exit %d; ", shQuote("fleet: "+err.Error()), rcInputFlag)
	}
	return pre
}

// runUpdateWith is runUpdate with its output writer, runner and CAPTURE
// injected, so a test can drive the whole CLI path — plan resolution, the
// executor, the headless capture, the report — without a cobra.Command, a
// real ssh, or a write into the operator's own state directory.
//
// capture used to be resolved in here via newRunLogOutput(), which meant
// every test driving this path wrote a real file under ~/.local/state/fleet
// — the writer and the runner were injected but the one dependency that
// touches the developer's home was not. Pass updexec.Discard{} for a test
// that does not care; newRunLogOutput() is what production passes.
func runUpdateWith(out io.Writer, hosts []string, r runner.Runner, capture updexec.Output) error {
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

	// Answer "what do I have to supply, and why" without reading the plan.
	if flagUpdateListIn {
		listInputs(out, plan)
		return nil
	}

	// The plan may declare values it cannot run without. Resolve them before
	// a single host is contacted: discovering a missing token halfway through
	// a fleet leaves half of it converged and half not.
	vals, err := parseInputFlags(plan, flagUpdateInputs)
	if err != nil {
		return err
	}
	if err := validateInputDelivery(plan, vals); err != nil {
		return err
	}

	if flagUpdateDryRun {
		// Once per host only when a per-host input makes the preview differ
		// between them; the ordinary plan prints once, as it always has.
		if !plan.HasHostInputs() {
			return printDryRunFor(out, plan, local, flagUpdateReset, hosts[0], vals)
		}
		for _, host := range hosts {
			fmt.Fprintf(out, "=== %s ===\n", host)
			if err := printDryRunFor(out, plan, local, flagUpdateReset, host, vals); err != nil {
				return err
			}
		}
		return nil
	}

	if missing := missingInputs(plan, hosts, vals); len(missing) > 0 {
		return fmt.Errorf("the plan needs values this run has no answer for: %s\n  supply each with --input <id>=<value> (per-host: --input <host>:<id>=<value>)\n  a confidential value must come from a file or the environment: --input <id>=@<path> or --input <id>=env:<NAME>",
			strings.Join(missing, ", "))
	}

	// One capture value, reused across every host: it carries no per-host
	// state (Open is keyed by the host/header arguments it is called with
	// each time), so reconstructing it per host was pure churn.
	reports := make([]updexec.HostReport, 0, len(hosts))
	for _, host := range hosts {
		ex := buildExecutor(r, capture, local, plan, host, vals)
		reports = append(reports, ex.RunHost(host, plan))
	}

	if flagJSON {
		if err := printJSONReport(out, plan, reports); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "plan: %s\n", plan.Source)
		for _, rep := range reports {
			printHostReport(out, plan, rep)
		}
	}
	// The exit code must always reflect the reports, JSON or not — printing
	// nil from printJSONReport (which reports only a MARSHAL/WRITE failure,
	// never a host failure) used to be returned as-is, so `fleet update
	// --json` exited 0 even when every host failed.
	return exitErrorForReports(reports)
}

var updateCmd = &cobra.Command{
	Use: "update <host>...",
	Short: "Update hosts from a fleet.yaml plan (today's dotfiles fetch+ff+" +
		"install.sh when none is configured)",
	// --list-inputs asks about the plan, not a host, so it is the one form
	// that needs no host argument.
	Args: func(cmd *cobra.Command, args []string) error {
		if flagUpdateListIn {
			return nil
		}
		return cobra.MinimumNArgs(1)(cmd, args)
	},
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
	updateCmd.Flags().BoolVar(&flagUpdateListIn, "list-inputs", false, "describe every value this plan needs, then exit")
	updateCmd.Flags().StringArrayVar(&flagUpdateInputs, "input", nil, "value for a plan-declared input: <id>=<value>, <id>=@<file>, <id>=env:<NAME>, or <host>:<id>=… for a per-host one; repeatable")
	rootCmd.AddCommand(updateCmd)
}
