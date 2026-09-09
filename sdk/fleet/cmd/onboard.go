package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

// onboardStep is what a first run should offer, if anything.
type onboardStep int

const (
	onboardNone onboardStep = iota
	onboardOfferCreate
	onboardOfferScan
)

// onboardDecision decides what to offer someone whose fleet is empty.
//
// interactive is not optional: a script or CI run has nobody to answer a
// prompt, and blocking on stdin there would hang forever. Those callers get
// printed guidance instead, which is why this returns onboardNone rather than
// asking.
func onboardDecision(hosts int, cfgExists, interactive bool) onboardStep {
	if hosts > 0 || !interactive {
		return onboardNone
	}
	if !cfgExists {
		return onboardOfferCreate
	}
	return onboardOfferScan
}

// identityOrder is the preference among conventional key names: modern and
// smaller first. An unconventional name is still usable, it just loses a tie.
var identityOrder = []string{"id_ed25519", "id_ecdsa", "id_rsa"}

// pickIdentity chooses which existing key to use.
//
// Looking for an existing key ALWAYS comes before offering to make one:
// generating a second key when a perfectly good one is present is how a machine
// ends up with credentials nobody can account for, and how a host ends up
// authorizing one key while fleet offers another.
func pickIdentity(present []string) string {
	have := make(map[string]bool, len(present))
	for _, p := range present {
		have[p] = true
	}
	for _, want := range identityOrder {
		if have[want] {
			return want
		}
	}
	// No conventional name: take an unconventional one deterministically
	// rather than whichever the filesystem happened to list first.
	others := append([]string(nil), present...)
	sort.Strings(others)
	if len(others) > 0 {
		return others[0]
	}
	return ""
}

// privateKeysIn lists key basenames that have BOTH halves present. A lone .pub
// cannot authenticate, and a private key with no .pub is not something to hand
// to ssh-copy-id, so requiring the pair keeps the choice usable for both.
func privateKeysIn(dir string) []string {
	pubs, err := filepath.Glob(filepath.Join(dir, "*.pub"))
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range pubs {
		priv := strings.TrimSuffix(p, ".pub")
		if _, err := os.Stat(priv); err == nil {
			out = append(out, filepath.Base(priv))
		}
	}
	return out
}

func onboardCreateMessage(path string) string {
	return fmt.Sprintf("fleet keeps its inventory in %s, which does not exist yet.\n"+
		"  it would be created with mode 0600 and left otherwise empty —\n"+
		"  no hosts are added without you saying so.", path)
}

// ensureFleet runs the first-run flow when the fleet is empty. It returns true
// if the caller should re-read the config because something changed.
//
// It is deliberately quiet when there is nothing to offer: someone with hosts
// already must never be nagged.
func ensureFleet(cmd *cobra.Command, hosts int) (bool, error) {
	out := cmd.OutOrStdout()
	_, cfgErr := os.Stat(flagConfig)
	cfgExists := cfgErr == nil
	interactive := stdinIsTerminal()

	step := onboardDecision(hosts, cfgExists, interactive)
	if step == onboardNone {
		// Non-interactive with an empty fleet still deserves an explanation,
		// or the operator sees an empty table and no reason for it.
		if hosts == 0 && !interactive {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"no fleet hosts in %s — run `fleet discover --scan` on a terminal to find some\n", flagConfig)
		}
		return false, nil
	}

	fmt.Fprintln(out, "no fleet hosts configured yet.")
	if step == onboardOfferCreate {
		fmt.Fprintln(out, onboardCreateMessage(flagConfig))
		if !askYesNo(cmd, "create it?") {
			fmt.Fprintln(out, "nothing changed")
			return false, nil
		}
		if err := os.MkdirAll(filepath.Dir(flagConfig), 0o700); err != nil {
			return false, err
		}
		if err := os.WriteFile(flagConfig, nil, sshConfigMode); err != nil {
			return false, err
		}
		fmt.Fprintf(out, "created %s\n", flagConfig)
	}

	reportIdentity(cmd)

	fmt.Fprintln(out, "\nthe fleet is empty. a scan sweeps this machine's subnet for hosts\n"+
		"  listening on :22 and offers the ones it can identify. nothing is\n"+
		"  written without a second confirmation.")
	if !askYesNo(cmd, "scan the local subnet now?") {
		fmt.Fprintf(out, "you can add hosts later with `fleet add <alias>` or `fleet discover --scan`\n")
		return false, nil
	}
	if err := runScan(cmd); err != nil {
		return false, err
	}
	return true, nil
}

// reportIdentity names the key fleet will offer, or explains its absence.
// Finding an existing key always comes first; generating is only offered when
// there is genuinely nothing to use.
func reportIdentity(cmd *cobra.Command) {
	out := cmd.OutOrStdout()
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".ssh")
	found := privateKeysIn(dir)
	if key := pickIdentity(found); key != "" {
		fmt.Fprintf(out, "using the existing key ~/.ssh/%s (%d found)\n", key, len(found))
		return
	}
	fmt.Fprintln(out, "no SSH key pair found in ~/.ssh — hosts will report auth-failed until one exists.")
	if !askYesNo(cmd, "generate ~/.ssh/id_ed25519 now?") {
		fmt.Fprintln(out, "skipped; create one later with `ssh-keygen -t ed25519`")
		return
	}
	if err := generateKey(cmd, filepath.Join(dir, "id_ed25519")); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "could not generate a key: %v\n", err)
		return
	}
	fmt.Fprintln(out, "generated ~/.ssh/id_ed25519 — authorize it on a host with `A` in the TUI")
}

// isTerminal reports whether f is a real terminal that can answer a prompt.
//
// It asks the descriptor, rather than inspecting the file MODE. The common
// `Mode()&os.ModeCharDevice` idiom is wrong for this: /dev/null is itself a
// character device, so a script run with `</dev/null` was classified as
// interactive, printed a question nobody could see, and took the EOF as "no".
// Found by running it, not by review.
//
// charmbracelet/x/term is already in the module graph via bubbletea, so this
// costs no new dependency.
func isTerminal(f *os.File) bool { return term.IsTerminal(f.Fd()) }

// stdinIsTerminal reports whether someone is there to answer a prompt.
func stdinIsTerminal() bool { return isTerminal(os.Stdin) }

// generateKey creates an ed25519 pair with no passphrase, only ever after an
// explicit yes. -N "" is deliberate: a passphrase fleet cannot supply would
// make every unattended probe prompt, which is the problem multiplexing and
// key auth exist to remove.
func generateKey(cmd *cobra.Command, path string) error {
	c := exec.Command("ssh-keygen", "-t", "ed25519", "-f", path, "-N", "", "-C", "fleet")
	c.Stdout, c.Stderr = cmd.OutOrStdout(), cmd.ErrOrStderr()
	return c.Run()
}
