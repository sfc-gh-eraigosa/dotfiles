package dash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana/grafanatest"
)

func load(t *testing.T, uid string) Dashboard {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(grafanatest.Dir(), "dashboards", uid+".json"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func kinds(d Dashboard) map[Kind]int {
	m := map[Kind]int{}
	for _, p := range d.Panels {
		m[p.Kind]++
	}
	return m
}

func TestParseFixtures(t *testing.T) {
	cases := map[string]struct {
		title                          string
		row, chart, meter, unsupported int
	}{
		"conductor-metrics":                {"Conductor — metrics against goals", 3, 8, 6, 0},
		"ci-lab":                           {"CI — lab runners", 5, 10, 18, 4}, // 16 stat + 2 bargauge; 4 tables
		"7d57716318ee0dddbac5a7f451fb7753": {"Node Exporter / Nodes", 4, 6, 1, 1},
		"058020e04168bfdea0c52269cb699df2": {"Kubernetes / Compute Resources / Nodes Overview", 0, 5, 0, 0},
	}
	for uid, want := range cases {
		d := load(t, uid)
		k := kinds(d)
		if d.Title != want.title || d.UID != uid {
			t.Errorf("%s: title %q uid %q", uid, d.Title, d.UID)
		}
		if k[KindRow] != want.row || k[KindChart] != want.chart || k[KindMeter] != want.meter || k[KindUnsupported] != want.unsupported {
			t.Errorf("%s: kinds %v, want row %d chart %d meter %d unsupported %d", uid, k, want.row, want.chart, want.meter, want.unsupported)
		}
	}
}

func TestOrderIsGridPos(t *testing.T) {
	d := load(t, "ci-lab")
	for i := 1; i < len(d.Panels); i++ {
		a, b := d.Panels[i-1].GridPos, d.Panels[i].GridPos
		if a.Y > b.Y || (a.Y == b.Y && a.X > b.X) {
			t.Fatalf("panel %d (%s) at %+v comes after %+v", i, d.Panels[i].Title, b, a)
		}
	}
}

func TestPanelDetails(t *testing.T) {
	d := load(t, "conductor-metrics")
	var stat *Panel
	for i := range d.Panels {
		if d.Panels[i].Type == "stat" {
			stat = &d.Panels[i]
			break
		}
	}
	if stat == nil || stat.Unit != "percentunit" || stat.Decimals == nil || *stat.Decimals != 2 {
		t.Fatalf("stat panel = %+v", stat)
	}
	if len(stat.Thresholds) != 2 || stat.Thresholds[1].Value == nil || *stat.Thresholds[1].Value != 0.6 {
		t.Fatalf("thresholds = %+v", stat.Thresholds)
	}
	if len(stat.Targets) == 0 || stat.Targets[0].Expr == "" || stat.Targets[0].Datasource.UID != "${datasource}" {
		t.Fatalf("targets = %+v", stat.Targets)
	}
	unsupported := 0
	for _, p := range load(t, "ci-lab").Panels {
		if p.Kind == KindUnsupported {
			unsupported++
			if p.Reason != "table not supported" {
				t.Errorf("reason = %q", p.Reason)
			}
		}
	}
	if unsupported == 0 {
		t.Fatal("no unsupported panels found in ci-lab")
	}
}

func TestVariables(t *testing.T) {
	d := load(t, "7d57716318ee0dddbac5a7f451fb7753")
	got := map[string]Variable{}
	for _, v := range d.Vars {
		got[v.Name] = v
	}
	if got["cluster"].Type != "query" || !got["cluster"].IncludeAll || got["cluster"].Set {
		t.Fatalf("cluster = %+v", got["cluster"])
	}
	if got["datasource"].Type != "datasource" || got["datasource"].Plugin != "prometheus" {
		t.Fatalf("datasource var = %+v", got["datasource"])
	}
	c := load(t, "ci-lab")
	if v := c.Vars[0]; !v.Set || len(v.Values) != 1 || v.Values[0] != "prometheus" {
		t.Fatalf("ci datasource current = %+v", v)
	}
}

func TestEdgeShapes(t *testing.T) {
	raw := map[string]any{"dashboard": map[string]any{
		"uid": "x", "title": "edges",
		"panels": []any{
			map[string]any{"type": "row", "title": "hidden", "collapsed": true, "gridPos": map[string]any{"y": 0},
				"panels": []any{map[string]any{"type": "timeseries", "title": "inside", "gridPos": map[string]any{"y": 1}}}},
			map[string]any{"type": "timeseries", "title": "lib", "libraryPanel": map[string]any{"uid": "l"}, "gridPos": map[string]any{"y": 2}},
			map[string]any{"type": "timeseries", "title": "loki", "datasource": map[string]any{"type": "loki", "uid": "l"},
				"targets": []any{map[string]any{"refId": "A", "expr": "{job=\"x\"}"}}, "gridPos": map[string]any{"y": 3}},
			map[string]any{"type": "graph", "title": "old graph", "gridPos": map[string]any{"y": 4},
				"targets": []any{map[string]any{"refId": "A", "expr": "up", "hide": true}, map[string]any{"refId": "B", "expr": "up"}}},
		}}}
	b, _ := json.Marshal(raw)
	d, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Panels) != 4 {
		t.Fatalf("panels = %d (a collapsed row hides its children)", len(d.Panels))
	}
	if !d.Panels[0].Collapsed || d.Panels[0].Hidden != 1 {
		t.Fatalf("collapsed row = %+v", d.Panels[0])
	}
	if d.Panels[1].Kind != KindUnsupported || d.Panels[1].Reason != "library panel" {
		t.Fatalf("library panel = %+v", d.Panels[1])
	}
	if d.Panels[2].Kind != KindUnsupported || d.Panels[2].Reason != "loki datasource not supported" {
		t.Fatalf("loki panel = %+v", d.Panels[2])
	}
	if d.Panels[3].Kind != KindChart || len(d.Panels[3].Targets) != 1 || d.Panels[3].Targets[0].RefID != "B" {
		t.Fatalf("graph panel: hidden targets must be dropped: %+v", d.Panels[3])
	}
	if _, err := Parse([]byte("not json")); err == nil {
		t.Fatal("garbage parsed")
	}
}
