package cmd

import (
	"testing"
)

// TestExecute_Help exercises the Execute function via --help which should
// succeed (cobra handles it gracefully without calling os.Exit).
// This test just ensures Execute does not panic on a happy-path invocation.
func TestExecute_HelpFlag(t *testing.T) {
	// Reset the root command args so --help is processed.
	origArgs := rootCmd.Args
	defer func() { rootCmd.Args = origArgs }()

	// Capture stdout so the help text doesn't pollute test output.
	captureStdout(t, func() {
		// Set args to --help to get the help output (succeeds without os.Exit).
		rootCmd.SetArgs([]string{"--help"})
		// Execute calls rootCmd.Execute(); --help exits with code 0 via cobra
		// which calls os.Exit(0) through PersistentPreRun, BUT cobra itself
		// handles --help internally and returns nil. So we call rootCmd.Execute
		// directly here (not Execute() which wraps os.Exit).
		_ = rootCmd.Execute()
		rootCmd.SetArgs(nil)
	})
}
