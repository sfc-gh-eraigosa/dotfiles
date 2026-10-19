package render

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/dash"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana/grafanatest"
)

var update = flag.Bool("update", false, "rewrite the golden files")

type bearer string

func (b bearer) Apply(r *http.Request) { r.Header.Set("Authorization", "Bearer "+string(b)) }

type fixturePanel struct {
	name   string
	panel  dash.Panel
	series []grafana.Series
}

// fixturePanels loads every captured panel query with its panel definition,
// and fetches its series through the real client against the fake Grafana.
func fixturePanels(t *testing.T) []fixturePanel {
	t.Helper()
	s := grafanatest.New(t)
	c := grafana.NewHTTP(s.URL, bearer("fixture"), 5*time.Second)
	files, _ := filepath.Glob(filepath.Join(grafanatest.Dir(), "queries", "*", "*.json"))
	var out []fixturePanel
	for _, f := range files {
		uid := filepath.Base(filepath.Dir(f))
		b, _ := os.ReadFile(filepath.Join(grafanatest.Dir(), "dashboards", uid+".json"))
		d, err := dash.Parse(b)
		if err != nil {
			t.Fatal(err)
		}
		var q struct {
			Panel struct {
				ID int `json:"id"`
			} `json:"panel"`
			Request struct {
				From, To string
				Queries  []grafana.Query `json:"queries"`
			} `json:"request"`
		}
		raw, _ := os.ReadFile(f)
		if err := json.Unmarshal(raw, &q); err != nil {
			t.Fatal(err)
		}
		var p dash.Panel
		for _, pp := range d.Panels {
			if pp.ID == q.Panel.ID {
				p = pp
			}
		}
		res, err := c.Query(context.Background(), grafana.QueryRequest{From: time.Now().Add(-time.Hour), To: time.Now(), Queries: q.Request.Queries})
		if err != nil {
			t.Fatal(err)
		}
		short := uid
		if len(short) > 12 {
			short = short[:8]
		}
		out = append(out, fixturePanel{name: strings.TrimSuffix(short+"-"+filepath.Base(f), ".json"), panel: p, series: res.Series})
	}
	if len(out) != 10 {
		t.Fatalf("expected the 10 captured panels, got %d", len(out))
	}
	return out
}

func draw(fp fixturePanel, w int, o Opts) []string {
	switch fp.panel.Kind {
	case dash.KindChart:
		return Chart(fp.panel, fp.series, w, ChartHeight, o)
	case dash.KindMeter:
		return Meter(fp.panel, fp.series, w, o)
	default:
		return Placeholder(fp.panel, fp.panel.Reason, w)
	}
}

func checkGolden(t *testing.T, name string, lines []string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".txt")
	got := strings.Join(lines, "\n") + "\n"
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
		t.Errorf("%s differs from golden:\n--- got\n%s--- want\n%s", name, got, want)
	}
}

// E5 (render level): every captured panel at 80 and 120 columns and at a
// single column, plain (no color) so the goldens read as text.
func TestGoldenFixturePanels(t *testing.T) {
	plain := Opts{Color: false}
	for _, fp := range fixturePanels(t) {
		for _, w := range []int{80, 120, 1} {
			checkGolden(t, fmt.Sprintf("%s-w%d", fp.name, w), draw(fp, w, plain))
		}
	}
}

func TestGoldenEdges(t *testing.T) {
	plain := Opts{}
	row := dash.Panel{Kind: dash.KindRow, Title: "Jobs"}
	collapsed := dash.Panel{Kind: dash.KindRow, Title: "Hidden things", Collapsed: true, Hidden: 3}
	lib := dash.Panel{Kind: dash.KindUnsupported, Title: "Shared panel", Reason: "library panel"}
	checkGolden(t, "row-w80", Row(row, 80, plain))
	checkGolden(t, "row-collapsed-w80", Row(collapsed, 80, plain))
	checkGolden(t, "placeholder-w80", Placeholder(lib, lib.Reason, 80))
	checkGolden(t, "needs-var-w80", Placeholder(dash.Panel{Title: "CPU"}, "needs var $instance (set views.<v>.vars.instance)", 80))
	checkGolden(t, "status-stale-w80", Status("stale 2m14s · grafana /api/ds/query: HTTP 502", 80, plain))
	empty := Chart(dash.Panel{Kind: dash.KindChart, Title: "No data yet"}, nil, 80, ChartHeight, plain)
	checkGolden(t, "chart-empty-w80", empty)
	nan := []grafana.Series{{Name: "x", Times: []time.Time{time.Unix(0, 0), time.Unix(60, 0)}, Values: []float64{math.NaN(), math.NaN()}}}
	checkGolden(t, "meter-nan-w80", Meter(dash.Panel{Kind: dash.KindMeter, Title: "Gone", Unit: "percent"}, nan, 80, plain))
}

// Whatever the width, nothing panics, no line is wider than the width,
// and a chart is always ChartHeight rows.
func TestWidthInvariant(t *testing.T) {
	fps := fixturePanels(t)
	for _, o := range []Opts{{}, {Color: true}} {
		for w := 1; w <= 130; w++ {
			for _, fp := range fps {
				lines := draw(fp, w, o)
				if fp.panel.Kind == dash.KindChart && len(lines) != ChartHeight {
					t.Fatalf("%s at %d: %d rows", fp.name, w, len(lines))
				}
				for _, l := range lines {
					if n := runewidth.StringWidth(StripANSI(l)); n > w {
						t.Fatalf("%s at %d (color %v): line is %d wide: %q", fp.name, w, o.Color, n, l)
					}
				}
			}
		}
	}
}

func TestColorOnlyWhenAsked(t *testing.T) {
	fp := fixturePanels(t)[0]
	if strings.Contains(strings.Join(draw(fp, 80, Opts{}), ""), "\x1b[") {
		t.Fatal("escape codes without Color")
	}
	if !strings.Contains(strings.Join(draw(fp, 80, Opts{Color: true}), ""), "\x1b[") {
		t.Fatal("no escape codes with Color")
	}
}

// Without color, a meter past a threshold still says so (NO_COLOR, spec F3).
func TestThresholdMarkersWithoutColor(t *testing.T) {
	v80, v90 := 80.0, 90.0
	p := dash.Panel{Kind: dash.KindMeter, Title: "Disk", Unit: "percent", Min: ptr(0), Max: ptr(100),
		Thresholds: []dash.Threshold{{Color: "green"}, {Color: "orange", Value: &v80}, {Color: "red", Value: &v90}}}
	at := func(v float64) string {
		s := []grafana.Series{{Name: "d", Times: []time.Time{time.Unix(0, 0)}, Values: []float64{v}}}
		return strings.Join(Meter(p, s, 60, Opts{}), "\n")
	}
	if strings.Contains(at(50), "!") {
		t.Fatalf("50%% marked: %s", at(50))
	}
	if !strings.Contains(at(85), " !") || strings.Contains(at(85), "!!") {
		t.Fatalf("85%% should be warn: %s", at(85))
	}
	if !strings.Contains(at(95), "!!") {
		t.Fatalf("95%% should be critical: %s", at(95))
	}
}

func ptr(f float64) *float64 { return &f }
