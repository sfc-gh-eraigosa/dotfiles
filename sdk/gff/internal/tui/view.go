package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/style"
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

	launch := m.pageIdx == 0 && !m.anyExpanded()

	// Viewport windowing (skipped in the tiny launch state).
	rowsStart, rowsEnd := 0, len(m.rows)
	moreAbove, moreBelow := 0, 0
	if !launch && m.height > 0 {
		overhead := 4 // breadcrumb + blank + blank + help line
		if m.errMsg != "" {
			overhead++
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
			if m.cursor < m.scrollTop {
				m.scrollTop = m.cursor
			}
			if m.cursor > m.scrollTop+inner-1 {
				m.scrollTop = m.cursor - inner + 1
			}
			if m.scrollTop > len(m.rows)-inner {
				m.scrollTop = len(m.rows) - inner
			}
			if m.scrollTop < 0 {
				m.scrollTop = 0
			}
			rowsStart, rowsEnd = m.scrollTop, m.scrollTop+inner
			if rowsEnd > len(m.rows) {
				rowsEnd = len(m.rows)
			}
			moreAbove, moreBelow = rowsStart, len(m.rows)-rowsEnd
			m.lastInner = inner
		} else {
			m.scrollTop, m.lastInner = 0, budget
		}
	}

	if moreAbove > 0 {
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  … %d more above", moreAbove)))
		sb.WriteString("\n")
	}
	for i := rowsStart; i < rowsEnd; i++ {
		r := m.rows[i]
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		if r.isArea {
			indicator := "▶"
			if m.expanded[r.area] {
				indicator = "▼"
			}
			line := fmt.Sprintf("%s%s %s", cursor, indicator, r.area)
			if i == m.cursor {
				sb.WriteString(cursorStyle.Render(line))
			} else {
				sb.WriteString(headerStyle.Render(line))
			}
			if ns := m.areaNamespaces(r.area); ns != "" {
				sb.WriteString(dimStyle.Render("  · " + ns))
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
			if i == m.cursor {
				sb.WriteString(cursorStyle.Render(fmt.Sprintf("%s  %-40s  %-6s  %-9s  %s  %s",
					cursor, path, val, marker, layer, desc)))
			} else {
				sb.WriteString(dimStyle.Render(fmt.Sprintf("  %-40s", path)))
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

	if launch {
		sb.WriteString(m.renderLaunchPanel(pal))
	}

	sb.WriteString("\n")
	if m.errMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(pal.Red)
		if noColor() {
			errStyle = lipgloss.NewStyle()
		}
		sb.WriteString(errStyle.Render(m.errMsg))
		sb.WriteString("\n")
	}
	sb.WriteString(dimStyle.Render("↑/↓ move  ←/→ category  PgUp/PgDn page  Enter expand/details  Space toggle  q quit"))
	return sb.String()
}

// renderBreadcrumb renders the category pager header; the active page is
// bracketed and emphasized.
func (m *Model) renderBreadcrumb(pal style.Colors) string {
	act := lipgloss.NewStyle().Bold(true).Foreground(pal.Text)
	dim := lipgloss.NewStyle().Foreground(pal.Grey)
	parts := make([]string, 0, len(m.pages))
	for i, p := range m.pages {
		if i == m.pageIdx {
			parts = append(parts, act.Render("["+p.label+"]"))
		} else {
			parts = append(parts, dim.Render(p.label))
		}
	}
	return strings.Join(parts, " · ")
}

// anyExpanded reports whether any area is currently expanded.
func (m *Model) anyExpanded() bool {
	for _, v := range m.expanded {
		if v {
			return true
		}
	}
	return false
}

// areaNamespaces returns the comma-joined namespaces defining an area.
func (m *Model) areaNamespaces(area string) string {
	seen := map[string]bool{}
	var out []string
	for _, item := range m.items {
		if areaOf(item.Feature.GetPath()) == area {
			if ns := item.Namespace(); ns != "" && !seen[ns] {
				seen[ns] = true
				out = append(out, ns)
			}
		}
	}
	return strings.Join(out, ", ")
}

// renderLaunchPanel shows first-run help plus the sources/registry story.
func (m *Model) renderLaunchPanel(pal style.Colors) string {
	dim := lipgloss.NewStyle().Foreground(pal.Grey)
	bold := lipgloss.NewStyle().Bold(true).Foreground(pal.Purple)
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(dim.Render("git fast features — layered flags persisted in git"))
	sb.WriteString("\n\n")
	sb.WriteString(dim.Render("  ↑/↓ move · ←/→ category pages · PgUp/PgDn page"))
	sb.WriteString("\n")
	sb.WriteString(dim.Render("  Enter expand an area or open feature details · Space toggle (bool) or pick (choice) · q quit"))
	sb.WriteString("\n\n")

	sb.WriteString(bold.Render("SOURCES"))
	sb.WriteString(dim.Render(" — the registry (~/.config/gff/sources.yaml) each area resolves from"))
	sb.WriteString("\n")
	if len(m.Sources) == 0 {
		sb.WriteString(dim.Render("  (no sources registered — run `gff install` in a repo with a flag file)"))
		sb.WriteString("\n")
	}
	for _, src := range m.Sources {
		line := "  ● " + src.Namespace
		if src.URL != "" {
			line += "  " + src.URL
		}
		if src.Commit != "" {
			line += "  @" + src.Commit
		}
		sb.WriteString(dim.Render(line))
		sb.WriteString("\n")
	}
	return sb.String()
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
	sb.WriteString(dim.Render("Esc/Enter back  q back"))
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
