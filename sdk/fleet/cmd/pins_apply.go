package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/pins"
)

// pinsTrackKey is the gff choice flag (manifest|latest|pin) that says whether
// installs follow the manifest, upstream latest, or exact pins.
const pinsTrackKey = "pins.track"

// pinsTrackMode reads pins.track from the repo's gff, failing open to
// "manifest": an unreadable flag must never change what a host converges to.
func pinsTrackMode(repo string) string {
	return pinsTrackModeFrom(&featflag.GFF{Repo: repo})
}

func pinsTrackModeFrom(src featflag.Source) string {
	if src == nil {
		return "manifest"
	}
	v, err := src.Strings(pinsTrackKey)
	if err != nil || len(v) == 0 {
		return "manifest"
	}
	switch v[0] {
	case "manifest", "latest", "pin":
		return v[0]
	}
	return "manifest"
}

// runInstallerScript runs `bash <repo>/<installer>` with the terminal attached,
// so sudo prompts and progress reach the operator.
func runInstallerScript(ctx context.Context, repo, installer string, out io.Writer) error {
	c := exec.CommandContext(ctx, "bash", filepath.Join(repo, filepath.FromSlash(installer)))
	c.Dir = repo
	c.Stdin = os.Stdin
	c.Stdout, c.Stderr = out, out
	return c.Run()
}

// runPinsInstalled prints what is installed here against what the manifest
// wants: a table, or the frozen JSON shape `fleet status` parses.
func runPinsInstalled(ctx context.Context, m *pins.Manifest, d *pinsDeps, asJSON bool, w io.Writer) error {
	rows, sum := pins.CheckInstalled(ctx, m, d.exec, d.trackMode(m.Root))
	if asJSON {
		return writeJSON(w, struct {
			Rows    []pins.InstalledRow `json:"rows"`
			Summary pins.Summary        `json:"summary"`
		}{rows, sum})
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TOOL\tTRACK\tWANT\tINSTALLED\tSTATE")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Tool, r.Track, r.Want, dash(r.Installed), r.State)
	}
	_ = tw.Flush()
	fmt.Fprintf(w, "ok %d  drift %d  missing %d  unknown %d\n", sum.OK, sum.Drift, sum.Missing, sum.Unknown)
	return nil
}

// pinsApplyOpts drives runPinsApply.
type pinsApplyOpts struct {
	M      *pins.Manifest
	D      *pinsDeps
	Tools  []string
	DryRun bool
	Out    io.Writer
}

// installerGroup is one installer and the tools it will converge.
type installerGroup struct {
	Path  string
	Tools []string
}

// runPinsApply converges drifted tools by running each distinct installer once.
// A tool that is merely missing is installed only when named (it may be absent
// on purpose: its gff flag is off). Rows with no installer are listed, not run.
// Every installer is attempted; the error reports which failed.
func runPinsApply(ctx context.Context, o pinsApplyOpts) error {
	rows, _ := pins.CheckInstalled(ctx, o.M, o.D.exec, o.D.trackMode(o.M.Root))
	byTool := map[string]pins.InstalledRow{}
	for _, r := range rows {
		byTool[r.Tool] = r
	}
	for _, t := range o.Tools {
		if _, ok := byTool[t]; !ok {
			return fmt.Errorf("apply: unknown tool %q", t)
		}
	}

	named := map[string]bool{}
	for _, t := range o.Tools {
		named[t] = true
	}
	var groups []installerGroup
	var reportOnly []string
	for _, r := range rows { // manifest order => deterministic groups
		selected := r.State == "drift" || (named[r.Tool] && r.State == "missing")
		if len(named) > 0 && !named[r.Tool] {
			continue
		}
		if !selected {
			if named[r.Tool] {
				fmt.Fprintf(o.Out, "%s: %s — nothing to apply\n", r.Tool, r.State)
			}
			continue
		}
		e, _ := o.M.Get(r.Tool)
		if e.Installer == "-" || e.Installer == "" {
			reportOnly = append(reportOnly, r.Tool)
			continue
		}
		if err := checkInstallerPath(e.Installer); err != nil {
			return fmt.Errorf("apply: %s: %w", r.Tool, err)
		}
		placed := false
		for i := range groups {
			if groups[i].Path == e.Installer {
				groups[i].Tools = append(groups[i].Tools, r.Tool)
				placed = true
				break
			}
		}
		if !placed {
			groups = append(groups, installerGroup{Path: e.Installer, Tools: []string{r.Tool}})
		}
	}

	for _, t := range reportOnly {
		fmt.Fprintf(o.Out, "%s: no installer of its own — run install.sh\n", t)
	}
	if len(groups) == 0 {
		if len(reportOnly) == 0 && len(named) == 0 {
			fmt.Fprintln(o.Out, "pins: nothing to apply")
		}
		return nil
	}

	var failed []string
	for _, g := range groups {
		tools := strings.Join(g.Tools, ", ")
		if o.DryRun {
			fmt.Fprintf(o.Out, "would run bash %s (%s)\n", filepath.Join(o.M.Root, g.Path), tools)
			continue
		}
		fmt.Fprintf(o.Out, "==> %s (%s)\n", g.Path, tools)
		if err := o.D.runInstaller(ctx, o.M.Root, g.Path, o.Out); err != nil {
			fmt.Fprintf(o.Out, "FAILED %s: %v\n", g.Path, err)
			failed = append(failed, g.Path)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("apply: %d installer(s) failed: %s", len(failed), strings.Join(failed, ", "))
	}
	return nil
}

// checkInstallerPath keeps an installer inside the repo: relative, no "..".
func checkInstallerPath(p string) error {
	if filepath.IsAbs(p) || p != filepath.ToSlash(filepath.Clean(p)) || strings.HasPrefix(p, "../") || p == ".." {
		return errors.New("installer path must be repo-relative: " + p)
	}
	return nil
}

func newPinsApplyCmd(d *pinsDeps) *cobra.Command {
	var dry bool
	c := &cobra.Command{
		Use:   "apply [TOOL...]",
		Short: "Run the installers for drifted tools (name a tool to also install a missing one)",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadPinsManifest()
			if err != nil {
				return err
			}
			return runPinsApply(cmd.Context(), pinsApplyOpts{M: m, D: d, Tools: args, DryRun: dry, Out: cmd.OutOrStdout()})
		},
	}
	c.Flags().BoolVar(&dry, "dry-run", false, "print which installers would run; run nothing")
	return c
}
