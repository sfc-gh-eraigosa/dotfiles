package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/version"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/overlay"
)

// noColor returns true when the NO_COLOR env variable is set or stdout is not
// a TTY. We check the env variable; in a TUI the output is always the program's
// internal buffer, so we rely solely on NO_COLOR.
func noColor() bool {
	return os.Getenv("NO_COLOR") != ""
}

// layerColor returns a lipgloss style for the given layer string, using the
// theme-resolved palette (internal/style — shared with list.go).
func layerColor(pal style.Colors, layer string) lipgloss.Style {
	if noColor() {
		return lipgloss.NewStyle()
	}
	switch layer {
	case "user-override":
		return lipgloss.NewStyle().Foreground(pal.Orange)
	case "system-override":
		return lipgloss.NewStyle().Foreground(pal.Red)
	case "repo-live":
		return lipgloss.NewStyle().Foreground(pal.Green)
	case "user-snapshot":
		return lipgloss.NewStyle().Foreground(pal.Blue)
	default:
		return lipgloss.NewStyle().Foreground(pal.Grey)
	}
}

// isOverride returns true when the layer is a user or system override.
func isOverride(layer resolve.Layer) bool {
	return layer == resolve.LayerUserOverride || layer == resolve.LayerSystemOverride
}

// valueStr formats the effective value of a Resolved for display.
func valueStr(r resolve.Resolved) string {
	switch v := r.Value.GetKind().(type) {
	case *gffv1.Value_BoolValue:
		if v.BoolValue {
			return "true"
		}
		return "false"
	case *gffv1.Value_ChoiceValue:
		return strings.Join(v.ChoiceValue.GetSelected(), ",")
	}
	return "?"
}

// optionValueStr returns the typed value of a ChoiceOption as a string.
func optionValueStr(opt *gffv1.ChoiceOption) string {
	switch v := opt.Value.(type) {
	case *gffv1.ChoiceOption_StringValue:
		return v.StringValue
	case *gffv1.ChoiceOption_IntValue:
		return fmt.Sprintf("%d", v.IntValue)
	case *gffv1.ChoiceOption_FloatValue:
		return fmt.Sprintf("%g", v.FloatValue)
	case *gffv1.ChoiceOption_BoolValue:
		return fmt.Sprintf("%v", v.BoolValue)
	}
	return ""
}

// View returns the string to display. In picker mode it renders the overlay;
// in list mode it renders the collapsible tree.
func (m *Model) View() string {
	switch m.mode {
	case modePicker:
		return m.viewPicker()
	case modeDetail:
		return m.viewDetail()
	case modeHelp:
		return m.viewHelp()
	case modeSearch, modeCommand:
		return m.viewList()
	}
	return m.viewList()
}

