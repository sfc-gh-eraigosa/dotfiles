package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
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

		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip errors
			}
			if info.IsDir() && info.Name() == ".git" {
				repoDir := filepath.Dir(path)
				if isDirty(repoDir) {
					fmt.Printf("[DIRTY] %s\n", repoDir)
				}
				return filepath.SkipDir // Don't look inside .git
			}
			return nil
		})

		if err != nil {
			fmt.Printf("Error scanning: %v\n", err)
		}
	},
}

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
