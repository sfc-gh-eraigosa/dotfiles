// Package tui implements the bubbletea TUI for gff: a browsable, collapsible
// tree of feature flags with in-place toggle and provenance display.
package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/overrides"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/cmdline"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/nav"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/search"
)

// screenMode controls what the main loop renders.
type screenMode int

const (
	modeList    screenMode = iota // normal collapsible tree view
	modePicker                    // option picker overlay (choice flags)
	modeDetail                    // per-feature detail: attributes + layer provenance
	modeHelp                      // help overlay (?/F1 from any view)
	modeSearch                    // the / prompt is open (every key is text)
	modeCommand                   // the : prompt is open (every key is text)
)

// SourceInfo is one registry entry for the launch panel.
type SourceInfo struct {
	Namespace string
	URL       string
	Commit    string
}

// page is one breadcrumb entry: the All view or one second-path-segment
// category (alphabetical), navigated with ←/→.
type page struct {
	label     string
	component string // "" = All
}

// row is a rendered line in the list: either an area header or a feature row.
type row struct {
	isArea  bool
	area    string           // set when isArea == true
	ns      string           // owning namespace (area AND feature rows)
	item    resolve.Resolved // set when isArea == false
	itemIdx int              // index into m.items (for toggle)
}

// pickerEntry is one option in the picker overlay.
type pickerEntry struct {
	opt      *gffv1.ChoiceOption
	selected bool
}

// Model is the bubbletea model for the gff TUI.
//
// Layout: areas are collapsed by default; pressing Enter on an area header
// expands/collapses it. Within an expanded area each feature occupies one row.
// Space on a bool row toggles the value via overrides.Write. Space on a choice
// row opens a picker overlay. q quits.
type Model struct {
	items    []resolve.Resolved
	p        paths.Paths
	width    int
	height   int
	cur      nav.Cursor      // cursor + viewport over m.rows (libs/tui/nav)
	expanded map[string]bool // area name → expanded
	rows     []row           // flattened render list rebuilt on each expand/collapse
	mode     screenMode

	// picker state
	pickerItemIdx int           // which m.items entry is being picked
	pickerEntries []pickerEntry // copy of options with transient selection state
	pickerCursor  int
	pickerIsMulti bool

	// errMsg surfaces a failed override write in the footer; cleared on the
	// next successful write. A failed write must never flip the row.
	errMsg string

	// Explain provides the per-layer provenance story for the detail view.
	// Wired by cmd to resolve.Resolver.Explain; nil => definition-only detail.
	Explain func(key string) (resolve.Resolved, []resolve.LayerInfo, error)

	// Sources is the registry listing rendered on the launch panel so it is
	// clear where each area's flags come from. Wired by cmd.
	Sources []SourceInfo

	// breadcrumb pager
	pages   []page
	pageIdx int

	// detail state
	detailItem   resolve.Resolved
	detailLayers []resolve.LayerInfo
	detailIdx    int // index into m.items backing the detail view

	// search state: the sdk state machine plus gff's row anchor (indices
	// shift when an area expands; the key survives a rebuild).
	search       search.State
	searchAnchor string

	// command line: declared here so the view can render the : prompt;
	// the verbs are registered in command.go.
	cmd cmdline.State
	reg cmdline.Registry

	scopeNS      string     // namespace the breadcrumb pages derive from
	helpReturn   screenMode // view to return to when help closes
	pickerReturn screenMode // view to return to when the picker closes
}

// NewModel constructs a Model from a resolved item slice and a Paths for writes.
// The w parameter is kept for future/test use; pass nil for normal operation.
func NewModel(items []resolve.Resolved, p paths.Paths) *Model {
	m := &Model{
		items:    items,
		p:        p,
		expanded: make(map[string]bool),
	}
	m.buildRows()
	if len(m.rows) > 0 {
		m.scopeNS = m.rows[0].ns
	}
	m.buildPages()
	return m
}

