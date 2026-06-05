package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect and validate gss configuration",
}

// configPrintCmd dumps the effective, fully-resolved configuration
// (built-in defaults → ~/.config/gss/config.yaml → GSS_* env). On first
// run it writes a commented stub so the user has a documented file to edit.
var configPrintCmd = &cobra.Command{
	Use:   "print",
	Short: "Print the effective resolved configuration as YAML",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := config.DefaultConfigPath()
		if created, err := config.WriteStubIfMissing(path, config.SystemClock{}); err != nil {
			return err
		} else if created {
			fmt.Fprintf(os.Stderr, "wrote default config stub to %s\n", path)
		}
		cfg, err := config.Load(config.Options{Path: path})
		if err != nil {
			return err
		}
		out, err := cfg.Marshal()
		if err != nil {
			return err
		}
		fmt.Print(string(out))
		return nil
	},
}

// configCheckCmd validates the config file and confirms the configured
// tools (git, gh) resolve on PATH (design.md → "Configuration").
var configCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate the config file and confirm configured tools are present",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := config.DefaultConfigPath()
		cfg, err := config.Load(config.Options{Path: path})
		if err != nil {
			return err
		}
		var problems int
		for _, tool := range []struct{ name, bin string }{
			{"git", cfg.Tools.Git},
			{"gh", cfg.Tools.GH},
		} {
			if _, err := exec.LookPath(tool.bin); err != nil {
				fmt.Fprintf(os.Stderr, "✗ %s (%q) not found on PATH\n", tool.name, tool.bin)
				problems++
			}
		}
		if problems > 0 {
			return fmt.Errorf("config check failed: %d problem(s)", problems)
		}
		fmt.Printf("config OK: %s\n", path)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configPrintCmd)
	configCmd.AddCommand(configCheckCmd)
	rootCmd.AddCommand(configCmd)
}
