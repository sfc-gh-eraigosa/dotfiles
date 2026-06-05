package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/scan"
)

var scanCmd = &cobra.Command{
	Use:   "scan [directory]",
	Short: "Scan a directory for dirty git repositories",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}

		fmt.Printf("Scanning %s for dirty repositories...\n", root)

		// Delegate the walk to internal/scan (PR-13); inject the existing
		// isDirty check so the [DIRTY] output stays byte-identical.
		sc := &scan.Scanner{IsDirty: isDirty}
		dirty, err := sc.Scan(root)
		if err != nil {
			fmt.Printf("Error scanning: %v\n", err)
			return
		}
		fmt.Print(scan.Format(dirty))
	},
}

// isDirty reports whether the repo at path has uncommitted changes. Kept in
// the cmd package because cmd/status_test.go (TestIsDirty) exercises it; it
// is functionally equivalent to internal/scan.GitDirty and can be retired
// in a later cleanup once that test is migrated.
func isDirty(path string) bool {
	c := exec.Command("git", "-C", path, "status", "--porcelain")
	out, err := c.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

func init() {
	rootCmd.AddCommand(scanCmd)
}
