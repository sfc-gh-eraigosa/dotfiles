package pins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const seed = `# versions manifest
# tool track version upstream installer probe

go          pin     @.go-version  go:                    -                                  go version
uv          pin     0.12.16       gh:astral-sh/uv        opt/scripts/system/install_uv.sh   uv --version
rust        floor   1.96.0        gh:rust-lang/rust      opt/scripts/system/install_rust.sh rustc --version
herdr       latest  -             -                      opt/scripts/system/install_herdr.sh herdr --version
nerd-fonts  pin     3.4.0         gh:ryanoasis/nerd-fonts -                                 -
`

func parseSeed(t *testing.T) (*Manifest, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".go-version"), []byte("1.26.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Parse(strings.NewReader(seed), root, filepath.Join(root, "versions.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	return m, root
}

func TestParseRows(t *testing.T) {
	m, _ := parseSeed(t)
	if len(m.Entries) != 5 {
		t.Fatalf("entries = %d, want 5", len(m.Entries))
	}
	g, ok := m.Get("go")
	if !ok || g.Version != "1.26.1" || g.VersionFile != ".go-version" || g.Track != TrackPin || g.Upstream != "go:" || g.Installer != "-" || g.Probe != "go version" {
		t.Errorf("go entry wrong: %+v", g)
	}
	r, _ := m.Get("rust")
	if r.Track != TrackFloor || r.Probe != "rustc --version" || r.Line != 6 {
		t.Errorf("rust entry wrong: %+v", r)
	}
	h, _ := m.Get("herdr")
	if h.Track != TrackLatest || h.Version != "-" {
		t.Errorf("herdr entry wrong: %+v", h)
	}
	if n, _ := m.Get("nerd-fonts"); n.Probe != "-" {
		t.Errorf("probe = %q", n.Probe)
	}
	if _, ok := m.Get("nope"); ok {
		t.Error("Get(nope) should miss")
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"dup", "a pin 1.0 - - -\na pin 1.1 - - -\n", "line 2"},
		{"track", "# c\na bogus 1.0 - - -\n", "line 2"},
		{"cols", "a pin 1.0\n", "line 1"},
		{"badname", "A pin 1.0 - - -\n", "line 1"},
		{"badver", "a pin x1 - - -\n", "line 1"},
		{"dash on pin", "a pin - - - -\n", "line 1"},
		{"missing file", "a pin @nope.txt - - -\n", "line 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(c.in), t.TempDir(), "x")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want mention of %q", err, c.want)
			}
		})
	}
}

func TestParseStripsLeadingV(t *testing.T) {
	m, err := Parse(strings.NewReader("a pin v1.2.3 - - -\n"), t.TempDir(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if e, _ := m.Get("a"); e.Version != "1.2.3" {
		t.Errorf("version = %q", e.Version)
	}
}

func TestLoad(t *testing.T) {
	root := t.TempDir()
	if _, err := Load(root); err == nil {
		t.Fatal("Load without a manifest should fail")
	}
	dir := filepath.Join(root, "opt", "profiles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "versions.tsv"), []byte("a pin 1.0 - - -\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if m.Root != root || m.Path != filepath.Join(dir, "versions.tsv") || len(m.Entries) != 1 {
		t.Errorf("manifest = %+v", m)
	}
}

func TestManifestSetPreservesEverythingElse(t *testing.T) {
	m, _ := parseSeed(t)
	path := m.Path
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Set("uv", "v0.13.0"); err != nil {
		t.Fatal(err)
	}
	if err := m.Set("rust", "1.97.0"); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := strings.Replace(seed, "uv          pin     0.12.16 ", "uv          pin     0.13.0  ", 1)
	want = strings.Replace(want, "rust        floor   1.96.0 ", "rust        floor   1.97.0 ", 1)
	if string(got) != want {
		t.Errorf("saved manifest differs:\n--- got\n%s\n--- want\n%s", got, want)
	}
	if e, _ := m.Get("uv"); e.Version != "0.13.0" {
		t.Errorf("entry not updated: %+v", e)
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp*"))
	if len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
	// Longer version than the column: still a single-space-safe rewrite.
	if err := m.Set("uv", "10.20.30-rc1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	m2, err := Load2(t, path, m.Root)
	if err != nil {
		t.Fatal(err)
	}
	if e, _ := m2.Get("uv"); e.Version != "10.20.30-rc1" || e.Upstream != "gh:astral-sh/uv" {
		t.Errorf("reparse = %+v", e)
	}
}

// Load2 parses path with an explicit root (Load derives the path from root).
func Load2(t *testing.T, path, root string) (*Manifest, error) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return Parse(f, root, path)
}

func TestManifestSetFileRow(t *testing.T) {
	m, root := parseSeed(t)
	if err := os.WriteFile(m.Path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Set("go", "1.26.3"); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, ".go-version"))
	if string(b) != "1.26.3\n" {
		t.Errorf(".go-version = %q", b)
	}
	got, _ := os.ReadFile(m.Path)
	if string(got) != seed {
		t.Error("manifest must not change when the version lives in a file")
	}
	if e, _ := m.Get("go"); e.Version != "1.26.3" {
		t.Errorf("entry = %+v", e)
	}
}

func TestManifestSetRefusals(t *testing.T) {
	m, _ := parseSeed(t)
	if err := m.Set("herdr", "1.0.0"); err == nil {
		t.Error("Set on a latest row must fail")
	}
	if err := m.Set("ghost", "1.0.0"); err == nil {
		t.Error("Set on an unknown tool must fail")
	}
	if err := m.Set("uv", "not-a-version"); err == nil {
		t.Error("Set with a non-version must fail")
	}
}

func TestSaveWithoutChangesIsNoop(t *testing.T) {
	m, _ := parseSeed(t)
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.Path); !os.IsNotExist(err) {
		t.Error("Save with no Set must not write")
	}
}
