package cmd

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/drift"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/reach"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

// tuiMode is the explicit interaction mode. Every key event is routed by mode
// (tui_keys.go) — no hidden flag combinations.
type tuiMode int

const (
	modeNormal tuiMode = iota
	modeSearch
	modeAnswers // pre-wave form: sudo password + install.sh prompt answers
	modeConfirm
	modeHelp
)

// answerField is the cursor within the pre-wave answer form.
type answerField int

const (
	fieldSudo answerField = iota
	fieldWindows
	fieldGemini
	answerFieldCount
)

// updPhase is where a host sits in the update engine.
type updPhase int

const (
	updQueued   updPhase = iota // waiting for a job slot (or the fallback queue)
	updPrecheck                 // running `sudo -n true` to pick a lane
	updRunning                  // update in flight
	updOK
	updFail
)

// updState is a host's update status plus the tail of its captured output.
// Background updates capture output instead of showing it (there is no
// terminal to show it on), so a failure must carry its own explanation.
type updState struct {
	phase updPhase
	log   string
}

type searchState struct {
	input     string
	re        *regexp.Regexp
	err       string
	committed bool
}

type viewport struct{ top, height, width int }

// tuiModel is the whole dashboard as one value: pure data in, pure frame out.
//
// In-flight ownership invariant: a host is in EXACTLY ONE of pending (being
// polled), updating (owned by the update engine), waking (owned by the
// reachability ladder), or resolved. Refresh skips `updating` and `waking`
// hosts; either completion clears its claim and re-polls. No row is ever
// owned by two async paths at once.
type tuiModel struct {
	rows    []Row
	pending map[string]bool
	cursor  string // ALIAS, not index — survives the worst-first re-sort
	vp      viewport
	mode    tuiMode

	search   searchState
	selected map[string]bool
	vAnchor  *string

	// update engine (background-first)
	updating  map[string]updState
	bgQueue   []string
	iaQueue   []string
	jobs      int // max concurrent background updates
	running   int // slots in use
	updateRef string
	ans       answers     // pre-supplied answers for this wave (password: memory only)
	ansField  answerField // cursor in the answer form

	// reachability ladder — its own ownership set, same invariant as updating
	waking map[string]bool
	wake   waker // nil = --no-wake

	// ansPath is where the non-secret prompt preferences persist. INJECTED,
	// not resolved here: a model that reached os.UserConfigDir() itself made
	// every test write to the developer's real config, which is both a
	// hermeticity bug and a rude thing to do to someone's home directory.
	// Empty (the test default) disables persistence entirely.
	ansPath string

	hosts   map[string]sshconf.Host
	run     runner.Runner
	base    Baseliner
	now     time.Time
	spinner string // frame injected; tests keep it fixed for stable goldens
	status  string
	quitReq bool
}

func newTUIModel(hosts []sshconf.Host, r runner.Runner, base Baseliner, now time.Time, ref string, jobs int) tuiModel {
	m := tuiModel{
		pending:   map[string]bool{},
		selected:  map[string]bool{},
		updating:  map[string]updState{},
		waking:    map[string]bool{},
		hosts:     map[string]sshconf.Host{},
		jobs:      jobs,
		updateRef: ref,
		run:       r,
		base:      base,
		now:       now,
		vp:        viewport{height: 20, width: 100},
		spinner:   "⠋",
	}
	for _, h := range hosts {
		m.hosts[h.Alias] = h
		m.pending[h.Alias] = true
		m.rows = append(m.rows, Row{Alias: h.Alias, Class: "polling"})
	}
	sortWorstFirst(m.rows)
	if len(m.rows) > 0 {
		m.cursor = m.rows[0].Alias
	}
	return m
}

func (m tuiModel) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.rows))
	for _, r := range m.rows {
		cmds = append(cmds, pollHostWake(m.hosts[r.Alias], m.peersFor(r.Alias), m.run, m.base, m.wake))
	}
	return tea.Batch(cmds...)
}

// ---- helpers over the row list -------------------------------------------

func (m tuiModel) indexOf(alias string) int {
	for i, r := range m.rows {
		if r.Alias == alias {
			return i
		}
	}
	return -1
}

func (m *tuiModel) setRow(row Row) {
	for i := range m.rows {
		if m.rows[i].Alias == row.Alias {
			m.rows[i] = row
			return
		}
	}
	m.rows = append(m.rows, row)
}

