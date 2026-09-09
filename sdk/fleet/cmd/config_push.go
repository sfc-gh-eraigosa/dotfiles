package cmd

import (
	"fmt"
	"net"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/cfgplan"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/spf13/cobra"
)

// Push is the dangerous direction. A bad write costs SSH access to the target,
// and fleet cannot repair a host it can no longer reach — the transport it
// would need is the thing that broke. Everything below exists to make that
// outcome very unlikely, and human-recoverable when it happens anyway.
const (
	stagePath  = "~/.ssh/config.fleet-new"
	livePath   = "~/.ssh/config"
	validateOn = "fleet-validation-probe" // any alias; we only care that the file PARSES
)

var (
	pushAll      bool
	pushDryRun   bool
	pushYes      bool
	pushRetarget bool
)

// pushPlan computes what writing our config onto the target would do, and
// returns the target's CURRENT text alongside it.
//
// Returning that text is not a convenience: the plan must be applied to the
// target's own config so the merge preserves everything the target has that we
// do not model. Applying it to an empty string would replace the file and
// silently delete every unmodelled host and directive on that machine.
func pushPlan(r runner.Runner, target, localText string, o cfgplan.Opts) (cfgplan.Plan, string, error) {
	remote, err := readRemoteConfig(r, target)
	if err != nil {
		return cfgplan.Plan{}, "", err
	}
	p, err := cfgplan.Build(remote, localText, o)
	return p, remote, err
}

// selfRetarget reports whether the plan would change how we reach the very host
// we are writing to. An Add is exempt: a brand-new block cannot alter the route
// already in use.
func selfRetarget(p cfgplan.Plan, target string) bool {
	for _, c := range p.Changes {
		if c.Alias != target || c.Kind != cfgplan.Update {
			continue
		}
		for _, f := range c.Fields {
			switch f.Name {
			case "HostName", "Port", "User", "IdentityFile":
				return true
			}
		}
	}
	return false
}

// remoteInstall stages, VALIDATES, backs up, then replaces — in that order.
//
// Validation is the load-bearing step: ssh parses the staged file before it can
// become the live one, so a malformed config is rejected while the working one
// is still in place. The staging file is removed on rejection rather than left
// as litter that a later run might mistake for progress.
func remoteInstall(r runner.Runner, target, text string) error {
	if _, err := r.RunStdin(target, text, "cat > "+stagePath+" && chmod 600 "+stagePath); err != nil {
		return fmt.Errorf("%s: staging: %w", target, err)
	}
	if _, err := r.Run(target, "ssh -F "+stagePath+" -G "+validateOn+" >/dev/null 2>&1"); err != nil {
		// Leave the live config untouched and clean up after ourselves.
		_, _ = r.Run(target, "rm -f "+stagePath)
		return fmt.Errorf("%s: staged config failed to parse — nothing was installed: %w", target, err)
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	install := fmt.Sprintf("cp -p %s %s.bak-%s 2>/dev/null; mv %s %s",
		livePath, livePath, stamp, stagePath, livePath)
	if _, err := r.Run(target, install); err != nil {
		return fmt.Errorf("%s: installing: %w", target, err)
	}
	return nil
}

// verifyTarget re-probes after a write. Silence here means we may have just
// locked ourselves out, so it is reported loudly and names where the backup is.
func verifyTarget(r runner.Runner, target string) error {
	if _, err := r.Run(target, "true"); err != nil {
		return fmt.Errorf("%s STOPPED ANSWERING after the push — restore %s.bak-* on that machine "+
			"out of band (console or physical access); fleet cannot reach it to fix this", target, livePath)
	}
	return nil
}

var configPushCmd = &cobra.Command{
	Use:   "push <target>...",
	Short: "Publish ssh-config host entries FROM this machine TO fleet hosts",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		local, err := readConfig(flagConfig)
		if err != nil {
			return err
		}
		r := runner.Exec{}
		var failures int

		// One target at a time, each with its own plan and its own decision:
		// a batch confirmation would hide which machine is about to change.
		for _, target := range args {
			if isLoopbackHost(sourceHostName(target), net.LookupIP) {
				fmt.Fprintf(cmd.ErrOrStderr(), "SKIP %s — resolves to this machine\n", target)
				failures++
				continue
			}
			p, remoteText, err := pushPlan(r, target, local, cfgplan.Opts{Marker: flagMarker, All: pushAll})
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "FAIL %s: %v\n", target, err)
				failures++
				continue
			}
			fmt.Fprintf(out, "\npush to %s:\n%s", target, renderPlan(p))
			if p.Empty() {
				continue
			}
			if selfRetarget(p, target) && !pushRetarget {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"SKIP %s — this would change how we reach %s itself; re-run with --allow-self-retarget if you mean it\n",
					target, target)
				failures++
				continue
			}
			// Apply onto the TARGET's text so its own entries survive.
			next, err := p.Apply(remoteText)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "FAIL %s: %v\n", target, err)
				failures++
				continue
			}
			if pushDryRun {
				fmt.Fprintf(out, "--- would write %s on %s ---\n%s", livePath, target, next)
				continue
			}
			if !pushYes && !askYesNo(cmd, fmt.Sprintf("write this to %s?", target)) {
				fmt.Fprintln(out, "nothing changed")
				continue
			}
			if err := remoteInstall(r, target, next); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "FAIL %v\n", err)
				failures++
				continue
			}
			if err := verifyTarget(r, target); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "!!!! %v\n", err)
				failures++
				continue
			}
			fmt.Fprintf(out, "ok   %s updated (backup kept as %s.bak-*)\n", target, livePath)
		}
		if failures > 0 {
			return fmt.Errorf("%d target(s) failed", failures)
		}
		return nil
	},
}

func init() {
	configPushCmd.Flags().BoolVar(&pushAll, "all", false, "publish every concrete Host block, not just marked ones")
	configPushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "print what would be written without writing")
	configPushCmd.Flags().BoolVar(&pushYes, "yes", false, "skip the confirmation prompt (non-interactive)")
	configPushCmd.Flags().BoolVar(&pushRetarget, "allow-self-retarget", false,
		"permit a push that changes how we reach the target itself")
	configCmd.AddCommand(configPushCmd)
}
