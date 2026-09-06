//go:build example

// Package main is the composition proof for sdk/libs/tui: a 30-row list with
// vim navigation, / search, a : command line (mark <row>), a confirm dialog
// (d), and the help overlay. Not installed; run with
// `go run -tags example ./tui/example`.
package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/cmdline"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/nav"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/overlay"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/search"
)

type mode int

const (
	modeList mode = iota
	modeSearch
	modeCommand
	modeHelp
	modeConfirm
)

const actDelete keymap.Action = "delete"

type model struct {
	rows    []string
	marked  map[int]bool
	cur     nav.Cursor
	keys    keymap.Map
	search  search.State
	cmd     cmdline.State
	reg     cmdline.Registry
	confirm overlay.Confirm
	mode    mode
	status  string
}

func newModel(n int) *model {
	m := &model{marked: map[int]bool{}}
	for i := 1; i <= n; i++ {
		m.rows = append(m.rows, fmt.Sprintf("row %02d", i))
	}
	m.cur.SetLen(len(m.rows))
	m.keys = keymap.Vim.Without(keymap.PageLeft, keymap.PageRight).Merge(
		keymap.Binding{Action: actDelete, Keys: []string{"d"}, Help: "delete the row (asks first)"},
	)
	m.reg.Register(cmdline.Standard(
		func() { m.mode = modeHelp },
		func(p string) { m.runSearch(p) },
	)...)
	m.reg.Register(cmdline.Spec{
		Name: "mark", Help: "mark <row>",
		// Row names contain a space ("row 25"), so the : parser hands them
		// over as two tokens: the command re-joins them.
		Run: func(args []string) (tea.Cmd, error) {
			if len(args) == 0 {
				return nil, fmt.Errorf("usage: :mark <row>")
			}
			name := strings.Join(args, " ")
			for i, r := range m.rows {
				if r == name {
					m.marked[i] = true
					m.status = "marked " + r
					return nil, nil
				}
			}
			return nil, fmt.Errorf("unknown row: %s", name)
		},
		// argIdx 0 completes whole names ("row 01" …); argIdx 1 completes the
		// number token so a full ":mark row 25" cycles onto itself.
		Complete: func(argIdx int, prefix string) []string {
			var out []string
			seen := map[string]bool{}
			for _, r := range m.rows {
				cand := r
				if argIdx > 0 {
					f := strings.Fields(r)
					if argIdx >= len(f) {
						continue
					}
					cand = f[argIdx]
				}
				if strings.HasPrefix(cand, prefix) && !seen[cand] {
					seen[cand] = true
					out = append(out, cand)
				}
			}
			return out
		},
	})
	return m
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) hit(i int) bool { return m.search.Re != nil && m.search.Re.MatchString(m.rows[i]) }

func (m *model) runSearch(p string) {
	m.search.Start(m.cur.Pos, m.cur.Top)
	m.search.Input.SetText(p)
	re, err := search.Compile(p)
	if err != nil {
		m.status = "bad pattern: " + err.Error()
		return
	}
	m.search.Re = re
	m.search.Collect(len(m.rows), m.hit)
	if i, ok := m.search.First(m.cur.Pos); ok {
		m.cur.To(i)
	}
	if _, notFound := m.search.Commit(); notFound {
		m.status = "pattern not found: " + p
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.cur.SetHeight(msg.Height - 4)
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeHelp:
		m.mode = modeList
		return m, nil
	case modeConfirm:
		if m.confirm.Key(msg) == overlay.Yes {
			i := m.cur.Pos
			m.rows = append(m.rows[:i], m.rows[i+1:]...)
			m.cur.SetLen(len(m.rows))
			m.status = "deleted"
		} else {
			m.status = "cancelled"
		}
		m.mode = modeList
		return m, nil
	case modeSearch:
		switch m.search.Key(msg) {
		case search.Cancelled:
			pos, top := m.search.Cancel()
			m.cur.To(pos)
			m.cur.Top = top
			m.mode = modeList
		case search.Submitted:
			if committed, notFound := m.search.Commit(); !committed {
				m.status = "bad pattern: " + m.search.Err
			} else if notFound {
				m.status = "pattern not found: " + m.search.Pattern
			}
			m.mode = modeList
		case search.Typed:
			m.search.Collect(len(m.rows), m.hit)
			if i, ok := m.search.First(m.search.AnchorPos); ok {
				m.cur.To(i)
			}
		}
		return m, nil
	case modeCommand:
		ev := m.cmd.Key(msg, &m.reg)
		switch ev.Kind {
		case cmdline.Cancelled:
			m.mode = modeList
		case cmdline.Submitted:
			m.mode = modeList
			cmd, err := m.reg.Run(ev.Command)
			if err != nil {
				m.status = err.Error()
			}
			return m, cmd
		}
		return m, nil
	}
	if m.cur.Key(msg, m.keys) {
		return m, nil
	}
	handled, cmd := keymap.Dispatch(m.keys, msg, keymap.Handlers{
		keymap.Search:         func() tea.Cmd { m.search.Start(m.cur.Pos, m.cur.Top); m.mode = modeSearch; return nil },
		keymap.Command:        func() tea.Cmd { m.mode = modeCommand; return nil },
		keymap.Help:           func() tea.Cmd { m.mode = modeHelp; return nil },
		keymap.Quit:           func() tea.Cmd { return tea.Quit },
		keymap.NextMatch:      func() tea.Cmd { m.jump(1); return nil },
		keymap.PrevMatch:      func() tea.Cmd { m.jump(-1); return nil },
		keymap.ClearHighlight: func() tea.Cmd { m.search.Hide(); return nil },
		actDelete: func() tea.Cmd {
			m.confirm = overlay.Confirm{Title: "delete " + m.rows[m.cur.Pos] + "?", YesLabel: "delete"}
			m.mode = modeConfirm
			return nil
		},
	})
	_ = handled
	return m, cmd
}

func (m *model) jump(dir int) {
	if !m.search.Visible && !m.search.Rearm() {
		return
	}
	m.search.Collect(len(m.rows), m.hit)
	if i, ok := m.search.Next(m.cur.Pos, dir); ok {
		m.cur.To(i)
	} else {
		m.status = "pattern not found: " + m.search.Pattern
	}
}

func (m *model) View() string {
	switch m.mode {
	case modeHelp:
		return overlay.Help(overlay.Plain{}, "example — keys", m.keys, "any key closes")
	case modeConfirm:
		return m.confirm.Render(overlay.Plain{})
	}
	var b strings.Builder
	b.WriteString("example list\n\n")
	m.search.Collect(len(m.rows), m.hit)
	s, e := m.cur.Visible()
	for i := s; i < e; i++ {
		g := "  "
		if m.search.IsMatch(i) {
			g = "* "
		}
		if i == m.cur.Pos {
			g = "> "
		}
		mark := ""
		if m.marked[i] {
			mark = "  ✓"
		}
		b.WriteString(g + m.rows[i] + mark + "\n")
	}
	b.WriteString("\n")
	switch m.mode {
	case modeSearch:
		b.WriteString("/" + m.search.Input.Render("▌"))
		if m.search.Err != "" {
			b.WriteString("\n" + m.search.Err)
		}
	case modeCommand:
		b.WriteString(":" + m.cmd.Input.Render("▌"))
	default:
		if badge := m.search.Badge(m.cur.Pos); badge != "" {
			b.WriteString(badge + "  ")
		}
		b.WriteString(m.keys.HeaderHint("  "))
		if m.status != "" {
			b.WriteString("\n" + m.status)
		}
	}
	return b.String()
}
