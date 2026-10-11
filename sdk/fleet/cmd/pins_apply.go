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
const pinsTrackKey = "pins.track.mode"

// pinsTrackMode resolves the host's track mode in the same order pins_want
// does: GFF_PINS_TRACK, then GFF_PINS_TRACK_MODE (install.sh's gff export),
// then gff itself. Failing open to "manifest": an unreadable flag must never
// change what a host converges to.
func pinsTrackMode(repo string) string {
	return pinsTrackModeEnv(os.Getenv, &featflag.GFF{Repo: repo})
}

func pinsTrackModeEnv(getenv func(string) string, src featflag.Source) string {
	for _, k := range []string{"GFF_PINS_TRACK", "GFF_PINS_TRACK_MODE"} {
		switch v := getenv(k); v {
		case "manifest", "latest", "pin":
			return v
		}
	}
	return pinsTrackModeFrom(src)
}

// installerGate is install.sh's gff gate around one installer. optIn mirrors
// gff_opt_in (fail-CLOSED: runs only on an explicit true); the rest mirror
// gff_on (fail-open: runs unless explicitly false).
type installerGate struct {
	key   string
	optIn bool
}

// installerGates maps each manifest installer to the flag install.sh checks
// before running it. TestEveryRepoInstallerHasAGate keeps it complete: apply
// must never run an installer install.sh would have skipped (a gitleaks run
// would even delete the --off marker that keeps the privacy guard off it).
var installerGates = map[string]installerGate{
	"opt/scripts/system/install_uv.sh":        {key: "install.tools.uv"},
	"opt/scripts/system/install_glow.sh":      {key: "install.tools.glow", optIn: true},
	"opt/scripts/system/install_rust.sh":      {key: "install.runtime.rust"},
	"opt/scripts/system/install_sops.sh":      {key: "install.tools.sops"},
	"opt/scripts/system/install_yq.sh":        {key: "install.tools.yq"},
	"opt/scripts/git/install_gitleaks.sh":     {key: "install.git.gitleaks"},
	"opt/scripts/system/install_k8s_tools.sh": {key: "install.tools.k8s"},
	"opt/scripts/system/install_herdr.sh":     {key: "install.tools.herdr"},
}

// gffInstallerGate answers an installer's gate from the repo's gff. An
// installer with no known gate is refused: running unknown code by default is
// the wrong side to fail on.
func gffInstallerGate(repo, installer string) (bool, string) {
	g, ok := installerGates[installer]
	if !ok {
		return false, "no gff gate known for " + installer
	}
	return gateAllows(&featflag.GFF{Repo: repo}, g), g.key
}

func gateAllows(src featflag.Source, g installerGate) bool {
	v, err := src.Bool(g.key)
	if err != nil {
		return !g.optIn
	}
	return v
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
// so sudo prompts and progress reach the operator. The host's track mode is
// exported the way install.sh's `gff export` does, so a standalone apply
// converges to the same versions an install would; PINS_TOOLS names the tools
// this run is for, so a shared installer (k8s) leaves the others alone.
func runInstallerScript(ctx context.Context, repo, installer, mode string, tools []string, out io.Writer) error {
	c := exec.CommandContext(ctx, "bash", filepath.Join(repo, filepath.FromSlash(installer)))
	c.Dir = repo
	c.Env = append(os.Environ(), "GFF_PINS_TRACK_MODE="+mode, "GFF_PINS_TRACK="+mode, "PINS_TOOLS="+strings.Join(tools, ","))
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
	mode := o.D.trackMode(o.M.Root)
	rows, _ := pins.CheckInstalled(ctx, o.M, o.D.exec, mode)
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
		// A row wanted at `latest` (track latest, e.g. herdr, or every row
		// under pins.track.mode=latest) reads ok offline: there is no pin to
		// compare. Re-run its installer whenever the tool is installed; it
		// resolves upstream latest and no-ops when already current. Without
		// this only install.sh kept such tools on latest, never a pins.yaml
		// rollout. A tool that is not installed is still left alone.
		selected := r.State == "drift" || (named[r.Tool] && r.State == "missing") ||
			(r.Want == "latest" && r.State == "ok")
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
		if ok, flag := o.D.gate(o.M.Root, e.Installer); !ok {
			fmt.Fprintf(o.Out, "%s: skipped — gff %s is off on this host (install.sh skips it too)\n", r.Tool, flag)
			continue
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
		if err := o.D.runInstaller(ctx, o.M.Root, g.Path, mode, g.Tools, o.Out); err != nil {
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
