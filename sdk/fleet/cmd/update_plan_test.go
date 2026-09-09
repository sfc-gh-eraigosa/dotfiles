package cmd

import (
	"os"
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
