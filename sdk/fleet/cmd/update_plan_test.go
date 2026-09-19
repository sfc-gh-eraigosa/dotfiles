package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// TestLoadPlanPrefersFileFlag asserts --file wins over everything else, and
// that a missing --file is a hard error (never silently falls back to the
// built-in plan the way a missing gff-selected path does).
func TestLoadPlanPrefersFileFlag(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "fleet.yaml")
	if err := os.WriteFile(file, []byte(updplan.DefaultYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	src := featflag.Static{Bools: map[string]bool{featflag.KeyEnabled: false}}
	p, err := loadPlan(file, src, "/does/not/matter")
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != file {
		t.Fatalf("Source = %q, want %q", p.Source, file)
	}

	missing := filepath.Join(dir, "missing.yaml")
	if _, err := loadPlan(missing, src, "/does/not/matter"); err == nil {
		t.Fatal("a missing --file must be a hard error")
	}
}

// TestLoadPlanUsesBuiltInWhenDisabled asserts fleet.update.enabled=false pins
// the built-in plan regardless of what gff's config selection or any file on
// disk says.
func TestLoadPlanUsesBuiltInWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// Even with a valid file sitting at the default location, disabled wins.
	must(t, os.MkdirAll(filepath.Join(dir, "fleet"), 0o700))
	must(t, os.WriteFile(filepath.Join(dir, "fleet", "fleet.yaml"), []byte(updplan.DefaultYAML), 0o600))

	src := featflag.Static{Bools: map[string]bool{featflag.KeyEnabled: false}, Strs: map[string][]string{featflag.KeyConfig: {"home"}}}
	p, err := loadPlan("", src, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Source, "built-in default") || !strings.Contains(p.Source, "fleet.update.enabled=false") {
		t.Fatalf("Source = %q, want it to name the disabled flag", p.Source)
	}
}

// TestLoadPlanUsesBuiltInWhenNoFile asserts a missing file at the resolved
// location falls back to the built-in plan and names the path it looked for.
func TestLoadPlanUsesBuiltInWhenNoFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	src := featflag.Static{Bools: map[string]bool{featflag.KeyEnabled: true}, Strs: map[string][]string{featflag.KeyConfig: {"home"}}}
	p, err := loadPlan("", src, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "fleet", "fleet.yaml")
	if !strings.Contains(p.Source, "built-in default") || !strings.Contains(p.Source, want) {
		t.Fatalf("Source = %q, want it to name %q", p.Source, want)
	}
}

// TestLoadPlanReadsTheConfiguredPath asserts the "home" location resolves
// under XDG_CONFIG_HOME and is actually read.
func TestLoadPlanReadsTheConfiguredPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "fleet", "fleet.yaml")
	must(t, os.MkdirAll(filepath.Dir(path), 0o700))
	must(t, os.WriteFile(path, []byte(updplan.DefaultYAML), 0o600))

	src := featflag.Static{Bools: map[string]bool{featflag.KeyEnabled: true}, Strs: map[string][]string{featflag.KeyConfig: {"home"}}}
	p, err := loadPlan("", src, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.Source, path) {
		t.Fatalf("Source = %q, want prefix %q", p.Source, path)
	}
}

// TestLoadPlanReadsTheRepoLocation asserts the "repo" location resolves under
// <repoDir>/opt/etc/fleet/fleet.yaml.
func TestLoadPlanReadsTheRepoLocation(t *testing.T) {
	repoDir := t.TempDir()
	path := filepath.Join(repoDir, "opt", "etc", "fleet", "fleet.yaml")
	must(t, os.MkdirAll(filepath.Dir(path), 0o700))
	must(t, os.WriteFile(path, []byte(updplan.DefaultYAML), 0o600))

	src := featflag.Static{Bools: map[string]bool{featflag.KeyEnabled: true}, Strs: map[string][]string{featflag.KeyConfig: {"repo"}}}
	p, err := loadPlan("", src, repoDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.Source, path) {
		t.Fatalf("Source = %q, want prefix %q", p.Source, path)
	}
}

