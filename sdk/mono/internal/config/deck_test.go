package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decked() Config {
	c := Config{Prefix: "ctrl+b", Title: Title{Size: "5%"}}
	c.Panes = []Pane{
		{Name: "htop", Size: "60%", Cmd: "htop-fit", Fallbacks: []string{"htop"}},
		{Name: "mid", Size: "30%", Tools: []Tool{
			{Name: "gpu", Views: []View{{Name: "nvtop", Cmd: "nvtop", Fallbacks: []string{"nvitop"}}}},
			{Name: "gdash", Probe: "gdash doctor --quiet", Zoom: true, Views: []View{
				{Name: "conductor", Cmd: "gdash conductor"},
				{Name: "ci", Cmd: "gdash ci"},
			}},
		}},
		{Name: "gsl", Size: "5%", Cmd: "gsl status", Every: "1s", Zoomable: boolp(false)},
	}
	return c
}

func boolp(b bool) *bool { return &b }

// E16: a config written by #407 loads and saves back byte for byte.
func TestCompat407RoundTrip(t *testing.T) {
	want, err := os.ReadFile("testdata/default-407.json")
	if err != nil {
		t.Fatal(err)
	}
	var c Config
	if err := json.Unmarshal(want, &c); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("#407 config no longer valid: %v", err)
	}
	p := filepath.Join(t.TempDir(), "c.json")
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != string(want) {
		t.Fatalf("round trip changed the file:\n%s\nwant\n%s", got, want)
	}
}

