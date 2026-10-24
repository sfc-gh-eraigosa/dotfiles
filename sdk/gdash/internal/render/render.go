// Package render draws gdash panels as terminal text: braille line charts,
// htop-style meters, row headers, placeholders and a status row (spec F3).
// It is pure — a panel and its series in, lines out — so every frame is a
// golden file and no terminal is needed to test it.
package render

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/dash"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/theme"
)

// ChartHeight is a chart's rows: title, six plot rows, legend (spec F3).
const ChartHeight = 8

// Opts are the drawing choices that do not depend on the data.
type Opts struct {
	Color   bool          // ANSI color escapes; off under NO_COLOR
	Palette theme.Palette // the sdk's shared scheme (sdk/libs/tui/theme); zero = dark
}

func (o Opts) palette() theme.Palette {
	if o.Palette.Name != "" {
		return o.Palette
	}
	p, _ := theme.Get(theme.Dark)
	return p
}

// colTitle marks a bold title segment (no color of its own).
const colTitle = "bold"

type seg struct {
	text  string
	color string // "" = none; colTitle = bold; else a theme color
}

// renderLine joins segments, truncated to w display columns.
func renderLine(segs []seg, w int, o Opts) string {
	var b strings.Builder
	left := w
	for _, s := range segs {
		if left <= 0 {
			break
		}
		t := s.text
		if runewidth.StringWidth(t) > left {
			t = runewidth.Truncate(t, left, "")
		}
		left -= runewidth.StringWidth(t)
		if t == "" {
			continue
		}
		if o.Color && s.color != "" {
			if s.color == colTitle {
				b.WriteString("\x1b[1m" + t + theme.Reset)
			} else {
				b.WriteString(theme.Paint(t, s.color))
			}
			continue
		}
		b.WriteString(t)
	}
	return b.String()
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// StripANSI removes color escapes (for measuring and for tests).
func StripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= w {
		return s
	}
	if w == 1 {
		return runewidth.Truncate(s, 1, "")
	}
	return runewidth.Truncate(s, w, "…")
}

// Row is a row header: "── Jobs ────", or "▸ Title (3 hidden) ───" when collapsed.
func Row(p dash.Panel, w int, o Opts) []string {
	label := "── " + p.Title + " "
	if p.Collapsed {
		label = "▸ " + p.Title + " (" + strconv.Itoa(p.Hidden) + " hidden) "
	}
	label = fit(label, w)
	pad := w - runewidth.StringWidth(label)
	if pad < 0 {
		pad = 0
	}
	return []string{renderLine([]seg{{label, colTitle}, {strings.Repeat("─", pad), o.palette().Dim}}, w, o)}
}

// Placeholder is one line: "▢ <title> — <reason>".
func Placeholder(p dash.Panel, reason string, w int) []string {
	return []string{fit("▢ "+p.Title+" — "+reason, w)}
}

// Status is the status row (staleness, errors, the refresh clock).
func Status(text string, w int, o Opts) []string {
	return []string{renderLine([]seg{{fit(text, w), o.palette().Warn}}, w, o)}
}

