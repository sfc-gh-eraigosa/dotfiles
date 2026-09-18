package cmd

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
	"github.com/spf13/cobra"
)

var (
	tuiUpdateRef string
	tuiJobs      int
	tuiFile      string
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive host list: vim navigation, search, selection, concurrent updates",
	RunE: func(cmd *cobra.Command, _ []string) error {
		// Both flags reach a remote shell / bound concurrency, so they are
		// validated before a single host is contacted. An UNSET --update-ref
		// (the default) means "don't touch the plan's own branch" — it is
		// validated only when the operator actually gave one.
		if tuiUpdateRef != "" && !validRef(tuiUpdateRef) {
			return fmt.Errorf("invalid --update-ref %q: must be a git ref (letters, digits, . _ / -)", tuiUpdateRef)
		}
		if tuiJobs < 1 {
			return fmt.Errorf("invalid --jobs %d: must be at least 1", tuiJobs)
		}
		// The plan is loaded ONCE, here, through the same leaf-D loader the
		// headless `fleet update` uses — the TUI grows no second plan loader.
		// --update-ref is validated against the resolved plan before a single
		// host is contacted, the same way --ref is validated for `update`.
		plan, policy, err := resolveTUIPlan(tuiFile, tuiUpdateRef, flagRepo, &featflag.GFF{Repo: flagRepo})
		if err != nil {
			return err
		}
		// The frame glyphs are decided once, here, from the same resolution:
		// the package default is rounded so tests stay deterministic.
		th = newTheme(wantASCIIBorders(policy.asciiBorders))
		// A MISSING config is an empty fleet, not a failure: on a fresh
		// machine there is nothing to read yet, and refusing to start is
		// how `fleet` became unusable on exactly the host that needed
		// setting up.
		rawStr, err := readConfig(flagConfig)
		if err != nil {
			return fmt.Errorf("reading %s: %w", flagConfig, err)
		}
		raw := []byte(rawStr)
		all, err := sshconf.Parse(string(raw), flagMarker)
		if err != nil {
			return err
		}
		// An empty fleet is where a first run begins, not an error. Offer to
		// set one up before opening a dashboard with nothing in it.
		if len(all) == 0 {
			changed, err := ensureFleet(cmd, len(all))
			if err != nil {
				return err
			}
			if changed {
				rawStr, err := readConfig(flagConfig)
				if err != nil {
					return err
				}
				if all, err = sshconf.Parse(rawStr, flagMarker); err != nil {
					return err
				}
			}
			if len(all) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no hosts to show yet — nothing to display.")
				return nil
			}
		}
		base, err := newGitBaseline(flagRepo, flagRef)
		if err != nil {
			return err
		}
		p, err := wakePolicy()
		if err != nil {
			return err
		}
		// No synchronous probing here: the model opens instantly and streams
		// rows in as each host answers (spec F1).
		r := runner.Exec{}
		m := newTUIModel(selectHosts(all, nil), r, base, time.Now(), tuiUpdateRef, tuiJobs, plan)
		m.wake = newWaker(r, p)
		// Who are WE? Detected here, at the one impure edge, and handed to the
		// model as data — the model never reads the hostname itself, or every
		// test would render whatever machine it happened to run on.
		m.setLocal(detectLocal())
		// Persistence is wired HERE, not in newTUIModel, so the model stays a
		// pure value and tests never touch a real config directory.
		wireTUIPaths(&m)
		// Re-passed to the interactive handoff's self-exec (`fleet update
		// <alias> --file <tuiFile> ...`) so a routed host resolves the SAME
		// plan the TUI itself loaded, not gff's own (possibly different)
		// resolution.
		m.file = tuiFile
		// Re-passed to the interactive handoff's self-exec so a routed host
		// resolves gff/the plan against the SAME checkout the TUI loaded
		// from, not the child's own --repo default.
		m.repo = flagRepo
		// The lane policy came out of the SAME gff resolution as the plan
		// (resolveTUIPlan). Fail-closed: no gff, or a key that cannot
		// resolve, means no host changes; the flag's own default (true) only
		// applies when it resolves.
		m.policy = policy
		_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
		return err
	},
}

// resolveTUIPlan loads the tui command's update plan (via loadPlan, exactly
// like `fleet update`) and, ONLY when ref is non-empty, applies --update-ref
// through plan.WithRef, validated before a single host is contacted. An
// empty ref (the flag's default) leaves the plan's own branch untouched —
// this used to apply plan.WithRef(tuiUpdateRef) unconditionally with a
// default of "main", which silently overrode the plan's own dotfiles branch
// (e.g. a [release] plan) on bare `fleet`/`fleet tui`, and refused to start
// at all against a multi-repo plan with no "dotfiles" repo (WithRef's
// ambiguous-repo error). Extracted so a test can drive it without cobra or
// a real ssh config.
//
// It also returns the background lane's policy (bgPolicy), derived from the
// ONE featflag resolution this function performs: the plan and the policy
// must come from the same checkout and the same flag state, and each extra
// resolution forks `git config` and re-parses the feature file. --file pins
// the PLAN without consulting gff; the lane policy still reads gff (it has
// nowhere else to come from), which is why src is a parameter — tests pass
// nil (or a featflag.Static) and never touch the real feature file.
func resolveTUIPlan(file, ref, repoDir string, src featflag.Source) (updplan.Plan, bgPolicy, error) {
	settings := featflag.Resolve(src, "", repoDir)
	policy := policyFrom(settings)
	var (
		plan updplan.Plan
		err  error
	)
	if file != "" {
		plan, err = readPlanFile(file, "")
	} else {
		plan, err = planFromSettings(settings)
	}
	if err != nil {
		return updplan.Plan{}, bgPolicy{}, err
	}
	if ref == "" {
		return plan, policy, nil
	}
	plan, err = plan.WithRef(ref)
	if err != nil {
		return updplan.Plan{}, bgPolicy{}, fmt.Errorf("invalid --update-ref %q: %w", ref, err)
	}
	return plan, policy, nil
}

// wantASCIIBorders decides the frame glyphs for this run. The gff flag
// (fleet.tui.ascii-borders) is the host-scoped preference: set it on the
// machine you run `fleet` from. FLEET_ASCII_BORDERS is the per-session
// override for a client that differs from the host's default — "1" forces
// ASCII, "0" forces the rounded frames back, anything else (including
// unset) defers to the flag. Explicit values only: `!= ""` would have made
// `FLEET_ASCII_BORDERS=0` turn ASCII on.
func wantASCIIBorders(fromFlag bool) bool {
	switch os.Getenv("FLEET_ASCII_BORDERS") {
	case "1":
		return true
	case "0":
		return false
	}
	return fromFlag
}

func init() {
	tuiCmd.Flags().StringVar(&flagRef, "ref", "origin/main", "baseline git ref (what hosts are compared against)")
	tuiCmd.Flags().StringVar(&tuiUpdateRef, "update-ref", "", "git ref to update hosts TO (default: the plan's own branch)")
	tuiCmd.Flags().IntVar(&tuiJobs, "jobs", 4, "max concurrent background updates")
	tuiCmd.Flags().StringVar(&tuiFile, "file", "", "explicit fleet.yaml plan path (the plan skips gff resolution; lane flags such as fleet.update.sudo-timestamp-global still read gff)")
	rootCmd.AddCommand(tuiCmd)
}
