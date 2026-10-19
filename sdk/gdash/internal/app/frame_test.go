package app

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/dash"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana/grafanatest"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/render"
)

var update = flag.Bool("update", false, "rewrite the golden files")

type bearer string

func (b bearer) Apply(r *http.Request) { r.Header.Set("Authorization", "Bearer "+string(b)) }

func liveModel(t *testing.T, view config.View, w, h int) *Model {
	t.Helper()
	s := grafanatest.New(t)
	c := grafana.NewHTTP(s.URL, bearer("fixture"), 5*time.Second)
	clk := &clock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	m := New(Options{Fetcher: NewLive(c, view), Range: "1h", Refresh: "30s", View: "fixture", Render: render.Opts{}, Now: clk.now, Tick: instantTick})
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	run(m, m.Init())
	return m
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".txt")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v (run with -update)", path, err)
	}
	if got != string(want) {
		t.Errorf("%s differs:\n--- got\n%s\n--- want\n%s", name, got, want)
	}
}

// E5 (app level): whole frames of real lab dashboards at 80×24, 120×40 and
// one column, through the live fetcher against the fake Grafana.
func TestFrames(t *testing.T) {
	cases := []struct {
		name string
		view config.View
		w, h int
	}{
		{"conductor-80x24", config.View{Dashboard: "conductor-metrics"}, 80, 24},
		{"conductor-120x40", config.View{Dashboard: "conductor-metrics"}, 120, 40},
		{"ci-120x40", config.View{Dashboard: "CI — lab runners"}, 120, 40},
		{"k3s-nodes-80x24", config.View{Dashboard: "058020e04168bfdea0c52269cb699df2"}, 80, 24},
		{"nodes-needs-var-80x24", config.View{Dashboard: "Node Exporter / Nodes"}, 80, 24},
		{"conductor-1x24", config.View{Dashboard: "conductor-metrics"}, 1, 24},
	}
	for _, tc := range cases {
		m := liveModel(t, tc.view, tc.w, tc.h)
		frame := m.View()
		lines := strings.Split(frame, "\n")
		if len(lines) != tc.h {
			t.Errorf("%s: %d lines, want %d", tc.name, len(lines), tc.h)
		}
		for i, l := range lines {
			if n := runewidth.StringWidth(render.StripANSI(l)); n > tc.w {
				t.Errorf("%s line %d is %d wide", tc.name, i, n)
			}
		}
		golden(t, tc.name, frame)
	}
}

func TestPanelSubset(t *testing.T) {
	m := liveModel(t, config.View{Dashboard: "conductor-metrics", Panels: []string{"Dispatch pass rate"}}, 80, 24)
	out := m.View()
	if strings.Contains(out, "── ") || strings.Count(out, "Dispatch pass rate") < 1 {
		t.Fatalf("subset should show only the named panels, no rows:\n%s", out)
	}
}

func TestUnknownDashboardIsANamedError(t *testing.T) {
	m := liveModel(t, config.View{Dashboard: "no such dashboard"}, 80, 10)
	if !strings.Contains(m.View(), "no dashboard matches") {
		t.Fatalf("frame:\n%s", m.View())
	}
}

func TestVarsOverrideReachesQueries(t *testing.T) {
	m := liveModel(t, config.View{Dashboard: "Node Exporter / Nodes", Vars: map[string]string{"cluster": "", "instance": "192.0.2.128:9100"}}, 80, 24)
	if strings.Contains(m.View(), "needs var") {
		t.Fatalf("overrides did not resolve the variables:\n%s", m.View())
	}
	fmt.Fprint(os.Stderr, "")
}

// A view can name panels inside a collapsed row, and a title that matches
// nothing says so instead of drawing nothing.
func TestPanelSubsetFindsCollapsedAndReportsMissing(t *testing.T) {
	d := dashWithCollapsedRow(t)
	l := &Live{view: config.View{Panels: []string{"inside", "no such panel"}}, d: &d}
	got := l.selected()
	if len(got) != 2 || got[0].Title != "inside" || got[1].Kind != dash.KindUnsupported || !strings.Contains(got[1].Reason, "no panel") {
		t.Fatalf("selected = %+v", got)
	}
}

func dashWithCollapsedRow(t *testing.T) dash.Dashboard {
	t.Helper()
	raw := `{"dashboard":{"panels":[{"type":"row","title":"r","collapsed":true,"gridPos":{"y":0},
		"panels":[{"type":"timeseries","title":"inside","gridPos":{"y":1}}]}]}}`
	d, err := dash.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return d
}
