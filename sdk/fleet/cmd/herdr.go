package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/herdrmach"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/spf13/cobra"
)

// The result classes of `fleet herdr sync` (spec F5). wouldAdd appears only
// under --dry-run and never fails the run.
const (
	herdrAlready       = "already"
	herdrSaved         = "saved"
	herdrNeedsApproval = "needs-approval"
	herdrFailed        = "failed"
	herdrWouldAdd      = "would-add"
)

// herdrSyncResult is one host's outcome; it is also the --json row.
type herdrSyncResult struct {
	Host   string `json:"host"`
	Result string `json:"result"`
	Detail string `json:"detail"`
}

// herdrSyncDeps is every impure edge of the sync, injected so tests need no
// herdr, no ssh config and no terminal.
type herdrSyncDeps struct {
	allHosts func() ([]sshconf.Host, error)    // every concrete ssh-config host
	local    func() localHost                  // the machine fleet is running on
	herdrBin func() string                     // the local herdr client; "" = not installed here
	list     func(bin string) herdrmach.Runner // runs `herdr machine list --json` through bin
	tty      func() bool                       // someone is there to answer herdr's prompts
	add      func(argv []string) error         // runs `herdr machine add` attached to the terminal
}

// herdrListTimeout bounds `herdr machine list`; the tui runs it on a keystroke.
const herdrListTimeout = 10 * time.Second

// herdrExec runs herdrmach argv through the resolved local binary.
type herdrExec struct{ bin string }

func (h herdrExec) Output(ctx context.Context, argv []string) ([]byte, error) {
	out, err := runLocal(ctx, h.bin, argv[1:]...)
	return []byte(out), err
}

// listSavedMachines is the one bounded `herdr machine list` both the CLI and
// the tui `S` key use.
func listSavedMachines(bin string) ([]herdrmach.Machine, error) {
	ctx, cancel := context.WithTimeout(context.Background(), herdrListTimeout)
	defer cancel()
	return herdrmach.List(ctx, herdrExec{bin: bin})
}

func defaultHerdrSyncDeps() herdrSyncDeps {
	return herdrSyncDeps{
		allHosts: func() ([]sshconf.Host, error) {
			raw, err := readConfig(flagConfig)
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", flagConfig, err)
			}
			return sshconf.Parse(raw, flagMarker)
		},
		local:    detectLocal,
		herdrBin: func() string { return localHerdr(exec.LookPath, fileExists, os.Getenv("HOME")) },
		list:     func(bin string) herdrmach.Runner { return herdrExec{bin: bin} },
		tty:      stdinIsTerminal,
		add: func(argv []string) error {
			// herdr may prompt to prepare the remote install, so it gets the
			// terminal. Its stdout goes to stderr to keep ours (and --json)
			// clean.
			c := exec.Command(argv[0], argv[1:]...)
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stderr, os.Stderr
			return c.Run()
		},
	}
}

// herdrSyncTargets picks the hosts to sync. With no names it is every
// in-fleet host except this machine (herdr cannot save itself). Names are
// honoured even outside the fleet — naming a host is consent — but a name
// ssh config does not know, or one that is this machine, is reported failed
// rather than silently dropped.
func herdrSyncTargets(all []sshconf.Host, me localHost, names []string) (targets []string, failed []herdrSyncResult) {
	if len(names) == 0 {
		for _, h := range all {
			if h.Fleet && !isLocalHost(h, me) {
				targets = append(targets, h.Alias)
			}
		}
		return targets, nil
	}
	byAlias := make(map[string]sshconf.Host, len(all))
	for _, h := range all {
		byAlias[h.Alias] = h
	}
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if seen[n] {
			continue // one result per host; a repeat would add it twice
		}
		seen[n] = true
		h, ok := byAlias[n]
		switch {
		case !ok:
			failed = append(failed, herdrSyncResult{n, herdrFailed, "not in ssh config"})
		case isLocalHost(h, me):
			failed = append(failed, herdrSyncResult{n, herdrFailed, "is this machine; herdr cannot save itself"})
		default:
			targets = append(targets, n)
		}
	}
	return targets, failed
}

