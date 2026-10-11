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
	"sync"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/pins"
)

const (
	// pinsWorkers bounds concurrent upstream lookups: enough to make a
	// 14-row manifest feel instant, few enough not to trip GitHub's abuse
	// limits.
	pinsWorkers = 6
	// pinsLookupTimeout bounds one upstream lookup.
	pinsLookupTimeout = 15 * time.Second
)

// pinsDeps is everything `fleet pins` needs from outside the process, so tests
// run without a network, a gh binary or a shell.
type pinsDeps struct {
	resolver   func(token string) pins.Resolver
	credential func(ctx context.Context) string
	// Installed-mode seams (see pins_apply.go).
	exec         pins.Exec
	trackMode    func(repo string) string
	runInstaller func(ctx context.Context, repo, installer, mode string, out io.Writer) error
}

func defaultPinsDeps() *pinsDeps {
	return &pinsDeps{
		resolver: func(token string) pins.Resolver { return pins.NewHTTPResolver(nil, token) },
		credential: func(ctx context.Context) string {
			return pinsToken(ctx, os.Getenv, exec.LookPath, runLocal)
		},
		exec:         pins.ShellExec,
		trackMode:    pinsTrackMode,
		runInstaller: runInstallerScript,
	}
}

// runLocal runs a local command and returns its stdout.
func runLocal(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// pinsToken is the GitHub credential for upstream lookups: $GITHUB_TOKEN, else
// `gh auth token` when gh is installed. "" means unauthenticated (60 req/h),
// which is fine for a manifest of this size.
func pinsToken(ctx context.Context, getenv func(string) string, lookPath func(string) (string, error),
	run func(context.Context, string, ...string) (string, error)) string {
	if v := strings.TrimSpace(getenv("GITHUB_TOKEN")); v != "" {
		return v
	}
	if _, err := lookPath("gh"); err != nil {
		return ""
	}
	out, err := run(ctx, "gh", "auth", "token")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// lookup is one row's upstream answer.
type lookup struct {
	Rel pins.Release
	Err error
}

// lookupAll resolves every entry's upstream through a pool of at most workers
// goroutines. The result is index-aligned with es, so output order never
// depends on which lookup finished first.
func lookupAll(ctx context.Context, es []pins.Entry, res pins.Resolver, workers int) []lookup {
	out := make([]lookup, len(es))
	idx := make(chan int)
	var wg sync.WaitGroup
	if workers > len(es) {
		workers = len(es)
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range idx {
				lctx, cancel := context.WithTimeout(ctx, pinsLookupTimeout)
				rel, err := res.Latest(lctx, es[i].Upstream)
				cancel()
				out[i] = lookup{Rel: rel, Err: err}
			}
		}()
	}
	for i := range es {
		idx <- i
	}
	close(idx)
	wg.Wait()
	return out
}

// pinsStatusRow is one line of `fleet pins status` (and its --json form).
type pinsStatusRow struct {
	Tool     string `json:"tool"`
	Track    string `json:"track"`
	Pin      string `json:"pin"`
	Latest   string `json:"latest"`
	Gap      string `json:"gap"`
	Released string `json:"released"`
	Error    string `json:"error,omitempty"`
}

// pinsStatusRows compares each manifest row with its upstream. A failed lookup
// marks only that row (Gap "error"); it never fails the command.
func pinsStatusRows(ctx context.Context, m *pins.Manifest, res pins.Resolver, workers int) []pinsStatusRow {
	look := lookupAll(ctx, m.Entries, res, workers)
	rows := make([]pinsStatusRow, len(m.Entries))
	for i, e := range m.Entries {
		r := pinsStatusRow{Tool: e.Tool, Track: string(e.Track), Pin: e.Version}
		if e.Track == pins.TrackLatest {
			r.Pin = "latest"
		}
		l := look[i]
		switch {
		case errors.Is(l.Err, pins.ErrNoUpstream):
			r.Latest, r.Gap = "n/a", "-"
		case l.Err != nil:
			r.Gap, r.Error = "error", l.Err.Error()
		default:
			r.Latest = l.Rel.Version
			if !l.Rel.Published.IsZero() {
				r.Released = l.Rel.Published.UTC().Format("2006-01-02")
			}
			r.Gap = "-"
			if e.Track != pins.TrackLatest {
				r.Gap = pins.Gap(e.Version, l.Rel.Version)
			}
		}
		rows[i] = r
	}
	return rows
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func renderPinsStatus(w io.Writer, rows []pinsStatusRow) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TOOL\tTRACK\tPIN\tLATEST\tGAP\tRELEASED")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Tool, r.Track, r.Pin, dash(r.Latest), r.Gap, dash(r.Released))
	}
	_ = tw.Flush()
	for _, r := range rows {
		if r.Error != "" {
			fmt.Fprintf(w, "! %s: %s\n", r.Tool, r.Error)
		}
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func renderPinsStatusJSON(w io.Writer, rows []pinsStatusRow) error {
	return writeJSON(w, struct {
		Rows []pinsStatusRow `json:"rows"`
	}{rows})
}

// pinsBumpOpts drives runPinsBump.
type pinsBumpOpts struct {
	M      *pins.Manifest
	Res    pins.Resolver
	Tools  []string
	All    bool
	To     string
	DryRun bool
	Out    io.Writer
}

// runPinsBump moves pin/floor rows to a newer version and saves the manifest
// (or the @file a row points at) once, atomically. Nothing is written when any
// requested tool is refused, or with --dry-run.
func runPinsBump(ctx context.Context, o pinsBumpOpts) error {
	switch {
	case o.All && len(o.Tools) > 0:
		return errors.New("bump: name tools or pass --all, not both — pass either")
	case o.To != "" && (o.All || len(o.Tools) != 1):
		return errors.New("bump: --to needs exactly one TOOL")
	case !o.All && len(o.Tools) == 0:
		return errors.New("bump: name a tool or pass --all")
	}

	var es []pins.Entry
	if o.All {
		for _, e := range o.M.Entries {
			if e.Track != pins.TrackLatest {
				es = append(es, e)
			}
		}
	} else {
		for _, t := range o.Tools {
			e, ok := o.M.Get(t)
			switch {
			case !ok:
				return fmt.Errorf("bump: unknown tool %q", t)
			case e.Track == pins.TrackLatest:
				return fmt.Errorf("bump: %s tracks latest; nothing to bump", t)
			}
			es = append(es, e)
		}
	}

	var look []lookup
	if o.To == "" {
		look = lookupAll(ctx, es, o.Res, pinsWorkers)
	}

	changed := 0
	for i, e := range es {
		next := strings.TrimPrefix(o.To, "v")
		if o.To == "" {
			l := look[i]
			if l.Err != nil {
				if !o.All {
					return fmt.Errorf("bump: %s: %w", e.Tool, l.Err)
				}
				fmt.Fprintf(o.Out, "%s: skipped (lookup failed: %v)\n", e.Tool, l.Err)
				continue
			}
			if pins.Compare(l.Rel.Version, e.Version) <= 0 {
				if !o.All {
					fmt.Fprintf(o.Out, "%s: up to date (%s)\n", e.Tool, e.Version)
				}
				continue
			}
			next = l.Rel.Version
		}
		if err := o.M.Set(e.Tool, next); err != nil {
			return fmt.Errorf("bump: %w", err)
		}
		fmt.Fprintf(o.Out, "%s %s → %s\n", e.Tool, e.Version, next)
		changed++
	}
	if changed == 0 {
		return nil
	}
	if o.DryRun {
		fmt.Fprintln(o.Out, "(dry-run: nothing written)")
		return nil
	}
	return o.M.Save()
}

func loadPinsManifest() (*pins.Manifest, error) {
	m, err := pins.Load(flagRepo)
	if err != nil {
		return nil, fmt.Errorf("%w (is --repo %s a dotfiles checkout?)", err, flagRepo)
	}
	return m, nil
}

// newPinsCmd builds `fleet pins`. Wiring only: the work is in the run* funcs.
func newPinsCmd(d *pinsDeps) *cobra.Command {
	pc := &cobra.Command{
		Use:   "pins",
		Short: "Show and move the version pins in opt/profiles/versions.tsv",
		Args:  cobra.NoArgs,
	}

	var installed bool
	status := &cobra.Command{
		Use:   "status",
		Short: "Compare each pin with its upstream latest (--installed: with what is installed here)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := loadPinsManifest()
			if err != nil {
				return err
			}
			if installed {
				return runPinsInstalled(cmd.Context(), m, d, flagJSON, cmd.OutOrStdout())
			}
			rows := pinsStatusRows(cmd.Context(), m, d.resolver(d.credential(cmd.Context())), pinsWorkers)
			if flagJSON {
				return renderPinsStatusJSON(cmd.OutOrStdout(), rows)
			}
			renderPinsStatus(cmd.OutOrStdout(), rows)
			return nil
		},
	}

	status.Flags().BoolVar(&installed, "installed", false, "compare with what is installed on this machine instead of upstream")

	var all, dry bool
	var to string
	bump := &cobra.Command{
		Use:   "bump (TOOL... | --all)",
		Short: "Raise pin/floor rows to the newest upstream release (or --to VERSION)",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadPinsManifest()
			if err != nil {
				return err
			}
			o := pinsBumpOpts{M: m, Tools: args, All: all, To: to, DryRun: dry, Out: cmd.OutOrStdout()}
			if to == "" { // --to never touches the network
				o.Res = d.resolver(d.credential(cmd.Context()))
			}
			return runPinsBump(cmd.Context(), o)
		},
	}
	bump.Flags().BoolVar(&all, "all", false, "every pin/floor row that is behind upstream")
	bump.Flags().StringVar(&to, "to", "", "set this exact version (one TOOL; no network)")
	bump.Flags().BoolVar(&dry, "dry-run", false, "print what would change; write nothing")

	pc.AddCommand(status, bump, newPinsApplyCmd(d))
	return pc
}

func init() { rootCmd.AddCommand(newPinsCmd(defaultPinsDeps())) }
