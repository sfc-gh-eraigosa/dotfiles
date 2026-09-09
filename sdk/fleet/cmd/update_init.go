package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
	"github.com/spf13/cobra"
)

// writeDefaultPlan writes updplan.DefaultYAML to path (dir 0700, file 0644),
// refusing to overwrite an existing file unless overwrite is true.
func writeDefaultPlan(path string, overwrite bool) (wrote bool, err error) {
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return false, fmt.Errorf("update init: %s already exists; pass --overwrite to replace it", path)
		} else if !os.IsNotExist(err) {
			return false, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(updplan.DefaultYAML), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// printDefaultPlan writes updplan.DefaultYAML to w, for --print.
func printDefaultPlan(w io.Writer) error {
	_, err := io.WriteString(w, updplan.DefaultYAML)
	return err
}

var (
	flagInitFile      string
	flagInitOverwrite bool
	flagInitPrint     bool
)

// updateInitCmd is a subcommand of updateCmd. It deliberately SHADOWS a host
// literally named "init" — `fleet update init` runs this subcommand, never
// an update of a host called "init". That is an acceptable trade: "init" is
// an unusual hostname, and a plan-authoring verb needs a stable, memorable
// name more than that corner case needs protecting.
var updateInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write the default fleet.yaml update plan",
	Long: "init writes the built-in default update plan (today's fetch+ff+install.sh, " +
		"byte for byte) to the resolved plan location so it can be edited into a real " +
		"multi-repo plan.\n\n" +
		"NOTE: this subcommand shadows any fleet host literally named \"init\" — " +
		"`fleet update init` always runs this, never an update of a host called \"init\".",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagInitPrint {
			return printDefaultPlan(cmd.OutOrStdout())
		}
		path := flagInitFile
		if path == "" {
			path = defaultPlanPath()
		}
		if path == "" {
			return fmt.Errorf("update init: could not resolve a config directory for the plan")
		}
		if _, err := writeDefaultPlan(path, flagInitOverwrite); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
		return nil
	},
}

func init() {
	updateInitCmd.Flags().StringVar(&flagInitFile, "file", "", "path to write (default: the resolved plan location)")
	updateInitCmd.Flags().BoolVar(&flagInitOverwrite, "overwrite", false, "replace an existing file")
	updateInitCmd.Flags().BoolVar(&flagInitPrint, "print", false, "print the default plan to stdout instead of writing")
	updateCmd.AddCommand(updateInitCmd)
}
