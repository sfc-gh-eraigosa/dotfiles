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
	if m.mode == modePicker {
		return m.viewPicker()
	}
	return m.viewList()
}

// viewList renders the collapsible area/feature tree.
func (m *Model) viewList() string {
	var sb strings.Builder

	pal := style.Active()
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(pal.Purple)
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(pal.Text)
	dimStyle := lipgloss.NewStyle().Foreground(pal.Grey)

	for i, r := range m.rows {
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
			line := fmt.Sprintf("%s  %-40s  %-6s  %-9s  %s  %s",
				cursor, path, val, marker, layerRendered, desc)

			if i == m.cursor {
				sb.WriteString(cursorStyle.Render(fmt.Sprintf("%s  %-40s  %-6s  %-9s  %s  %s",
					cursor, path, val, marker, layer, desc)))
			} else {
				_ = line
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

	sb.WriteString("\n")
	if m.errMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(pal.Red)
		if noColor() {
			errStyle = lipgloss.NewStyle()
		}
		sb.WriteString(errStyle.Render(m.errMsg))
		sb.WriteString("\n")
	}
	sb.WriteString(dimStyle.Render("↑/↓ navigate  Enter expand/collapse  Space toggle  q quit"))
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
