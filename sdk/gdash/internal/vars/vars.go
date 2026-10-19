// Package vars expands Grafana template variables in panel queries the way
// Grafana does for one chart width and range (spec F2): $var, ${var},
// ${var:format}, [[var]], All and multi-value, and the $__interval family.
// It also resolves a panel's datasource reference to a real datasource.
package vars

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/dash"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana"
)

// DefaultScrape is the scrape interval assumed when the datasource sets none.
const DefaultScrape = 15 * time.Second

type value struct {
	set      bool
	all      bool
	allValue string
	list     []string
}

// Env is everything needed to expand one panel's queries.
type Env struct {
	Interval     time.Duration // $__interval
	RateInterval time.Duration // $__rate_interval
	Range        time.Duration // $__range

	values map[string]value
	ds     []grafana.Datasource
	dsVars map[string]bool
}

// New builds an Env for a dashboard's variables, the view's overrides, the
// datasources Grafana lists, and a range drawn in maxPoints points.
func New(vs []dash.Variable, overrides map[string]string, ds []grafana.Datasource, rng time.Duration, maxPoints int) Env {
	e := Env{Range: rng, values: map[string]value{}, ds: ds, dsVars: map[string]bool{}}
	for _, v := range vs {
		if v.Type == "datasource" {
			e.dsVars[v.Name] = true
		}
		val := value{set: v.Set, allValue: v.AllValue, list: v.Values}
		if len(v.Values) == 1 && (v.Values[0] == "$__all" || (v.IncludeAll && v.Values[0] == "All")) {
			val.all, val.list = true, nil
		}
		e.values[v.Name] = val
	}
	for name, o := range overrides {
		val := e.values[name]
		val.set = true
		if o == "All" || o == "$__all" {
			val.all, val.list = true, nil
		} else {
			val.all, val.list = false, []string{o}
		}
		e.values[name] = val
	}
	scrape := time.Duration(0)
	if d := defaultDS(ds); d != nil && d.TimeInterval != "" {
		if s, err := time.ParseDuration(d.TimeInterval); err == nil {
			scrape = s
		}
	}
	e.Interval = time.Duration(grafana.IntervalMs(rng, maxPoints, scrape)) * time.Millisecond
	if scrape == 0 {
		scrape = DefaultScrape
	}
	e.RateInterval = max(4*scrape, e.Interval+scrape)
	return e
}

func defaultDS(ds []grafana.Datasource) *grafana.Datasource {
	for i := range ds {
		if ds[i].IsDefault {
			return &ds[i]
		}
	}
	return nil
}

var refRE = regexp.MustCompile(`\$\{([^}]+)\}|\[\[([^\]]+)\]\]|\$(\w+)`)

// Expand substitutes every variable reference in expr. A declared variable
// with no value is left in place and returned, so the panel can say "needs
// var"; a $name the dashboard does not declare is left alone.
func (e Env) Expand(expr string) (string, []string) {
	missing := map[string]bool{}
	out := refRE.ReplaceAllStringFunc(expr, func(m string) string {
		sub := refRE.FindStringSubmatch(m)
		spec := sub[1] + sub[2] + sub[3]
		name, format, _ := strings.Cut(spec, ":")
		if v, ok := e.builtin(name); ok {
			return v
		}
		val, declared := e.values[name]
		if !declared {
			// Not a variable of this dashboard (a label_replace "$1", an
			// unknown builtin): Grafana leaves it as it is, and so does gdash.
			return m
		}
		if !val.set {
			missing[name] = true
			return m
		}
		s, ok := render(val, format)
		if !ok {
			missing[name+":"+format] = true
			return m
		}
		return s
	})
	names := make([]string, 0, len(missing))
	for n := range missing {
		names = append(names, n)
	}
	sort.Strings(names)
	return out, names
}

func (e Env) builtin(name string) (string, bool) {
	switch name {
	case "__interval":
		return FormatDuration(e.Interval), true
	case "__interval_ms":
		return strconv.FormatInt(e.Interval.Milliseconds(), 10), true
	case "__rate_interval":
		return FormatDuration(e.RateInterval), true
	case "__range":
		return FormatDuration(e.Range), true
	case "__range_s":
		return strconv.FormatInt(int64(e.Range.Seconds()), 10), true
	case "__range_ms":
		return strconv.FormatInt(e.Range.Milliseconds(), 10), true
	}
	return "", false
}

// render formats a value: no format and "regex" escape for a regex matcher,
// "pipe", "csv" and "raw" join; anything else is unsupported.
func render(v value, format string) (string, bool) {
	switch format {
	case "", "regex", "pipe", "csv", "raw":
	default:
		return "", false
	}
	if v.all {
		if v.allValue != "" {
			return v.allValue, true
		}
		return ".*", true
	}
	if len(v.list) == 1 {
		if format == "regex" {
			return regexp.QuoteMeta(v.list[0]), true
		}
		return v.list[0], true
	}
	switch format {
	case "pipe":
		return strings.Join(v.list, "|"), true
	case "csv", "raw":
		return strings.Join(v.list, ","), true
	}
	// The result lands inside a PromQL double-quoted string, where a lone
	// backslash is an invalid escape: double it, as Grafana does.
	esc := make([]string, len(v.list))
	for i, s := range v.list {
		esc[i] = strings.ReplaceAll(regexp.QuoteMeta(s), `\`, `\\`)
	}
	return "(" + strings.Join(esc, "|") + ")", true
}

// Datasource resolves a panel or target datasource reference: a variable
// (by uid or name, "default" or unset meaning the default datasource), a
// plain uid or name, or nothing (the default).
func (e Env) Datasource(ref dash.DSRef) (grafana.DSRef, error) {
	want := ref.UID
	if name, ok := dash.VarRef(ref.UID); ok {
		want = ""
		if v := e.values[name]; v.set && len(v.list) == 1 {
			want = v.list[0]
		}
	}
	if want == "" || strings.EqualFold(want, "default") {
		if d := defaultDS(e.ds); d != nil {
			return grafana.DSRef{Type: d.Type, UID: d.UID}, nil
		}
		return grafana.DSRef{}, fmt.Errorf("no default datasource in Grafana")
	}
	for _, d := range e.ds {
		if d.UID == want || strings.EqualFold(d.Name, want) {
			return grafana.DSRef{Type: d.Type, UID: d.UID}, nil
		}
	}
	return grafana.DSRef{}, fmt.Errorf("datasource %q not found", want)
}

// FormatDuration writes a duration the way Grafana does in queries: whole
// hours, else whole minutes, else seconds rounded up (at least 1s).
func FormatDuration(d time.Duration) string {
	s := int64(math.Ceil(d.Seconds()))
	switch {
	case s < 1:
		return "1s"
	case s%3600 == 0:
		return strconv.FormatInt(s/3600, 10) + "h"
	case s%60 == 0:
		return strconv.FormatInt(s/60, 10) + "m"
	}
	return strconv.FormatInt(s, 10) + "s"
}
