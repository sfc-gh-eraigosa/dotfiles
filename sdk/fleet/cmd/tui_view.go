package cmd

import (
	"fmt"
	"sort"
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
	dialog, panel                 lipgloss.Style
	markSel, markOK, markFail     lipgloss.Style
	logHosts                      []lipgloss.Style
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
		dialog: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("6")).Padding(0, 1),
		// Every section gets the same frame as the answers dialog, so the
		// screen reads as separated panels rather than one run-on block.
		panel: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8")).Padding(0, 1),
		// The selection dot is navy; it only turns red or green to report an
		// update's OUTCOME. Colour therefore always means the same thing:
		// blue = chosen, green = succeeded, red = failed.
		// One colour per host in the log pane, so interleaved output from a
		// concurrent wave can be told apart at a glance. Reds are excluded:
		// red already means "this update failed" everywhere else, and a host
		// that merely happened to be assigned it would read as broken.
		logHosts: []lipgloss.Style{
			lipgloss.NewStyle().Foreground(lipgloss.Color("33")),  // blue
			lipgloss.NewStyle().Foreground(lipgloss.Color("208")), // orange
			lipgloss.NewStyle().Foreground(lipgloss.Color("141")), // purple
			lipgloss.NewStyle().Foreground(lipgloss.Color("37")),  // teal
			lipgloss.NewStyle().Foreground(lipgloss.Color("178")), // gold
			lipgloss.NewStyle().Foreground(lipgloss.Color("169")), // pink
			lipgloss.NewStyle().Foreground(lipgloss.Color("45")),  // cyan
			lipgloss.NewStyle().Foreground(lipgloss.Color("113")), // light green
		},
		markSel:  lipgloss.NewStyle().Foreground(lipgloss.Color("25")),
		markOK:   lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		markFail: lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
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

// banner is the intro header: tool identity, version, and primary key hints
// framed in the same panel border as the rest of the dashboard.
func (m tuiModel) banner() string {
	var b strings.Builder
	b.WriteString(th.title.Render("🛰️  "+versionString()) + "\n")
	b.WriteString(th.dim.Render(headerHints(0)) + "\n")
	b.WriteString(th.dim.Render(headerHints(1)))
	return th.panel.Width(m.panelWidth()).Render(b.String())
}

func (m tuiModel) View() string {
	var b strings.Builder
	b.WriteString(m.banner())
	b.WriteString("\n")

	if m.mode == modeHelp {
		return b.String() + m.helpView()
	}
	if len(m.rows) == 0 {
		empty := "no fleet hosts found\n" + th.dim.Render(
			"run `fleet discover` to see adoptable ssh-config hosts, then `fleet add <alias>`")
		return b.String() + th.panel.Width(m.panelWidth()).Render(empty) + "\n"
	}

	var list strings.Builder
	// The header carries the SAME prefix width as a row (cursor + dot + space),
	// or every column label sits four cells left of the data under it.
	list.WriteString(strings.Repeat(" ", rowMarkPrefix) +
		th.header.Render(fmt.Sprintf("%-16s %-9s %-*s %-13s %-22s %s",
			"HOST", "COMMIT", branchColWidth, "BRANCH", "LAST RUN", "STATUS", "UPDATE")) + "\n")

	h := m.visibleRows()
	end := m.vp.top + h
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := m.vp.top; i < end; i++ {
		list.WriteString(trunc(m.rowView(i), m.panelWidth()) + "\n")
	}
	b.WriteString(th.panel.Width(m.panelWidth()).Render(strings.TrimRight(list.String(), "\n")) + "\n")

	if m.logOpen {
		b.WriteString(m.logView() + "\n")
	}
	b.WriteString("\n" + m.statusView())
	return b.String()
}

// logView is the framed streaming pane. It sits BELOW the host list rather
// than over it: the progress column and per-host FAIL text stay visible, so
// the log adds detail instead of replacing the summary.
// headerHints renders the always-visible key strip from keyHelp, split across
// two banner rows, so adding a key to the map is enough to make it
// discoverable. row 0 is the first half, row 1 the second.
func headerHints(row int) string {
	var all []string
	for _, k := range keyHelp {
		if k.hdr {
			all = append(all, k.icon+" "+k.keys+": "+shortWhat(k.what))
		}
	}
	half := (len(all) + 1) / 2
	if row == 0 {
		return strings.Join(all[:half], "  ")
	}
	return strings.Join(all[half:], "  ")
}

