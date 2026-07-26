// Package tui implements the bubbletea TUI for gff: a browsable, collapsible
// tree of feature flags with in-place toggle and provenance display.
package tui

import (
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/overrides"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
)

// screenMode controls what the main loop renders.
type screenMode int

const (
	modeList   screenMode = iota // normal collapsible tree view
	modePicker                   // option picker overlay (choice flags)
)

// row is a rendered line in the list: either an area header or a feature row.
type row struct {
	isArea  bool
	area    string          // set when isArea == true
	item    resolve.Resolved // set when isArea == false
	itemIdx int             // index into m.items (for toggle)
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
	w        io.Writer // used by tests to capture output (nil = unused)
	width    int
	height   int
	cursor   int            // index into m.rows (the rendered row list)
	expanded map[string]bool // area name → expanded
	rows     []row           // flattened render list rebuilt on each expand/collapse
	mode     screenMode

	// picker state
	pickerItemIdx  int           // which m.items entry is being picked
	pickerEntries  []pickerEntry // copy of options with transient selection state
	pickerCursor   int
	pickerIsMulti  bool
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
	return m
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
		if m.mode == modePicker {
			return m.updatePicker(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

// updateList handles key events in list mode.
func (m *Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}

	case tea.KeyDown:
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}

	case tea.KeyEnter:
		if m.cursor >= 0 && m.cursor < len(m.rows) {
			r := m.rows[m.cursor]
			if r.isArea {
				// Toggle expand/collapse.
				m.expanded[r.area] = !m.expanded[r.area]
				m.buildRows()
				// Keep cursor in-bounds after rebuild.
				if m.cursor >= len(m.rows) {
					m.cursor = len(m.rows) - 1
				}
				if m.cursor < 0 {
					m.cursor = 0
				}
			}
		}

	case tea.KeyRunes:
		if len(msg.Runes) == 0 {
			break
		}
		switch msg.Runes[0] {
		case 'q', 'Q':
			return m, tea.Quit

		case ' ':
			if m.cursor >= 0 && m.cursor < len(m.rows) {
				r := m.rows[m.cursor]
				if !r.isArea {
					return m.activateFeature(r)
				}
			}
		}
	}
	return m, nil
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
		_ = overrides.Write(m.p, item.Feature.GetPath(), newVal)
		// Refresh the item in m.items so the view reflects the new state.
		m.items[r.itemIdx] = resolve.Resolved{
			Feature: item.Feature,
			Value:   newVal,
			Layer:   resolve.LayerUserOverride,
		}
		m.buildRows()

	case *gffv1.Feature_ChoiceDefault:
		// Open picker.
		cd := item.Feature.GetChoiceDefault()
		// Build picker entries with the current effective selection.
		currentSel := selectedSet(item.Value)
		entries := make([]pickerEntry, len(cd.GetOptions()))
		for i, opt := range cd.GetOptions() {
			entries[i] = pickerEntry{
				opt:      opt,
				selected: currentSel[opt.GetId()],
			}
		}
		m.pickerItemIdx = r.itemIdx
		m.pickerEntries = entries
		m.pickerCursor = 0
		m.pickerIsMulti = cd.GetMode() == gffv1.ChoiceMode_CHOICE_MODE_MULTI
		m.mode = modePicker
	}
	return m, nil
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
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}

	case tea.KeyDown:
		if m.pickerCursor < len(m.pickerEntries)-1 {
			m.pickerCursor++
		}

	case tea.KeyEscape:
		m.mode = modeList

	case tea.KeyEnter:
		// For radio (single) mode, Enter on a cursor item selects it and confirms.
		if !m.pickerIsMulti {
			m.togglePickerEntry()
		}
		// Confirm the picker: write the selection and return to list.
		m.confirmPicker()
		m.mode = modeList

	case tea.KeyRunes:
		if len(msg.Runes) == 0 {
			break
		}
		switch msg.Runes[0] {
		case ' ':
			// Toggle the focused picker entry.
			m.togglePickerEntry()
		case 'q', 'Q':
			m.mode = modeList
		}
	}
	return m, nil
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
	_ = overrides.Write(m.p, item.Feature.GetPath(), newVal)
	m.items[m.pickerItemIdx] = resolve.Resolved{
		Feature: item.Feature,
		Value:   newVal,
		Layer:   resolve.LayerUserOverride,
	}
	m.buildRows()
}

// buildRows rebuilds the flattened row list from m.items and m.expanded.
// Areas are sorted in the order they first appear in m.items.
func (m *Model) buildRows() {
	// Collect unique ordered areas.
	var areas []string
	seen := make(map[string]bool)
	for _, item := range m.items {
		area := areaOf(item.Feature.GetPath())
		if !seen[area] {
			seen[area] = true
			areas = append(areas, area)
		}
	}

	// Build item index by area.
	byArea := make(map[string][]int)
	for idx, item := range m.items {
		area := areaOf(item.Feature.GetPath())
		byArea[area] = append(byArea[area], idx)
	}

	m.rows = m.rows[:0]
	for _, area := range areas {
		m.rows = append(m.rows, row{isArea: true, area: area})
		if m.expanded[area] {
			for _, idx := range byArea[area] {
				m.rows = append(m.rows, row{isArea: false, item: m.items[idx], itemIdx: idx})
			}
		}
	}
}

// areaOf returns the first dotted segment of a feature path (e.g. "install").
func areaOf(path string) string {
	if i := strings.IndexByte(path, '.'); i >= 0 {
		return path[:i]
	}
	return path
}
