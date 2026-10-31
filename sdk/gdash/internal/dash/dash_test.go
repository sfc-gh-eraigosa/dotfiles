package dash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		"ci-lab":                           {"CI — lab runners", 5, 10, 18, 0}, // 16 stat + 2 bargauge; 4 tables
		"7d57716318ee0dddbac5a7f451fb7753": {"Node Exporter / Nodes", 4, 6, 1, 0},
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
		if uid == "ci-lab" && k[KindTable] != 4 {
			t.Errorf("ci-lab: %d tables, want 4", k[KindTable])
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
}

// A table keeps what gdash needs to lay it out like Grafana: the organize
// transformation, the sort (a sortBy transformation or the panel option)
// and per-column unit overrides.
func TestTablePanel(t *testing.T) {
	var top *Panel
	for _, p := range load(t, "ci-lab").Panels {
		if p.ID == 32 {
			top = &p
		}
	}
	if top == nil || top.Kind != KindTable || top.Table == nil {
		t.Fatalf("panel 32 = %+v", top)
	}
	tb := top.Table
	if !tb.Exclude["Time"] || tb.Rename["Value"] != "mean" || tb.Rename["job_name"] != "job" {
		t.Fatalf("organize = %+v", tb)
	}
	if tb.SortBy != "mean" || !tb.SortDesc {
		t.Fatalf("sort = %q desc %v", tb.SortBy, tb.SortDesc)
	}
	var disk *Panel
	for _, p := range load(t, "7d57716318ee0dddbac5a7f451fb7753").Panels {
		if p.ID == 9 {
			disk = &p
		}
	}
	if g := disk.Table.GroupBy; g["mountpoint"] != "" || g["Value #A"] != "lastNotNull" || len(g) != 3 {
		t.Fatalf("groupBy = %+v", g)
	}
	if disk.Table.SortBy != "Mounted on" || disk.Table.SortDesc {
		t.Fatalf("disk sort = %q", disk.Table.SortBy)
	}
	raw := `{"panels":[{"type":"table","title":"Waiting","gridPos":{"y":1},
	  "options":{"sortBy":[{"displayName":"Waiting","desc":true}]},
	  "transformations":[{"id":"organize","options":{"excludeByName":{"job":true},"indexByName":{"task":0,"Value":1},"renameByName":{"Value":"Waiting"}}}],
	  "fieldConfig":{"defaults":{"unit":"none"},"overrides":[{"matcher":{"id":"byName","options":"Waiting"},"properties":[{"id":"unit","value":"s"},{"id":"custom.width","value":90}]}]},
	  "targets":[{"refId":"A","expr":"x","instant":true,"format":"table"}]}]}`
	d, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	tb = d.Panels[0].Table
	if d.Panels[0].Kind != KindTable || tb.SortBy != "Waiting" || !tb.SortDesc || !tb.Exclude["job"] {
		t.Fatalf("table = %+v", d.Panels[0])
	}
	if tb.Index["task"] != 0 || tb.Index["Value"] != 1 || tb.Units["Waiting"] != "s" {
		t.Fatalf("index/units = %+v", tb)
	}
}

// Options that change what a chart or meter shows: noValue text, stacking.
func TestDisplayOptions(t *testing.T) {
	raw := `{"panels":[
	  {"type":"stat","title":"Tier-3","gridPos":{"y":1},"fieldConfig":{"defaults":{"noValue":"no tier-3 runs"}}},
	  {"type":"timeseries","title":"Calls","gridPos":{"y":2},"fieldConfig":{"defaults":{"custom":{"stacking":{"group":"A","mode":"normal"}}}}},
	  {"type":"timeseries","title":"Plain","gridPos":{"y":3},"fieldConfig":{"defaults":{"custom":{"stacking":{"mode":"none"}}}}}]}`
	d, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if d.Panels[0].NoValue != "no tier-3 runs" {
		t.Fatalf("noValue = %q", d.Panels[0].NoValue)
	}
	if !d.Panels[1].Stacked || d.Panels[2].Stacked {
		t.Fatalf("stacked = %v, %v", d.Panels[1].Stacked, d.Panels[2].Stacked)
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

// Saved dashboards may leave refId out; Grafana assigns A, B, C… on load,
// and gdash must too, or a multi-target query has duplicate empty refIds
// (found live on Kubernetes / Nodes Overview: HTTP 400).
func TestMissingRefIDsAreAssigned(t *testing.T) {
	d := load(t, "058020e04168bfdea0c52269cb699df2")
	for _, p := range d.Panels {
		seen := map[string]bool{}
		for _, tg := range p.Targets {
			if tg.RefID == "" || seen[tg.RefID] {
				t.Fatalf("%s: refIds %+v", p.Title, p.Targets)
			}
			seen[tg.RefID] = true
		}
	}
	raw := `{"dashboard":{"panels":[{"type":"timeseries","targets":[{"refId":"B","expr":"a"},{"expr":"b"},{"expr":"c"}]}]}}`
	d2, _ := Parse([]byte(raw))
	got := []string{}
	for _, tg := range d2.Panels[0].Targets {
		got = append(got, tg.RefID)
	}
	if strings.Join(got, ",") != "B,A,C" {
		t.Fatalf("refIds = %v, want B,A,C (skip the taken letter)", got)
	}
}

// A dashboard's own time picker is the window its panels were written for.
func TestParseDefaultRange(t *testing.T) {
	cases := map[string]time.Duration{
		`"time":{"from":"now-7d","to":"now"}`:               7 * 24 * time.Hour,
		`"time":{"from":"now-15m","to":"now"}`:              15 * time.Minute,
		`"time":{"from":"now-7d/d","to":"now"}`:             0, // rounded: not a plain span
		`"time":{"from":"now-2d","to":"now-1d"}`:            0, // ends in the past
		`"time":{"from":"2026-10-01T00:00:00Z","to":"now"}`: 0, // absolute
		`"time":{"from":"now-1y","to":"now"}`:               0, // a unit gdash has no duration for
		`"timezone":"utc"`:                                  0, // no time block at all
	}
	for body, want := range cases {
		d, err := Parse([]byte(`{"title":"t",` + body + `}`))
		if err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		if d.Range != want {
			t.Errorf("%s: Range = %v, want %v", body, d.Range, want)
		}
	}
}