// shortWhat trims the overlay's fuller phrasing down to a header-sized label.
func shortWhat(s string) string {
	if i := strings.IndexAny(s, "(·"); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	for _, cut := range []string{"toggle this ", "toggle ", "show / hide the streaming ", "regex "} {
		s = strings.TrimPrefix(s, cut)
	}
	if i := strings.Index(s, " selection"); i > 0 {
		s = s[:i]
	}
	if i := strings.Index(s, " to cursor"); i > 0 {
		s = s[:i]
	}
	return s
}

func (m tuiModel) logView() string {
	h := m.logHeight()
	var body strings.Builder

	live := m.liveAliases()
	title := "logs"
	if len(live) > 0 {
		// The legend is coloured to match the lines, so it doubles as a key.
		coloured := make([]string, 0, len(live))
		for _, a := range live {
			coloured = append(coloured, m.hostStyle(a).Render(a))
		}
		title = "logs — streaming: " + strings.Join(coloured, ", ")
	}
	mode := "following"
	if !m.logFollow {
		mode = fmt.Sprintf("scrolled %d/%d", m.logTop+1, len(m.logs))
	}
	keys := "tab: focus  l: hide"
	if m.logFocus {
		// Say the keys are HERE now, or the operator has no way to know why
		// j/k stopped moving the host cursor.
		keys = th.markSel.Render("◀ keys here") + th.dim.Render("   gg/G  /  n/N  tab: back")
		if m.logSearch.committed && m.logSearch.input != "" {
			keys += th.dim.Render("   /" + m.logSearch.input)
		}
	}
	body.WriteString(th.header.Render(title) + th.dim.Render("   "+mode+"   ") + keys + "\n")

	if !m.logActive() {
		// Collapsed to a single framed line: still visibly its own section, but
		// it must not cost the fleet view a fifth of the screen to say nothing.
		return th.panel.Width(m.panelWidth()).Render(
			th.dim.Render("📜 logs: idle — output appears here during an update  (l: hide)"))
	}

	start := m.logStart(h)
	for i := start; i < len(m.logs) && i < start+h; i++ {
		e := m.logs[i]
		// hh:mm:ss first, then the host, then the line. Short on purpose: the
		// date is the session's, and seconds are what matter when reading how
		// long a step took.
		stamp := "        "
		if !e.at.IsZero() {
			stamp = e.at.Format("15:04:05")
		}
		text := trunc(e.line, m.logWidth()-logGutter)
		if m.logSearch.re != nil && m.logSearch.re.MatchString(e.alias+" "+e.line) {
			text = th.match.Render(text)
		}
		fmt.Fprintf(&body, "%s %s %s\n",
			th.dim.Render(stamp),
			m.hostStyle(e.alias).Render(fmt.Sprintf("%-14s│", trunc(e.alias, 14))),
			text)
	}
	return th.panel.Width(m.panelWidth()).Render(strings.TrimRight(body.String(), "\n"))
}

// logStart is the first visible line: pinned to the tail while following, so
// a running install keeps its newest output on screen without any input.
// hostStyle is a host's colour in the log pane. Assignment is by first
// appearance and held in the model, so a host keeps ONE colour for the whole
// session: colouring by line position instead would make a host's tag change
// every time another host interleaved a line, which is the opposite of
// telling them apart.
func (m tuiModel) hostStyle(alias string) lipgloss.Style {
	if i, ok := m.logColor[alias]; ok {
		return th.logHosts[i%len(th.logHosts)]
	}
	return th.statusBar
}

func (m tuiModel) logStart(h int) int {
	if m.logFollow {
		if s := len(m.logs) - h; s > 0 {
			return s
		}
		return 0
	}
	if m.logTop > len(m.logs)-1 {
		return maxInt(0, len(m.logs)-1)
	}
	return m.logTop
}

// logGutter is the timestamp + host columns and their separators.
const logGutter = 8 + 1 + 14 + 1 + 1 + 1

func (m tuiModel) logWidth() int {
	w := m.vp.width - 4
	if w < 40 {
		w = 40
	}
	return w
}