// componentOf returns the second dotted segment of a canonical key path.
func componentOf(path string) string {
	parts := strings.Split(path, ".")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

// buildPages derives the breadcrumb for the current namespace scope: the All
// page first, then one page per distinct second path segment, alphabetically.
func (m *Model) buildPages() {
	areas := map[string]bool{}
	comps := map[string]bool{}
	for _, item := range m.items {
		if m.scopeNS != "" && item.Namespace() != m.scopeNS {
			continue
		}
		areas[areaOf(item.Feature.GetPath())] = true
		if c := componentOf(item.Feature.GetPath()); c != "" {
			comps[c] = true
		}
	}
	allLabel := "(All)"
	if len(areas) == 1 {
		for a := range areas {
			allLabel = a + " (All)"
		}
	}
	m.pages = []page{{label: allLabel}}
	names := make([]string, 0, len(comps))
	for c := range comps {
		names = append(names, c)
	}
	sort.Strings(names)
	for _, c := range names {
		m.pages = append(m.pages, page{label: c, component: c})
	}
}

// Init satisfies tea.Model; no initial commands needed.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update handles key events and window resizes.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modePicker:
			return m.updatePicker(msg)
		case modeDetail:
			return m.updateDetail(msg)
		case modeHelp:
			return m.updateHelp(msg)
		case modeSearch:
			return m.updateSearch(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

// qualifiedKey returns the §3.2 fully-qualified form <namespace>:<key> for an
// item. Every Explain call MUST use it: unqualified keys bind focus-first (the
// CWD repo), which is the WRONG item when the same path exists in two sources
// (owner-reported: drilling into a snapshot row showed the repo's detail).
func qualifiedKey(item resolve.Resolved) string {
	if ns := item.Namespace(); ns != "" {
		return ns + ":" + item.Feature.GetPath()
	}
	return item.Feature.GetPath()
}

// rescope re-derives the breadcrumb pages when the cursor crosses into a row
// owned by a different namespace (All page only — category pages are already
// single-namespace).
func (m *Model) rescope() {
	if m.pageIdx != 0 || m.cur.Pos < 0 || m.cur.Pos >= len(m.rows) {
		return
	}
	if ns := m.rows[m.cur.Pos].ns; ns != "" && ns != m.scopeNS {
		m.scopeNS = ns
		m.buildPages()
	}
}

// openDetail enters the detail view for a feature row, resolving the
// per-layer story through the Explain hook when wired.
func (m *Model) openDetail(r row) {
	m.detailIdx = r.itemIdx
	if m.Explain != nil {
		res, layers, err := m.Explain(qualifiedKey(r.item))
		if err != nil {
			m.errMsg = "explain failed: " + err.Error()
			return
		}
		m.detailItem, m.detailLayers = res, layers
	} else {
		m.detailItem, m.detailLayers = r.item, nil
	}
	m.errMsg = ""
	m.mode = modeDetail
}

// updateDetail handles key events in the detail view. Space acts through the
// SAME writers the list uses (overrides.Write via toggle/picker); 'u' clears
// the user override via overrides.Unset — no new write paths.
func (m *Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape, tea.KeyEnter:
		m.mode = modeList

	case tea.KeyF1:
		m.helpReturn = modeDetail
		m.mode = modeHelp

	case tea.KeySpace:
		m.detailAct()

	case tea.KeyRunes:
		if len(msg.Runes) > 0 {
			switch msg.Runes[0] {
			case 'q', 'Q':
				m.mode = modeList
			case '?':
				m.helpReturn = modeDetail
				m.mode = modeHelp
			case ' ':
				m.detailAct()
			case 'u', 'U':
				if err := overrides.Unset(m.p, m.detailItem.Feature.GetPath()); err != nil {
					m.errMsg = "unset failed: " + err.Error()
					return m, nil
				}
				m.errMsg = ""
				m.refreshDetail()
			}
		}
	}
	return m, nil
}

// detailAct is Space in the detail view: toggle a bool in place, or open the
// picker for a choice (returning here on confirm/cancel).
func (m *Model) detailAct() {
	item := m.detailItem
	switch item.Feature.Default.(type) {
	case *gffv1.Feature_BoolDefault:
		newVal := &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: !item.Value.GetBoolValue()}}
		if err := overrides.Write(m.p, item.Feature.GetPath(), newVal); err != nil {
			m.errMsg = "write failed: " + err.Error()
			return
		}
		m.errMsg = ""
		m.refreshDetail()
	case *gffv1.Feature_ChoiceDefault:
		m.openPicker(m.detailIdx, item, modeDetail)
	}
}

