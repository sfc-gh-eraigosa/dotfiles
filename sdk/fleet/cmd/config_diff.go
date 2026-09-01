package cmd

import (
	"fmt"
	"net"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/cfgplan"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/spf13/cobra"
)

var diffAll bool

// diffBothWays computes what a pull would do and what a push would do, WITHOUT
// performing either.
//
// It is the only place both directions appear together, and it is read-only by
// construction: it returns plans and writes nothing. Showing both is not a
// bidirectional operation — it is how an operator decides which one-way verb
// they actually want.
func diffBothWays(local, remote string, o cfgplan.Opts) (cfgplan.Plan, cfgplan.Plan, error) {
	inbound, err := cfgplan.Build(local, remote, o)
	if err != nil {
		return cfgplan.Plan{}, cfgplan.Plan{}, err
	}
	outbound, err := cfgplan.Build(remote, local, o)
	if err != nil {
		return cfgplan.Plan{}, cfgplan.Plan{}, err
	}
	return inbound, outbound, nil
}

var configDiffCmd = &cobra.Command{
	Use:   "diff <host>",
	Short: "Show how this machine's ssh config differs from a fleet host, changing nothing",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		host := args[0]
		out := cmd.OutOrStdout()
		if isLoopbackHost(sourceHostName(host), net.LookupIP) {
			return fmt.Errorf("%s resolves to this machine — a diff needs a different host", host)
		}
		local, err := readConfig(flagConfig)
		if err != nil {
			return err
		}
		// Fetch the host's config once; the plan pushPlan also computes is
		// recomputed below alongside its inbound twin, so only the text matters.
		remote, err := readRemoteConfig(runner.Exec{}, host)
		if err != nil {
			return err
		}
		inbound, outbound, err := diffBothWays(local, remote, cfgplan.Opts{Marker: flagMarker, All: diffAll, Source: host})
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "a pull from %s would:\n%s", host, renderPlan(inbound))
		fmt.Fprintf(out, "\na push to %s would:\n%s", host, renderPlan(outbound))
		fmt.Fprintln(out, "\nnothing was changed — run `fleet config pull` or `fleet config push` to act")
		return nil
	},
}

func init() {
	configDiffCmd.Flags().BoolVar(&diffAll, "all", false, "compare every concrete Host block, not just marked ones")
	configCmd.AddCommand(configDiffCmd)
}