// legendNames names each series: the target's legendFormat when it has one,
// else the labels that differ between the panel's series, else the metric.
func legendNames(p dash.Panel, series []grafana.Series) []string {
	formats := map[string]string{}
	for _, t := range p.Targets {
		formats[t.RefID] = t.LegendFormat
	}
	differ := map[string]bool{}
	for _, s := range series {
		for k, v := range s.Labels {
			if k == "__name__" {
				continue
			}
			for _, o := range series {
				if o.Labels[k] != v {
					differ[k] = true
				}
			}
		}
	}
	keys := make([]string, 0, len(differ))
	for k := range differ {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	names := make([]string, len(series))
	for i, s := range series {
		if f := formats[s.RefID]; f != "" && f != "__auto" {
			names[i] = legendRE.ReplaceAllStringFunc(f, func(m string) string {
				return s.Labels[strings.TrimSpace(m[2:len(m)-2])]
			})
			continue
		}
		var parts []string
		for _, k := range keys {
			parts = append(parts, k+"="+s.Labels[k])
		}
		switch {
		case len(parts) > 0:
			names[i] = strings.Join(parts, " ")
		case s.Labels["__name__"] != "":
			names[i] = s.Labels["__name__"]
		default:
			names[i] = s.Name
		}
	}
	return names
}

var legendRE = regexp.MustCompile(`\{\{\s*[^}]+\s*\}\}`)

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Chart draws a braille line chart: a title row, h-2 plot rows with min/max
// on a y-axis, and a legend with each series' last value.
//
// A legend too wide for one line wraps onto more (up to maxLegendLines),
// and the plot gives those rows up so the chart keeps its height.
func Chart(p dash.Panel, series []grafana.Series, w, h int, o Opts) []string {
	if h < 3 {
		h = 3
	}
	legend := chartLegend(p, series, w, min(maxLegendLines, h-2), o)
	plotRows := h - 1 - len(legend)
	lines := make([]string, 0, h)
	lines = append(lines, renderLine([]seg{{fit(p.Title, w), colTitle}}, w, o))

	plotted := series
	if p.Stacked {
		plotted = stack(series)
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	var t0, t1 time.Time
	for _, s := range plotted {
		for i, v := range s.Values {
			if !finite(v) || i >= len(s.Times) {
				continue
			}
			lo, hi = math.Min(lo, v), math.Max(hi, v)
			if t0.IsZero() || s.Times[i].Before(t0) {
				t0 = s.Times[i]
			}
			if s.Times[i].After(t1) {
				t1 = s.Times[i]
			}
		}
	}
	if math.IsInf(lo, 1) { // no data at all
		for r := 0; r < h-2; r++ {
			text := ""
			if r == (h-2)/2 {
				text = centre("no data", w)
			}
			lines = append(lines, renderLine([]seg{{text, o.palette().Dim}}, w, o))
		}
		return append(lines, "")
	}
	if p.Min != nil {
		lo = math.Min(lo, *p.Min)
	}
	if p.Max != nil {
		hi = math.Max(hi, *p.Max)
	}
	if hi == lo {
		hi = lo + 1
	}
	hiLabel, loLabel := FormatValue(hi, p.Unit, p.Decimals), FormatValue(lo, p.Unit, p.Decimals)
	lw := max(runewidth.StringWidth(hiLabel), runewidth.StringWidth(loLabel))
	pw := w - lw - 1
	if pw < 4 { // too narrow for an axis: plot only
		lw, pw = 0, w
	}

	dw, dh := pw*2, plotRows*4
	cells := make([][]rune, plotRows)
	colors := make([][]string, plotRows)
	for r := range cells {
		cells[r] = make([]rune, pw)
		colors[r] = make([]string, pw)
	}
	span := t1.Sub(t0)
	xOf := func(t time.Time) int {
		if span <= 0 {
			return dw / 2
		}
		return int(math.Round(float64(t.Sub(t0)) / float64(span) * float64(dw-1)))
	}
	yOf := func(v float64) int { // 0 = top
		return dh - 1 - int(math.Round((v-lo)/(hi-lo)*float64(dh-1)))
	}
	dot := func(x, y int, color string) {
		if x < 0 || y < 0 || x >= dw || y >= dh {
			return
		}
		r, c := y/4, x/2
		cells[r][c] |= brailleBit(x%2, y%4)
		colors[r][c] = color
	}
	pal := o.palette()
	for si, s := range plotted {
		color := pal.SeriesColor(si)
		px, py, have := 0, 0, false
		for i, v := range s.Values {
			if i >= len(s.Times) {
				break
			}
			if !finite(v) {
				have = false
				continue
			}
			x, y := xOf(s.Times[i]), yOf(v)
			if have {
				line(px, py, x, y, func(a, b int) { dot(a, b, color) })
			} else {
				dot(x, y, color)
			}
			px, py, have = x, y, true
		}
	}
	for r := 0; r < plotRows; r++ {
		var segs []seg
		if lw > 0 {
			label := ""
			switch r {
			case 0:
				label = hiLabel
			case plotRows - 1:
				label = loLabel
			}
			segs = append(segs, seg{strings.Repeat(" ", lw-runewidth.StringWidth(label)) + label, pal.Dim}, seg{"│", pal.Dim})
		}
		for c := 0; c < pw; c++ {
			ch := ' '
			if cells[r][c] != 0 {
				ch = 0x2800 + cells[r][c]
			}
			segs = append(segs, seg{string(ch), colors[r][c]})
		}
		lines = append(lines, renderLine(mergeSegs(segs), w, o))
	}

	return append(lines, legend...)
}

// maxLegendLines is the most rows a chart's legend may take from its plot.
const maxLegendLines = 3

// chartLegend lays out "● name value" entries, wrapping between entries,
// in at most maxLines lines; the last line is cut if they still do not fit.
func chartLegend(p dash.Panel, series []grafana.Series, w, maxLines int, o Opts) []string {
	if len(series) == 0 {
		return []string{""}
	}
	maxLines = max(maxLines, 1)
	pal := o.palette()
	names := legendNames(p, series)
	var rows [][]seg
	var cur []seg
	curW := 0
	for i, s := range series {
		last, ok := s.Last()
		val := "—"
		if ok {
			val = FormatValue(last, p.Unit, p.Decimals)
		}
		text := " " + names[i] + " " + val
		ew := 1 + runewidth.StringWidth(text)
		if curW > 0 && curW+2+ew > w && len(rows) < maxLines-1 {
			rows, cur, curW = append(rows, cur), nil, 0
		}
		if curW > 0 {
			cur = append(cur, seg{"  ", ""})
			curW += 2
		}
		cur = append(cur, seg{"●", pal.SeriesColor(i)}, seg{text, ""})
		curW += ew
	}
	rows = append(rows, cur)
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = renderLine(r, w, o)
	}
	return out
}

// mergeSegs joins neighbouring segments of the same color (fewer escapes).
func mergeSegs(in []seg) []seg {
	var out []seg
	for _, s := range in {
		if n := len(out); n > 0 && out[n-1].color == s.color {
			out[n-1].text += s.text
			continue
		}
		out = append(out, s)
	}
	return out
}

func centre(s string, w int) string {
	s = fit(s, w)
	pad := (w - runewidth.StringWidth(s)) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + s
}

// brailleBit is the dot bit for column cx (0..1), row cy (0..3) of a cell.
func brailleBit(cx, cy int) rune {
	if cx == 0 {
		return [...]rune{0x01, 0x02, 0x04, 0x40}[cy]
	}
	return [...]rune{0x08, 0x10, 0x20, 0x80}[cy]
}

// line plots the points between two dots (Bresenham).
func line(x0, y0, x1, y1 int, plot func(x, y int)) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	e := dx + dy
	for {
		plot(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * e
		if e2 >= dy {
			e += dy
			x0 += sx
		}
		if e2 <= dx {
			e += dx
			y0 += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Meter draws one htop-style row per series: "label [||||    42.1%]". A panel
// with no known maximum (not a percentage, no max set) shows the value alone.
func Meter(p dash.Panel, series []grafana.Series, w int, o Opts) []string {
	if len(series) == 0 {
		return []string{renderLine([]seg{{fit(p.Title+"  "+noValue(p), w), o.palette().Dim}}, w, o)}
	}
	names := legendNames(p, series)
	lo, hi, bar := 0.0, 0.0, false
	switch {
	case p.Max != nil:
		hi, bar = *p.Max, true
	case p.Unit == "percent":
		hi, bar = 100, true
	case p.Unit == "percentunit":
		hi, bar = 1, true
	}
	if p.Min != nil {
		lo = *p.Min
	}
	if !bar && p.Type == "bargauge" { // Grafana scales bars to the largest value
		for _, s := range series {
			if v, ok := s.Last(); ok && v > hi {
				hi = v
			}
		}
		bar = hi > lo
	}
	// Rows share a label column and a value column so their bars line up
	// and compare (a bar gauge's rows are one chart).
	type meterRow struct {
		label, val, mark, color string
		v                       float64
		ok                      bool
	}
	pal := o.palette()
	rows := make([]meterRow, len(series))
	labelW, valW := 0, 0
	for i, s := range series {
		r := meterRow{label: p.Title}
		if len(series) > 1 {
			r.label = names[i]
		}
		r.v, r.ok = s.Last()
		if !r.ok {
			r.v = math.NaN()
		}
		r.val = FormatValue(r.v, p.Unit, p.Decimals)
		if !r.ok && p.NoValue != "" {
			r.val = p.NoValue
		}
		lvl := thresholdLevel(p.Thresholds, r.v)
		if !o.Color {
			switch lvl {
			case levelWarn:
				r.mark = " !"
			case levelCrit:
				r.mark = " !!"
			}
		}
		r.color = map[level]string{levelOK: pal.OK, levelWarn: pal.Warn, levelCrit: pal.Crit}[lvl]
		labelW = max(labelW, runewidth.StringWidth(r.label))
		if r.ok {
			valW = max(valW, runewidth.StringWidth(r.val+r.mark))
		}
		rows[i] = r
	}
	labelW = min(max(w*3/10, 1), labelW)
	barW := w - labelW - 1 - 2 - valW - 1
	out := make([]string, 0, len(series))
	for _, r := range rows {
		label := fit(r.label, labelW)
		if len(rows) > 1 {
			label += strings.Repeat(" ", labelW-runewidth.StringWidth(label))
		}
		if !bar || barW < 3 || !r.ok {
			out = append(out, renderLine([]seg{{label + " ", ""}, {r.val, r.color}, {r.mark, ""}}, w, o))
			continue
		}
		frac := (r.v - lo) / (hi - lo)
		frac = math.Max(0, math.Min(1, frac))
		filled := int(math.Round(frac * float64(barW)))
		pad := valW - runewidth.StringWidth(r.val+r.mark)
		out = append(out, renderLine([]seg{
			{label + " [", ""},
			{strings.Repeat("|", filled), r.color},
			{strings.Repeat(" ", barW-filled+1+pad), ""},
			{r.val, r.color},
			{r.mark + "]", ""},
		}, w, o))
	}
	return out
}

// noValue is what a panel shows for no value: its noValue text, else "—".
func noValue(p dash.Panel) string {
	if p.NoValue != "" {
		return p.NoValue
	}
	return "—"
}

// stack returns the series with each one's points raised by the sum of the
// series before it at the same instant (Grafana's normal stacking; a null
// point adds nothing and stays null).
func stack(series []grafana.Series) []grafana.Series {
	below := map[int64]float64{}
	out := make([]grafana.Series, len(series))
	for i, s := range series {
		s.Values = append([]float64(nil), s.Values...)
		for j, v := range s.Values {
			if j >= len(s.Times) || !finite(v) {
				continue
			}
			t := s.Times[j].UnixMilli()
			below[t] += v
			s.Values[j] = below[t]
		}
		out[i] = s
	}
	return out
}

// thresholdLevel is the level of the last step at or below v.
func thresholdLevel(steps []dash.Threshold, v float64) level {
	if len(steps) == 0 || !finite(v) {
		return levelNone
	}
	l := levelNone
	for _, s := range steps {
		if s.Value == nil || v >= *s.Value {
			l = colorLevel(s.Color)
		}
	}
	return l
}
