package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/pins"
)

func pinsModel(width int, s *pins.Summary) tuiModel {
	m := tipModel(Row{Alias: "h", Class: "up-to-date", Commit: "abc1234", Pins: s})
	m.vp.width = width
	return m
}

func TestPinsThresholdIsDerivedFromTheColumns(t *testing.T) {
	want := rowPrefixWidth + pinsColWidth + updateMinWidth
	if got := pinsMinInner(); got != want {
		t.Fatalf("pinsMinInner = %d, want %d", got, want)
	}
	for _, tc := range []struct {
		width int
		show  bool
	}{{100, false}, {pinsMinInner() + 6 - 1, false}, {pinsMinInner() + 6, true}, {200, true}} {
		m := pinsModel(tc.width, nil)
		if m.showPins() != tc.show {
			t.Errorf("width %d: showPins=%v want %v (inner %d, min %d)", tc.width, !tc.show, tc.show, m.panelInner(), pinsMinInner())
		}
	}
}

func TestWidePanelShowsPinsColumnAlignedUnderItsHeader(t *testing.T) {
	m := pinsModel(200, &pins.Summary{Drift: 2})
	out := ansi.Strip(m.listPanel())
	lines := strings.Split(out, "\n")
	var hdr, row string
	for _, l := range lines {
		switch {
		case strings.Contains(l, "HOST"):
			hdr = l
		case strings.Contains(l, "abc1234"):
			row = l
		}
	}
	if !strings.Contains(hdr, "PINS") || !strings.Contains(row, "2 drift") {
		t.Fatalf("header/row missing PINS:\n%s", out)
	}
	if strings.Index(hdr, "PINS") != strings.Index(row, "2 drift") {
		t.Errorf("PINS misaligned:\n%s\n%s", hdr, row)
	}
	if strings.Index(hdr, "PINS") > strings.Index(hdr, "STATUS") || strings.Index(hdr, "LAST RUN") > strings.Index(hdr, "PINS") {
		t.Errorf("PINS must sit between LAST RUN and STATUS: %s", hdr)
	}
	if !strings.Contains(hdr, "UPDATE") {
		t.Errorf("UPDATE truncated: %s", hdr)
	}
}

func TestNarrowPanelRendersWithoutPins(t *testing.T) {
	m := pinsModel(100, &pins.Summary{Drift: 2})
	out := ansi.Strip(m.listPanel())
	if strings.Contains(out, "PINS") || strings.Contains(out, "drift") {
		t.Fatalf("narrow panel must not show PINS:\n%s", out)
	}
	if !strings.Contains(out, "UPDATE") {
		t.Fatalf("UPDATE must be visible:\n%s", out)
	}
	m2 := pinsModel(100, nil)
	if m.listPanel() != m2.listPanel() {
		t.Error("narrow render must not depend on pins data")
	}
}

func TestPinsCellsOkAndNone(t *testing.T) {
	for s, want := range map[*pins.Summary]string{nil: "-", {}: "ok"} {
		m := pinsModel(200, s)
		if row := ansi.Strip(m.rowView(0)); !strings.Contains(row, " "+want+" ") {
			t.Errorf("want %q cell in %q", want, row)
		}
	}
}

func TestRowTipNamesDriftWhenPinsColumnIsHidden(t *testing.T) {
	m := pinsModel(100, &pins.Summary{Drift: 2})
	got := m.rowTip()
	for _, w := range []string{"h:", "2 tools off their pin", "fleet pins status --installed"} {
		if !strings.Contains(got, w) {
			t.Errorf("tip %q missing %q", got, w)
		}
	}
	if tip := pinsModel(200, &pins.Summary{Drift: 2}).rowTip(); tip != "" {
		t.Errorf("column visible: tip should stay empty, got %q", tip)
	}
	if tip := pinsModel(100, &pins.Summary{}).rowTip(); tip != "" {
		t.Errorf("no drift: tip should be empty, got %q", tip)
	}
	if tip := pinsModel(100, &pins.Summary{Drift: 1}).rowTip(); !strings.Contains(tip, "1 tool off its pin") {
		t.Errorf("singular: %q", tip)
	}
}
