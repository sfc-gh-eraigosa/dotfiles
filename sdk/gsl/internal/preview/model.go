package preview

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/gh/fake"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git/fake"
	mcpfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp/fake"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/render"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

// tickMsg is sent on every 1-second timer tick (unexported for TUI use).
type tickMsg time.Time

// TickMsg is the exported alias of tickMsg used in tests to send synthetic tick
// events via Model.Update without needing a real clock.
type TickMsg = tickMsg

// Fixture names used for the fixture cycle.
const (
	FixtureClean = "clean repo (root)"
	FixtureDirty = "dirty worktree"
)

// fixtureNames lists all fixture names in display order.
var fixtureNames = []string{FixtureClean, FixtureDirty}

// styleOrder is the cycle order for styles in the TUI.
var styleOrder = func() []string {
	bs := style.Builtins()
	names := make([]string, 0, len(bs))
	for k := range bs {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}()

// Model is the bubbletea model for the gsl preview TUI.
type Model struct {
	// fixtureName is the name of the currently selected fixture.
	fixtureName string
	// fixtureIdx is the index into fixtureNames.
	fixtureIdx int
	// styleIdx is the index into styleOrder.
	styleIdx int
	// segmentEnabled tracks per-segment enable state (by type name).
	segmentEnabled map[string]bool
	// cfg is the base config (all segments enabled; may be mutated via toggles).
	cfg config.Config
	// now is the current time (ticked every second).
	now time.Time
	// clock overrides time.Now (nil = real clock).
	clock func() time.Time
	// quitting is set to true when the user presses q/ctrl+c.
	quitting bool
	// windowWidth is the terminal width from the last tea.WindowSizeMsg (0 = unknown).
	windowWidth int
}

// NewModel creates a new Model with the default (clean-repo) fixture, powerline
// style, and all segments enabled. clock may be nil (real time) or a fixed
// function (tests / --once).
func NewModel(clock func() time.Time) Model {
	cfg := config.Default()
	enabled := make(map[string]bool, len(cfg.Segments))
	for _, seg := range cfg.Segments {
		enabled[seg.Type] = true
	}
	now := time.Now()
	if clock != nil {
		now = clock()
	}
	return Model{
		fixtureName:    fixtureNames[0],
		fixtureIdx:     0,
		styleIdx:       0,
		segmentEnabled: enabled,
		cfg:            cfg,
		now:            now,
		clock:          clock,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tickCmd()
}

// tickCmd returns a command that waits 1 second then sends tickMsg.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "s", "S":
			// Cycle styles.
			m.styleIdx = (m.styleIdx + 1) % len(styleOrder)
		case "f", "F":
			// Cycle fixtures.
			m.fixtureIdx = (m.fixtureIdx + 1) % len(fixtureNames)
			m.fixtureName = fixtureNames[m.fixtureIdx]
		case "1":
			m.segmentEnabled["dirgit"] = !m.segmentEnabled["dirgit"]
		case "2":
			m.segmentEnabled["repo"] = !m.segmentEnabled["repo"]
		case "3":
			m.segmentEnabled["ai"] = !m.segmentEnabled["ai"]
		case "4":
			m.segmentEnabled["time"] = !m.segmentEnabled["time"]
		}

	case tickMsg:
		if m.clock != nil {
			m.now = m.clock()
		} else {
			m.now = time.Time(msg)
		}
		return m, tickCmd()

	case tea.WindowSizeMsg:
		// Track the terminal width so renderLine can apply compaction for fidelity.
		if msg.Width > 0 {
			m.windowWidth = msg.Width
		}
	}

	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	var sb strings.Builder

	sb.WriteString("gsl preview  [s] cycle style  [f] cycle fixture  [1-4] toggle segments  [q] quit\n")
	sb.WriteString(fmt.Sprintf("style: %-12s  fixture: %s\n", m.currentStyleName(), m.fixtureName))
	sb.WriteString(fmt.Sprintf("segments: %s\n", m.segmentBadges()))
	sb.WriteString("─────────────────────────────────────────────────────\n")
	sb.WriteString(m.renderLine())
	sb.WriteString("\n")

	return sb.String()
}

// currentStyleName returns the currently selected style name.
func (m Model) currentStyleName() string {
	if len(styleOrder) == 0 {
		return "powerline"
	}
	return styleOrder[m.styleIdx]
}

