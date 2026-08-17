package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/drift"
)

const spinnerInterval = 120 * time.Millisecond

// theme holds every style in one place. lipgloss degrades automatically on
// dumb/NO_COLOR terminals, and tests pin the ASCII profile so frames are
// byte-stable without a TTY.
type theme struct {
	title, header, statusBar, dim lipgloss.Style
	cursor, selected, match       lipgloss.Style
	byClass                       map[string]lipgloss.Style
	ok, fail, running             lipgloss.Style
}

func newTheme() theme {
	c := func(s string) lipgloss.Color { return lipgloss.Color(s) }
	return theme{
		title:     lipgloss.NewStyle().Bold(true).Foreground(c("6")),
		header:    lipgloss.NewStyle().Bold(true).Underline(true),
		statusBar: lipgloss.NewStyle().Foreground(c("6")),
		dim:       lipgloss.NewStyle().Faint(true),
		cursor:    lipgloss.NewStyle().Bold(true),
		selected:  lipgloss.NewStyle().Foreground(c("5")),
		match:     lipgloss.NewStyle().Reverse(true),
		ok:        lipgloss.NewStyle().Foreground(c("2")),
		fail:      lipgloss.NewStyle().Foreground(c("1")).Bold(true),
		running:   lipgloss.NewStyle().Foreground(c("4")),
		byClass: map[string]lipgloss.Style{
			string(drift.UpToDate):    lipgloss.NewStyle().Foreground(c("2")),
			string(drift.Behind):      lipgloss.NewStyle().Foreground(c("3")),
			string(drift.Divergent):   lipgloss.NewStyle().Foreground(c("5")),
			string(drift.Unknown):     lipgloss.NewStyle().Faint(true),
			string(drift.Unreachable): lipgloss.NewStyle().Foreground(c("1")).Bold(true),
			"polling":                 lipgloss.NewStyle().Faint(true),
		},
	}
}

var th = newTheme()

func (m tuiModel) View() string {
	var b strings.Builder
	b.WriteString(th.title.Render("fleet") + "  " +
		th.dim.Render("?: help  /: search  space: select  u: update  s: ssh  r: refresh  q: quit"))
	b.WriteString("\n\n")

	if m.mode == modeHelp {
		return b.String() + m.helpView()
	}
	if len(m.rows) == 0 {
		b.WriteString("  no fleet hosts found\n")
		b.WriteString(th.dim.Render(
			"  run `fleet discover` to see adoptable ssh-config hosts, then `fleet add <alias>`"))
		return b.String() + "\n"
	}

	b.WriteString("  " + th.header.Render(fmt.Sprintf("%-16s %-9s %-13s %-22s %s",
		"HOST", "COMMIT", "LAST RUN", "STATUS", "UPDATE")) + "\n")

	h := m.visibleRows()
	end := m.vp.top + h
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := m.vp.top; i < end; i++ {
		b.WriteString(m.rowView(i) + "\n")
	}

	b.WriteString("\n" + m.statusView())
	return b.String()
}

func (m tuiModel) rowView(i int) string {
	r := m.rows[i]

	mark := " "
	if m.isSelected(i) {
		mark = th.selected.Render("●")
	}
	cur := "  "
	if r.Alias == m.cursor {
		cur = th.cursor.Render("> ")
	}

	commit := r.Commit
	if commit == "" {
		commit = "-"
	}
	age := drift.FormatAge(m.now, r.Age)

	status := statusLabel(r)
	if r.Note != "" {
		status += " (" + r.Note + ")"
	}
	if m.pending[r.Alias] {
		status = m.spinner + " polling"
	}
	style, ok := th.byClass[r.Class]
	if !ok {
		style = lipgloss.NewStyle()
	}

	alias := r.Alias
	if m.matches(r) {
		alias = th.match.Render(alias)
		alias += strings.Repeat(" ", max0(16-len(r.Alias)))
	} else {
		alias = fmt.Sprintf("%-16s", alias)
	}

	line := fmt.Sprintf("%s%s%s %-9s %-13s %s", cur, mark, alias, commit, age,
		style.Render(fmt.Sprintf("%-22s", status)))
	return line + " " + m.updateCell(r.Alias)
}

// updateCell renders the update engine's per-host state — the live feedback
// that makes concurrent background updates legible.
func (m tuiModel) updateCell(alias string) string {
	s, ok := m.updating[alias]
	if !ok {
		return ""
	}
	switch s.phase {
	case updQueued:
		return th.dim.Render("queued")
	case updPrecheck:
		return th.dim.Render(m.spinner + " precheck")
	case updRunning:
		return th.running.Render(m.spinner + " updating")
	case updOK:
		return th.ok.Render("ok")
	case updFail:
		msg := "FAIL"
		if s.log != "" {
			// A remote failure can emit an arbitrarily long line; the row must
			// stay inside the terminal, so the cause is truncated to fit.
			msg += ": " + truncate(firstLine(s.log), m.failWidth())
		}
		return th.fail.Render(msg)
	}
	return ""
}

// failWidth is the space left for a failure message after the fixed columns.
func (m tuiModel) failWidth() int {
	w := m.vp.width - 73
	if w < 12 {
		w = 12
	}
	return w
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func (m tuiModel) statusView() string {
	switch m.mode {
	case modeSearch:
		s := "/" + m.search.input
		if m.search.err != "" {
			return th.fail.Render(s + "  " + m.search.err)
		}
		return th.statusBar.Render(s) + th.dim.Render(
			fmt.Sprintf("   %d match(es)  enter: keep  esc: cancel", len(m.matchIndexes())))
	case modeConfirm:
		t := m.updateTargets()
		return th.statusBar.Render(fmt.Sprintf("update %d host(s) → %s: %s",
			len(t), m.updateRef, strings.Join(t, ", "))) +
			th.dim.Render("   y: go  n/esc: cancel")
	}

	pos := fmt.Sprintf("%d/%d", m.indexOf(m.cursor)+1, len(m.rows))
	var bits []string
	bits = append(bits, pos)
	if n := len(m.selectedAliases()); n > 0 {
		bits = append(bits, fmt.Sprintf("%d selected", n))
	}
	if m.search.committed && m.search.input != "" {
		bits = append(bits, "/"+m.search.input)
	}
	if n := m.running; n > 0 {
		bits = append(bits, fmt.Sprintf("%d updating", n))
	}
	if n := len(m.bgQueue) + len(m.iaQueue); n > 0 {
		bits = append(bits, fmt.Sprintf("%d queued", n))
	}
	line := th.statusBar.Render(strings.Join(bits, " · "))
	if m.status != "" {
		line += th.dim.Render("   " + m.status)
	}
	return line
}

func (m tuiModel) helpView() string {
	var b strings.Builder
	b.WriteString(th.header.Render("keys") + "\n")
	for _, k := range keyHelp {
		b.WriteString(fmt.Sprintf("  %-18s %s\n", k.keys, th.dim.Render(k.what)))
	}
	b.WriteString("\n" + th.dim.Render("any key to close"))
	return b.String()
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

func joinTrim(ss []string) string { return strings.Join(ss, " | ") }