// refreshDetail re-resolves the detail item after a write so the layer table
// tells the current truth, and mirrors the new state into the list items.
func (m *Model) refreshDetail() {
	if m.Explain != nil {
		if res, layers, err := m.Explain(qualifiedKey(m.detailItem)); err == nil {
			m.detailItem, m.detailLayers = res, layers
			if m.detailIdx >= 0 && m.detailIdx < len(m.items) {
				m.items[m.detailIdx] = res
			}
			m.buildRows()
		}
	}
}

// refreshItem re-resolves one item after a write/unset so the row tells the
// current truth (needs the Explain hook; without it the row goes stale until
// the next full resolve).
func (m *Model) refreshItem(idx int) {
	if m.Explain == nil || idx < 0 || idx >= len(m.items) {
		return
	}
	if res, _, err := m.Explain(qualifiedKey(m.items[idx])); err == nil {
		m.items[idx] = res
		m.buildRows()
	}
}

// updateHelp closes the overlay back to wherever it was opened from.
func (m *Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape, tea.KeyEnter:
		m.mode = m.helpReturn
	case tea.KeyRunes:
		if len(msg.Runes) > 0 {
			switch msg.Runes[0] {
			case 'q', 'Q', '?':
				m.mode = m.helpReturn
			}
		}
	}
	return m, nil
}

// updateList handles key events in list mode. Motions and the vim grammar
// come from libs/tui (gffKeys = keymap.Vim + gff's own actions); only the
// gff-specific actions are handled here.
func (m *Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.cur.Key(msg, gffKeys) { // j/k/gg/G/^d/^u/^f/^b and the arrows/PgUp/PgDn
		m.rescope()
		return m, nil
	}
	if a, ok := gffKeys.Lookup(msg); ok {
		switch a {
		case keymap.PageLeft:
			m.turnPage(-1)
			return m, nil
		case keymap.PageRight:
			m.turnPage(1)
			return m, nil
		case keymap.Help:
			m.helpReturn = modeList
			m.mode = modeHelp
			return m, nil
		case keymap.Quit:
			return m, tea.Quit
		case keymap.Search:
			m.startSearch()
			return m, nil
		case keymap.NextMatch:
			m.jump(1)
			return m, nil
		case keymap.PrevMatch:
			m.jump(-1)
			return m, nil
		case keymap.ClearHighlight:
			m.noh()
			return m, nil
		}
	}
	switch msg.Type {
	case tea.KeyEnter:
		if m.cur.Pos >= 0 && m.cur.Pos < len(m.rows) {
			r := m.rows[m.cur.Pos]
			if r.isArea {
				// Toggle expand/collapse (namespace-qualified group key).
				k := r.ns + "\x00" + r.area
				m.expanded[k] = !m.expanded[k]
				m.buildRows() // SetLen keeps the cursor in-bounds after the rebuild
			} else {
				// Feature row: open the detail view (attributes + layers).
				m.openDetail(r)
			}
		}

	// Real terminals deliver the spacebar as KeySpace (bubbletea key.go maps
	// the ' ' rune to it); KeyRunes{' '} only occurs from synthetic input.
	case tea.KeySpace:
		if m.cur.Pos >= 0 && m.cur.Pos < len(m.rows) {
			r := m.rows[m.cur.Pos]
			if !r.isArea {
				return m.activateFeature(r)
			}
		}

	case tea.KeyRunes:
		if len(msg.Runes) == 0 {
			break
		}
		switch msg.Runes[0] {
		case 'u', 'U':
			// Clear the user override for the cursor row — same
			// overrides.Unset path as `gff unset` and the detail view.
			if m.cur.Pos >= 0 && m.cur.Pos < len(m.rows) {
				r := m.rows[m.cur.Pos]
				if !r.isArea {
					if err := overrides.Unset(m.p, r.item.Feature.GetPath()); err != nil {
						m.errMsg = "unset failed: " + err.Error()
						return m, nil
					}
					m.errMsg = ""
					m.refreshItem(r.itemIdx)
				}
			}

		case ' ':
			if m.cur.Pos >= 0 && m.cur.Pos < len(m.rows) {
				r := m.rows[m.cur.Pos]
				if !r.isArea {
					return m.activateFeature(r)
				}
			}
		}
	}
	return m, nil
}