func TestDeckValidates(t *testing.T) {
	if err := decked().Validate(); err != nil {
		t.Fatal(err)
	}
	bad := map[string]func(*Config){
		"cmd and tools":       func(c *Config) { c.Panes[1].Cmd = "x" },
		"neither":             func(c *Config) { c.Panes[1].Tools = nil },
		"tool without views":  func(c *Config) { c.Panes[1].Tools[0].Views = nil },
		"duplicate tool":      func(c *Config) { c.Panes[1].Tools[1].Name = "gpu" },
		"duplicate view":      func(c *Config) { c.Panes[1].Tools[1].Views[1].Name = "conductor" },
		"bad tool name":       func(c *Config) { c.Panes[1].Tools[0].Name = "GPU!" },
		"empty view cmd":      func(c *Config) { c.Panes[1].Tools[1].Views[0].Cmd = " " },
		"bad view every":      func(c *Config) { c.Panes[1].Tools[1].Views[0].Every = "fast" },
		"bad middle":          func(c *Config) { c.Middle = "triple" },
		"negative nav_esc_ms": func(c *Config) { n := -1; c.NavEscMs = &n },
		"bad nav_key":         func(c *Config) { c.NavKey = "ctrl+shift+q" },
	}
	for name, mut := range bad {
		c := decked()
		mut(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestPlainPaneIsAOneByOneDeck(t *testing.T) {
	c := decked()
	d := c.Panes[0].Deck()
	if len(d) != 1 || d[0].Name != "htop" || len(d[0].Views) != 1 || d[0].Views[0].Cmd != "htop-fit" || d[0].Views[0].Fallbacks[0] != "htop" {
		t.Fatalf("plain pane deck = %+v", d)
	}
	if c.Panes[0].IsDeck() || !c.Panes[1].IsDeck() {
		t.Fatal("IsDeck")
	}
}

func TestDeckGetSet(t *testing.T) {
	c := decked()
	for key, want := range map[string]string{
		"panes.mid.tools.gdash.probe":               "gdash doctor --quiet",
		"panes.mid.tools.gdash.zoom":                "true",
		"panes.mid.tools.gdash.views.ci.cmd":        "gdash ci",
		"panes.mid.tools.gpu.views.nvtop.fallbacks": "nvitop",
		"panes.gsl.zoomable":                        "false",
		"panes.htop.zoomable":                       "true",
		"middle":                                    "split",
		"nav_key":                                   "space",
		"nav_esc_ms":                                "300",
	} {
		if got, err := c.Get(key); err != nil || got != want {
			t.Errorf("Get(%s) = %q %v, want %q", key, got, err, want)
		}
	}
	c2, err := c.Set("panes.mid.tools.gdash.views.ci.cmd", "gdash ci --range 6h")
	if err != nil || c2.Panes[1].Tools[1].Views[1].Cmd != "gdash ci --range 6h" {
		t.Fatalf("set view cmd: %v", err)
	}
	if c.Panes[1].Tools[1].Views[1].Cmd != "gdash ci" {
		t.Fatal("Set mutated the receiver")
	}
	if _, err := c.Set("panes.mid.tools.gdash.views.nope.cmd", "x"); err == nil {
		t.Fatal("unknown view accepted")
	}
	if _, err := c.Set("panes.mid.cmd", "x"); err == nil {
		t.Fatal("a cmd on a deck pane was accepted")
	}
	c3, err := c.Set("middle", "single")
	if err != nil || c3.Middle != "single" {
		t.Fatalf("set middle: %v", err)
	}
	c4, err := c.Set("nav_esc_ms", "0")
	if err != nil || c4.EscMs() != 0 {
		t.Fatalf("nav_esc_ms 0 must disable double-esc: %v", err)
	}
	for _, k := range []string{"panes.mid.tools.gdash.views.ci.cmd", "middle", "panes.gsl.zoomable"} {
		found := false
		for _, kk := range c.Keys() {
			if kk == k {
				found = true
			}
		}
		if !found {
			t.Errorf("Keys() is missing %s", k)
		}
	}
}

func TestAddRemoveToolAndView(t *testing.T) {
	c := decked()
	c2, err := c.AddView("mid", "gdash", View{Name: "k3s-nodes", Cmd: "gdash k3s-nodes"}, 1)
	if err != nil || c2.Panes[1].Tools[1].Views[1].Name != "k3s-nodes" {
		t.Fatalf("AddView: %v %+v", err, c2.Panes[1].Tools[1].Views)
	}
	if _, err := c.AddView("mid", "gdash", View{Name: "ci", Cmd: "x"}, -1); err == nil {
		t.Fatal("duplicate view accepted")
	}
	c3, err := c2.RemoveView("mid", "gdash", "conductor")
	if err != nil || len(c3.Panes[1].Tools[1].Views) != 2 {
		t.Fatalf("RemoveView: %v", err)
	}
	c4, err := c.AddTool("mid", Tool{Name: "logs", Views: []View{{Name: "journal", Cmd: "journalctl -f"}}}, -1)
	if err != nil || len(c4.Panes[1].Tools) != 3 {
		t.Fatalf("AddTool: %v", err)
	}
	c5, err := c4.RemoveTool("mid", "gpu")
	if err != nil || c5.Panes[1].Tools[0].Name != "gdash" {
		t.Fatalf("RemoveTool: %v", err)
	}
	if _, err := c5.RemoveTool("mid", "gdash"); err != nil {
		t.Fatal(err)
	}
	only, _ := c.RemoveTool("mid", "gpu")
	if _, err := only.RemoveTool("mid", "gdash"); err == nil || !strings.Contains(err.Error(), "remove the pane") {
		t.Fatalf("removing a deck's last tool should point at removing the pane: %v", err)
	}
}

// E27 (config half): middle: single merges every middle pane into one deck.
func TestMiddleSingle(t *testing.T) {
	c := decked()
	c.Middle = "single"
	eff := c.Effective()
	if len(eff) != 2 || eff[0].Name != "mid" || eff[1].Name != "gsl" {
		t.Fatalf("effective panes = %v", names(eff))
	}
	if eff[0].Size != "90%" {
		t.Fatalf("merged size = %s, want 90%%", eff[0].Size)
	}
	var tools []string
	for _, tl := range eff[0].Tools {
		tools = append(tools, tl.Name)
	}
	if strings.Join(tools, ",") != "htop,gpu,gdash" {
		t.Fatalf("merged tools = %v", tools)
	}
	if !eff[0].Tools[2].Zoom {
		t.Fatal("tool settings lost in the merge")
	}
	split := decked()
	if got := names(split.Effective()); strings.Join(got, ",") != "htop,mid,gsl" {
		t.Fatalf("split effective = %v", got)
	}
	one := Config{Prefix: "ctrl+b", Title: Title{Size: "5%"}, Middle: "single", Panes: []Pane{{Name: "top", Size: "100%", Cmd: "top"}}}
	if got := one.Effective(); len(got) != 1 || got[0].Name != "top" || got[0].IsDeck() {
		t.Fatalf("a single middle pane should be left alone: %+v", got)
	}
}

func names(ps []Pane) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

// Review finding 1: middle: single must not collide with a fixed pane named
// mid, nor fold two merged tools that share a name.
func TestMiddleSingleRefusesCollisions(t *testing.T) {
	f := false
	c := Config{Prefix: "ctrl+b", Title: Title{Size: "5%"}, Middle: MiddleSingle, Panes: []Pane{
		{Name: "a", Size: "40%", Cmd: "a"},
		{Name: "b", Size: "40%", Cmd: "b"},
		{Name: "mid", Size: "5%", Cmd: "gsl status", Zoomable: &f},
	}}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "mid") {
		t.Fatalf("a fixed pane named mid with middle: single: %v", err)
	}
	d := Config{Prefix: "ctrl+b", Title: Title{Size: "5%"}, Middle: MiddleSingle, Panes: []Pane{
		{Name: "gpu", Size: "40%", Cmd: "nvtop"},
		{Name: "deck", Size: "40%", Tools: []Tool{{Name: "gpu", Views: []View{{Name: "x", Cmd: "x"}}}}},
	}}
	if err := d.Validate(); err == nil || !strings.Contains(err.Error(), "gpu") {
		t.Fatalf("two merged tools named gpu: %v", err)
	}
}
