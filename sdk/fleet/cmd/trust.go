package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/hostkey"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshfail"
	applog "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/log"
)

var trustPin string

// errNotTrusted is a refusal, not a fault: the operator declined, or the pin
// did not match. Nothing was written.
var errNotTrusted = errors.New("host key NOT trusted — nothing changed")

type trustOpts struct {
	Alias string
	Pin   string // --fingerprint: accept only if the host presents exactly this
	In    io.Reader
	Out   io.Writer
	X     hostkey.Exec
	R     runner.Runner
	Now   time.Time
	// Audit records an accepted key. It is a parameter so tests never write
	// into the operator's real fleet.log (see the injected-capture invariant).
	Audit func(fields map[string]any)
}

// runTrust shows what known_hosts holds beside what the host presents, and
// replaces the first with the second only on an explicit decision: the alias
// typed back (a reflexive `y` is exactly the habit a MITM warning must not
// meet), or a --fingerprint pin that matches. The change is backed up,
// audited, and verified with a real probe.
func runTrust(o trustOpts) error {
	t, err := hostkey.Resolve(o.X, o.Alias)
	if err != nil {
		return err
	}
	stored, err := hostkey.Known(o.X, t)
	if err != nil {
		return err
	}
	scanned, err := hostkey.Scan(o.X, t)
	if err != nil {
		return err
	}
	presented := hostkey.Keys(scanned)
	verdict := hostkey.Compare(stored, presented)

	switch verdict {
	case hostkey.Same:
		fmt.Fprintf(o.Out, "%s (%s): presented key is already trusted — nothing to do\n", o.Alias, t.HostName)
		return probeAfterTrust(o, false)
	case hostkey.Changed:
		fmt.Fprintf(o.Out, "!!!! HOST KEY CHANGED for %s (%s)\n", o.Alias, t.HostName)
		fmt.Fprintln(o.Out, "     Expected after a reinstall or when DHCP hands the address to another machine.")
		fmt.Fprintln(o.Out, "     Otherwise it is the man-in-the-middle case ssh refused to guess about.")
		fmt.Fprintf(o.Out, "  trusted now (%s):\n", strings.Join(t.KnownHosts, ", "))
		for _, k := range stored {
			fmt.Fprintf(o.Out, "    %s\n", k)
		}
	case hostkey.New:
		fmt.Fprintf(o.Out, "%s (%s): no trusted key yet (first contact)\n", o.Alias, t.HostName)
	}
	fmt.Fprintln(o.Out, "  presented by the host:")
	for _, k := range presented {
		fmt.Fprintf(o.Out, "    %s\n", k)
	}
	fmt.Fprintln(o.Out, "  verify on the host itself (console, or a path you already trust):")
	fmt.Fprintln(o.Out, "    for f in /etc/ssh/ssh_host_*_key.pub; do ssh-keygen -lf \"$f\"; done")

	how, err := decideTrust(o, presented)
	if err != nil {
		return err
	}

	// A pin vouches for exactly one key: write that one, never the host's
	// other (unchecked) keys alongside it.
	accepted := scanned
	if o.Pin != "" {
		accepted = hostkey.OnlyFingerprint(scanned, o.Pin)
	}
	entries, err := hostkey.Entries(o.X, t, accepted)
	if err != nil {
		return err
	}
	backups, err := hostkey.Replace(o.X, t, entries, o.Now.UTC().Format("20060102T150405Z"))
	if err != nil {
		return err
	}
	o.Audit(map[string]any{
		"host":     o.Alias,
		"hostname": t.HostName,
		"verdict":  string(verdict),
		"old":      joinKeys(stored),
		"new":      joinKeys(hostkey.Keys(accepted)),
		"names":    strings.Join(t.Names(), ","),
		"backup":   strings.Join(backups, ","),
		"via":      how,
	})
	fmt.Fprintf(o.Out, "ok   %s: trusted %s\n", o.Alias, joinKeys(hostkey.Keys(accepted)))
	for _, b := range backups {
		fmt.Fprintf(o.Out, "     previous file kept as %s\n", b)
	}
	return probeAfterTrust(o, true)
}

// decideTrust returns how the key was accepted, or errNotTrusted.
func decideTrust(o trustOpts, presented []hostkey.Key) (string, error) {
	if o.Pin != "" {
		if !hostkey.HasFingerprint(presented, o.Pin) {
			fmt.Fprintf(o.Out, "the host does not present %s\n", o.Pin)
			return "", errNotTrusted
		}
		return "fingerprint " + o.Pin, nil
	}
	fmt.Fprintf(o.Out, "type the alias (%s) to trust the presented key: ", o.Alias)
	sc := bufio.NewScanner(o.In)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != o.Alias {
		fmt.Fprintln(o.Out)
		return "", errNotTrusted
	}
	return "typed alias", nil
}

// probeAfterTrust proves the fix with the same BatchMode probe status uses. A
// host that still refuses is not fixed, and saying "trusted" alone would be the
// unearned success fleet exists to catch.
func probeAfterTrust(o trustOpts, changed bool) error {
	if _, err := o.R.Run(o.Alias, "true"); err != nil {
		why := sshfail.Note(err)
		if why == "" {
			why = err.Error()
		}
		if !changed {
			return fmt.Errorf("%s still refuses, and not over its host key: %s", o.Alias, why)
		}
		return fmt.Errorf("%s still refuses after the key was replaced: %s", o.Alias, why)
	}
	fmt.Fprintf(o.Out, "ok   %s answers\n", o.Alias)
	return nil
}

func joinKeys(ks []hostkey.Key) string {
	s := make([]string, len(ks))
	for i, k := range ks {
		s[i] = k.String()
	}
	return strings.Join(s, ", ")
}

var trustCmd = &cobra.Command{
	Use:   "trust <alias>",
	Short: "Re-trust a host whose SSH host key changed (or was never accepted)",
	Long: "Compare the host key known_hosts holds with the one the host presents now,\n" +
		"and replace it only after you type the alias back (or pass a matching\n" +
		"--fingerprint). known_hosts is backed up first and every replacement is\n" +
		"logged at WARN in fleet.log, so a key change never passes silently.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTrust(trustOpts{
			Alias: args[0],
			Pin:   trustPin,
			In:    cmd.InOrStdin(),
			Out:   cmd.OutOrStdout(),
			X:     hostkey.Run,
			R:     runner.Exec{},
			Now:   time.Now(),
			Audit: func(f map[string]any) {
				applog.Default().WithFields(f).Warn("host key trusted")
			},
		})
	},
}

func init() {
	trustCmd.Flags().StringVar(&trustPin, "fingerprint", "",
		"accept only if the host presents exactly this fingerprint (SHA256:...); skips the prompt")
	rootCmd.AddCommand(trustCmd)
}
