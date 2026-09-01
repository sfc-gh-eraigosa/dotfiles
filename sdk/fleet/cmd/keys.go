package cmd

import (
	"fmt"
	"io"
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
		// Restricting to named hosts lets a `config pull` authorize only what
		// it just added, instead of re-pushing keys to the whole fleet.
		if hosts, err = checkHosts(hosts, keysSyncHosts); err != nil {
			return err
		}
		if len(hosts) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no matching fleet hosts — nothing to authorize")
			return nil
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
		// Restricting to named hosts lets a `config pull` authorize only what
		// it just added, instead of re-pushing keys to the whole fleet.
		if hosts, err = checkHosts(hosts, keysSyncHosts); err != nil {
			return err
		}
		if len(hosts) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no matching fleet hosts — nothing to authorize")
			return nil
		}
		r := runner.Exec{}
		var failures int
		names := make([]string, 0, len(mine))
		for n := range mine {
			names = append(names, n)
		}
		sort.Strings(names) // deterministic output across runs
		for _, h := range hosts {
			for _, name := range names {
				pub := mine[name]
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

// keysSyncHosts restricts `keys sync` to named aliases.
var keysSyncHosts []string

func init() {
	keysSyncCmd.Flags().StringSliceVar(&keysSyncHosts, "host", nil,
		"authorize only these fleet hosts (repeatable); default is every fleet host")
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

// PruneResult separates "there was nothing foreign" from "the operator said
// no". Collapsing both into a bare false made the command prompt
// "remove the above key(s)?" with nothing printed above it.
type PruneResult struct {
	Found   int  // foreign keys detected
	Changed bool // removals actually applied
}

// removeKeyCmd builds the remote command that deletes ONE exact line from an
// authorized_keys file.
//
// `grep -vxF` exits 1 when it selects no lines, which is the normal case when
// removing a host's LAST key — so the status must be tolerated explicitly.
// Chaining with && (as this first did) aborted and left the key in place.
// Exit >1 is a real grep error and still fails.
func removeKeyCmd(path, key string) string {
	return fmt.Sprintf(
		`tmp=$(mktemp) && { grep -vxF %[2]q %[1]s > "$tmp"; s=$?; [ "$s" -le 1 ]; } `+
			`&& cat "$tmp" > %[1]s && rm -f "$tmp" && chmod 600 %[1]s`,
		path, key)
}

// pruneHost reports which authorized keys are foreign to the managed set and
// applies the removal ONLY when confirmed.
//
// This replaces the absorbed script's --prune, which blanket-overwrote
// authorized_keys from the workstation's *.pub and so silently deleted CI
// keys, other machines and colleagues. Removals are printed first, one per
// line, and each is a targeted delete of that exact line.
func pruneHost(out io.Writer, r runner.Runner, host string, local []string, confirmed bool) (PruneResult, error) {
	d, err := keysDiffFor(r, host, local)
	if err != nil {
		return PruneResult{}, err
	}
	res := PruneResult{Found: len(d.ToRemove)}
	if res.Found == 0 {
		return res, nil
	}
	fmt.Fprintf(out, "%s: would remove %d authorized key(s):\n", host, res.Found)
	for _, k := range d.ToRemove {
		fmt.Fprintf(out, "  - %s\n", k)
	}
	if !confirmed {
		fmt.Fprintf(out, "%s: not confirmed — nothing changed\n", host)
		return res, nil
	}
	for _, k := range d.ToRemove {
		if _, err := r.Run(host, removeKeyCmd(authorizedKeys, k)); err != nil {
			return res, fmt.Errorf("%s: removing key: %w", host, err)
		}
	}
	res.Changed = true
	fmt.Fprintf(out, "%s: removed %d key(s)\n", host, res.Found)
	return res, nil
}

var pruneYes bool

var keysPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Show authorized keys not in the managed set, and remove them on confirmation",
	RunE: func(cmd *cobra.Command, _ []string) error {
		mine, err := managedKeys()
		if err != nil {
			return err
		}
		local := make([]string, 0, len(mine))
		for _, pub := range mine {
			local = append(local, pub)
		}
		hosts, err := fleetHosts()
		if err != nil {
			return err
		}
		// Restricting to named hosts lets a `config pull` authorize only what
		// it just added, instead of re-pushing keys to the whole fleet.
		if hosts, err = checkHosts(hosts, keysSyncHosts); err != nil {
			return err
		}
		if len(hosts) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no matching fleet hosts — nothing to authorize")
			return nil
		}
		r := runner.Exec{}
		out := cmd.OutOrStdout()
		for _, h := range hosts {
			// One diff pass per host. Prompt only when something was
			// actually found, then apply in a second, confirmed pass.
			res, err := pruneHost(out, r, h.Alias, local, pruneYes)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", err)
				continue
			}
			if res.Found == 0 || res.Changed {
				continue // nothing foreign, or --yes already applied it
			}
			if !askYesNo(cmd, fmt.Sprintf("remove the above key(s) from %s?", h.Alias)) {
				fmt.Fprintf(out, "%s: skipped\n", h.Alias)
				continue
			}
			if _, err := pruneHost(out, r, h.Alias, local, true); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", err)
			}
		}
		return nil
	},
}

// askYesNo prompts on stdin; anything but an explicit yes is a no.
func askYesNo(cmd *cobra.Command, q string) bool {
	fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N] ", q)
	var ans string
	if _, err := fmt.Fscanln(cmd.InOrStdin(), &ans); err != nil {
		return false
	}
	ans = strings.ToLower(strings.TrimSpace(ans))
	return ans == "y" || ans == "yes"
}

func init() {
	keysPruneCmd.Flags().BoolVar(&pruneYes, "yes", false, "skip the confirmation prompt (non-interactive)")
	keysCmd.AddCommand(keysPruneCmd)
}
