package app

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/dash"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/vars"
)

// Data is one fetched dashboard, ready to draw.
type Data struct {
	Title  string
	Panels []PanelData
}

// PanelData is a panel with what came back for it.
type PanelData struct {
	Panel   dash.Panel
	Series  []grafana.Series
	Err     string   // the query failed, or Grafana reported an error
	Missing []string // template variables it could not resolve
	Reason  string   // set when resolution made the panel unsupported
}

// Fetcher gets a dashboard's data for a chart width and a range. reload asks
// for the dashboard JSON again rather than the cached copy.
type Fetcher interface {
	Fetch(ctx context.Context, width int, rng time.Duration, reload bool) (Data, error)
}

// Live fetches from a Grafana: the dashboard (resolved and cached), its
// datasources, then every panel's queries, at most four at a time.
type Live struct {
	c    grafana.Client
	view config.View
	uid  string
	d    *dash.Dashboard
	ds   []grafana.Datasource
}

// NewLive is a Fetcher for one view.
func NewLive(c grafana.Client, view config.View) *Live { return &Live{c: c, view: view} }

// Fetch implements Fetcher.
func (l *Live) Fetch(ctx context.Context, width int, rng time.Duration, reload bool) (Data, error) {
	if l.uid == "" {
		uid, err := grafana.Resolve(ctx, l.c, l.view.Dashboard)
		if err != nil {
			return Data{}, err
		}
		l.uid = uid
	}
	if l.d == nil || reload {
		raw, err := l.c.Dashboard(ctx, l.uid)
		if err != nil {
			return Data{}, err
		}
		d, err := dash.Parse(raw)
		if err != nil {
			return Data{}, err
		}
		ds, err := l.c.Datasources(ctx)
		if err != nil {
			return Data{}, err
		}
		l.d, l.ds = &d, ds
	}
	panels := l.selected()
	maxDP := grafana.MaxDataPoints(width)
	env := vars.New(l.d.Vars, l.view.Vars, l.ds, rng, maxDP)
	now := time.Now()
	out := Data{Title: l.d.Title, Panels: make([]PanelData, len(panels))}
	var reqs []grafana.QueryRequest
	var idx []int
	for i, p := range panels {
		pd := PanelData{Panel: p}
		if p.Kind == dash.KindChart || p.Kind == dash.KindMeter {
			q, missing, reason := l.queries(p, env, maxDP)
			pd.Missing, pd.Reason = missing, reason
			if len(missing) == 0 && reason == "" && len(q) > 0 {
				reqs = append(reqs, grafana.QueryRequest{From: now.Add(-rng), To: now, Queries: q})
				idx = append(idx, i)
			}
		}
		out.Panels[i] = pd
	}
	results := grafana.FetchAll(ctx, l.c, reqs)
	failed := 0
	var firstErr string
	for k, res := range results {
		pd := &out.Panels[idx[k]]
		pd.Series = res.Series
		if len(res.Errors) > 0 {
			var msgs []string
			for _, m := range res.Errors {
				msgs = append(msgs, m)
			}
			sort.Strings(msgs)
			pd.Err = strings.Join(msgs, "; ")
			if res.Errors["*"] != "" {
				failed++
				if firstErr == "" {
					firstErr = res.Errors["*"]
				}
			}
		}
	}
	// Every request failed outright (Grafana down, credential refused):
	// that is a fetch failure, so the screen keeps its last good data.
	if len(results) > 0 && failed == len(results) {
		return out, errors.New(firstErr)
	}
	return out, nil
}

// selected is the view's panels: all of them, or only the titled subset
// (without row headers).
func (l *Live) selected() []dash.Panel {
	if len(l.view.Panels) == 0 {
		return l.d.Panels
	}
	want := map[string]bool{}
	for _, t := range l.view.Panels {
		want[t] = true
	}
	var out []dash.Panel
	for _, p := range l.d.Panels {
		if p.Kind != dash.KindRow && want[p.Title] {
			out = append(out, p)
		}
	}
	return out
}

func (l *Live) queries(p dash.Panel, env vars.Env, maxDP int) ([]grafana.Query, []string, string) {
	var out []grafana.Query
	var missing []string
	seen := map[string]bool{}
	for _, t := range p.Targets {
		expr, miss := env.Expand(t.Expr)
		for _, m := range miss {
			if !seen[m] {
				seen[m] = true
				missing = append(missing, m)
			}
		}
		ds, err := env.Datasource(t.Datasource)
		if err != nil {
			return nil, nil, err.Error()
		}
		if ds.Type != "prometheus" {
			return nil, nil, ds.Type + " datasource not supported"
		}
		out = append(out, grafana.Query{
			RefID: t.RefID, Datasource: ds, Expr: expr,
			IntervalMs: env.Interval.Milliseconds(), MaxDataPoints: maxDP,
			Instant: t.Instant, Range: !t.Instant,
		})
	}
	return out, missing, ""
}
