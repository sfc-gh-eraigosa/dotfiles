package cmd

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/cfgplan"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshfail"
	"github.com/spf13/cobra"
)

// remoteConfigCmd is a pure READ. A pull must never mutate its source — the
// same discipline the wake ladder holds to.
const remoteConfigCmd = "cat ~/.ssh/config 2>/dev/null"

var (
	pullAll    bool
	pullDryRun bool
	pullYes    bool
)

// pullPlan fetches the source's config and computes what importing it would do.
// It writes nothing, anywhere.
func pullPlan(r runner.Runner, source, localText string, o cfgplan.Opts) (cfgplan.Plan, error) {
	out, err := r.Run(source, remoteConfigCmd)
	if err != nil {
		// A trust failure is not a network failure; saying so sends the
		// operator to the right machine.
		if n := sshfail.Note(err); n != "" {
			return cfgplan.Plan{}, fmt.Errorf("%s: %s", source, n)
		}
		return cfgplan.Plan{}, fmt.Errorf("%s: %w", source, err)
	}
	if strings.TrimSpace(out) == "" {
		return cfgplan.Plan{}, fmt.Errorf("%s: no readable ~/.ssh/config", source)
	}
	return cfgplan.Build(localText, out, o)
}

// sourceHostName asks the local ssh client what a source alias resolves to,
// so the loopback guard can fire before any connection is opened.
func sourceHostName(alias string) string {
	out, err := exec.Command("ssh", "-G", alias).Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "hostname" {
			return f[1]
		}
	}
	return ""
}

var configPullCmd = &cobra.Command{
	Use:   "pull <source>",
	Short: "Import ssh-config host entries FROM one fleet host into this machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source := args[0]
		out := cmd.OutOrStdout()

		if isLoopbackHost(sourceHostName(source), net.LookupIP) {
			return fmt.Errorf("%s resolves to this machine — a pull needs a different host", source)
		}

		local, err := readConfig(flagConfig)
		if err != nil {
			return err
		}
		p, err := pullPlan(runner.Exec{}, source, local, cfgplan.Opts{
			Marker: flagMarker, All: pullAll, Source: source,
		})
		if err != nil {
			return err
		}

		fmt.Fprintf(out, "pull from %s:\n%s", source, renderPlan(p))
		if p.Empty() {
			// Nothing to do is success, not a failure.
			return nil
		}

		next, err := p.Apply(local)
		if err != nil {
			return err
		}
		if pullDryRun {
			return applyConfig(out, flagConfig, next, true)
		}
		if !pullYes && !askYesNo(cmd, "apply these changes?") {
			fmt.Fprintln(out, "nothing changed")
			return nil
		}
		if err := applyConfig(out, flagConfig, next, false); err != nil {
			return err
		}
		fmt.Fprintf(out, "updated %s (a timestamped backup was written alongside it)\n", flagConfig)

		if miss := missingIdentities(p, localFileExists); len(miss) > 0 {
			fmt.Fprintf(out, "\n%d imported host(s) reference a key that is not on this machine:\n", len(miss))
			for _, m := range miss {
				fmt.Fprintf(out, "  %s\n", m)
			}
			fmt.Fprintln(out, "they will report auth-failed until a key exists and is authorized")
		}
		return nil
	},
}

func init() {
	configPullCmd.Flags().BoolVar(&pullAll, "all", false, "import every concrete Host block, not just marked ones")
	configPullCmd.Flags().BoolVar(&pullDryRun, "dry-run", false, "print the resulting config without writing")
	configPullCmd.Flags().BoolVar(&pullYes, "yes", false, "skip the confirmation prompt (non-interactive)")
	configCmd.AddCommand(configPullCmd)
}
