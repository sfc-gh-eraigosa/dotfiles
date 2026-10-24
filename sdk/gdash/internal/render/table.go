package render

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/dash"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana"
)

// TableRows is the most rows a table draws; the rest are counted.
const TableRows = 8

type column struct {
	name  string // the data's name: a label, "Value" or "Value #A"
	field string // Grafana's field name (after groupBy), which organize keys on
	title string // the displayed (renamed) name
	value bool   // a number column
	unit  string
}

type tableRow struct {
	labels map[string]string
	values map[string]float64 // value column name → last value
}

// TableView draws a table panel the way Grafana's table format lays out a
// Prometheus result: a row per label set, a column per label, then the value
// ("Value", or "Value #<ref>" for each query when there are several). The
// panel's organize transformation, sort and unit overrides apply.
func TableView(p dash.Panel, series []grafana.Series, w int, o Opts) []string {
	pal := o.palette()
	lines := []string{renderLine([]seg{{fit(p.Title, w), colTitle}}, w, o)}
	cols, cells, widths := tableCells(p, series)
	if len(cells) == 0 {
		msg := p.NoValue
		if msg == "" {
			msg = "no data"
		}
		return append(lines, renderLine([]seg{{fit(msg, w), pal.Dim}}, w, o))
	}
	shrink(widths, w)

	line := func(vals []string, color string) string {
		var segs []seg
		for c, v := range vals {
			if c > 0 {
				segs = append(segs, seg{"  ", ""})
			}
			v = fit(v, widths[c])
			pad := strings.Repeat(" ", max(widths[c]-runewidth.StringWidth(v), 0))
			if cols[c].value {
				v = pad + v
			} else if c < len(vals)-1 {
				v += pad
			}
			segs = append(segs, seg{v, color})
		}
		return renderLine(mergeSegs(segs), w, o)
	}
	titles := make([]string, len(cols))
	for c, col := range cols {
		titles[c] = col.title
	}
	lines = append(lines, line(titles, colTitle))
	for r := range cells {
		if r == TableRows {
			more := "… " + strconv.Itoa(len(cells)-TableRows) + " more"
			return append(lines, renderLine([]seg{{fit(more, w), pal.Dim}}, w, o))
		}
		lines = append(lines, line(cells[r], ""))
	}
	return lines
}

// TableWidth is the width a table needs to show every cell whole (its title
// aside), for a layout deciding whether it fits a column.
func TableWidth(p dash.Panel, series []grafana.Series) int {
	_, cells, widths := tableCells(p, series)
	if len(cells) == 0 {
		return 0
	}
	return 2*(len(widths)-1) + sum(widths)
}

// tableCells is the table as text: its columns, sorted rows of formatted
// cells, and each column's natural width.
func tableCells(p dash.Panel, series []grafana.Series) ([]column, [][]string, []int) {
	tb := p.Table
	if tb == nil {
		tb = &dash.Table{}
	}
	cols, rows := tableData(p, tb, series)
	sortRows(rows, cols, tb)
	cells := make([][]string, len(rows))
	widths := make([]int, len(cols))
	for c, col := range cols {
		widths[c] = runewidth.StringWidth(col.title)
	}
	for r, row := range rows {
		cells[r] = make([]string, len(cols))
		for c, col := range cols {
			v := row.labels[col.name]
			if col.value {
				n, ok := row.values[col.name]
				if !ok {
					n = math.NaN()
				}
				v = FormatValue(n, col.unit, p.Decimals)
			}
			cells[r][c] = v
			widths[c] = max(widths[c], runewidth.StringWidth(v))
		}
	}
	return cols, cells, widths
}

func sum(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}

// tableData merges the series into rows and orders the visible columns.
func tableData(p dash.Panel, tb *dash.Table, series []grafana.Series) ([]column, []*tableRow) {
	valueName := func(ref string) string {
		if len(p.Targets) > 1 {
			return "Value #" + ref
		}
		return "Value"
	}
	var rows []*tableRow
	byKey := map[string]*tableRow{}
	labelSet := map[string]bool{}
	var valueCols []string
	seenValue := map[string]bool{}
	for _, s := range series {
		keys := make([]string, 0, len(s.Labels))
		for k := range s.Labels {
			keys = append(keys, k)
			labelSet[k] = true
		}
		sort.Strings(keys)
		var id strings.Builder
		for _, k := range keys {
			id.WriteString(k + "\x00" + s.Labels[k] + "\x00")
		}
		row := byKey[id.String()]
		if row == nil {
			row = &tableRow{labels: s.Labels, values: map[string]float64{}}
			byKey[id.String()] = row
			rows = append(rows, row)
		}
		vn := valueName(s.RefID)
		if !seenValue[vn] {
			seenValue[vn] = true
			valueCols = append(valueCols, vn)
		}
		if v, ok := s.Last(); ok {
			row.values[vn] = v
		}
	}
	labels := make([]string, 0, len(labelSet))
	for k := range labelSet {
		labels = append(labels, k)
	}
	sort.Strings(labels)

	var cols []column
	add := func(name string, value bool) {
		shown := name
		if tb.GroupBy != nil {
			calc, kept := tb.GroupBy[name]
			if !kept || (calc == "") == value {
				return // groupBy keeps its keys and its aggregated values only
			}
			if calc != "" {
				shown = name + " (" + calc + ")"
			}
		}
		if tb.Exclude[shown] {
			return
		}
		c := column{name: name, field: shown, title: shown, value: value}
		if r := tb.Rename[shown]; r != "" {
			c.title = r
		}
		if value {
			c.unit = p.Unit
			if u, ok := tb.Units[shown]; ok {
				c.unit = u
			}
			if u, ok := tb.Units[c.title]; ok {
				c.unit = u
			}
		}
		cols = append(cols, c)
	}
	for _, l := range labels {
		add(l, false)
	}
	for _, v := range valueCols {
		add(v, true)
	}
	if len(tb.Index) > 0 {
		pos := func(c column) int {
			if i, ok := tb.Index[c.field]; ok {
				return i
			}
			return len(cols) + 1000
		}
		sort.SliceStable(cols, func(i, j int) bool { return pos(cols[i]) < pos(cols[j]) })
	}
	return cols, rows
}

// sortRows applies the panel's sort: numbers by value (missing last),
// labels as text. An unknown sort column leaves the query's order.
func sortRows(rows []*tableRow, cols []column, tb *dash.Table) {
	if tb.SortBy == "" {
		return
	}
	for _, c := range cols {
		if c.title != tb.SortBy && c.field != tb.SortBy {
			continue
		}
		sort.SliceStable(rows, func(i, j int) bool {
			if c.value {
				a, aok := rows[i].values[c.name]
				b, bok := rows[j].values[c.name]
				if aok != bok {
					return aok
				}
				if tb.SortDesc {
					return a > b
				}
				return a < b
			}
			a, b := rows[i].labels[c.name], rows[j].labels[c.name]
			if tb.SortDesc {
				return a > b
			}
			return a < b
		})
		return
	}
}

// shrink narrows the widest column, one cell at a time, until the columns
// and their two-space gaps fit in w (each keeps at least one cell).
func shrink(widths []int, w int) {
	for 2*(len(widths)-1)+sum(widths) > w {
		widest := 0
		for i, x := range widths {
			if x > widths[widest] {
				widest = i
			}
		}
		if widths[widest] <= 1 {
			return // renderLine truncates what still does not fit
		}
		widths[widest]--
	}
}
