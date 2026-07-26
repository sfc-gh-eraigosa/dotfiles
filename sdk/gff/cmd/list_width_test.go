package cmd

// Width-aware pretty table (owner-reported on the PR #187 review): the styled
// list must fit the terminal's column count instead of overflowing and
// letting the terminal wrap mid-cell, which mangles the borders.

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestPrettyTableHonorsWidth(t *testing.T) {
	rows := [][]string{
		{"install.pkg.common-core", "bool", "true", "repo-live",
			"Runs the pkg-install block that installs the curated common-core packages plus the legacy yum path and a very long tail of explanatory text"},
		{"install.ai.claude", "bool", "true", "repo-live", "Runs the Claude Code blocks."},
	}
	out := renderPrettyTable(rows, 80)
	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 80 {
			t.Errorf("line %d exceeds the 80-column budget: width=%d %q", i, w, line)
		}
	}
}

func TestPrettyTableUnconstrainedWhenWidthUnknown(t *testing.T) {
	rows := [][]string{{"a.b.c", "bool", "true", "repo-live", "short"}}
	out := renderPrettyTable(rows, 0)
	if !strings.Contains(out, "a.b.c") {
		t.Fatalf("unconstrained render lost content: %q", out)
	}
}
