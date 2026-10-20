package config

import (
	"strings"
	"testing"
)

// C1: the default layout (spec F8 default, F10).
func TestDefaultLayout(t *testing.T) {
	c := Default()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range c.Panes {
		names = append(names, p.Name+"="+p.Size)
	}
	if got := strings.Join(names, " "); got != "mid=30% htop=60% gsl=5%" {
		t.Fatalf("panes = %s", got)
	}
	mid := c.Panes[0]
	if !mid.IsDeck() || len(mid.Tools) != 2 || mid.Tools[0].Name != "gdash" || mid.Tools[1].Name != "gpu" {
		t.Fatalf("mid = %+v", mid)
	}
	g := mid.Tools[0]
	var views []string
	for _, v := range g.Views {
		views = append(views, v.Name+":"+v.Cmd)
	}
	if !g.Zoom || g.Probe != "gdash doctor --quiet" ||
		strings.Join(views, ",") != "conductor:gdash conductor,k3s-nodes:gdash k3s-nodes,nodes:gdash nodes,ci:gdash ci" {
		t.Fatalf("gdash tool = %+v", g)
	}
	if c.Panes[2].CanZoom() || !c.Panes[1].CanZoom() || !mid.CanZoom() {
		t.Fatal("gsl must be fixed; htop and mid part of the middle")
	}
	if c.MiddleMode() != MiddleSingle {
		t.Fatal("default middle must be single")
	}
	eff := c.Effective()
	var tools []string
	for _, t := range eff[0].Tools {
		tools = append(tools, t.Name)
	}
	if len(eff) != 2 || eff[0].Size != "90%" || strings.Join(tools, ",") != "gdash,gpu,htop" {
		t.Fatalf("default middle should open on gdash, then gpu, then htop: %+v", eff)
	}
}
