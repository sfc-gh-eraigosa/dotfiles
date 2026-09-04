package cmd

import (
	"fmt"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
	"github.com/spf13/cobra"
)

// UpdateResult reports what happened to one host under the legacy
// updateHost compatibility shim.
type UpdateResult struct {
	Skipped bool
	Reason  string
}

// validRef is kept as a thin alias to updplan.ValidRef, which is the same
// charset rule PLUS the git check-ref-format hardening (no leading '-', no
// "..", no "@{", no ".lock" suffix — a leading '-' is a git option once
// interpolated bare).
func validRef(ref string) bool { return updplan.ValidRef(ref) }

// remoteUpdateScript is the update command for one host, parameterised by the
// git ref to move to (default `main`). Shared by the headless path and the TUI
// so there is exactly one definition of "update a host". Callers pass only
// refs that have passed validRef.
//
// It makes exactly ONE network call. The previous form ran `git fetch origin`
// AND `git pull --ff-only origin <ref>`, and a pull performs its own fetch —
// so every update contacted the remote twice for data the first call had
// already brought down. That is not merely wasteful: it doubles the exposure
// to a transient network fault, and one bit us on the Jetson —
//
//	From github.com:...  299953c..c6fccf8   <- fetch reached GitHub
//	Already on 'main'                       <- checkout fine
//	ssh: Could not resolve hostname github.com: Temporary failure in name
//	fatal: Could not read from remote repository.
//
// — where DNS answered for the fetch and failed for the redundant pull
// seconds later, failing an update that already had everything it needed
// locally.
//
// Merging FETCH_HEAD rather than origin/<ref> keeps tag and branch refs both
// working, exactly as the pull form did.
func remoteUpdateScript(ref string) string { return updateScript(ref, false) }

// resetToFetched forces the clone onto the fetched commit, for a host whose
// branch has diverged and can no longer fast-forward.
//
// It is destructive, so it preserves first — the same guarantee `--force`
// makes. EVERYTHING currently on the host (local commits AND uncommitted
// files) is committed to a `fleet-reset/<ts>` branch before the reset, so a
// `git reset --hard` can never be the thing that loses an operator's work.
// `git add -A` rather than a stash: a stash commit's tree excludes untracked
// files, which is how the original rescue silently dropped them.
// NOTE: built by concatenation, never fmt.Sprintf — the shell's date format
// (%Y%m%dT%H%M%SZ) collides with printf verbs, and `go vet` rightly refuses it.
func resetToFetched(ref string) string {
	return `ts=$(date -u +%Y%m%dT%H%M%SZ) && ` +
		`git checkout -q -b "fleet-reset/$ts" && git add -A && ` +
		`{ git -c user.email=fleet@local -c user.name=fleet commit -q -m "fleet pre-reset $ts" || true; } && ` +
		`git checkout -q "` + ref + `" && git reset --hard FETCH_HEAD`
}

// updateScript is the remote update, optionally forcing the clone onto the
// fetched commit instead of fast-forwarding. Kept as a thin wrapper for the
// TUI (leaf E still calls it); the CLI's own path is runUpdate/the executor.
func updateScript(ref string, reset bool) string {
	move := "git merge --ff-only FETCH_HEAD"
	if reset {
		move = resetToFetched(ref)
	}
	return fmt.Sprintf(
		"cd ~/git/dotfiles && git fetch origin %[1]s && git checkout %[1]s && "+
			"%[2]s && ./install.sh", ref, move)
}

// rescueWorktree preserves uncommitted work before a --force update.
//
// It commits the dirty state (tracked AND untracked, via `git add -A`) onto a
// rescue branch, returns the clone to its original branch — now clean, so
// pull --ff-only can proceed — and materialises the rescue branch as its own
// worktree for the operator to inspect. Nothing is ever discarded.
//
// The plan originally specified `git stash push -u` + `git branch <n> stash@{0}`.
// Verified against git 2.47.3 (evidence/task12/rescue-verification.txt): that
// approach recovers tracked modifications but SILENTLY LOSES untracked files,
// because a stash commit's tree excludes them (they live in stash^3). An
// operator inspecting the rescue worktree would conclude their untracked work
// was gone. The temp-commit form below recovers both.
const rescueWorktree = `cd ~/git/dotfiles && ts=$(date -u +%Y%m%dT%H%M%SZ) && ` +
	`orig=$(git rev-parse --abbrev-ref HEAD) && ` +
	`git checkout -q -b "fleet-rescue/$ts" && git add -A && ` +
	`git -c user.email=fleet@local -c user.name=fleet commit -q -m "fleet rescue $ts" && ` +
	`git checkout -q "$orig" && ` +
	`mkdir -p ~/.local/state/dotfiles/rescue && ` +
	`git worktree add ~/.local/state/dotfiles/rescue/$ts "fleet-rescue/$ts"`

// updateHost is a thin compatibility shim over the plan executor, kept for
// its migrated tests. It runs the built-in default plan (today's update)
// against host, targeting ref, with local: rescue substituted in when force
// is set (skip is the default plan's own policy already).
func updateHost(r runner.Runner, host, ref string, force bool) (UpdateResult, error) {
	plan := updplan.Default()
	if ref != "" {
		var err error
		plan, err = plan.WithRef(ref)
		if err != nil {
			return UpdateResult{}, err
		}
	}
	var local updplan.Local
	if force {
		local = updplan.LocalRescue
	}
	ex := updexec.Executor{IO: updexec.Console{R: r}, Local: local}
	rep := ex.RunHost(host, plan)
	for _, res := range rep.Results {
		if res.Status == updexec.Skipped {
			return UpdateResult{Skipped: true, Reason: res.Reason}, nil
		}
	}
	if rep.Failed() {
		return UpdateResult{}, rep.Err()
	}
	return UpdateResult{}, nil
}

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
// through, from the resolved CLI flags. Out is nil in tasks 20-21; task 23
// wires in the headless capture.
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

	out := cmd.OutOrStdout()

	if flagUpdateDryRun {
		return printDryRun(out, plan, local, flagUpdateReset)
	}

	r := runner.Exec{}
	reports := make([]updexec.HostReport, 0, len(hosts))
	var failures int
	for _, host := range hosts {
		ex := buildExecutor(r, newRunLogOutput(host), local)
		rep := ex.RunHost(host, plan)
		reports = append(reports, rep)
		if rep.Failed() {
			failures++
		}
	}

	if flagJSON {
		return printJSONReport(out, plan, reports)
	}

	fmt.Fprintf(out, "plan: %s\n", plan.Source)
	for _, rep := range reports {
		printHostReport(out, plan, rep)
	}
	if failures > 0 {
		return fmt.Errorf("%d host(s) not updated", failures)
	}
	return nil
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
