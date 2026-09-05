package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// TestInitWritesTheDefaultPlanOnce asserts the file lands 0644 in a 0700
// directory, and a second run without --overwrite refuses rather than
// silently clobbering an operator's edited plan.
func TestInitWritesTheDefaultPlanOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "fleet.yaml")

	wrote, err := writeDefaultPlan(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("first run must write the file")
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("file mode = %v, want 0644", fi.Mode().Perm())
	}
	dfi, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dfi.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v, want 0700", dfi.Mode().Perm())
	}

	if _, err := writeDefaultPlan(path, false); err == nil {
		t.Fatal("a second run without --overwrite must refuse to clobber the file")
	}

	if _, err := writeDefaultPlan(path, true); err != nil {
		t.Fatalf("--overwrite must be allowed to replace it: %v", err)
	}
}

// TestInitPrintToStdout asserts --print writes the default plan text to the
// given writer instead of touching disk.
func TestInitPrintToStdout(t *testing.T) {
	var buf strings.Builder
	if err := printDefaultPlan(&buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != updplan.DefaultYAML {
		t.Fatalf("printed text does not match updplan.DefaultYAML:\n%s", buf.String())
	}
}

// TestInitOutputParsesToDefault asserts what init writes actually parses
// back to updplan.Default() — the round-trip DefaultYAML itself promises.
func TestInitOutputParsesToDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fleet.yaml")
	if _, err := writeDefaultPlan(path, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := updplan.Parse(data)
	if err != nil {
		t.Fatalf("init's own output does not parse: %v", err)
	}
	want := updplan.Default()
	got.Source, want.Source = "", ""
	if len(got.Steps) != len(want.Steps) || len(got.Repos) != len(want.Repos) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
