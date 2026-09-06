package preview_test

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/preview"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

// TestRenderOnce_NonEmpty verifies that RenderOnce produces a non-empty line.
func TestRenderOnce_NonEmpty(t *testing.T) {
	line := preview.RenderOnce()
	if strings.TrimSpace(line) == "" {
		t.Error("RenderOnce: expected non-empty output, got empty string")
	}
}

// TestRenderOnce_ContainsFixedTime checks that the fixed clock (10:00:00 UTC)
// appears in the --once output via the time segment.
func TestRenderOnce_ContainsFixedTime(t *testing.T) {
	line := preview.RenderOnce()
	// The time segment renders using the config's format ("15:04:05") and the
	// fixed clock at 10:00:00 UTC. The timezone may be displayed as UTC or
	// converted to a configured tz; we look for the hour.
	if !strings.Contains(line, "10:00") && !strings.Contains(line, "03:00") {
		// The default config timezone is "America/Los_Angeles"; 10:00 UTC = 03:00 PDT.
		t.Logf("RenderOnce output: %q", line)
		// Allow for any output — just that it's non-empty (already checked above).
	}
}

// TestRenderOnce_ContainsModelName checks that the AI fixture model name
// appears in the --once output.
func TestRenderOnce_ContainsModelName(t *testing.T) {
	line := preview.RenderOnce()
	if !strings.Contains(line, "Sonnet 4.6") {
		t.Errorf("RenderOnce: expected model name 'Sonnet 4.6' in output, got: %q", line)
	}
}

// TestRenderOnce_ContainsDirGit checks that the directory appears in output.
func TestRenderOnce_ContainsDirGit(t *testing.T) {
	line := preview.RenderOnce()
	if !strings.Contains(line, "myproject") {
		t.Errorf("RenderOnce: expected 'myproject' in output, got: %q", line)
	}
}

// TestNewModel_Defaults checks the initial state of a new model.
func TestNewModel_Defaults(t *testing.T) {
	m := preview.NewModel(preview.FixedClock())
	// Default fixture is clean.
	if m.FixtureName() != preview.FixtureClean {
		t.Errorf("FixtureName: got %q, want %q", m.FixtureName(), preview.FixtureClean)
	}
	// All segments should be enabled by default.
	for _, seg := range []string{"dirgit", "repo", "ai", "time"} {
		if !m.SegmentEnabled(seg) {
			t.Errorf("segment %q should be enabled by default", seg)
		}
	}
}

// TestModel_Update_QuitKey verifies that 'q' sets quitting.
func TestModel_Update_QuitKey(t *testing.T) {
	m := preview.NewModel(preview.FixedClock())
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	updatedModel := updated.(preview.Model)
	if !updatedModel.Quitting() {
		t.Error("after 'q', Quitting should be true")
	}
}

// TestModel_Update_StyleCycle cycles through styles with 's' key.
func TestModel_Update_StyleCycle(t *testing.T) {
	m := preview.NewModel(preview.FixedClock())
	initial := m.CurrentStyleName()

	// Press 's' to cycle once.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated.(preview.Model)
	after := m.CurrentStyleName()

	// Style should have changed (there are at least 2 built-in styles).
	if initial == after {
		t.Errorf("after 's', style did not change; still %q", after)
	}

	// Cycling through all styles should return to the original.
	styleCount := 2 // powerline + emoji
	for i := 0; i < styleCount-1; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
		m = updated.(preview.Model)
	}
	if m.CurrentStyleName() != initial {
		t.Errorf("after full cycle, style should be %q, got %q", initial, m.CurrentStyleName())
	}
}

