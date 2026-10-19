package vars

import (
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/dash"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana"
)

var testDS = []grafana.Datasource{
	{UID: "alertmanager", Name: "Alertmanager", Type: "alertmanager"},
	{UID: "prometheus", Name: "Prometheus", Type: "prometheus", IsDefault: true, TimeInterval: "30s"},
}

func env(t *testing.T, vs []dash.Variable, overrides map[string]string) Env {
	t.Helper()
	return New(vs, overrides, testDS, time.Hour, 120) // 1h over 120 points
}

// E3: every reference form, All, multi-value and the formats.
func TestExpand(t *testing.T) {
	vs := []dash.Variable{
		{Name: "node", Type: "query", Values: []string{"jetson1"}, Set: true},
		{Name: "multi", Type: "custom", Values: []string{"a.b", "c"}, Set: true, Multi: true},
		{Name: "all", Type: "query", Values: []string{"$__all"}, Set: true, IncludeAll: true},
		{Name: "allv", Type: "query", Values: []string{"$__all"}, Set: true, IncludeAll: true, AllValue: "job-.+"},
		{Name: "unset", Type: "query"},
		{Name: "empty", Type: "query"},
	}
	e := env(t, vs, map[string]string{"empty": ""})
	cases := []struct{ in, want string }{
		{`up{node="$node"}`, `up{node="jetson1"}`},
		{`up{node="${node}"}`, `up{node="jetson1"}`},
		{`up{node="[[node]]"}`, `up{node="jetson1"}`},
		{`up{node=~"${node:regex}"}`, `up{node=~"jetson1"}`},
		{`up{x=~"$multi"}`, `up{x=~"(a\.b|c)"}`},
		{`up{x=~"${multi:pipe}"}`, `up{x=~"a.b|c"}`},
		{`up{x=~"${multi:csv}"}`, `up{x=~"a.b,c"}`},
		{`up{x=~"${multi:raw}"}`, `up{x=~"a.b,c"}`},
		{`up{x=~"$all"}`, `up{x=~".*"}`},
		{`up{x=~"$allv"}`, `up{x=~"job-.+"}`},
		{`up{cluster="$empty"}`, `up{cluster=""}`},
		{`rate(x[$__rate_interval])`, `rate(x[2m])`}, // max(4×30s, 30s+30s)
		{`rate(x[$__interval])`, `rate(x[30s])`},
		{`x / $__range_s`, `x / 3600`},
		{`increase(x[$__range])`, `increase(x[1h])`},
	}
	for _, tc := range cases {
		got, missing := e.Expand(tc.in)
		if got != tc.want || len(missing) != 0 {
			t.Errorf("Expand(%s) = %s missing=%v, want %s", tc.in, got, missing, tc.want)
		}
	}
	for _, in := range []string{`up{x="$unset"}`, `up{x="$nosuch"}`, `up{x="${node:json}"}`, `up{x="$__nosuch"}`} {
		if _, missing := e.Expand(in); len(missing) == 0 {
			t.Errorf("Expand(%s): an unresolvable reference was not reported", in)
		}
	}
}

func TestOverrides(t *testing.T) {
	vs := []dash.Variable{{Name: "instance", Type: "query"}}
	e := env(t, vs, map[string]string{"instance": "192.0.2.66:9100"})
	if got, m := e.Expand(`up{instance="$instance"}`); got != `up{instance="192.0.2.66:9100"}` || len(m) != 0 {
		t.Fatalf("override: %s %v", got, m)
	}
	e = env(t, []dash.Variable{{Name: "c", Type: "query", IncludeAll: true}}, map[string]string{"c": "All"})
	if got, _ := e.Expand(`up{c=~"$c"}`); got != `up{c=~".*"}` {
		t.Fatalf("All override: %s", got)
	}
}

// E4: $__rate_interval per the formula; the ds scrape interval when set,
// else 15s.
func TestRateInterval(t *testing.T) {
	e := env(t, nil, nil)
	if e.Interval != 30*time.Second || e.RateInterval != 2*time.Minute {
		t.Fatalf("1h/120 with a 30s scrape: interval %v rate %v", e.Interval, e.RateInterval)
	}
	noScrape := []grafana.Datasource{{UID: "p", Name: "P", Type: "prometheus", IsDefault: true}}
	e2 := New(nil, nil, noScrape, 24*time.Hour, 120) // interval 12m
	if e2.Interval != 12*time.Minute || e2.RateInterval != 12*time.Minute+15*time.Second {
		t.Fatalf("24h/120 with the default 15s scrape: interval %v rate %v", e2.Interval, e2.RateInterval)
	}
	e3 := New(nil, nil, testDS, 5*time.Minute, 2000) // would be < 1s; floored at the 30s scrape
	if e3.Interval != 30*time.Second {
		t.Fatalf("interval below the scrape interval: %v", e3.Interval)
	}
}

func TestDatasource(t *testing.T) {
	cases := []struct {
		name string
		vars []dash.Variable
		ref  dash.DSRef
		want string
	}{
		{"var by uid", []dash.Variable{{Name: "datasource", Type: "datasource", Values: []string{"prometheus"}, Set: true}}, dash.DSRef{Type: "prometheus", UID: "${datasource}"}, "prometheus"},
		{"var by name", []dash.Variable{{Name: "datasource", Type: "datasource", Values: []string{"Prometheus"}, Set: true}}, dash.DSRef{UID: "$datasource"}, "prometheus"},
		{"var default", []dash.Variable{{Name: "datasource", Type: "datasource", Values: []string{"default"}, Set: true}}, dash.DSRef{UID: "${datasource}"}, "prometheus"},
		{"var unset", []dash.Variable{{Name: "datasource", Type: "datasource"}}, dash.DSRef{UID: "${datasource}"}, "prometheus"},
		{"plain uid", nil, dash.DSRef{Type: "prometheus", UID: "prometheus"}, "prometheus"},
		{"empty ref", nil, dash.DSRef{}, "prometheus"},
	}
	for _, tc := range cases {
		got, err := env(t, tc.vars, nil).Datasource(tc.ref)
		if err != nil || got.UID != tc.want || got.Type != "prometheus" {
			t.Errorf("%s: %+v %v", tc.name, got, err)
		}
	}
	if _, err := env(t, nil, nil).Datasource(dash.DSRef{UID: "nope"}); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("unknown datasource: %v", err)
	}
}

func TestFormatDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{30 * time.Second: "30s", 2 * time.Minute: "2m", time.Hour: "1h", 90 * time.Second: "90s", 1500 * time.Millisecond: "2s", 0: "1s"} {
		if got := FormatDuration(d); got != want {
			t.Errorf("FormatDuration(%v) = %s, want %s", d, got, want)
		}
	}
}