// segmentBadges returns a compact display of enabled/disabled segments.
func (m Model) segmentBadges() string {
	types := []string{"dirgit", "repo", "ai", "time"}
	labels := map[string]string{"dirgit": "[1]dirgit", "repo": "[2]repo", "ai": "[3]ai", "time": "[4]time"}
	parts := make([]string, 0, len(types))
	for _, t := range types {
		lbl := labels[t]
		if !m.segmentEnabled[t] {
			lbl = "(" + lbl + ")"
		}
		parts = append(parts, lbl)
	}
	return strings.Join(parts, "  ")
}

// discardWriter satisfies io.Writer for style.Resolve (discard warnings).
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// renderLine renders one status-line frame against the current fixture, style,
// and segment toggles. It applies the detect-once / fit loop so the preview
// matches what the real status-line command renders at the current window width.
func (m Model) renderLine() string {
	cfg := m.buildConfig()
	st := style.Resolve(discardWriter{}, m.currentStyleName(), nil, false)
	deps := m.buildDeps()
	segs := render.BuildSegments(cfg, deps)

	// Detect-once: all subprocess I/O happens here, exactly once.
	ctx := context.Background()
	datas := render.Detect(ctx, cfg, st, segs)

	// Use the known window width when available; fall back to $COLUMNS then 80.
	cols := m.windowWidth
	if cols <= 0 {
		cols = term.Columns(nil) // nil source → $COLUMNS then 80
	}

	return render.Fit(datas, st, cols)
}

// buildConfig returns a config derived from m.cfg with segment enables applied.
func (m Model) buildConfig() config.Config {
	cfg := m.cfg
	segs := make([]config.Segment, len(cfg.Segments))
	copy(segs, cfg.Segments)
	for i := range segs {
		segs[i].Enabled = m.segmentEnabled[segs[i].Type]
	}
	cfg.Segments = segs
	cfg.Style = m.currentStyleName()
	return cfg
}

// buildDeps returns render.Deps wired to the current fixture fakes + fixed
// clock pinned to m.now.
func (m Model) buildDeps() render.Deps {
	var gitResponses []gitfake.Response
	now := m.now

	switch m.fixtureName {
	case FixtureDirty:
		gitResponses = DirtyGitResponses()
		return render.Deps{
			Payload:      DirtyRepoPayload(),
			Cwd:          "/home/user/myproject/.worktrees/feature-x",
			Branch:       "feature-x",
			RegistryPath: "",
			Git:          &gitfake.Runner{Script: gitResponses},
			GH:           &ghfake.Runner{},
			MCP:          &mcpfake.Runner{Default: mcpfake.Response{Stdout: []byte("")}},
			MCPOpts:      mcp.ActiveCountOptions{},
			Clock:        func() time.Time { return now },
		}
	default: // FixtureClean
		gitResponses = CleanGitResponses()
		return render.Deps{
			Payload:      CleanRepoPayload(),
			Cwd:          "/home/user/myproject",
			Branch:       "main",
			RegistryPath: "",
			Git:          &gitfake.Runner{Script: gitResponses},
			GH:           &ghfake.Runner{},
			MCP:          &mcpfake.Runner{Default: mcpfake.Response{Stdout: []byte("")}},
			MCPOpts:      mcp.ActiveCountOptions{},
			Clock:        func() time.Time { return now },
		}
	}
}

// RenderOnce renders a single status-line frame against the clean fixture,
// using the powerline style and a fixed clock. Suitable for --once / CI.
func RenderOnce() string {
	clock := FixedClock()
	m := NewModel(clock)
	line := m.renderLine()
	return line
}

// ── Exported accessors for testing ────────────────────────────────────────────

// FixtureName returns the name of the currently selected fixture.
func (m Model) FixtureName() string { return m.fixtureName }

// CurrentStyleName returns the name of the currently selected style.
func (m Model) CurrentStyleName() string { return m.currentStyleName() }

// SegmentEnabled reports whether the given segment type is enabled.
func (m Model) SegmentEnabled(segType string) bool { return m.segmentEnabled[segType] }

// Quitting reports whether the model has received a quit signal.
func (m Model) Quitting() bool { return m.quitting }

// Now returns the current time tracked by the model.
func (m Model) Now() time.Time { return m.now }

// WindowWidth returns the terminal width tracked by the model (0 = unknown).
func (m Model) WindowWidth() int { return m.windowWidth }

// WithWindowWidth returns a copy of the model with the given window width set.
// Used in tests to inject a synthetic terminal width without sending a real
// tea.WindowSizeMsg.
func (m Model) WithWindowWidth(w int) Model {
	m.windowWidth = w
	return m
}

// RenderLineForTest calls renderLine and returns the result. Exported for
// testing only so external test packages can assert on compaction behaviour.
func (m Model) RenderLineForTest() string { return m.renderLine() }
