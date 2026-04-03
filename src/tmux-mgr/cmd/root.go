package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/eraigosa/dotfiles/src/tmux-mgr/pkg/tmux"
	"github.com/spf13/cobra"
)

var (
	// LogFile handles the logging stream.
	LogFile *os.File
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
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".config", "tmux-mgr")
	os.MkdirAll(logDir, 0755)

	f, err := os.OpenFile(filepath.Join(logDir, "tmux-mgr.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error opening log file: %v\n", err)
	} else {
		log.SetOutput(f)
		LogFile = f
	}

	rootCmd.PersistentFlags().BoolVarP(&Tmgr.Verbose, "verbose", "v", false, "Enable verbose output")
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Printf("Execution error: %v", err)
		os.Exit(1)
	}
}

// CloseLog closes the log file.
func CloseLog() {
	if LogFile != nil {
		LogFile.Close()
	}
}