// turnPage moves dir pages through the breadcrumb with wraparound.
func (m *Model) turnPage(dir int) {
	n := len(m.pages)
	if n <= 1 {
		return
	}
	m.pageIdx = ((m.pageIdx+dir)%n + n) % n
	m.buildRows()
	m.cur.To(0)
	m.cur.Top = 0
}

// activateFeature handles Space on a feature row.
func (m *Model) activateFeature(r row) (tea.Model, tea.Cmd) {
	item := r.item
	switch item.Feature.Default.(type) {
	case *gffv1.Feature_BoolDefault:
		// Toggle: flip the current effective bool value.
		cur := item.Value.GetBoolValue()
		newVal := &gffv1.Value{
			Kind: &gffv1.Value_BoolValue{BoolValue: !cur},
		}
		if err := overrides.Write(m.p, item.Feature.GetPath(), newVal); err != nil {
			m.errMsg = "write failed: " + err.Error()
			return m, nil // file unchanged — do not flip the row
		}
		m.errMsg = ""
		// Refresh the item in place — WithValue preserves the unexported
		// namespace; a bare literal would drop it and the row would vanish
		// from namespace-scoped pages (owner-reported bug).
		m.items[r.itemIdx] = item.WithValue(newVal, resolve.LayerUserOverride)
		m.buildRows()

	case *gffv1.Feature_ChoiceDefault:
		m.openPicker(r.itemIdx, item, modeList)
	}
	return m, nil
}

// openPicker opens the option picker for a choice item; ret names the view to
// return to on confirm/cancel (list or detail).
func (m *Model) openPicker(itemIdx int, item resolve.Resolved, ret screenMode) {
	cd := item.Feature.GetChoiceDefault()
	currentSel := selectedSet(item.Value)
	entries := make([]pickerEntry, len(cd.GetOptions()))
	for i, opt := range cd.GetOptions() {
		entries[i] = pickerEntry{opt: opt, selected: currentSel[opt.GetId()]}
	}
	m.pickerItemIdx = itemIdx
	m.pickerEntries = entries
	m.pickerCursor = 0
	m.pickerIsMulti = cd.GetMode() == gffv1.ChoiceMode_CHOICE_MODE_MULTI
	m.pickerReturn = ret
	m.mode = modePicker
}

// selectedSet builds a set of currently-selected option ids from a Value.
func selectedSet(v *gffv1.Value) map[string]bool {
	sel := make(map[string]bool)
	if cv := v.GetChoiceValue(); cv != nil {
		for _, id := range cv.GetSelected() {
			sel[id] = true
		}
	}
	return sel
}

// updatePicker handles key events in picker mode.
func (m *Model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.pickerMove(-1)

	case tea.KeyDown:
		m.pickerMove(1)

	case tea.KeyF1:
		m.helpReturn = modePicker
		m.mode = modeHelp

	case tea.KeyEscape:
		m.mode = m.pickerExitMode()

	case tea.KeyEnter:
		// For radio (single) mode, Enter on a cursor item selects it and confirms.
		if !m.pickerIsMulti {
			m.togglePickerEntry()
		}
		// Confirm the picker: write the selection and return whence we came.
		m.confirmPicker()
		m.mode = m.pickerExitMode()
		if m.mode == modeDetail {
			m.refreshDetail()
		}

	// Same real-terminal semantics as updateList: the spacebar is KeySpace.
	case tea.KeySpace:
		m.togglePickerEntry()

	case tea.KeyRunes:
		if len(msg.Runes) == 0 {
			break
		}
		switch msg.Runes[0] {
		case ' ':
			// Toggle the focused picker entry.
			m.togglePickerEntry()
		case 'q', 'Q':
			m.mode = m.pickerExitMode()
		case '?':
			m.helpReturn = modePicker
			m.mode = modeHelp
		case 'j':
			m.pickerMove(1)
		case 'k':
			m.pickerMove(-1)
		}
	}
	return m, nil
}