// resort keeps the cursor on its alias — the whole reason cursor is alias-keyed.
func (m *tuiModel) resort() {
	sortWorstFirst(m.rows)
	if m.indexOf(m.cursor) < 0 && len(m.rows) > 0 {
		m.cursor = m.rows[0].Alias
	}
	m.clampViewport()
}

// clampViewport enforces vp.top <= indexOf(cursor) < vp.top+height so the
// cursor is always on screen and the header never scrolls away.
func (m *tuiModel) clampViewport() {
	h := m.visibleRows()
	if h < 1 {
		h = 1
	}
	i := m.indexOf(m.cursor)
	if i < 0 {
		m.vp.top = 0
		return
	}
	if i < m.vp.top {
		m.vp.top = i
	}
	if i >= m.vp.top+h {
		m.vp.top = i - h + 1
	}
	max := len(m.rows) - h
	if max < 0 {
		max = 0
	}
	if m.vp.top > max {
		m.vp.top = max
	}
	if m.vp.top < 0 {
		m.vp.top = 0
	}
}

// visibleRows is the terminal height minus the chrome (title, header, status).
func (m tuiModel) visibleRows() int {
	h := m.vp.height - 5
	if h < 1 {
		h = 1
	}
	return h
}

func (m *tuiModel) moveTo(i int) {
	if len(m.rows) == 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i > len(m.rows)-1 {
		i = len(m.rows) - 1
	}
	m.cursor = m.rows[i].Alias
	m.clampViewport()
}

func (m *tuiModel) move(d int) {
	i := m.indexOf(m.cursor)
	if i < 0 {
		i = 0
	}
	m.moveTo(i + d)
}

// ---- search ---------------------------------------------------------------

// rowText is what a search pattern matches against: the rendered line, so
// `/behind` and `/unreachable` work as naturally as `/hostname`.
func (m tuiModel) rowText(r Row) string {
	// Branch is in the haystack on purpose: `/feature` + select-all + `u` is
	// the targeting workflow the branch column exists to enable.
	return strings.Join([]string{r.Alias, r.Commit, branchCell(r), statusLabel(r), r.Note}, " ")
}

// compileSearch applies vim smartcase: case-insensitive unless the pattern
// contains an uppercase letter. An invalid pattern keeps the previous compiled
// regexp so the highlight does not flicker while typing `[a-`.
func (m *tuiModel) compileSearch() {
	s := m.search.input
	if s == "" {
		m.search.re, m.search.err = nil, ""
		return
	}
	pat := s
	if s == strings.ToLower(s) {
		pat = "(?i)" + s
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		m.search.err = "bad pattern: " + cleanReErr(err)
		return
	}
	m.search.re, m.search.err = re, ""
}

func cleanReErr(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 {
		s = s[i+2:]
	}
	return s
}

func (m tuiModel) matches(r Row) bool {
	return m.search.re != nil && m.search.re.MatchString(m.rowText(r))
}

func (m tuiModel) matchIndexes() []int {
	var out []int
	for i, r := range m.rows {
		if m.matches(r) {
			out = append(out, i)
		}
	}
	return out
}

// jumpMatch moves to the next (d=1) or previous (d=-1) match, wrapping.
func (m *tuiModel) jumpMatch(d int) {
	idx := m.matchIndexes()
	if len(idx) == 0 {
		m.status = "no matches"
		return
	}
	cur := m.indexOf(m.cursor)
	if d > 0 {
		for _, i := range idx {
			if i > cur {
				m.moveTo(i)
				return
			}
		}
		m.moveTo(idx[0]) // wrap
		return
	}
	for k := len(idx) - 1; k >= 0; k-- {
		if idx[k] < cur {
			m.moveTo(idx[k])
			return
		}
	}
	m.moveTo(idx[len(idx)-1]) // wrap
}

// ---- selection ------------------------------------------------------------

// visualRange is the anchor..cursor span while in visual mode.
func (m tuiModel) visualRange() (int, int, bool) {
	if m.vAnchor == nil {
		return 0, 0, false
	}
	a, b := m.indexOf(*m.vAnchor), m.indexOf(m.cursor)
	if a < 0 || b < 0 {
		return 0, 0, false
	}
	if a > b {
		a, b = b, a
	}
	return a, b, true
}

func (m tuiModel) isSelected(i int) bool {
	if a, b, ok := m.visualRange(); ok && i >= a && i <= b {
		return true
	}
	return m.selected[m.rows[i].Alias]
}

// selectedAliases returns the selection in table order — the order the batch
// confirm strip lists and the queue runs.
func (m tuiModel) selectedAliases() []string {
	var out []string
	for i, r := range m.rows {
		if m.isSelected(i) {
			out = append(out, r.Alias)
		}
	}
	return out
}

