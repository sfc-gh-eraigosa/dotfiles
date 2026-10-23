package render

import (
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/dash"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana"
)

func one(ref string, v float64, labels map[string]string) grafana.Series {
	return grafana.Series{RefID: ref, Name: "Value", Labels: labels, Times: []time.Time{time.Unix(60, 0)}, Values: []float64{v}}
}

// The second-chance "Third-chance questions waiting" table: labels become
// columns, organize hides and renames, the override unit formats the value,
// and the sort puts the longest wait first.
func TestTableView(t *testing.T) {
	p := dash.Panel{Kind: dash.KindTable, Title: "Third-chance questions waiting", Targets: []dash.Target{{RefID: "A"}},
		Table: &dash.Table{
			Exclude: map[string]bool{"job": true, "__name__": true},
			Rename:  map[string]string{"task": "Task", "question": "Question", "Value": "Waiting"},
			Index:   map[string]int{},
			SortBy:  "Waiting", SortDesc: true,
			Units: map[string]string{"Waiting": "s"},
		}}
	series := []grafana.Series{
		one("A", 90, map[string]string{"__name__": "q", "job": "c", "task": "t-1", "question": "which model?"}),
		one("A", 7200, map[string]string{"__name__": "q", "job": "c", "task": "t-2", "question": "retry?"}),
	}
	got := TableView(p, series, 60, Opts{})
	want := []string{
		"Third-chance questions waiting",
		"Question      Task  Waiting",
		"retry?        t-2      2.0h",
		"which model?  t-1     1.5min",
	}
	if got[0] != want[0] || strings.TrimRight(got[1], " ") != want[1] {
		t.Fatalf("header:\n%s", strings.Join(got, "\n"))
	}
	if !strings.HasPrefix(got[2], "retry?") || !strings.HasPrefix(got[3], "which model?") {
		t.Fatalf("sort by Waiting desc:\n%s", strings.Join(got, "\n"))
	}
	if !strings.Contains(got[2], FormatValue(7200, "s", nil)) {
		t.Fatalf("unit override not applied:\n%s", strings.Join(got, "\n"))
	}
	for _, l := range got {
		if strings.Contains(l, "job") || strings.Contains(l, "__name__") {
			t.Fatalf("excluded column shown:\n%s", strings.Join(got, "\n"))
		}
	}
}

// Several queries in one table merge rows by label set, a "Value #<ref>"
// column each, as Grafana's table format does.
func TestTableMergesQueries(t *testing.T) {
	p := dash.Panel{Kind: dash.KindTable, Title: "Disk", Unit: "decbytes", Targets: []dash.Target{{RefID: "A"}, {RefID: "B"}}, Table: &dash.Table{}}
	series := []grafana.Series{
		one("A", 1e9, map[string]string{"mountpoint": "/"}),
		one("B", 4e8, map[string]string{"mountpoint": "/"}),
		one("A", 2e9, map[string]string{"mountpoint": "/data"}),
	}
	got := TableView(p, series, 80, Opts{})
	if !strings.Contains(got[1], "Value #A") || !strings.Contains(got[1], "Value #B") {
		t.Fatalf("header: %q", got[1])
	}
	if len(got) != 4 {
		t.Fatalf("want title, header, 2 rows:\n%s", strings.Join(got, "\n"))
	}
	gb, mb := FormatValue(1e9, "decbytes", nil), FormatValue(4e8, "decbytes", nil)
	if !strings.Contains(got[2], gb) || !strings.Contains(got[2], mb) || !strings.Contains(got[3], "—") {
		t.Fatalf("rows:\n%s", strings.Join(got, "\n"))
	}
}

// A long table stops at TableRows and says how many it left out; an empty
// one says so; a narrow one shrinks its widest column, never the width.
func TestTableLimits(t *testing.T) {
	p := dash.Panel{Kind: dash.KindTable, Title: "Jobs", NoValue: "nothing waiting", Targets: []dash.Target{{RefID: "A"}}, Table: &dash.Table{}}
	if got := TableView(p, nil, 40, Opts{}); len(got) != 2 || !strings.Contains(got[1], "nothing waiting") {
		t.Fatalf("empty: %q", got)
	}
	var many []grafana.Series
	for i := 0; i < TableRows+3; i++ {
		many = append(many, one("A", float64(i), map[string]string{"job": strings.Repeat("x", 30) + string(rune('a'+i))}))
	}
	got := TableView(p, many, 30, Opts{})
	if len(got) != 2+TableRows+1 || !strings.Contains(got[len(got)-1], "3 more") {
		t.Fatalf("long:\n%s", strings.Join(got, "\n"))
	}
	for w := 1; w <= 40; w++ {
		for _, l := range TableView(p, many, w, Opts{Color: true}) {
			if n := runewidth.StringWidth(StripANSI(l)); n > w {
				t.Fatalf("width %d: %q is %d wide", w, l, n)
			}
		}
	}
}

