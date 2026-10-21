package cmd

import (
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
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

func TestRenderFlags(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	defer func() { colorFlag, glyphsFlag = "auto", "auto" }()
	colorFlag, glyphsFlag = "auto", "auto"
	if m, ascii, err := renderFlags(env(map[string]string{"NO_COLOR": "1"})); err != nil || m != style.ModeNever || ascii {
		t.Errorf("NO_COLOR: %v %v %v", m, ascii, err)
	}
	colorFlag, glyphsFlag = "truecolor", "ascii"
	if m, ascii, err := renderFlags(env(map[string]string{"NO_COLOR": "1"})); err != nil || m != style.ModeTruecolor || !ascii {
		t.Errorf("flags: %v %v %v", m, ascii, err)
	}
	colorFlag = "rainbow"
	if _, _, err := renderFlags(env(nil)); err == nil {
		t.Error("bad --color accepted")
	}
	colorFlag, glyphsFlag = "auto", "fancy"
	if _, _, err := renderFlags(env(nil)); err == nil {
		t.Error("bad --glyphs accepted")
	}
}
