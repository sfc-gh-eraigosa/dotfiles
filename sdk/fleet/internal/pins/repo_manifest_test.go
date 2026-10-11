package pins

import (
	"os"
	"path/filepath"
	"testing"
)

// The REAL opt/profiles/versions.tsv must parse with the Go reader too. The
// shell side (opt/lib/pins_test.sh) checks it with awk; this keeps the two
// readers from drifting, so a manifest edit that only one of them accepts fails
// the PR rather than a host's `fleet pins` run.
func TestRepoManifestParses(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "opt", "profiles", "versions.tsv")); err != nil {
		t.Skipf("not inside the dotfiles repo: %v", err)
	}
	m, err := Load(root)
	if err != nil {
		t.Fatalf("versions.tsv does not parse: %v", err)
	}
	if len(m.Entries) == 0 {
		t.Fatal("versions.tsv has no rows")
	}
	for _, e := range m.Entries {
		if e.Track != TrackLatest && ExtractVersion(e.Version) == "" {
			t.Errorf("%s: %s row has no usable version %q (line %d)", e.Tool, e.Track, e.Version, e.Line)
		}
		if e.Installer != "-" {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(e.Installer))); err != nil {
				t.Errorf("%s: installer %s does not exist", e.Tool, e.Installer)
			}
		}
	}
}