// TestLoadPlanRefusesAWorldWritableFile asserts a shared-mode plan file is
// refused outright rather than trusted as executable config.
func TestLoadPlanRefusesAWorldWritableFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "fleet.yaml")
	must(t, os.WriteFile(file, []byte(updplan.DefaultYAML), 0o666))
	// WriteFile's mode is filtered by the process umask (CI runners use 022,
	// which yields 0644 and is rightly accepted); Chmod is not.
	must(t, os.Chmod(file, 0o666))

	src := featflag.Static{Bools: map[string]bool{featflag.KeyEnabled: true}}
	if _, err := loadPlan(file, src, "/repo"); err == nil {
		t.Fatal("a world-writable plan file must be refused")
	}
	// The same file at 0644 is the normal case and must load.
	must(t, os.Chmod(file, 0o644))
	if _, err := loadPlan(file, src, "/repo"); err != nil {
		t.Fatalf("a user-owned 0644 plan file must load: %v", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// --- plan discovery in the repo (dotfiles#351 feature 1) -------------------
//
// fleet should find a repo's OWN plan instead of only ever running the
// dotfiles one. The search order is fixed and first-hit-wins; what it must
// never do is quietly change which plan an existing user gets, which is why
// discovery only outranks the gff/home selection when the repo was actually
// CHOSEN (an explicit --repo, or a cwd inside that repo).

// writePlan drops a minimal, valid, correctly-owned plan at path.
func writePlan(t *testing.T, path, id string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "version: 1\nupdate:\n  repos:\n    r:\n      path: ~/" + id + "\n  steps:\n    - id: " + id + "\n      kind: sync\n      repo: r\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// planID returns the plan's single step id — which file was loaded.
func planID(p updplan.Plan) string {
	if len(p.Steps) == 0 {
		return ""
	}
	return p.Steps[0].ID
}

func TestDiscoverPlanFileSearchOrder(t *testing.T) {
	// Every slot, checked one at a time: the earlier location must win, and
	// removing it must fall through to the next.
	repo := t.TempDir()
	order := []string{
		"fleet.yaml",
		".github/fleet.yaml",
		".fleet/fleet.yaml",
		"opt/etc/fleet/fleet.yaml",
	}
	// Seed every slot; then delete from the front and re-check each time.
	for _, rel := range order {
		writePlan(t, filepath.Join(repo, rel), "x")
	}
	for i, want := range order {
		got, ok := discoverPlanFile(repo)
		if !ok {
			t.Fatalf("slot %d: nothing discovered, want %s", i, want)
		}
		if got != filepath.Join(repo, want) {
			t.Fatalf("slot %d: discovered %s, want %s", i, got, filepath.Join(repo, want))
		}
		if err := os.Remove(got); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := discoverPlanFile(repo); ok {
		t.Fatal("an empty repo discovered a plan")
	}
	// A repo that does not exist is not an error, just no discovery.
	if _, ok := discoverPlanFile(filepath.Join(repo, "nope")); ok {
		t.Fatal("a missing repo discovered a plan")
	}
	if _, ok := discoverPlanFile(""); ok {
		t.Fatal("an empty repoDir discovered a plan")
	}
}

func TestDiscoveryWinsWhenTheRepoWasChosen(t *testing.T) {
	// `fleet --repo <other>` (or a cwd inside it): that repo's plan is the
	// point of naming it, so it outranks whatever gff selects.
	repo := t.TempDir()
	writePlan(t, filepath.Join(repo, "fleet.yaml"), "discovered")
	home := filepath.Join(t.TempDir(), "home.yaml")
	writePlan(t, home, "gffselected")

	p, err := loadPlanFor("", featflag.Static{Strs: map[string][]string{featflag.KeyConfig: {"home"}}}, repo, true, func() (string, error) { return home, nil })
	if err != nil {
		t.Fatal(err)
	}
	if got := planID(p); got != "discovered" {
		t.Fatalf("loaded %q, want the repo's own plan", got)
	}
	if !strings.Contains(p.Source, "discovered in") {
		t.Fatalf("Source %q does not say the plan was discovered", p.Source)
	}
}

func TestDiscoveryDefersToAnExistingSelectionOnTheDefaultRepo(t *testing.T) {
	// The regression this ordering exists to prevent: dotfiles SHIPS
	// opt/etc/fleet/fleet.yaml, so an unconditional discovery would silently
	// move every existing user off their ~/.config/fleet/fleet.yaml.
	repo := t.TempDir()
	writePlan(t, filepath.Join(repo, "opt/etc/fleet/fleet.yaml"), "tracked")
	home := filepath.Join(t.TempDir(), "home.yaml")
	writePlan(t, home, "gffselected")

	p, err := loadPlanFor("", featflag.Static{Strs: map[string][]string{featflag.KeyConfig: {"home"}}}, repo, false, func() (string, error) { return home, nil })
	if err != nil {
		t.Fatal(err)
	}
	if got := planID(p); got != "gffselected" {
		t.Fatalf("loaded %q, want the gff-selected plan left alone", got)
	}
}

func TestDiscoveryFillsInForAMissingSelection(t *testing.T) {
	// ...but with no home plan, the repo's own plan beats the BUILT-IN
	// dotfiles default. That is the "override the dotfiles default" case.
	repo := t.TempDir()
	writePlan(t, filepath.Join(repo, ".github/fleet.yaml"), "discovered")
	missing := filepath.Join(t.TempDir(), "absent.yaml")

	p, err := loadPlanFor("", featflag.Static{Strs: map[string][]string{featflag.KeyConfig: {"home"}}}, repo, false, func() (string, error) { return missing, nil })
	if err != nil {
		t.Fatal(err)
	}
	if got := planID(p); got != "discovered" {
		t.Fatalf("loaded %q, want the discovered plan", got)
	}
}

func TestExplicitFileBeatsDiscovery(t *testing.T) {
	repo := t.TempDir()
	writePlan(t, filepath.Join(repo, "fleet.yaml"), "discovered")
	explicit := filepath.Join(t.TempDir(), "explicit.yaml")
	writePlan(t, explicit, "explicit")

	p, err := loadPlanFor(explicit, featflag.Static{}, repo, true, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	if got := planID(p); got != "explicit" {
		t.Fatalf("loaded %q, want --file to win", got)
	}
}

func TestDisabledPinsTheBuiltInEvenWithADiscoverablePlan(t *testing.T) {
	// fleet.update.enabled=false means "the built-in plan, regardless of any
	// file" — discovery must not be a way around it.
	repo := t.TempDir()
	writePlan(t, filepath.Join(repo, "fleet.yaml"), "discovered")

	p, err := loadPlanFor("", featflag.Static{Bools: map[string]bool{featflag.KeyEnabled: false}}, repo, true, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	if planID(p) == "discovered" {
		t.Fatal("discovery overrode fleet.update.enabled=false")
	}
}

func TestRepoFromCwdDerivesTheGitWorkTree(t *testing.T) {
	// `cd playground && fleet tui` should operate on playground.
	repo := t.TempDir()
	// t.TempDir() can hand back a symlinked path (/tmp -> /private/tmp on
	// macOS); compare against what git itself reports.
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init unavailable: %v %s", err, out)
	}
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	top, err := exec.Command("git", "-C", repo, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	want := strings.TrimSpace(string(top))

	got, ok := repoFromCwd(sub)
	if !ok {
		t.Fatal("no repo derived from a directory inside a git work tree")
	}
	if got != want {
		t.Fatalf("derived %q, want %q", got, want)
	}
	// Outside any work tree: no answer, and no error.
	outside := t.TempDir()
	if _, ok := repoFromCwd(outside); ok {
		t.Skip("the temp dir is itself inside a git work tree; nothing to assert")
	}
}

func TestDiscoverySkipsAnUnsafeCandidateInsteadOfFailing(t *testing.T) {
	// git does not store the group-write bit, so a repo cloned under umask
	// 002 has a 664 plan file. An EXPLICIT --file should still fail loudly —
	// the user named that file — but a DISCOVERED one must not take the whole
	// command down: discovery is an offer, so an unsafe candidate is skipped
	// (with a warning) and the search carries on.
	repo := t.TempDir()
	unsafe := filepath.Join(repo, "fleet.yaml")
	writePlan(t, unsafe, "unsafe")
	if err := os.Chmod(unsafe, 0o664); err != nil {
		t.Fatal(err)
	}
	safe := filepath.Join(repo, ".github/fleet.yaml")
	writePlan(t, safe, "safe")
	if err := os.Chmod(safe, 0o644); err != nil {
		t.Fatal(err)
	}

	var warned strings.Builder
	p, err := loadPlanForWarn("", featflag.Static{}, repo, true, func() (string, error) { return "", nil }, &warned)
	if err != nil {
		t.Fatalf("a group-writable discovered plan failed the command: %v", err)
	}
	if got := planID(p); got != "safe" {
		t.Fatalf("loaded %q, want the next safe candidate", got)
	}
	if !strings.Contains(warned.String(), "chmod g-w") {
		t.Fatalf("warning %q does not name the fix", warned.String())
	}

	// No safe candidate at all: fall through, still no error.
	if err := os.Remove(safe); err != nil {
		t.Fatal(err)
	}
	warned.Reset()
	p, err = loadPlanForWarn("", featflag.Static{}, repo, true, func() (string, error) { return "", nil }, &warned)
	if err != nil {
		t.Fatalf("no safe candidate should fall through, not fail: %v", err)
	}
	if planID(p) == "unsafe" {
		t.Fatal("an unsafe plan was loaded")
	}

	// ...but --file on the same file is still a hard, explicit error.
	if _, err := loadPlanFor(unsafe, featflag.Static{}, repo, true, func() (string, error) { return "", nil }); err == nil {
		t.Fatal("--file on a group-writable plan should fail loudly")
	}
}
