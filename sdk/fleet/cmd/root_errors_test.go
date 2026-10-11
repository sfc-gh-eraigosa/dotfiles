package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func executeProbe(t *testing.T, args ...string) string {
	t.Helper()
	probe := &cobra.Command{
		Use:  "probe-errors <one>",
		Args: cobra.ExactArgs(1),
		RunE: func(*cobra.Command, []string) error { return errors.New("1 target(s) failed") },
	}
	rootCmd.AddCommand(probe)
	t.Cleanup(func() { rootCmd.RemoveCommand(probe) })
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs(append([]string{"probe-errors"}, args...))
	t.Cleanup(func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil); rootCmd.SetArgs(nil) })
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("want an error")
	}
	return buf.String()
}

// A failed RUN is not a usage mistake: dumping the flag table buried the one
// line that said what failed, and cobra plus Execute() each printed "Error:".
// Execute() is the single printer.
func TestARuntimeFailurePrintsNoUsageAndNoCobraError(t *testing.T) {
	got := executeProbe(t, "x")
	if strings.Contains(got, "Usage:") || strings.Contains(got, "Error:") {
		t.Fatalf("cobra printed for a runtime failure:\n%s", got)
	}
}

// A real usage mistake still shows usage.
func TestAWrongArgCountStillShowsUsage(t *testing.T) {
	if got := executeProbe(t); !strings.Contains(got, "Usage:") {
		t.Fatalf("usage missing for a bad invocation:\n%s", got)
	}
}
