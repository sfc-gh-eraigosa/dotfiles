package cmd

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
)

func TestConfigDeckVerbs(t *testing.T) {
	p := withConfig(t)
	if err := config.Save(p, deckConfig()); err != nil {
		t.Fatal(err)
	}
	steps := [][]string{
		{"config", "add-view", "mid", "gdash", "k3s-nodes", "--", "gdash", "k3s-nodes"},
		{"config", "add-tool", "mid", "logs", "--view", "journal", "--", "journalctl", "-f"},
		{"config", "set", "panes.mid.tools.gdash.views.ci.cmd", "gdash ci --range 6h"},
		{"config", "remove-view", "mid", "gdash", "conductor"},
		{"config", "remove-tool", "mid", "logs"},
		{"config", "set", "middle", "single"},
	}
	for _, s := range steps {
		if out, err := run(t, s...); err != nil {
			t.Fatalf("%v: %v\n%s", s, err, out)
		}
	}
	c, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	var views []string
	for _, v := range c.Panes[1].Tools[1].Views {
		views = append(views, v.Name+"="+v.Cmd)
	}
	if got := strings.Join(views, ","); got != "ci=gdash ci --range 6h,k3s-nodes=gdash k3s-nodes" {
		t.Fatalf("gdash views = %s", got)
	}
	if len(c.Panes[1].Tools) != 2 || c.Middle != "single" {
		t.Fatalf("tools %d, middle %q", len(c.Panes[1].Tools), c.Middle)
	}
	if _, err := run(t, "config", "remove-view", "mid", "gpu", "nvtop"); err == nil {
		t.Fatal("removing a tool's last view should be refused")
	}
}