// runHerdrSync saves each target as a herdr machine and reports one result
// per host. It is additive only: it never removes or renames a machine.
func runHerdrSync(ctx context.Context, d herdrSyncDeps, names []string, dryRun, asJSON bool, out io.Writer) error {
	bin := d.herdrBin()
	if bin == "" {
		return errors.New("herdr is not installed here (looked on PATH, ~/.local/bin, ~/opt/bin)")
	}
	all, err := d.allHosts()
	if err != nil {
		return err
	}
	targets, results := herdrSyncTargets(all, d.local(), names)

	// One list for the whole run, and only when there is a host to check.
	// If it fails we cannot tell what is saved, and a duplicate add would
	// prompt, so nothing is attempted.
	var saved []herdrmach.Machine
	if len(targets) > 0 {
		lctx, cancel := context.WithTimeout(ctx, herdrListTimeout)
		ms, err := herdrmach.List(lctx, d.list(bin))
		cancel()
		if err != nil {
			return fmt.Errorf("cannot read herdr machines: %w", err)
		}
		saved = ms
	}

	tty := d.tty()
	for _, alias := range targets {
		argv := herdrmach.AddArgv(alias)
		cmdline := strings.Join(argv, " ")
		r := herdrSyncResult{Host: alias}
		switch m, ok := herdrmach.Saved(saved, alias); {
		case ok:
			name := m.Label
			if name == "" {
				name = m.Target
			}
			r.Result, r.Detail = herdrAlready, "saved as "+name
		case dryRun:
			r.Result, r.Detail = herdrWouldAdd, cmdline
		case !tty:
			// herdr may need approval to prepare the remote; with nobody to
			// answer, say exactly what to run instead of hanging.
			r.Result, r.Detail = herdrNeedsApproval, "run: "+cmdline
		default:
			argv[0] = bin
			if err := d.add(argv); err != nil {
				r.Result, r.Detail = herdrFailed, fmt.Sprintf("%s: %v", cmdline, err)
			} else {
				r.Result, r.Detail = herdrSaved, "prefix+w in herdr to switch to it"
			}
		}
		results = append(results, r)
	}

	if err := writeHerdrSync(out, results, asJSON); err != nil {
		return err
	}
	for _, r := range results {
		if r.Result == herdrNeedsApproval || r.Result == herdrFailed {
			return exitError{code: 1}
		}
	}
	return nil
}

func writeHerdrSync(out io.Writer, results []herdrSyncResult, asJSON bool) error {
	if asJSON {
		if results == nil {
			results = []herdrSyncResult{}
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}
	if len(results) == 0 {
		_, err := fmt.Fprintln(out, "no fleet hosts to sync")
		return err
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, r := range results {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Host, r.Result, r.Detail)
	}
	return tw.Flush()
}

var herdrSyncDryRun bool

var herdrCmd = &cobra.Command{Use: "herdr", Short: "Work with herdr on fleet hosts"}

var herdrSyncCmd = &cobra.Command{
	Use:   "sync [host...]",
	Short: "Save fleet hosts as herdr machines so they show in the local herdr sidebar",
	Long: `Save every in-fleet host (or the named ones) as a herdr saved machine, so it
appears in the herdr you already have open; switch to it with prefix+w.

Each host is reported as already, saved, needs-approval or failed. herdr may
prompt before preparing a remote, so without a terminal nothing is added and
the command to run is printed instead. Additive only: machines are never
removed. Exits non-zero when any host is needs-approval or failed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHerdrSync(cmd.Context(), defaultHerdrSyncDeps(), args, herdrSyncDryRun, flagJSON, cmd.OutOrStdout())
	},
}

func init() {
	herdrSyncCmd.Flags().BoolVar(&herdrSyncDryRun, "dry-run", false, "print what would be added; change nothing")
	herdrCmd.AddCommand(herdrSyncCmd)
	rootCmd.AddCommand(herdrCmd)
}
