package cmd

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/spf13/cobra"
)

// sshConfigMode — ssh refuses to use a config group/world-writable.
const sshConfigMode = 0o600

// writeConfig replaces the ssh config, taking a timestamped backup of the
// previous content first. A bad write here costs SSH access to every machine,
// so the backup is not optional and happens before anything is truncated.
func writeConfig(path, content string) error {
	if old, err := os.ReadFile(path); err == nil {
		bak := fmt.Sprintf("%s.bak-%s", path, time.Now().UTC().Format("20060102T150405Z"))
		if err := os.WriteFile(bak, old, sshConfigMode); err != nil {
			return fmt.Errorf("backing up %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(content), sshConfigMode)
}

// applyConfig writes, or with dryRun prints what would be written.
func applyConfig(out io.Writer, path, content string, dryRun bool) error {
	if dryRun {
		fmt.Fprintf(out, "--- would write %s ---\n%s", path, content)
		return nil
	}
	return writeConfig(path, content)
}

func readConfig(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return string(b), nil
}

var (
	addDryRun bool
	addHost   sshconf.Host
)

var addCmd = &cobra.Command{
	Use:   "add <alias>",
	Short: "Add a host to the fleet",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cur, err := readConfig(flagConfig)
		if err != nil {
			return err
		}
		addHost.Alias = args[0]
		next, err := sshconf.Add(cur, addHost, flagMarker)
		if err != nil {
			return err
		}
		if err := applyConfig(cmd.OutOrStdout(), flagConfig, next, addDryRun); err != nil {
			return err
		}
		if !addDryRun {
			fmt.Fprintf(cmd.OutOrStdout(), "added %s to the fleet\n", addHost.Alias)
		}
		return nil
	},
}

var (
	rmPurge  bool
	rmDryRun bool
)

var removeCmd = &cobra.Command{
	Use:   "remove <alias>",
	Short: "Remove a host from the fleet (unmarks by default; --purge deletes the block)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cur, err := readConfig(flagConfig)
		if err != nil {
			return err
		}
		var next string
		if rmPurge {
			next, err = sshconf.Purge(cur, args[0])
		} else {
			next, err = sshconf.Unmark(cur, args[0], flagMarker)
		}
		if err != nil {
			return err
		}
		if err := applyConfig(cmd.OutOrStdout(), flagConfig, next, rmDryRun); err != nil {
			return err
		}
		if !rmDryRun {
			what := "removed from the fleet (ssh access kept)"
			if rmPurge {
				what = "purged from the ssh config entirely"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", args[0], what)
		}
		return nil
	},
}

func init() {
	addCmd.Flags().StringVar(&addHost.HostName, "hostname", "", "hostname or IP (required)")
	addCmd.Flags().StringVar(&addHost.User, "user", "", "ssh user")
	addCmd.Flags().StringVar(&addHost.Port, "port", "", "ssh port")
	addCmd.Flags().StringVar(&addHost.Identity, "identity", "", "identity file")
	addCmd.Flags().BoolVar(&addDryRun, "dry-run", false, "print the result without writing")
	_ = addCmd.MarkFlagRequired("hostname")

	removeCmd.Flags().BoolVar(&rmPurge, "purge", false, "delete the Host block entirely")
	removeCmd.Flags().BoolVar(&rmDryRun, "dry-run", false, "print the result without writing")

	rootCmd.AddCommand(addCmd, removeCmd)
}