// noValue replaces the dash a stat shows when its query returns nothing.
func TestMeterNoValue(t *testing.T) {
	p := dash.Panel{Kind: dash.KindMeter, Title: "Tier-3 rescue 7d", Unit: "percentunit", NoValue: "no tier-3 runs"}
	if got := Meter(p, nil, 60, Opts{}); !strings.Contains(got[0], "no tier-3 runs") {
		t.Fatalf("got %q", got)
	}
}

// A bar gauge with no max draws every bar against the largest value, as
// Grafana does, rather than printing bare numbers.
func TestBarGaugeScalesToLargest(t *testing.T) {
	p := dash.Panel{Kind: dash.KindMeter, Type: "bargauge", Title: "Tier funnel", Unit: "short",
		Targets: []dash.Target{{RefID: "A", LegendFormat: "exhausted"}, {RefID: "B", LegendFormat: "tier 2 tried"}, {RefID: "C", LegendFormat: "tier 3 tried"}}}
	series := []grafana.Series{one("A", 4, nil), one("B", 2, nil), one("C", 0, nil)}
	got := Meter(p, series, 60, Opts{})
	if len(got) != 3 {
		t.Fatalf("rows: %q", got)
	}
	bars := func(l string) int { return strings.Count(l, "|") }
	if !strings.Contains(got[0], "[") || bars(got[0]) <= bars(got[1]) || bars(got[1]) <= bars(got[2]) || bars(got[2]) != 0 {
		t.Fatalf("bars not proportional:\n%s", strings.Join(got, "\n"))
	}
	if bars(got[1])*2 < bars(got[0])-1 || bars(got[1])*2 > bars(got[0])+1 {
		t.Fatalf("2 should be half of 4:\n%s", strings.Join(got, "\n"))
	}
	stat := p
	stat.Type = "stat"
	if strings.Contains(Meter(stat, series, 60, Opts{})[0], "[") {
		t.Fatal("a stat without a max still shows the value alone")
	}
}

// A stacked chart plots each series on top of the ones before it, so its
// axis tops out at the sum; the legend still shows each series' own value.
func TestStackedChart(t *testing.T) {
	ts := []time.Time{time.Unix(0, 0), time.Unix(60, 0)}
	series := []grafana.Series{
		{Name: "a", Times: ts, Values: []float64{1, 1}},
		{Name: "b", Times: ts, Values: []float64{2, 2}},
	}
	p := dash.Panel{Kind: dash.KindChart, Title: "Calls", Unit: "short", Stacked: true}
	got := Chart(p, series, 60, ChartHeight, Opts{})
	if !strings.HasPrefix(strings.TrimSpace(got[1]), "3") {
		t.Fatalf("axis should top at 3:\n%s", strings.Join(got, "\n"))
	}
	if legend := got[len(got)-1]; !strings.Contains(legend, "a 1") || !strings.Contains(legend, "b 2") {
		t.Fatalf("legend: %q", legend)
	}
	p.Stacked = false
	if got := Chart(p, series, 60, ChartHeight, Opts{}); !strings.HasPrefix(strings.TrimSpace(got[1]), "2") {
		t.Fatalf("unstacked axis should top at 2:\n%s", strings.Join(got, "\n"))
	}
}

// A legend too long for one line wraps (whole entries, never cut mid-name)
// and the plot gives up rows so the chart keeps its height.
func TestLegendWraps(t *testing.T) {
	ts := []time.Time{time.Unix(0, 0), time.Unix(60, 0)}
	var series []grafana.Series
	names := []string{"agy · external", "conductor-review · external", "dispatch · external", "dispatch · internal", "draft · internal"}
	for i, n := range names {
		series = append(series, grafana.Series{Name: n, Times: ts, Values: []float64{float64(i), float64(i + 1)}})
	}
	got := Chart(dash.Panel{Kind: dash.KindChart, Title: "Calls"}, series, 60, ChartHeight, Opts{})
	if len(got) != ChartHeight {
		t.Fatalf("%d rows", len(got))
	}
	all := strings.Join(got, "\n")
	for i, n := range names {
		if !strings.Contains(all, n+" "+FormatValue(float64(i+1), "", nil)) {
			t.Fatalf("legend entry %q missing:\n%s", n, all)
		}
	}
}

// TableWidth is the width a table needs to show every cell whole.
func TestTableWidth(t *testing.T) {
	p := dash.Panel{Kind: dash.KindTable, Title: "T", Targets: []dash.Target{{RefID: "A"}}, Table: &dash.Table{}}
	series := []grafana.Series{one("A", 7, map[string]string{"task": "270-long-task-name"})}
	want := runewidth.StringWidth("270-long-task-name") + 2 + runewidth.StringWidth("Value")
	if got := TableWidth(p, series); got != want {
		t.Fatalf("TableWidth = %d, want %d", got, want)
	}
	for _, l := range TableView(p, series, want, Opts{}) {
		if strings.Contains(l, "…") {
			t.Fatalf("cut at its own width: %q", l)
		}
	}
}
