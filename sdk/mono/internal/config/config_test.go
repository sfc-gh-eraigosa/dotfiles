package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultIsValidAndMatchesDesign(t *testing.T) {
	c := Default()
	if err := c.Validate(); err != nil {
		t.Fatalf("Default() invalid: %v", err)
	}
	got := []string{c.Title.Size}
	for _, p := range c.Panes {
		got = append(got, p.Name+"="+p.Size)
	}
	want := "5%,htop=60%,gpu=30%,gsl=5%"
	if strings.Join(got, ",") != want {
		t.Fatalf("default layout = %s, want %s", strings.Join(got, ","), want)
	}
	if c.Panes[0].Cmd != "htop-fit" || len(c.Panes[0].Fallbacks) != 1 || c.Panes[0].Fallbacks[0] != "htop" {
		t.Fatalf("htop pane should prefer htop-fit, then htop: %+v", c.Panes[0])
	}
	if c.Prefix != "ctrl+b" || c.Panes[2].Every != "1s" || c.Panes[2].Cmd != "gsl status" {
		t.Fatalf("unexpected default: %+v", c)
	}
}

func TestLoadMissingIsDefault(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Panes[0].Name != "htop" {
		t.Fatalf("got %+v", c)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.json")
	c, err := Default().Set("panes.gpu.cmd", "nvitop")
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Panes[1].Cmd != "nvitop" {
		t.Fatalf("round trip lost the edit: %+v", got.Panes[1])
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

func TestLoadRejectsBadFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	_ = os.WriteFile(p, []byte(`{"panes":[{"name":"x","size":"0%","cmd":"htop"}]}`), 0o644)
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "panes.x.size") {
		t.Fatalf("want a panes.x.size error, got %v", err)
	}
}

func TestSetValidationNamesFieldAndChangesNothing(t *testing.T) {
	c := Default()
	cases := []struct{ key, val, want string }{
		{"panes.htop.size", "0%", "panes.htop.size"},
		{"panes.htop.size", "abc", "panes.htop.size"},
		{"panes.htop.cmd", " ", "panes.htop.cmd"},
		{"panes.gsl.every", "10ms", "panes.gsl.every"},
		{"prefix", "ctrl+c", "prefix"},
		{"prefix", "alt+b", "prefix"},
		{"title.size", "200%", "title.size"},
		{"panes.nope.cmd", "x", "no pane named"},
		{"panes.htop.color", "x", "unknown pane field"},
		{"bogus", "x", "unknown key"},
	}
	for _, tc := range cases {
		got, err := c.Set(tc.key, tc.val)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Set(%q,%q) err = %v, want mention of %q", tc.key, tc.val, err, tc.want)
		}
		if got.Panes[0].Size != "60%" || got.Prefix != "ctrl+b" {
			t.Errorf("Set(%q,%q) changed the config on error", tc.key, tc.val)
		}
	}
}

func TestSetGetEveryField(t *testing.T) {
	c := Default()
	for key, val := range map[string]string{
		"prefix":              "ctrl+g",
		"title.size":          "10%",
		"panes.htop.cmd":      "btop",
		"panes.htop.size":     "50%",
		"panes.gsl.every":     "2s",
		"panes.gpu.fallbacks": "nvitop, gpustat",
		"panes.gpu.probe":     "none",
		"panes.htop.cwd":      "/tmp",
	} {
		next, err := c.Set(key, val)
		if err != nil {
			t.Fatalf("Set(%q): %v", key, err)
		}
		got, _ := next.Get(key)
		want := strings.ReplaceAll(val, ", ", ",")
		if got != want {
			t.Errorf("Get(%q) = %q, want %q", key, got, want)
		}
	}
	if c.Panes[1].Fallbacks[0] != "nvitop" || len(c.Panes[1].Fallbacks) != 1 {
		t.Fatalf("Set mutated the receiver: %+v", c.Panes[1])
	}
}

func TestAddRemoveMove(t *testing.T) {
	c := Default()
	c, err := c.Add(Pane{Name: "logs", Size: "10%", Cmd: "journalctl -f"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if order(c) != "htop,logs,gpu,gsl" {
		t.Fatalf("add: %s", order(c))
	}
	if _, err := c.Add(Pane{Name: "logs", Size: "1%", Cmd: "x"}, -1); err == nil {
		t.Fatal("duplicate name accepted")
	}
	if c, err = c.Move("logs", 5); err != nil || order(c) != "htop,gpu,gsl,logs" {
		t.Fatalf("move down: %s %v", order(c), err)
	}
	if c, err = c.Move("logs", -1); err != nil || order(c) != "htop,gpu,logs,gsl" {
		t.Fatalf("move up: %s %v", order(c), err)
	}
	if c, err = c.Remove("gpu"); err != nil || order(c) != "htop,logs,gsl" {
		t.Fatalf("remove: %s %v", order(c), err)
	}
	one := Config{Prefix: "ctrl+b", Title: Title{Size: "5%"}, Panes: []Pane{{Name: "a", Size: "1%", Cmd: "x"}}}
	if _, err := one.Remove("a"); err == nil {
		t.Fatal("removing the last pane accepted")
	}
}

func TestPrefixByte(t *testing.T) {
	if b, err := PrefixByte("ctrl+b"); err != nil || b != 0x02 {
		t.Fatalf("ctrl+b = %#x %v", b, err)
	}
	if b, _ := PrefixByte("ctrl+a"); b != 0x01 {
		t.Fatalf("ctrl+a = %#x", b)
	}
}

func TestPathHonorsEnv(t *testing.T) {
	t.Setenv("MONO_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/x")
	if p, _ := Path(); p != "/x/mono/config.json" {
		t.Fatalf("Path() = %s", p)
	}
	t.Setenv("MONO_CONFIG", "/y.json")
	if p, _ := Path(); p != "/y.json" {
		t.Fatalf("Path() with MONO_CONFIG = %s", p)
	}
}

func order(c Config) string {
	var n []string
	for _, p := range c.Panes {
		n = append(n, p.Name)
	}
	return strings.Join(n, ",")
}

func TestSaveWritesThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.json")
	link := filepath.Join(dir, "config.json")
	if err := Save(real, Default()); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	c, _ := Default().Set("panes.htop.size", "40%")
	if err := Save(link, c); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("Save replaced the symlink with a regular file")
	}
	got, err := Load(real)
	if err != nil || got.Panes[0].Size != "40%" {
		t.Fatalf("target not updated: %+v %v", got.Panes[0], err)
	}
}
