package cmd

import (
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/internal/logging"
	"log"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/pkg/tmux"
	"github.com/spf13/cobra"
)

var (
	// Tmgr is the global instance of our tmux manager package.
	Tmgr = &tmux.Manager{}
)

var rootCmd = &cobra.Command{
	Use:   "tmux-mgr",
	Short: "A tmux session and window manager",
	Long:  `tmux-mgr is a CLI tool to manage tmux sessions, windows, and layouts efficiently.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Sync the verbose flag to the package manager
		Tmgr.Verbose, _ = cmd.Flags().GetBool("verbose")
	},
}

func init() {
	// Logging is the shared driver's now: rotated, under the state dir, and
	// configurable via $TMUX_MGR_LOG_FILE / $TMUX_MGR_LOG_LEVEL. It cannot
	// fail, so there is no error path to report here any more.
	logging.Init()

	rootCmd.PersistentFlags().BoolVarP(&Tmgr.Verbose, "verbose", "v", false, "Enable verbose output")
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Printf("Execution error: %v", err)
		os.Exit(1)
	}
}

// CloseLog is retained as a no-op so main.go's `defer cmd.CloseLog()` keeps
// compiling. The driver owns the file now: lumberjack writes each record and
// rotates on size, so there is no handle for the process to close, and
// nothing is buffered awaiting a flush.
func CloseLog() {}
