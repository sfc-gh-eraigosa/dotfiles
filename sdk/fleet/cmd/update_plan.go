package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	return loadPlanFor(file, src, repoDir, flagRepoChosen, func() (string, error) {
		return fleetConfigFile("fleet.yaml")
	})
}

// planSearchOrder is where a repo may keep its own fleet.yaml, first hit
// wins. Root first because that is where a small repo puts it; the dotted
// directories next because that is where a repo that already has a
// convention directory puts it; opt/etc/fleet last because that is the
// dotfiles layout, which predates discovery.
var planSearchOrder = []string{
	"fleet.yaml",
	".github/fleet.yaml",
	".fleet/fleet.yaml",
	"opt/etc/fleet/fleet.yaml",
}

// discoverPlanFile looks for a repo's own plan. A missing repo, an empty
// repoDir and a repo with no plan are all the same non-answer, never an
// error: discovery is an offer, and every caller has a fallback.
func discoverPlanFile(repoDir string) (string, bool) {
	c := discoverPlanFiles(repoDir)
	if len(c) == 0 {
		return "", false
	}
	return c[0], true
}

// discoverPlanFiles returns every candidate in search order, so a caller can
// skip one that fails the safety check and try the next.
func discoverPlanFiles(repoDir string) []string {
	if repoDir == "" {
		return nil
	}
	var found []string
	for _, rel := range planSearchOrder {
		path := filepath.Join(repoDir, rel)
		if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
			found = append(found, path)
		}
	}
	return found
}

// firstUsableDiscovered reads the first candidate that passes readPlanFile's
// ownership/mode check, warning about each one it has to skip.
//
// Why skip rather than fail: git does not store the group-write bit, so a
// repo cloned under umask 002 has a 664 fleet.yaml — an extremely ordinary
// state. Before discovery, that file was only ever read when a user pointed
// gff or --file at it, and failing loudly was right. Now that fleet finds it
// on its own, the same failure would take down `fleet` for anyone merely
// standing in such a repo. An EXPLICIT --file still fails hard: the user
// named that file, so they get the reason.
func firstUsableDiscovered(repoDir, note string, warn io.Writer) (updplan.Plan, bool) {
	for _, path := range discoverPlanFiles(repoDir) {
		p, err := readPlanFile(path, note)
		if err == nil {
			p.Source = fmt.Sprintf("%s (discovered in %s)", path, repoDir)
			if note != "" {
				p.Source = fmt.Sprintf("%s (discovered in %s) (gff: %s)", path, repoDir, note)
			}
			return p, true
		}
		if warn != nil {
			fmt.Fprintf(warn, "warning: skipping discovered plan %s: %v\n", path, err)
			fmt.Fprintf(warn, "         (if the mode is the problem: chmod g-w %s)\n", path)
		}
	}
	return updplan.Plan{}, false
}

// repoFromCwd reports the git work tree dir contains, so `cd <repo> && fleet
// tui` operates on that repo. Not being in a work tree is not an error.
func repoFromCwd(dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", false
	}
	top := strings.TrimSpace(string(out))
	return top, top != ""
}

// loadPlanFor is loadPlan with its two environmental inputs injected, so the
// resolution order is testable without a real XDG dir or a real cwd.
//
// repoChosen says the user actually named the repo — an explicit --repo, or a
// cwd inside a repo that is not the dotfiles default. It is what decides
// whether discovery outranks the gff selection; see planFromSettingsFor.
func loadPlanFor(file string, src featflag.Source, repoDir string, repoChosen bool, homePath func() (string, error)) (updplan.Plan, error) {
	return loadPlanForWarn(file, src, repoDir, repoChosen, homePath, os.Stderr)
}

// loadPlanForWarn is loadPlanFor with the warning sink injected, so a test
// can assert on what a skipped candidate reports.
func loadPlanForWarn(file string, src featflag.Source, repoDir string, repoChosen bool, homePath func() (string, error), warn io.Writer) (updplan.Plan, error) {
	if file != "" {
		return readPlanFile(file, "")
	}
	return planFromSettingsFor(featflag.Resolve(src, "", repoDir), repoDir, repoChosen, homePath, warn)
}

// planFromSettings is loadPlan's steps 2-4 given already-resolved Settings,
// so a caller that needs the Settings for something else too (the TUI's lane
// policy, resolveTUIPlan) resolves gff exactly once.
func planFromSettings(settings featflag.Settings, repoDir string, repoChosen bool) (updplan.Plan, error) {
	return planFromSettingsFor(settings, repoDir, repoChosen, func() (string, error) {
		return fleetConfigFile("fleet.yaml")
	}, os.Stderr)
}

// planFromSettingsFor resolves the plan, in order:
//
//  1. fleet.update.enabled=false pins the built-in plan — discovery is not a
//     way around that switch.
//  2. the repo's OWN plan, when the repo was chosen (--repo / cwd). Naming a
//     repo is what asking for its plan looks like.
//  3. the gff-selected path (or the caller's home default): present ⇒ use it.
//  4. the repo's own plan, when one was discovered. This is the "override the
//     dotfiles default" case — the built-in plan loses to a real one.
//  5. the built-in plan, naming the path it looked for.
//
// Steps 2 and 4 are the same file; what differs is whether it outranks the
// gff selection. That split is deliberate and load-bearing: dotfiles SHIPS
// opt/etc/fleet/fleet.yaml, so a discovery that always won would silently
// move every existing user off their ~/.config/fleet/fleet.yaml without
// anyone asking for it.
func planFromSettingsFor(settings featflag.Settings, repoDir string, repoChosen bool, homePath func() (string, error), warn io.Writer) (updplan.Plan, error) {
	if !settings.Enabled {
		p := updplan.Default()
		p.Source = "built-in default (fleet.update.enabled=false)"
		return p, nil
	}

	if repoChosen {
		if p, ok := firstUsableDiscovered(repoDir, settings.Note, warn); ok {
			return p, nil
		}
	}

	path := settings.ConfigPath
	if path == "" {
		var err error
		path, err = homePath()
		if err != nil {
			return updplan.Plan{}, fmt.Errorf("loadPlan: %w", err)
		}
	}

	p, err := readPlanFile(path, settings.Note)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return updplan.Plan{}, err
		}
		if !repoChosen {
			// Not tried above; the built-in default loses to a real plan.
			if dp, ok := firstUsableDiscovered(repoDir, settings.Note, warn); ok {
				return dp, nil
			}
		}
		p := updplan.Default()
		p.Source = fmt.Sprintf("built-in default (no %s)", path)
		return p, nil
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