// pickerMove moves the picker cursor by delta, clamped.
func (m *Model) pickerMove(delta int) {
	m.pickerCursor += delta
	if m.pickerCursor > len(m.pickerEntries)-1 {
		m.pickerCursor = len(m.pickerEntries) - 1
	}
	if m.pickerCursor < 0 {
		m.pickerCursor = 0
	}
}

// pickerExitMode returns the view the picker should fall back to.
func (m *Model) pickerExitMode() screenMode {
	if m.pickerReturn == modeDetail {
		return modeDetail
	}
	return modeList
}

// togglePickerEntry toggles the selection of the current picker entry.
func (m *Model) togglePickerEntry() {
	if m.pickerCursor < 0 || m.pickerCursor >= len(m.pickerEntries) {
		return
	}
	if m.pickerIsMulti {
		m.pickerEntries[m.pickerCursor].selected = !m.pickerEntries[m.pickerCursor].selected
	} else {
		// Radio: deselect all, select only this one.
		for i := range m.pickerEntries {
			m.pickerEntries[i].selected = (i == m.pickerCursor)
		}
	}
}

// confirmPicker writes the picker selection to the user override file and
// updates the in-memory item.
func (m *Model) confirmPicker() {
	var ids []string
	for _, e := range m.pickerEntries {
		if e.selected {
			ids = append(ids, e.opt.GetId())
		}
	}
	newVal := &gffv1.Value{
		Kind: &gffv1.Value_ChoiceValue{
			ChoiceValue: &gffv1.ChoiceSelection{Selected: ids},
		},
	}
	item := m.items[m.pickerItemIdx]
	if err := overrides.Write(m.p, item.Feature.GetPath(), newVal); err != nil {
		m.errMsg = "write failed: " + err.Error()
		return // file unchanged — do not flip the row
	}
	m.errMsg = ""
	m.items[m.pickerItemIdx] = item.WithValue(newVal, resolve.LayerUserOverride)
	m.buildRows()
}

// buildRows rebuilds the flattened row list from m.items and m.expanded.
// On a category page the rows are the matching features, flat (no area
// headers, nothing to expand). Areas are sorted in first-appearance order.
func (m *Model) buildRows() {
	if m.pageIdx > 0 && m.pageIdx < len(m.pages) {
		comp := m.pages[m.pageIdx].component
		m.rows = nil
		for i, item := range m.items {
			if componentOf(item.Feature.GetPath()) != comp {
				continue
			}
			if m.scopeNS != "" && item.Namespace() != m.scopeNS {
				continue
			}
			m.rows = append(m.rows, row{item: item, ns: item.Namespace(), itemIdx: i})
		}
		m.cur.SetLen(len(m.rows))
		m.collect()
		return
	}
	// One area row per (namespace, area) pair, first-appearance order, so two
	// sources sharing an area name stay visibly separate worlds.
	type nsArea struct{ ns, area string }
	var groups []nsArea
	seen := make(map[nsArea]bool)
	byGroup := make(map[nsArea][]int)
	for idx, item := range m.items {
		g := nsArea{ns: item.Namespace(), area: areaOf(item.Feature.GetPath())}
		if !seen[g] {
			seen[g] = true
			groups = append(groups, g)
		}
		byGroup[g] = append(byGroup[g], idx)
	}

	m.rows = m.rows[:0]
	for _, g := range groups {
		m.rows = append(m.rows, row{isArea: true, area: g.area, ns: g.ns})
		if m.expanded[g.ns+"\x00"+g.area] {
			for _, idx := range byGroup[g] {
				m.rows = append(m.rows, row{item: m.items[idx], ns: g.ns, itemIdx: idx})
			}
		}
	}
	m.cur.SetLen(len(m.rows))
	m.collect()
}

// areaOf returns the first dotted segment of a feature path (e.g. "install").
func areaOf(path string) string {
	if i := strings.IndexByte(path, '.'); i >= 0 {
		return path[:i]
	}
	return path
}
