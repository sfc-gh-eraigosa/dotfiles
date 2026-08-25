// Package cmd implements the wlink command line.
//
// wlink = WSL link: the link between this WSL box and the private network its
// fleet lives on — the tunnel carrying it, the resolver that makes its names
// resolvable, and whether it is currently usable.
package cmd

import (
	"fmt"
	"os"
	"runtime"
	"sync"

	applog "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/log"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Stamped by build.sh via -ldflags (see sdk/version.sh). Mirrors sdk/fleet.
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
	Dirty     = "false"
)

// VersionString renders the stamped build identity for `wlink --version`.
func VersionString() string {
	s := fmt.Sprintf("wlink %s (%s) built %s %s/%s",
		Version, Commit, BuildDate, runtime.GOOS, runtime.GOARCH)
	if Dirty == "true" {
		s += " [dirty]"
	}
	return s
}

// Logging is claimed exactly once, before any command body runs. Doing it more
// than once would be harmless but would mean two places own the tool identity;
// the counter lets the test prove there is only one.
var (
	loggingOnce      sync.Once
	loggingTool      string
	initLoggingCalls int
)

func initLogging() {
	loggingOnce.Do(func() {
		applog.SetDefaultTool("wlink")
		loggingTool = "wlink"
		initLoggingCalls++
	})
}

// Log is how every package in this module emits diagnostics. Nothing here
// hand-rolls a logger, a file writer, or rotation — that is the sdk contract,
// and libs/log guarantees construction never fails, so logging can never
// introduce a failure mode into a tool that rewrites /etc/resolv.conf.
func Log() *logrus.Logger { initLogging(); return applog.Default() }

func newRootCmd() *cobra.Command {
	var showVersion bool
	root := &cobra.Command{
		Use:   "wlink",
		Short: "WSL link — tunnel and resolver management for reaching your fleet by name",
		Long: "wlink reports whether this WSL box can reach its fleet by name, and pins a\n" +
			"resolver that knows those names — reversibly.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(*cobra.Command, []string) {
			initLogging()
		},
		RunE: func(c *cobra.Command, _ []string) error {
			if showVersion {
				fmt.Fprintln(c.OutOrStdout(), VersionString())
				return nil
			}
			return c.Help()
		},
	}
	root.Flags().BoolVar(&showVersion, "version", false, "print the stamped build version and exit")
	root.AddCommand(newPinCmd(), newUnpinCmd(), newStatusCmd(), newVerifyCmd(), newWaitCmd(), newDoctorCmd())
	return root
}

// Execute runs the CLI. Exit codes are part of the contract (spec §3):
// 0 success or a safe decline, 1 a real failure, 2 a usage error.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "wlink:", err)
		os.Exit(2)
	}
}
