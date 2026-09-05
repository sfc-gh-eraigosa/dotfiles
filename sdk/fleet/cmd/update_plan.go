package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// checkOwner reports the uid that owns fi, when the platform can say so. It
// is a var so a test can simulate a foreign owner without root. The default
// implementation reads syscall.Stat_t, which is populated on every unix
// platform fleet ships on (WSL2-Ubuntu, macOS, Linux).
var checkOwner = func(fi os.FileInfo) (uid int, ok bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}

// loadPlan resolves the update plan fleet update runs, in order:
//
//  1. --file, when given: must exist, or this is a hard error.
//  2. featflag.Resolve: Enabled == false pins the built-in plan.
//  3. the gff-selected (or caller-default) path: missing ⇒ built-in plan,
//     naming the path it looked for.
//  4. present ⇒ ownership/mode check, then updplan.Parse.
//
// repoDir is the --repo checkout, passed to featflag.Resolve for the "repo"
// config location and used nowhere else.
//
// The resolved path is stat'd exactly ONCE, inside readPlanFile: this used
// to stat here first to decide the missing-file fallback, then readPlanFile
// stat'd again to check ownership/mode — the SAME file, twice, on every
// call. errors.Is(err, fs.ErrNotExist) recovers the same "file is missing"
// fact from readPlanFile's own (wrapped) stat error instead.
func loadPlan(file string, src featflag.Source, repoDir string) (updplan.Plan, error) {
	if file != "" {
		return readPlanFile(file, "")
	}

	settings := featflag.Resolve(src, "", repoDir)
	if !settings.Enabled {
		p := updplan.Default()
		p.Source = "built-in default (fleet.update.enabled=false)"
		return p, nil
	}

	path := settings.ConfigPath
	if path == "" {
		var err error
		path, err = fleetConfigFile("fleet.yaml")
		if err != nil {
			return updplan.Plan{}, fmt.Errorf("loadPlan: %w", err)
		}
	}

	p, err := readPlanFile(path, settings.Note)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			p := updplan.Default()
			p.Source = fmt.Sprintf("built-in default (no %s)", path)
			return p, nil
		}
		return updplan.Plan{}, err
	}
	return p, nil
}

// readPlanFile stats, validates ownership/mode, reads, and parses one plan
// file. note, when non-empty, is appended to the recorded Source (e.g. the
// gff fallback note) on one line.
func readPlanFile(path, note string) (updplan.Plan, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return updplan.Plan{}, fmt.Errorf("loadPlan: %s: %w", path, err)
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return updplan.Plan{}, fmt.Errorf("loadPlan: %s: refusing a group/world-writable plan file (mode %v) — a plan file is executable config", path, fi.Mode().Perm())
	}
	uid, ok := checkOwner(fi)
	if !ok {
		return updplan.Plan{}, fmt.Errorf("loadPlan: %s: could not verify file ownership on this platform, refusing", path)
	}
	if uid != os.Getuid() {
		return updplan.Plan{}, fmt.Errorf("loadPlan: %s: refusing a plan file not owned by the current user", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return updplan.Plan{}, fmt.Errorf("loadPlan: %s: %w", path, err)
	}
	p, err := updplan.Parse(data)
	if err != nil {
		return updplan.Plan{}, fmt.Errorf("loadPlan: %s: %w", path, err)
	}
	p.Source = path
	if note != "" {
		p.Source = fmt.Sprintf("%s (gff: %s)", path, note)
	}
	return p, nil
}
