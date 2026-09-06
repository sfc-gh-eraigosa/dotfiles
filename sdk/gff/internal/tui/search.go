package tui

import (
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/search"
)

// rowKey identifies a row across buildRows rebuilds (indices shift when an
// area expands; keys do not). Used to anchor Esc-restore and first-match.
func rowKey(r row) string {
	if r.isArea {
		return "area:" + r.ns + "\x00" + r.area
	}
	return "item:" + strconv.Itoa(r.itemIdx)
}

// inScope is the single visibility rule shared by buildRows and search: the
// All page shows every namespace; a category page shows its component within
// the breadcrumb's namespace.
func (m *Model) inScope(item resolve.Resolved) bool {
	if m.pageIdx <= 0 || m.pageIdx >= len(m.pages) {
		return true
	}
	if componentOf(item.Feature.GetPath()) != m.pages[m.pageIdx].component {
		return false
	}
	return m.scopeNS == "" || item.Namespace() == m.scopeNS
}

func (m *Model) rowIndexOf(key string) int {
	for i, r := range m.rows {
		if rowKey(r) == key {
			return i
		}
	}
	return 0
}

func matchesItem(re interface{ MatchString(string) bool }, it resolve.Resolved) bool {
	return re.MatchString(it.Feature.GetPath()) || re.MatchString(it.Feature.GetDescription())
}

// hit is the search haystack: a feature row whose path OR description matches.
func (m *Model) hit(i int) bool {
	r := m.rows[i]
	return !r.isArea && m.search.Re != nil && matchesItem(m.search.Re, r.item)
}

// collect refreshes the match set for the current rows (buildRows calls it).
func (m *Model) collect() { m.search.Collect(len(m.rows), m.hit) }

func (m *Model) startSearch() {
	m.search.Start(m.cur.Pos, m.cur.Top)
	m.searchAnchor = ""
	if m.cur.Pos < len(m.rows) {
		m.searchAnchor = rowKey(m.rows[m.cur.Pos])
	}
	m.mode = modeSearch
}

// applySearch reveals matches (expanding areas on the All page), rebuilds the
// rows, and parks the cursor on the first hit at or after the anchor.
func (m *Model) applySearch() {
	if m.search.Re != nil && m.pageIdx == 0 {
		for _, it := range m.items {
			if matchesItem(m.search.Re, it) {
				m.expanded[it.Namespace()+"\x00"+areaOf(it.Feature.GetPath())] = true
			}
		}
	}
	m.buildRows() // → SetLen + collect()
	anchor := m.rowIndexOf(m.searchAnchor)
	if i, ok := m.search.First(anchor); ok {
		m.cur.To(i)
	} else {
		m.cur.To(anchor)
	}
	m.rescope()
}

func (m *Model) commitSearch() {
	m.mode = modeList
	committed, notFound := m.search.Commit()
	switch {
	case !committed:
		m.errMsg = "invalid pattern: " + m.search.Err
		m.search.Err = ""
	case notFound:
		m.errMsg = "pattern not found: " + m.search.Pattern
	}
}

func (m *Model) cancelSearch() {
	m.mode = modeList
	_, top := m.search.Cancel()
	m.buildRows()
	m.cur.To(m.rowIndexOf(m.searchAnchor))
	m.cur.Top = top
	m.rescope()
}

// jump is n (+1) / N (-1), re-arming after :noh.
func (m *Model) jump(dir int) {
	if m.search.Pattern == "" {
		return
	}
	if !m.search.Visible && !m.search.Rearm() {
		return
	}
	m.collect()
	i, ok := m.search.Next(m.cur.Pos, dir)
	if !ok {
		m.errMsg = "pattern not found: " + m.search.Pattern
		return
	}
	m.cur.To(i)
	m.errMsg = ""
	m.rescope()
}

// noh is Esc in the list: hide highlights, keep the pattern.
func (m *Model) noh() {
	m.search.Hide()
	m.errMsg = ""
}

// updateSearch handles keys while the / prompt is open.
func (m *Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.search.Key(msg) {
	case search.Cancelled:
		m.cancelSearch()
	case search.Submitted:
		m.commitSearch()
	case search.Typed:
		m.applySearch()
	}
	return m, nil
}