// viewList renders the breadcrumb header, the (viewport-windowed) rows, the
// launch help/sources panel when nothing is expanded yet, and the footer.
func (m *Model) viewList() string {
	pal := style.Active()
	var sb strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(pal.Purple)
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(pal.Text)
	dimStyle := lipgloss.NewStyle().Foreground(pal.Grey)

	// Fixed header: the category breadcrumb.
	sb.WriteString(m.renderBreadcrumb(pal))
	sb.WriteString("\n\n")

	// Viewport windowing: nav.Cursor keeps the cursor row visible.
	rowsStart, rowsEnd := 0, len(m.rows)
	moreAbove, moreBelow := 0, 0
	if m.height > 0 {
		overhead := 4 // breadcrumb + blank + blank + help/prompt line
		if m.errMsg != "" {
			overhead++
		}
		if m.mode == modeSearch && m.search.Err != "" {
			overhead++ // the inline pattern error renders under the prompt
		}
		budget := m.height - overhead
		if budget < 1 {
			budget = 1
		}
		if len(m.rows) > budget {
			inner := budget - 2 // reserve the two overflow-indicator lines
			if inner < 1 {
				inner = 1
			}
			m.cur.SetHeight(inner)
			rowsStart, rowsEnd = m.cur.Visible()
			moreAbove, moreBelow = rowsStart, len(m.rows)-rowsEnd
		} else {
			m.cur.SetHeight(budget)
			m.cur.Top = 0
		}
	}

	if moreAbove > 0 {
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  … %d more above", moreAbove)))
		sb.WriteString("\n")
	}
	for i := rowsStart; i < rowsEnd; i++ {
		r := m.rows[i]
		cursor := "  "
		if m.search.IsMatch(i) {
			cursor = "* " // a match; the cursor wins on its own row
		}
		if i == m.cur.Pos {
			cursor = "> "
		}

		if r.isArea {
			indicator := "▶"
			if m.expanded[r.ns+"\x00"+r.area] {
				indicator = "▼"
			}
			line := fmt.Sprintf("%s%s %s", cursor, indicator, r.area)
			if i == m.cur.Pos {
				sb.WriteString(cursorStyle.Render(line))
			} else {
				sb.WriteString(headerStyle.Render(line))
			}
			if r.ns != "" {
				sb.WriteString(dimStyle.Render("  · " + r.ns))
			}
		} else {
			item := r.item
			val := valueStr(item)
			layer := item.Layer.String()
			marker := "default"
			if isOverride(item.Layer) {
				marker = "override"
			}
			desc := item.Feature.GetDescription()
			path := item.Feature.GetPath()

			layerRendered := layerColor(pal, layer).Render(layer)
			// A matching row wears the gutter marker AND highlights its path
			// so the hit is visible without color (NO_COLOR renders the same
			// markers).
			pathStyle := dimStyle
			if m.search.IsMatch(i) {
				pathStyle = matchStyleFor(pal)
			}
			if i == m.cur.Pos {
				sb.WriteString(cursorStyle.Render(fmt.Sprintf("%s  %-40s  %-6s  %-9s  %s  %s",
					cursor, path, val, marker, layer, desc)))
			} else {
				sb.WriteString(pathStyle.Render(fmt.Sprintf("%s%-40s", cursor, path)))
				sb.WriteString("  ")
				sb.WriteString(val)
				sb.WriteString("  ")
				sb.WriteString(marker)
				sb.WriteString("  ")
				sb.WriteString(layerRendered)
				sb.WriteString("  ")
				sb.WriteString(desc)
			}
		}
		sb.WriteString("\n")
	}
	if moreBelow > 0 {
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  … %d more below", moreBelow)))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	if m.errMsg != "" {
		sb.WriteString(errStyleFor(pal).Render(m.errMsg))
		sb.WriteString("\n")
	}
	switch m.mode {
	case modeSearch:
		sb.WriteString("/" + m.search.Input.Render("▌"))
		if m.search.Err != "" {
			sb.WriteString("\n" + errStyleFor(pal).Render(m.search.Err))
		}
	case modeCommand:
		sb.WriteString(":" + m.cmd.Input.Render("▌"))
	default:
		hint := listHint()
		if b := m.search.Badge(m.cur.Pos); b != "" {
			hint = b + "  " + hint
		}
		sb.WriteString(dimStyle.Render(hint))
	}
	return sb.String()
}

