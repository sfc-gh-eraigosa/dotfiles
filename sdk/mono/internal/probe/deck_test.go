package probe

import (
	"context"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
)

func midDeck() config.Pane {
	return config.Pane{Name: "mid", Size: "30%", Tools: []config.Tool{
		{Name: "gpu", Views: []config.View{{Name: "nvtop", Cmd: "nvtop", Fallbacks: []string{"nvitop"}}}},
		{Name: "gdash", Probe: "gdash doctor --quiet", Zoom: true, Views: []config.View{
			{Name: "conductor", Cmd: "gdash conductor"},
			{Name: "ci", Cmd: "gdash ci"},
			{Name: "special", Cmd: "gdash special", Probe: "test -e /nope"},
		}},
	}}
}

func viewNames(c Choice) string {
	var out []string
	for _, v := range c.Views {
		out = append(out, v.Tool+"/"+v.View)
	}
	return strings.Join(out, " ")
}

// E17: dropping is per view, then per tool, then per pane.
func TestDeckDropsPerViewToolPane(t *testing.T) {
	nvsmi := "nvidia-smi -L >/dev/null 2>&1"
	cases := []struct {
		name  string
		env   *fakeEnv
		views string // usable views, in order; "" = pane dropped
	}{
		{"everything", &fakeEnv{path: names("nvtop", "nvidia-smi", "gdash"), ok: map[string]bool{nvsmi: true, "gdash doctor --quiet": true}}, "gpu/nvtop gdash/conductor gdash/ci"},
		{"no Grafana: gdash tool dropped", &fakeEnv{path: names("nvtop", "nvidia-smi", "gdash"), ok: map[string]bool{nvsmi: true}}, "gpu/nvtop"},
		{"no GPU: gpu tool dropped", &fakeEnv{path: names("nvtop", "gdash"), ok: map[string]bool{"gdash doctor --quiet": true}}, "gdash/conductor gdash/ci"},
		{"neither: pane dropped", &fakeEnv{path: names("nvtop")}, ""},
	}
	for _, tc := range cases {
		c := Choose(context.Background(), tc.env, []config.Pane{midDeck()})[0]
		if got := viewNames(c); got != tc.views {
			t.Errorf("%s: usable views %q, want %q", tc.name, got, tc.views)
		}
		if c.Usable() != (tc.views != "") {
			t.Errorf("%s: Usable = %v", tc.name, c.Usable())
		}
	}
}

func TestDeckToolProbeRunsOnce(t *testing.T) {
	env := &fakeEnv{path: names("gdash"), ok: map[string]bool{"gdash doctor --quiet": true}}
	Choose(context.Background(), env, []config.Pane{midDeck()})
	n := 0
	for _, r := range env.ran {
		if r == "gdash doctor --quiet" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("tool probe ran %d times, want once", n)
	}
}

func TestDeckChoiceCarriesViewSettings(t *testing.T) {
	env := &fakeEnv{path: names("gdash"), ok: map[string]bool{"gdash doctor --quiet": true}}
	c := Choose(context.Background(), env, []config.Pane{midDeck()})[0]
	if !c.Views[0].Zoom || c.Views[0].Cmd != "gdash conductor" {
		t.Fatalf("view choice = %+v", c.Views[0])
	}
	var reasons []string
	for _, d := range c.Dropped {
		reasons = append(reasons, d.Tool+"/"+d.View+": "+d.Reason)
	}
	joined := strings.Join(reasons, "; ")
	if !strings.Contains(joined, "gpu/nvtop") || !strings.Contains(joined, "gdash/special") {
		t.Fatalf("dropped views and reasons = %q", joined)
	}
}

// A plain pane still reports one view, so callers can treat every pane as a deck.
func TestPlainPaneHasOneView(t *testing.T) {
	c := Choose(context.Background(), &fakeEnv{path: names("htop")}, []config.Pane{{Name: "htop", Size: "60%", Cmd: "htop"}})[0]
	if len(c.Views) != 1 || c.Views[0].Tool != "htop" || c.Views[0].View != "htop" || c.Views[0].Cmd != "htop" {
		t.Fatalf("plain pane views = %+v", c.Views)
	}
}
