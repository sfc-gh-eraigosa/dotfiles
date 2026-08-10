package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/keys"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/spf13/cobra"
)

const authorizedKeys = "~/.ssh/authorized_keys"

// syncKeyToHost authorizes ONE PUBLIC key on a host.
//
// It never transfers private key material: the shell script this replaces
// scp'd the private key to every host, so one compromised machine yielded an
// identity valid everywhere. The remote append is grep-guarded so re-syncing
// is a no-op rather than growing the file, and any failure is returned with
// the host named rather than swallowed.
func syncKeyToHost(r runner.Runner, host, pub string) error {
	pub = strings.Join(strings.Fields(pub), " ")
	if pub == "" {
		return fmt.Errorf("%s: refusing to sync an empty key", host)
	}
	cmdline := fmt.Sprintf(
		"mkdir -p ~/.ssh && chmod 700 ~/.ssh && touch %[1]s && chmod 600 %[1]s && "+
			"grep -qxF %[2]q %[1]s || printf '%%s\\n' %[2]q >> %[1]s",
		authorizedKeys, pub)
	if _, err := r.Run(host, cmdline); err != nil {
		return fmt.Errorf("%s: authorizing key: %w", host, err)
	}
	return nil
}

// remoteKeys reads a host's authorized_keys as a line slice.
func remoteKeys(r runner.Runner, host string) ([]string, error) {
	out, err := r.Run(host, "cat "+authorizedKeys+" 2>/dev/null || true")
	if err != nil {
		return nil, fmt.Errorf("%s: reading authorized_keys: %w", host, err)
	}
	return strings.Split(out, "\n"), nil
}

// managedKeys returns the public keys fleet manages: every ~/.ssh/*.pub.
func managedKeys() (map[string]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(home, ".ssh", "*.pub"))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		line := strings.SplitN(strings.TrimSpace(string(b)), "\n", 2)[0]
		if strings.TrimSpace(line) == "" {
			continue
		}
		out[strings.TrimSuffix(filepath.Base(p), ".pub")] = strings.Join(strings.Fields(line), " ")
	}
	return out, nil
}

func fleetHosts() ([]sshconf.Host, error) {
	raw, err := os.ReadFile(flagConfig)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", flagConfig, err)
	}
	all, err := sshconf.Parse(string(raw), flagMarker)
	if err != nil {
		return nil, err
	}
	return selectHosts(all, nil), nil
}

var keysCmd = &cobra.Command{Use: "keys", Short: "Manage fleet SSH keys"}

var keysListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show which fleet hosts authorize each managed key",
	RunE: func(cmd *cobra.Command, _ []string) error {
		mine, err := managedKeys()
		if err != nil {
			return err
		}
		hosts, err := fleetHosts()
		if err != nil {
			return err
		}
		r := runner.Exec{}
		remote := map[string][]string{}
		for _, h := range hosts {
			rk, err := remoteKeys(r, h.Alias)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", err)
				continue
			}
			remote[h.Alias] = rk
		}
		names := make([]string, 0, len(mine))
		for n := range mine {
			names = append(names, n)
		}
		sort.Strings(names)
		out := cmd.OutOrStdout()
		for _, n := range names {
			var on []string
			for _, h := range hosts {
				for _, rk := range remote[h.Alias] {
					if strings.Join(strings.Fields(rk), " ") == mine[n] {
						on = append(on, h.Alias)
						break
					}
				}
			}
			if len(on) == 0 {
				on = []string{"(none)"}
			}
			fmt.Fprintf(out, "%-24s %s\n", n+":", strings.Join(on, ", "))
		}
		return nil
	},
}

var keysSyncCmd = &cobra.Command{
	Use:   "sync [key-name...]",
	Short: "Authorize managed PUBLIC keys on every fleet host",
	RunE: func(cmd *cobra.Command, args []string) error {
		mine, err := managedKeys()
		if err != nil {
			return err
		}
		if len(args) > 0 {
			sel := map[string]string{}
			for _, a := range args {
				pub, ok := mine[a]
				if !ok {
					return fmt.Errorf("no managed public key named %q (looked for ~/.ssh/%s.pub)", a, a)
				}
				sel[a] = pub
			}
			mine = sel
		}
		hosts, err := fleetHosts()
		if err != nil {
			return err
		}
		r := runner.Exec{}
		var failures int
		for _, h := range hosts {
			for name, pub := range mine {
				if err := syncKeyToHost(r, h.Alias, pub); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "FAIL %s <- %s: %v\n", h.Alias, name, err)
					failures++
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "ok   %s <- %s\n", h.Alias, name)
			}
		}
		if failures > 0 {
			return fmt.Errorf("%d host/key sync(s) failed", failures)
		}
		return nil
	},
}

func init() {
	keysCmd.AddCommand(keysListCmd, keysSyncCmd)
	rootCmd.AddCommand(keysCmd)
}

// keysDiffFor is used by prune/delete (Task 11).
func keysDiffFor(r runner.Runner, host string, local []string) (keys.Diff, error) {
	rk, err := remoteKeys(r, host)
	if err != nil {
		return keys.Diff{}, err
	}
	return keys.Compute(local, rk), nil
}
