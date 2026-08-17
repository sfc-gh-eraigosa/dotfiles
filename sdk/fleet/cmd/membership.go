package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
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

// addAction records what resolveAdd did, so the command can print the right
// message and skip a pointless write when nothing changed.
type addAction int

const (
	actionCreated addAction = iota // a brand-new Host block written from flags
	actionAdopted                  // an existing ssh-config block marked in place
	actionAlready                  // already in the fleet — no change
)

// resolveAdd decides how <alias> joins the fleet:
//   - already a concrete Host block and marked -> nothing to do.
//   - already a concrete Host block, unmarked  -> adopt it (Mark in place),
//     ignoring any connection flags so the operator need not re-type details
//     ssh already knows.
//   - absent -> create a new block; that genuinely needs a --hostname.
//
// It is a pure function of the config text so the branch logic is unit-tested.
func resolveAdd(cfg string, h sshconf.Host, marker string) (string, addAction, error) {
	hosts, err := sshconf.Parse(cfg, marker)
	if err != nil {
		return "", 0, err
	}
	for _, e := range hosts {
		if e.Alias != h.Alias {
			continue
		}
		if e.Fleet {
			return cfg, actionAlready, nil
		}
		next, err := sshconf.Mark(cfg, h.Alias, marker)
		return next, actionAdopted, err
	}
	if strings.TrimSpace(h.HostName) == "" {
		return "", 0, fmt.Errorf(
			"%q is not in the ssh config; pass --hostname to add a new host, or run `fleet discover` to adopt an existing one",
			h.Alias)
	}
	next, err := sshconf.Add(cfg, h, marker)
	return next, actionCreated, err
}

var addCmd = &cobra.Command{
	Use:   "add <alias>",
	Short: "Add a host to the fleet (adopts an existing ssh-config entry, or creates one with --hostname)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cur, err := readConfig(flagConfig)
		if err != nil {
			return err
		}
		addHost.Alias = args[0]
		next, act, err := resolveAdd(cur, addHost, flagMarker)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if act == actionAlready {
			fmt.Fprintf(out, "%s is already in the fleet\n", addHost.Alias)
			return nil
		}
		if err := applyConfig(out, flagConfig, next, addDryRun); err != nil {
			return err
		}
		if !addDryRun {
			switch act {
			case actionAdopted:
				fmt.Fprintf(out, "adopted %s into the fleet (marked its existing ssh config entry)\n", addHost.Alias)
			default:
				fmt.Fprintf(out, "added %s to the fleet\n", addHost.Alias)
			}
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
	addCmd.Flags().StringVar(&addHost.HostName, "hostname", "", "hostname or IP (required only when creating a new host)")
	addCmd.Flags().StringVar(&addHost.User, "user", "", "ssh user")
	addCmd.Flags().StringVar(&addHost.Port, "port", "", "ssh port")
	addCmd.Flags().StringVar(&addHost.Identity, "identity", "", "identity file")
	addCmd.Flags().BoolVar(&addDryRun, "dry-run", false, "print the result without writing")

	removeCmd.Flags().BoolVar(&rmPurge, "purge", false, "delete the Host block entirely")
	removeCmd.Flags().BoolVar(&rmDryRun, "dry-run", false, "print the result without writing")

	rootCmd.AddCommand(addCmd, removeCmd)
}
