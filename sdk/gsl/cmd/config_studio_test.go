package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/tui"
)

// E23: bare `gsl config` opens the studio (only on a terminal); subcommands
// keep their CLI behaviour and never open it.
func TestConfigCmd_Routing(t *testing.T) {
	oldTTY, oldRun := studioTTY, runStudio
	t.Cleanup(func() { studioTTY, runStudio = oldTTY, oldRun; rootCmd.SetArgs(nil) })
	opened := 0
	var got tui.Model
	runStudio = func(m tui.Model) error { opened++; got = m; return nil }

	withTempConfig(t, config.Default(), func() {
		studioTTY = func() bool { return false }
		rootCmd.SetArgs([]string{"config"})
		if err := rootCmd.Execute(); !errors.Is(err, errNoTerminal) || opened != 0 {
			t.Fatalf("no terminal: err=%v opened=%d", err, opened)
		}
		studioTTY = func() bool { return true }
		rootCmd.SetArgs([]string{"config"})
		if err := rootCmd.Execute(); err != nil || opened != 1 {
			t.Fatalf("bare config: err=%v opened=%d", err, opened)
		}
		if !strings.Contains(got.View(), "gsl config") {
			t.Errorf("studio view: %q", got.View())
		}
		for _, args := range [][]string{{"config", "get", "style"}, {"config", "set", "links", "plain"}, {"config", "validate"}} {
			rootCmd.SetArgs(args)
			_ = captureStdout(t, func() {
				if err := rootCmd.Execute(); err != nil {
					t.Errorf("%v: %v", args, err)
				}
			})
		}
		if opened != 1 {
			t.Errorf("a subcommand opened the studio (%d)", opened)
		}
		rootCmd.SetArgs([]string{"config", "bogus"})
		if err := rootCmd.Execute(); err == nil {
			t.Error("an unknown subcommand must fail, not open the studio")
		}
	})
}