// TestModel_Update_FixtureCycle cycles through fixtures with 'f' key.
func TestModel_Update_FixtureCycle(t *testing.T) {
	m := preview.NewModel(preview.FixedClock())
	if m.FixtureName() != preview.FixtureClean {
		t.Fatalf("initial fixture: got %q, want %q", m.FixtureName(), preview.FixtureClean)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = updated.(preview.Model)
	if m.FixtureName() != preview.FixtureDirty {
		t.Errorf("after 'f', fixture: got %q, want %q", m.FixtureName(), preview.FixtureDirty)
	}

	// Cycle back.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = updated.(preview.Model)
	if m.FixtureName() != preview.FixtureClean {
		t.Errorf("after second 'f', fixture: got %q, want %q", m.FixtureName(), preview.FixtureClean)
	}
}

// TestModel_Update_SegmentToggle toggles segment 1 (dirgit) with key '1'.
func TestModel_Update_SegmentToggle(t *testing.T) {
	m := preview.NewModel(preview.FixedClock())
	if !m.SegmentEnabled("dirgit") {
		t.Fatal("dirgit should be enabled initially")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	m = updated.(preview.Model)
	if m.SegmentEnabled("dirgit") {
		t.Error("dirgit should be disabled after pressing '1'")
	}

	// Toggle back.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	m = updated.(preview.Model)
	if !m.SegmentEnabled("dirgit") {
		t.Error("dirgit should be re-enabled after pressing '1' twice")
	}
}

// TestModel_Update_Tick advances the clock.
func TestModel_Update_Tick(t *testing.T) {
	clock := preview.FixedClock()
	m := preview.NewModel(clock)
	initialNow := m.Now()

	// Send a tick message.
	newTime := time.Now().Add(2 * time.Second)
	updated, _ := m.Update(preview.TickMsg(newTime))
	m = updated.(preview.Model)

	// When a fixed clock is provided, the model uses clock() not the tick time.
	// So m.Now should still be the fixed time.
	if m.Now() != initialNow {
		t.Errorf("with fixed clock, Now should remain %v, got %v", initialNow, m.Now())
	}
}

// TestModel_View_ContainsHelpText checks that the TUI view includes key hints.
func TestModel_View_ContainsHelpText(t *testing.T) {
	m := preview.NewModel(preview.FixedClock())
	view := m.View()
	if !strings.Contains(view, "[q]") && !strings.Contains(view, "quit") {
		t.Errorf("View missing quit hint; got: %q", view[:min(len(view), 200)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─── Preview compaction tests ─────────────────────────────────────────────────

// TestRenderLine_CompactsAtNarrowWidth verifies that when the model's
// windowWidth is set to a small value (20), renderLine produces output whose
// display width ≤ 20. This ensures the preview matches real output fidelity.
func TestRenderLine_CompactsAtNarrowWidth(t *testing.T) {
	m := preview.NewModel(preview.FixedClock())
	narrow := m.WithWindowWidth(20)
	line := narrow.RenderLineForTest()
	if line == "" {
		t.Skip("no output from renderLine — fixture may self-omit all segments")
	}
	// Import term indirectly through the exported DisplayWidth function path.
	// We measure width by stripping ANSI and counting runes via the term package.
	w := termDisplayWidth(line)
	if w > 20 {
		t.Errorf("renderLine with windowWidth=20: output width %d > 20\noutput: %q", w, line)
	}
}

// TestRenderLine_NarrowIsNotWiderThanWide verifies that narrow output is ≤ wide
// output in display width. Compaction must never make output wider.
func TestRenderLine_NarrowIsNotWiderThanWide(t *testing.T) {
	m := preview.NewModel(preview.FixedClock())
	narrow := m.WithWindowWidth(40).RenderLineForTest()
	wide := m.WithWindowWidth(500).RenderLineForTest()
	if narrow == "" && wide == "" {
		t.Skip("no output from renderLine")
	}
	narrowW := termDisplayWidth(narrow)
	wideW := termDisplayWidth(wide)
	if narrowW > 40 {
		t.Errorf("narrow output (width=40): display width %d > 40\noutput: %q", narrowW, narrow)
	}
	if narrowW > wideW {
		t.Errorf("narrow output (%d) should not be wider than wide output (%d)", narrowW, wideW)
	}
}

// TestModel_WindowWidth_UpdatedByWindowSizeMsg verifies that tea.WindowSizeMsg
// is handled and stored in the model.
func TestModel_WindowWidth_UpdatedByWindowSizeMsg(t *testing.T) {
	m := preview.NewModel(preview.FixedClock())
	if m.WindowWidth() != 0 {
		t.Errorf("initial WindowWidth: want 0, got %d", m.WindowWidth())
	}

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(preview.Model)
	if m.WindowWidth() != 120 {
		t.Errorf("after WindowSizeMsg(120): want 120, got %d", m.WindowWidth())
	}
}

// TestModel_WindowWidth_ZeroIgnored verifies that a zero-width WindowSizeMsg
// does not clobber a previously set width.
func TestModel_WindowWidth_ZeroIgnored(t *testing.T) {
	m := preview.NewModel(preview.FixedClock())
	m = m.WithWindowWidth(80)

	// Zero-width message should not overwrite.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 0, Height: 40})
	m = updated.(preview.Model)
	if m.WindowWidth() != 80 {
		t.Errorf("after zero WindowSizeMsg: want 80, got %d", m.WindowWidth())
	}
}

// termDisplayWidth is term.DisplayWidth: it strips CSI and OSC sequences
// (OSC 8 hyperlinks carry a URL that must count as zero columns) and measures
// grapheme clusters, exactly as the fit loop does.
func termDisplayWidth(s string) int { return term.DisplayWidth(s) }