// filteredRows is what the operator can currently see through the search: the
// rows an action should apply to. With no active pattern it is every row.
func (m tuiModel) filteredRows() []Row {
	if m.search.re == nil {
		return m.rows
	}
	var out []Row
	for _, r := range m.rows {
		if m.matches(r) {
			out = append(out, r)
		}
	}
	return out
}

// selectAllFiltered selects every visible row, or clears them when they are
// already all selected. A PARTIAL selection is completed rather than toggled
// off, so the key never destroys a selection the operator built by hand.
func (m *tuiModel) selectAllFiltered() {
	rows := m.filteredRows()
	if len(rows) == 0 {
		return
	}
	all := true
	for _, r := range rows {
		if !m.selected[r.Alias] {
			all = false
			break
		}
	}
	for _, r := range rows {
		if all {
			delete(m.selected, r.Alias)
		} else {
			m.selected[r.Alias] = true
		}
	}
	if all {
		m.status = "selection cleared"
	} else {
		m.status = fmt.Sprintf("%d host(s) selected", len(m.selected))
	}
}

// updateTargets is the selection, or the cursor row when nothing is selected.
func (m tuiModel) updateTargets() []string {
	if s := m.selectedAliases(); len(s) > 0 {
		return s
	}
	if m.cursor != "" {
		return []string{m.cursor}
	}
	return nil
}

// ---- update engine --------------------------------------------------------

// startUpdate seeds the engine: every target goes to precheck, which routes it
// to the background wave or the interactive fallback.
func (m *tuiModel) startUpdate(targets []string) tea.Cmd {
	var cmds []tea.Cmd
	for _, a := range targets {
		if _, busy := m.updating[a]; busy {
			continue
		}
		m.updating[a] = updState{phase: updPrecheck}
		delete(m.pending, a) // ownership moves to the engine
		cmds = append(cmds, precheckSudo(a, m.run))
	}
	m.status = fmt.Sprintf("updating %d host(s) → %s", len(cmds), m.updateRef)
	return tea.Batch(cmds...)
}

// pump fills free job slots from the background queue. When the wave is fully
// drained it releases the interactive fallback queue, one host at a time.
func (m *tuiModel) pump() tea.Cmd {
	var cmds []tea.Cmd
	for m.running < m.jobs && len(m.bgQueue) > 0 {
		a := m.bgQueue[0]
		m.bgQueue = m.bgQueue[1:]
		m.updating[a] = updState{phase: updRunning}
		m.running++
		cmds = append(cmds, bgUpdate(a, m.updateRef, m.ans, m.run))
	}
	// Interactive handoffs need the terminal to themselves, so they only run
	// once no background update can print over them.
	if m.running == 0 && len(m.bgQueue) == 0 && len(m.iaQueue) > 0 {
		a := m.iaQueue[0]
		m.iaQueue = m.iaQueue[1:]
		m.updating[a] = updState{phase: updRunning}
		m.running++
		return tea.Batch(append(cmds, interactiveHandoff(a, m.updateRef))...)
	}
	return tea.Batch(cmds...)
}

