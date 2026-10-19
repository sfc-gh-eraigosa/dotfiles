// Package dash turns a Grafana dashboard's JSON into the panels gdash draws:
// charts, meters, row headers and labelled placeholders, in gridPos order
// (spec F3). It knows nothing about HTTP or terminals.
package dash

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Kind is how gdash draws a panel.
type Kind int

const (
	KindChart       Kind = iota // timeseries, graph → braille line chart
	KindMeter                   // stat, gauge, bargauge → htop-style meter
	KindRow                     // a row header
	KindUnsupported             // a one-line placeholder saying why
)

func (k Kind) String() string {
	return [...]string{"chart", "meter", "row", "unsupported"}[k]
}

// DSRef is a datasource reference as the dashboard wrote it; UID may be a
// template variable such as "${datasource}".
type DSRef struct {
	Type string `json:"type"`
	UID  string `json:"uid"`
}

// Target is one visible query of a panel.
type Target struct {
	RefID        string
	Expr         string
	LegendFormat string
	Instant      bool
	Datasource   DSRef // the target's own, else the panel's
}

// GridPos is the panel's place on Grafana's 24-column grid.
type GridPos struct{ X, Y, W, H int }

// Threshold is one step: Value nil is the base step.
type Threshold struct {
	Color string
	Value *float64
}

// Panel is one thing on screen.
type Panel struct {
	ID         int
	Title      string
	Type       string
	Kind       Kind
	Reason     string // KindUnsupported: what to tell the user
	GridPos    GridPos
	Targets    []Target
	Unit       string
	Decimals   *int
	Min, Max   *float64
	Thresholds []Threshold
	Collapsed  bool    // KindRow: a collapsed row
	Hidden     int     // KindRow: how many panels the collapsed row hides
	Children   []Panel // KindRow, collapsed: the hidden panels (drawn only when a view names them)
}

// Variable is one templating entry.
type Variable struct {
	Name       string
	Type       string   // query, custom, constant, textbox, interval, datasource
	Plugin     string   // datasource variables: the plugin type, e.g. prometheus
	Values     []string // the saved current value(s)
	Set        bool     // whether the dashboard saved a current value at all
	IncludeAll bool
	AllValue   string
	Multi      bool
}

// Dashboard is the parsed whole.
type Dashboard struct {
	UID    string
	Title  string
	Panels []Panel
	Vars   []Variable
}

type wirePanel struct {
	ID           int             `json:"id"`
	Title        string          `json:"title"`
	Type         string          `json:"type"`
	Collapsed    bool            `json:"collapsed"`
	GridPos      GridPos         `json:"gridPos"`
	Datasource   json.RawMessage `json:"datasource"`
	LibraryPanel json.RawMessage `json:"libraryPanel"`
	Panels       []wirePanel     `json:"panels"`
	Targets      []struct {
		RefID        string          `json:"refId"`
		Expr         string          `json:"expr"`
		LegendFormat string          `json:"legendFormat"`
		Instant      bool            `json:"instant"`
		Hide         bool            `json:"hide"`
		Datasource   json.RawMessage `json:"datasource"`
	} `json:"targets"`
	FieldConfig struct {
		Defaults struct {
			Unit       string   `json:"unit"`
			Decimals   *int     `json:"decimals"`
			Min        *float64 `json:"min"`
			Max        *float64 `json:"max"`
			Thresholds struct {
				Steps []struct {
					Color string   `json:"color"`
					Value *float64 `json:"value"`
				} `json:"steps"`
			} `json:"thresholds"`
		} `json:"defaults"`
	} `json:"fieldConfig"`
}