// matchStyleFor highlights a search hit's path, plain under NO_COLOR.
func matchStyleFor(pal style.Colors) lipgloss.Style {
	if noColor() {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Bold(true).Foreground(pal.Orange)
}

// errStyleFor is the footer/error red, plain under NO_COLOR.
func errStyleFor(pal style.Colors) lipgloss.Style {
	if noColor() {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(pal.Red)
}

// renderBreadcrumb renders the category pager header; the active page is
// bracketed and emphasized.
func (m *Model) renderBreadcrumb(pal style.Colors) string {
	act := lipgloss.NewStyle().Bold(true).Foreground(pal.Text)
	dim := lipgloss.NewStyle().Foreground(pal.Grey)
	prefix := ""
	if m.multiNS() && m.scopeNS != "" {
		prefix = dim.Render(m.scopeNS + " ▸ ") // the scope the pages belong to
	}
	parts := make([]string, 0, len(m.pages))
	for i, p := range m.pages {
		if i == m.pageIdx {
			parts = append(parts, act.Render("["+p.label+"]"))
		} else {
			parts = append(parts, dim.Render(p.label))
		}
	}
	return prefix + strings.Join(parts, " · ")
}

// multiNS reports whether the items span more than one namespace.
func (m *Model) multiNS() bool {
	first := ""
	for _, it := range m.items {
		if first == "" {
			first = it.Namespace()
			continue
		}
		if it.Namespace() != first {
			return true
		}
	}
	return false
}

// viewHelp is the ?/F1 overlay: about + version, the key legend for the view
// it was opened from, and the full sources story (registry + discovered).
// The list view's legend renders straight from gffKeys via overlay.Help, so
// footer, overlay, and --help can never disagree.
func (m *Model) viewHelp() string {
	pal := style.Active()
	dim := lipgloss.NewStyle().Foreground(pal.Grey)
	bold := lipgloss.NewStyle().Bold(true).Foreground(pal.Purple)
	title := lipgloss.NewStyle().Bold(true).Foreground(pal.Text)
	var sb strings.Builder

	sb.WriteString(title.Render("gff — git fast features"))
	sb.WriteString(dim.Render("  · layered feature flags persisted in git"))
	sb.WriteString("\n")
	sb.WriteString(dim.Render(fmt.Sprintf("gff v%s (commit %s)", version.Version, version.Commit)))
	sb.WriteString("\n\n")

	sources := m.sourceLines()
	switch m.helpReturn {
	case modeDetail:
		sb.WriteString(bold.Render("KEYS"))
		sb.WriteString(dim.Render(" — detail view"))
		sb.WriteString("\n")
		sb.WriteString(dim.Render("  Space  toggle the bool / pick choice options (same writer as `gff set`)"))
		sb.WriteString("\n")
		sb.WriteString(dim.Render("  u      clear the user override for this key (same as `gff unset`)"))
		sb.WriteString("\n")
		sb.WriteString(dim.Render("  Esc/Enter  back to the list · q back · ?/F1 this help"))
		sb.WriteString("\n")
	case modePicker:
		sb.WriteString(bold.Render("KEYS"))
		sb.WriteString(dim.Render(" — option picker"))
		sb.WriteString("\n")
		sb.WriteString(dim.Render("  j/k ↑/↓ move · Space toggle an option (multi) · Enter select/confirm · Esc cancel · ?/F1 this help"))
		sb.WriteString("\n")
	default:
		sb.WriteString(overlay.Help(newPalette(), "KEYS — flag list", gffKeys, "Esc/?/q close",
			overlay.Section{Title: "SOURCES", Lines: sources}))
		return sb.String()
	}
	sb.WriteString("\n")

	sb.WriteString(bold.Render("SOURCES"))
	sb.WriteString("\n")
	cur := lipgloss.NewStyle().Bold(true).Foreground(pal.Text)
	for _, l := range sources {
		if strings.HasPrefix(l, "▶ ") {
			sb.WriteString(cur.Render("  " + l))
		} else {
			sb.WriteString(dim.Render("  " + l))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(dim.Render("Esc/?/q close"))
	return sb.String()
}

// sourceLines is the SOURCES block of the help overlay as plain lines: where
// flags come from, the current-scope source first carrying the ▶ pointer, the
// rest indented, then the legend. Callers style them.
func (m *Model) sourceLines() []string {
	out := []string{
		"— where flags come from. Namespaces are disjoint worlds: uniqueness is",
		"(namespace, key); an area name like 'install' is grouping only, never claimed.",
	}
	// The source the user is currently scoped to: the breadcrumb namespace,
	// or the detail item's own namespace when help was opened from the detail.
	curNS := m.scopeNS
	if m.helpReturn == modeDetail {
		curNS = m.detailItem.Namespace()
	}

	type srcLine struct {
		ns, text string
	}
	var lines []srcLine
	registered := map[string]bool{}
	for _, src := range m.Sources {
		registered[src.Namespace] = true
		text := "● " + src.Namespace
		if src.URL != "" {
			text += "  " + src.URL
		}
		if src.Commit != "" {
			text += "  @" + src.Commit
		}
		lines = append(lines, srcLine{ns: src.Namespace, text: text})
	}
	// Discovered origins (e.g. the CWD repo's live flag file) that are not in
	// the registry — listed so the multi-source picture is complete.
	seen := map[string]bool{}
	for _, it := range m.items {
		ns := it.Namespace()
		if ns == "" || registered[ns] || seen[ns] {
			continue
		}
		seen[ns] = true
		lines = append(lines, srcLine{ns: ns, text: "○ " + ns + "  · discovered in the current repo — not registered"})
	}
	if len(lines) == 0 {
		out = append(out, "(no sources registered — run `gff install` in a repo with a flag file)")
	}
	// Current-scope source first, carrying the pointer; the rest stay dim.
	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].ns == curNS && lines[j].ns != curNS
	})
	for _, l := range lines {
		if l.ns == curNS && curNS != "" {
			out = append(out, "▶ "+l.text)
		} else {
			out = append(out, "  "+l.text)
		}
	}
	// The section's key, separated below the entries: the LEFT pointer follows
	// the scope; the dot is registration status — two orthogonal signals.
	out = append(out,
		"",
		"key: ▶ current scope · ● registered · ○ discovered (not registered)",
		"(CLI twin: `gff sources` — same list, plus --json)",
	)
	return out
}

// plainValue formats a gffv1.Value for the detail view.
func plainValue(v *gffv1.Value) string {
	if v == nil {
		return "—"
	}
	switch k := v.GetKind().(type) {
	case *gffv1.Value_BoolValue:
		return fmt.Sprintf("%v", k.BoolValue)
	case *gffv1.Value_ChoiceValue:
		return strings.Join(k.ChoiceValue.GetSelected(), ",")
	}
	return "?"
}

// viewDetail renders one feature's attributes plus the per-layer story.
func (m *Model) viewDetail() string {
	pal := style.Active()
	title := lipgloss.NewStyle().Bold(true).Foreground(pal.Text)
	dim := lipgloss.NewStyle().Foreground(pal.Grey)
	bold := lipgloss.NewStyle().Bold(true).Foreground(pal.Purple)
	winStyle := lipgloss.NewStyle().Bold(true).Foreground(pal.Orange)

	it := m.detailItem
	feat := it.Feature
	var sb strings.Builder

	typeStr := "bool"
	if cd := feat.GetChoiceDefault(); cd != nil {
		typeStr = "choice · radio (single)"
		if cd.GetMode() == gffv1.ChoiceMode_CHOICE_MODE_MULTI {
			typeStr = "choice · checkbox (multi)"
		}
	}

	sb.WriteString(title.Render(feat.GetPath()))
	sb.WriteString(dim.Render("  (" + typeStr + ")"))
	sb.WriteString("\n")
	sb.WriteString(dim.Render(feat.GetDescription()))
	sb.WriteString("\n")
	sb.WriteString(dim.Render("namespace: " + it.Namespace()))
	sb.WriteString("\n\n")

	sb.WriteString("Effective: ")
	sb.WriteString(plainValue(it.Value))
	sb.WriteString("  ")
	sb.WriteString(layerColor(pal, it.Layer.String()).Render("(" + it.Layer.String() + ")"))
	sb.WriteString("\n\n")

	sb.WriteString(bold.Render("LAYERS"))
	sb.WriteString(dim.Render(" — every level this key passes through, lowest to highest"))
	sb.WriteString("\n")
	if len(m.detailLayers) == 0 {
		sb.WriteString(dim.Render("  (per-layer detail unavailable)"))
		sb.WriteString("\n")
	}
	for _, li := range m.detailLayers {
		name := layerColor(pal, li.Layer.String()).Render(fmt.Sprintf("%-16s", li.Layer.String()))
		contrib := "—"
		if li.Present {
			kind := "default"
			if li.Layer == resolve.LayerSystemOverride || li.Layer == resolve.LayerUserOverride {
				kind = "override"
			}
			contrib = kind + ": " + plainValue(li.Value)
		}
		sb.WriteString("  ")
		sb.WriteString(name)
		sb.WriteString("  ")
		sb.WriteString(contrib)
		if li.Winner {
			sb.WriteString(winStyle.Render("  ◀ winning"))
		}
		sb.WriteString("\n")
	}

	if cd := feat.GetChoiceDefault(); cd != nil {
		sb.WriteString("\n")
		sb.WriteString(bold.Render("OPTIONS"))
		sb.WriteString("\n")
		sel := selectedSet(it.Value)
		for _, opt := range cd.GetOptions() {
			mark := "( )"
			if sel[opt.GetId()] {
				mark = "(●)"
			}
			if cd.GetMode() == gffv1.ChoiceMode_CHOICE_MODE_MULTI {
				mark = "[ ]"
				if sel[opt.GetId()] {
					mark = "[x]"
				}
			}
			line := fmt.Sprintf("  %s  %-16s  %-30s  %s", mark, opt.GetId(), opt.GetDescription(), optionValueStr(opt))
			if sel[opt.GetId()] {
				sb.WriteString(lipgloss.NewStyle().Foreground(pal.Green).Render(line))
			} else {
				sb.WriteString(dim.Render(line))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(dim.Render("Space toggle/pick  u clear user override  Esc/Enter back  ? help"))
	return sb.String()
}

// viewPicker renders the option picker overlay.
func (m *Model) viewPicker() string {
	var sb strings.Builder

	item := m.items[m.pickerItemIdx]
	cd := item.Feature.GetChoiceDefault()
	modeStr := "radio (single)"
	if cd != nil && cd.GetMode() == gffv1.ChoiceMode_CHOICE_MODE_MULTI {
		modeStr = "checkbox (multi)"
	}

	pal := style.Active()
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(pal.Purple)
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(pal.Text)
	dimStyle := lipgloss.NewStyle().Foreground(pal.Grey)
	selectedStyle := lipgloss.NewStyle().Foreground(pal.Green)

	sb.WriteString(headerStyle.Render(
		fmt.Sprintf("Pick option for: %s (%s)", item.Feature.GetPath(), modeStr),
	))
	sb.WriteString("\n\n")

	for i, e := range m.pickerEntries {
		cursor := "  "
		if i == m.pickerCursor {
			cursor = "> "
		}

		selMark := "[ ]"
		if m.pickerIsMulti {
			if e.selected {
				selMark = "[x]"
			}
		} else {
			// Radio: show filled circle for selected.
			if e.selected {
				selMark = "(●)"
			} else {
				selMark = "( )"
			}
		}

		typedVal := optionValueStr(e.opt)
		line := fmt.Sprintf("%s%s  %-16s  %-30s  %s",
			cursor, selMark, e.opt.GetId(), e.opt.GetDescription(), typedVal)

		if i == m.pickerCursor {
			sb.WriteString(cursorStyle.Render(line))
		} else if e.selected {
			sb.WriteString(selectedStyle.Render(line))
		} else {
			sb.WriteString(dimStyle.Render(line))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	if m.pickerIsMulti {
		sb.WriteString(dimStyle.Render("↑/↓ navigate  Space toggle  Enter confirm  Esc cancel"))
	} else {
		sb.WriteString(dimStyle.Render("↑/↓ navigate  Enter select  Esc cancel"))
	}
	return sb.String()
}