// finishUpdate records the outcome, releases the slot, and re-polls the host —
// the invariant is "an update completion always refreshes its row".
func (m *tuiModel) finishUpdate(alias, log string, err error) tea.Cmd {
	if m.running > 0 {
		m.running--
	}
	st := updState{phase: updOK, log: log}
	if err != nil {
		st = updState{phase: updFail, log: strings.TrimSpace(log + " " + err.Error())}
	}
	m.updating[alias] = st
	cmds := []tea.Cmd{pollHost(m.hosts[alias], m.run, m.base)}
	if c := m.pump(); c != nil {
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}

// busy reports whether the engine still owns work — used by the quit guard.
func (m tuiModel) busy() bool {
	if m.running > 0 || len(m.bgQueue) > 0 || len(m.iaQueue) > 0 || len(m.waking) > 0 {
		return true
	}
	for _, s := range m.updating {
		if s.phase == updPrecheck || s.phase == updRunning || s.phase == updQueued {
			return true
		}
	}
	return false
}

// inFlight is true while an async path owns this host; refresh must skip it,
// and neither `u` nor `s` may claim it. Wake counts: a ladder that gets its
// row re-polled underneath it produces a verdict for a probe nobody asked for.
func (m tuiModel) inFlight(alias string) bool {
	if m.waking[alias] {
		return true
	}
	s, ok := m.updating[alias]
	return ok && (s.phase == updQueued || s.phase == updPrecheck || s.phase == updRunning)
}

// startWake claims each target and runs its ladder in the BACKGROUND lane.
// tea.ExecProcess would suspend the entire dashboard, which is the freeze this
// whole feature exists to remove.
func (m *tuiModel) startWake(targets []string) tea.Cmd {
	var cmds []tea.Cmd
	for _, a := range targets {
		if m.inFlight(a) {
			continue
		}
		m.waking[a] = true
		cmds = append(cmds, wakeHost(m.hosts[a], m.peersFor(a), m.run, m.wakePolicy()))
	}
	if len(cmds) == 0 {
		return nil
	}
	m.status = fmt.Sprintf("waking %d host(s)", len(cmds))
	return tea.Batch(cmds...)
}

// peersFor offers every other fleet host as a relay candidate, marking the
// ones whose rows have already resolved to something other than unreachable —
// relaying through a second sleeping host helps nobody.
func (m tuiModel) peersFor(target string) []reach.Peer {
	out := make([]reach.Peer, 0, len(m.rows))
	for _, r := range m.rows {
		if r.Alias == target {
			continue
		}
		h := m.hosts[r.Alias]
		name := h.HostName
		if name == "" {
			name = r.Alias
		}
		out = append(out, reach.Peer{
			Alias:     r.Alias,
			HostName:  name,
			Reachable: !m.pending[r.Alias] && r.Class != string(drift.Unreachable) && r.Class != "polling",
		})
	}
	return out
}

// wakePolicy is the model's ladder budget. The TUI always allows the ladder
// when the operator asks for it explicitly with `w`.
func (m tuiModel) wakePolicy() reach.Policy {
	return reach.Policy{Enabled: true, Budget: flagWakeTimeout, Retries: 2}
}

// refresh re-polls every host the update engine does not currently own (F2b).
func (m *tuiModel) refresh() tea.Cmd {
	var cmds []tea.Cmd
	for _, r := range m.rows {
		if m.inFlight(r.Alias) {
			continue
		}
		m.pending[r.Alias] = true
		cmds = append(cmds, pollHostWake(m.hosts[r.Alias], m.peersFor(r.Alias), m.run, m.base, m.wake))
	}
	m.status = fmt.Sprintf("refreshing %d host(s)", len(cmds))
	return tea.Batch(cmds...)
}

// ---- the bubbletea Update -------------------------------------------------

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.vp.height, m.vp.width = msg.Height, msg.Width
		m.clampViewport()
		return m, nil

	case hostRowMsg:
		delete(m.pending, msg.row.Alias)
		m.setRow(msg.row)
		m.resort()
		return m, nil

	case wakeDoneMsg:
		// Release the claim FIRST and unconditionally: a host left owned by a
		// failed ladder would be skipped by every later refresh, silently
		// freezing its row for the rest of the session.
		delete(m.waking, msg.alias)
		if msg.woke {
			m.status = fmt.Sprintf("%s woke via %s", msg.alias, msg.via)
		} else {
			m.status = fmt.Sprintf("%s stayed unreachable", msg.alias)
		}
		// Re-poll so the row shows its real drift class, not the stale
		// unreachable verdict that triggered the wake.
		m.pending[msg.alias] = true
		return m, pollHostWake(m.hosts[msg.alias], m.peersFor(msg.alias), m.run, m.base, m.wake)

	case precheckMsg:
		// Route the host: passwordless sudo → background wave; otherwise the
		// serial interactive queue, where its prompt can reach the operator.
		m.updating[msg.alias] = updState{phase: updQueued}
		if msg.interactive {
			m.iaQueue = append(m.iaQueue, msg.alias)
		} else {
			m.bgQueue = append(m.bgQueue, msg.alias)
		}
		return m, m.pump()

	case bgUpdateDoneMsg:
		return m, m.finishUpdate(msg.alias, msg.log, msg.err)

	case execDoneMsg:
		if msg.ssh {
			// An ssh visit returns with the terminal restored; re-poll so the
			// row reflects anything the operator changed by hand.
			return m, pollHost(m.hosts[msg.alias], m.run, m.base)
		}
		return m, m.finishUpdate(msg.alias, "", msg.err)

	case spinnerTickMsg:
		m.spinner = spinFrames[int(msg)%len(spinFrames)]
		return m, spinnerTick(int(msg) + 1)

	case tea.KeyMsg:
		return route(m, msg)
	}
	return m, nil
}

// sortedAliases is used by tests and the help overlay for stable ordering.
func sortedAliases(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for a := range set {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}