// liveAliases names the hosts currently streaming, so the pane header says
// whose output is arriving when several run at once.
func (m tuiModel) liveAliases() []string {
	var out []string
	for a := range m.streams {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// trunc cuts to a DISPLAY width, not a rune count: emoji and CJK occupy two
// cells, so counting runes overflows the frame (caught by the demo's width
// assertion when icons were added to the header).
func trunc(s string, n int) string {
	if n < 1 {
		n = 1
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if lipgloss.Width(b.String()+string(r)) > n-1 {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + "…"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m tuiModel) rowView(i int) string {
	r := m.rows[i]

	mark := m.markFor(i)
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
	// Waking wins over polling: the ladder is the slower, more interesting
	// thing happening to this row, and a row that looked merely "polling" for
	// eight seconds reads as a hang.
	if m.waking[r.Alias] {
		status = m.spinner + " waking"
	}
	style, ok := th.byClass[r.Class]
	if !ok {
		style = lipgloss.NewStyle()
	}

	// Truncate BEFORE padding: %-16s pads a short alias but does nothing to a
	// long one, so a 30-character hostname silently pushed the row past the
	// terminal edge regardless of width.
	name := truncate(r.Alias, aliasColWidth)
	var alias string
	if m.matches(r) {
		alias = th.match.Render(name)
		alias += strings.Repeat(" ", max0(aliasColWidth-lipgloss.Width(name)))
	} else {
		alias = fmt.Sprintf("%-*s", aliasColWidth, name)
	}

	// One space after the mark: without it the dot butted against the hostname.
	line := fmt.Sprintf("%s%s %s %-9s %-*s %-13s %s", cur, mark, alias, commit,
		branchColWidth, truncate(branchCell(r), branchColWidth), age,
		style.Render(fmt.Sprintf("%-22s", status)))
	return line + " " + m.updateCell(r.Alias)
}

// markFor is the row's status dot. Selection is navy; a finished update
// recolours it to its outcome so the list can be read at a glance without
// looking at the UPDATE column.
func (m tuiModel) markFor(i int) string {
	alias := m.rows[i].Alias
	if st, ok := m.updating[alias]; ok {
		switch st.phase {
		case updOK:
			return th.markOK.Render("●")
		case updFail:
			return th.markFail.Render("●")
		}
	}
	if m.isSelected(i) {
		return th.markSel.Render("●")
	}
	return " "
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
		// A remote failure can emit an arbitrarily long line; the row must stay
		// inside the terminal, so the cause is truncated to whatever is left.
		// When nothing useful is left, the cause is dropped entirely rather
		// than clamped to a minimum that would overflow the row.
		if w := m.failWidth(); s.log != "" && w >= 8 {
			msg += ": " + truncate(firstLine(s.log), w)
		}
		return th.fail.Render(msg)
	}
	return ""
}

// failWidth is the space left for a failure message after the fixed columns.
// branchColWidth is the BRANCH column. `feature/x\u2260main` — a live/installed
// mismatch — is 14, so 16 fits the common cases without crowding the failure
// message, and anything longer truncates (the full value is still searchable
// and present in --json).
const aliasColWidth = 16

const branchColWidth = 16

// failPrefix is what precedes a truncated failure cause.
const failPrefix = "FAIL: "

// rowPrefixWidth is EVERY column rowView prints before the update cell, summed
// from the same numbers as its format string:
//
//	2 cursor + 1 mark + 16 alias + 1 + 9 commit + 1 + branch + 1 + 13 age + 1 + 22 status + 1
//
// Keep it in sync with rowView. failWidth budgets the remaining space from it,
// so a stale value silently overflows the row — precisely what happened when
// the BRANCH column was added, and what the demo width guard caught.
// rowMarkPrefix is "> " + the status dot + its trailing space.
const rowMarkPrefix = 4

const rowPrefixWidth = 3 + 1 + aliasColWidth + 1 + 9 + 1 + branchColWidth + 1 + 13 + 1 + 22 + 1

// failWidth is the space left for a failure cause. It can legitimately go
// negative on a narrow terminal; callers must drop the cause rather than clamp
// to a floor, because a floor is what pushes the line past the terminal edge.
// failWidth is the room left for a failure message. It budgets against the
// PANEL's inner width, not the raw terminal: rows are framed now, so the
// border and padding are not available to the row.
func (m tuiModel) failWidth() int {
	return m.panelWidth() - rowPrefixWidth - len(failPrefix)
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
	case modeAnswers:
		return m.answersView()
	case modeConfirm:
		return m.confirmView()
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

// answersView renders the pre-wave form. The sudo entry is masked — the
// secret is never rendered, only its length.
func (m tuiModel) answersView() string {
	sel := func(f answerField, s string) string {
		if m.ansField == f {
			return th.cursor.Render("> " + s)
		}
		return "  " + s
	}
	val := func(v, empty string) string {
		if v == "" {
			return th.dim.Render(empty)
		}
		return th.statusBar.Render(v)
	}
	var b strings.Builder
	b.WriteString(th.header.Render("📋 unattended answers for this update") + "\n")
	b.WriteString(th.dim.Render("these pre-answer install.sh so it never stops to ask") + "\n\n")
	b.WriteString(sel(fieldSudo, fmt.Sprintf("%-25s %s", "🔑 sudo password",
		val(strings.Repeat("•", m.ans.secretLen()), "(empty = skip privileged steps)"))) + "\n")
	b.WriteString(sel(fieldWindows, fmt.Sprintf("%-25s %s", "🪟 windows setup [y/n/s]",
		val(m.ans.windows, "(unset = host decides)"))) + "\n")
	b.WriteString(sel(fieldGemini, fmt.Sprintf("%-25s %s", "🧹 gemini leftovers [y/k/n]",
		val(m.ans.gemini, "(unset = host decides)"))) + "\n")
	// j/k are deliberately NOT navigation here: the choice fields use
	// install.sh's own letters and `k` means "keep", so making it also mean
	// "up" would silently select the wrong answer. Arrows are the idiom the
	// host list already uses.
	b.WriteString("\n" + th.dim.Render("↑/↓ or tab: field   letters set the answer   enter: next   esc: cancel"))
	return th.panel.Width(m.panelWidth()).Render(b.String())
}

// panelWidth is the inner width every framed section shares, so their borders
// line up into a single column instead of a ragged stack.
func (m tuiModel) panelWidth() int {
	w := m.vp.width - 4
	if w < 20 {
		w = 20
	}
	return w
}

// confirmView is the gate before anything runs. Each thing the operator needs
// gets its own line — what will change, which hosts, what answers will be
// applied, and what the keys do — because packing them onto one line made the
// targets and the key hints read as a single run-on sentence.
//
// The answer summary is shown HERE because answers outlive their wave: an
// operator on wave three must see what is about to be applied without
// reopening the form. The credential appears only as a length mask.
func (m tuiModel) confirmView() string {
	t := m.updateTargets()
	var b strings.Builder

	host := "hosts"
	if len(t) == 1 {
		host = "host"
	}
	b.WriteString(th.header.Render(fmt.Sprintf("🚀 update %d %s → %s", len(t), host, m.updateRef)) + "\n")

	// The targets are the consequential part, so they are highlighted rather
	// than dimmed into the surrounding text.
	var marked []string
	for _, a := range t {
		marked = append(marked, th.markSel.Render("●")+" "+th.cursor.Render(a))
	}
	b.WriteString("   " + strings.Join(marked, "   ") + "\n\n")

	b.WriteString(th.dim.Render("   🔑 sudo ") + th.statusBar.Render(maskOrNone(m.ans.secretLen())) +
		th.dim.Render("   🪟 windows ") + th.statusBar.Render(orUnset(m.ans.windows)) +
		th.dim.Render("   🧹 gemini ") + th.statusBar.Render(orUnset(m.ans.gemini)) + "\n")
	if m.ans.forceReset() {
		b.WriteString("   " + th.fail.Render("⚠️  force reset") +
			th.dim.Render(" — each host is hard-reset onto origin; its current state is saved to a fleet-reset/<ts> branch first") + "\n")
	}
	b.WriteString("\n")

	b.WriteString(th.dim.Render("   ⏎ enter: ") + th.statusBar.Render("update") +
		th.dim.Render("     ✏️ e: edit answers     ⎋ esc: cancel"))

	return th.panel.Width(m.panelWidth()).Render(b.String())
}

func (m tuiModel) helpView() string {
	var b strings.Builder
	b.WriteString(th.header.Render("❓ keys") + "\n\n")
	for _, k := range keyHelp {
		fmt.Fprintf(&b, "  %s %-18s %s\n", k.icon, k.keys, th.dim.Render(k.what))
	}
	b.WriteString("\n" + th.dim.Render("any key to close"))
	return th.panel.Width(m.panelWidth()).Render(strings.TrimRight(b.String(), "\n"))
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