// Parse reads /api/dashboards/uid/<uid> output (or a bare dashboard object).
func Parse(raw []byte) (Dashboard, error) {
	var env struct {
		Dashboard json.RawMessage `json:"dashboard"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return Dashboard{}, fmt.Errorf("dashboard JSON: %w", err)
	}
	body := env.Dashboard
	if len(body) == 0 {
		body = raw
	}
	var w struct {
		UID        string      `json:"uid"`
		Title      string      `json:"title"`
		Panels     []wirePanel `json:"panels"`
		Templating struct {
			List []struct {
				Name       string          `json:"name"`
				Type       string          `json:"type"`
				Query      json.RawMessage `json:"query"`
				Current    json.RawMessage `json:"current"`
				IncludeAll bool            `json:"includeAll"`
				AllValue   *string         `json:"allValue"`
				Multi      bool            `json:"multi"`
			} `json:"list"`
		} `json:"templating"`
	}
	if err := json.Unmarshal(body, &w); err != nil {
		return Dashboard{}, fmt.Errorf("dashboard JSON: %w", err)
	}
	d := Dashboard{UID: w.UID, Title: w.Title}
	for _, v := range w.Templating.List {
		vr := Variable{Name: v.Name, Type: v.Type, IncludeAll: v.IncludeAll, Multi: v.Multi}
		if v.AllValue != nil {
			vr.AllValue = *v.AllValue
		}
		if v.Type == "datasource" {
			_ = json.Unmarshal(v.Query, &vr.Plugin)
		}
		vr.Values, vr.Set = currentValues(v.Current)
		if v.Type == "constant" && !vr.Set {
			var q string
			if json.Unmarshal(v.Query, &q) == nil {
				vr.Values, vr.Set = []string{q}, true
			}
		}
		d.Vars = append(d.Vars, vr)
	}
	panels := append([]wirePanel(nil), w.Panels...)
	sort.SliceStable(panels, func(i, j int) bool {
		a, b := panels[i].GridPos, panels[j].GridPos
		return a.Y < b.Y || (a.Y == b.Y && a.X < b.X)
	})
	for _, p := range panels {
		d.Panels = append(d.Panels, convert(p, d.Vars))
	}
	return d, nil
}

// currentValues reads a variable's saved current value: a string or a list.
func currentValues(raw json.RawMessage) ([]string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var c struct {
		Value json.RawMessage `json:"value"`
	}
	if json.Unmarshal(raw, &c) != nil || len(c.Value) == 0 || string(c.Value) == "null" {
		return nil, false
	}
	var one string
	if json.Unmarshal(c.Value, &one) == nil {
		return []string{one}, true
	}
	var many []string
	if json.Unmarshal(c.Value, &many) == nil {
		return many, true
	}
	return nil, false
}

// dsRef reads a datasource field: an object, or a legacy bare name.
func dsRef(raw json.RawMessage) DSRef {
	var r DSRef
	if len(raw) == 0 || string(raw) == "null" {
		return r
	}
	if json.Unmarshal(raw, &r) == nil {
		return r
	}
	var name string
	if json.Unmarshal(raw, &name) == nil {
		r.UID = name
	}
	return r
}

func convert(p wirePanel, vars []Variable) Panel {
	out := Panel{ID: p.ID, Title: p.Title, Type: p.Type, GridPos: p.GridPos}
	fd := p.FieldConfig.Defaults
	out.Unit, out.Decimals, out.Min, out.Max = fd.Unit, fd.Decimals, fd.Min, fd.Max
	for _, s := range fd.Thresholds.Steps {
		out.Thresholds = append(out.Thresholds, Threshold{Color: s.Color, Value: s.Value})
	}
	panelDS := dsRef(p.Datasource)
	for _, t := range p.Targets {
		if t.Hide {
			continue
		}
		ds := dsRef(t.Datasource)
		if ds.UID == "" && ds.Type == "" {
			ds = panelDS
		}
		out.Targets = append(out.Targets, Target{RefID: t.RefID, Expr: t.Expr, LegendFormat: t.LegendFormat, Instant: t.Instant, Datasource: ds})
	}
	assignRefIDs(out.Targets)
	switch {
	case p.Type == "row":
		out.Kind, out.Collapsed, out.Hidden = KindRow, p.Collapsed, len(p.Panels)
		for _, c := range p.Panels {
			out.Children = append(out.Children, convert(c, vars))
		}
		return out
	case len(p.LibraryPanel) > 0 && string(p.LibraryPanel) != "null":
		out.Kind, out.Reason = KindUnsupported, "library panel"
		return out
	case p.Type == "timeseries" || p.Type == "graph":
		out.Kind = KindChart
	case p.Type == "stat" || p.Type == "gauge" || p.Type == "bargauge":
		out.Kind = KindMeter
	default:
		out.Kind, out.Reason = KindUnsupported, p.Type+" not supported"
		return out
	}
	for _, t := range out.Targets {
		if typ := dsType(t.Datasource, vars); typ != "prometheus" {
			out.Kind, out.Reason = KindUnsupported, typ+" datasource not supported"
			return out
		}
	}
	return out
}

// dsType is a target's datasource plugin type. A "${var}" uid takes the
// datasource variable's plugin; an unset type defaults to prometheus (the
// only kind these dashboards use; gdash checks again when it resolves).
func dsType(ds DSRef, vars []Variable) string {
	if name, ok := varRef(ds.UID); ok {
		for _, v := range vars {
			if v.Name == name && v.Type == "datasource" && v.Plugin != "" {
				return v.Plugin
			}
		}
	}
	if ds.Type == "" || ds.Type == "datasource" {
		return "prometheus"
	}
	return ds.Type
}

// varRef reports whether s is exactly a variable reference, and its name.
func varRef(s string) (string, bool) {
	switch {
	case strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}"):
		return strings.SplitN(s[2:len(s)-1], ":", 2)[0], true
	case strings.HasPrefix(s, "$") && len(s) > 1:
		return s[1:], true
	case strings.HasPrefix(s, "[[") && strings.HasSuffix(s, "]]"):
		return s[2 : len(s)-2], true
	}
	return "", false
}

// VarRef is varRef for other packages.
func VarRef(s string) (string, bool) { return varRef(s) }

// assignRefIDs gives targets without a refId the first free letter, A, B, C…,
// as Grafana does when it loads a dashboard.
func assignRefIDs(ts []Target) {
	taken := map[string]bool{}
	for _, t := range ts {
		taken[t.RefID] = true
	}
	next := 0
	for i := range ts {
		if ts[i].RefID != "" {
			continue
		}
		for {
			id := refName(next)
			next++
			if !taken[id] {
				ts[i].RefID, taken[id] = id, true
				break
			}
		}
	}
}

// refName is A…Z, then AA, AB… like Grafana's.
func refName(n int) string {
	s := ""
	for n >= 0 {
		s = string(rune('A'+n%26)) + s
		n = n/26 - 1
	}
	return s
}
