package render

import (
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/dash"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/theme"
)

// E1: gdash draws with the sdk's shared palette (sdk/libs/tui/theme) — the
// same colors fleet and gsl use — not a palette of its own.
func TestColorsComeFromTheSharedTheme(t *testing.T) {
	v90 := 90.0
	meter := dash.Panel{Kind: dash.KindMeter, Title: "Disk", Unit: "percent", Thresholds: []dash.Threshold{{Color: "green"}, {Color: "red", Value: &v90}}}
	crit := []grafana.Series{{Name: "d", Times: []time.Time{time.Unix(0, 0)}, Values: []float64{95}}}
	chart := dash.Panel{Kind: dash.KindChart, Title: "CPU"}
	line := []grafana.Series{{Name: "a", Times: []time.Time{time.Unix(0, 0), time.Unix(60, 0)}, Values: []float64{1, 2}}}
	for _, name := range theme.Names() {
		p, _ := theme.Get(name)
		o := Opts{Color: true, Palette: p}
		if m := strings.Join(Meter(meter, crit, 60, o), ""); !strings.Contains(m, theme.FG(p.Crit)) {
			t.Errorf("%s: a critical meter is not drawn in the palette's crit color %q: %q", name, p.Crit, m)
		}
		if c := strings.Join(Chart(chart, line, 60, ChartHeight, o), ""); !strings.Contains(c, theme.FG(p.SeriesColor(0))) {
			t.Errorf("%s: series 0 is not the palette's first series color", name)
		}
		if s := strings.Join(Status("stale 1m", 40, o), ""); !strings.Contains(s, theme.FG(p.Warn)) {
			t.Errorf("%s: the status row is not the palette's warn color", name)
		}
	}
}

// The zero Opts.Palette is the dark palette, so callers that only set Color
// still match the rest of the sdk.
func TestZeroPaletteIsDark(t *testing.T) {
	p, _ := theme.Get(theme.Dark)
	if got := (Opts{Color: true}).palette(); got.Name != p.Name {
		t.Fatalf("default palette = %q", got.Name)
	}
}
