// Package search is the / prompt state machine: incremental smartcase regex,
// match collection over the caller's rows, n/N with wrap, :noh and re-arm.
package search

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/prompt"
)

// Event is what a key did to the prompt.
type Event int

const (
	Ignored Event = iota
	Typed
	Submitted
	Cancelled
)

// State is the search prompt plus the committed pattern n/N replay.
type State struct {
	Input     prompt.Line
	Pattern   string         // committed pattern (n/N)
	Re        *regexp.Regexp // live or committed pattern
	Err       string         // live compile error
	Matches   []int          // matching row indices, ascending
	Visible   bool           // highlights + badge shown
	AnchorPos int            // cursor when / was pressed
	AnchorTop int            // viewport top when / was pressed
	set       map[int]bool
}

// Compile applies vim smartcase: an empty pattern is nil (no matches, no
// error); a pattern with no uppercase letter is case-insensitive. Error text
// drops Go's "error parsing regexp: " prefix ("missing closing ]: `[ai`").
func Compile(p string) (*regexp.Regexp, error) {
	if p == "" {
		return nil, nil
	}
	if !strings.ContainsFunc(p, unicode.IsUpper) {
		p = "(?i)" + p
	}
	re, err := regexp.Compile(p)
	if err != nil {
		return nil, cleanErr(err)
	}
	return re, nil
}

type cleanError string

func (e cleanError) Error() string { return string(e) }

func cleanErr(err error) error {
	return cleanError(strings.TrimPrefix(err.Error(), "error parsing regexp: "))
}

// Start opens the prompt, remembering where the cursor was.
func (s *State) Start(pos, top int) {
	s.Input.Reset()
	s.Re, s.Err = nil, ""
	s.Matches, s.set = s.Matches[:0], nil
	s.Visible = true
	s.AnchorPos, s.AnchorTop = pos, top
}

// Key feeds one key to the prompt. Esc → Cancelled, Enter → Submitted,
// editing keys → Typed (with the pattern recompiled), anything else Ignored.
// The caller then calls Cancel / Commit / Collect accordingly.
func (s *State) Key(msg tea.KeyMsg) Event {
	switch msg.Type {
	case tea.KeyEscape:
		return Cancelled
	case tea.KeyEnter:
		return Submitted
	}
	if !s.Input.Handle(msg) {
		return Ignored
	}
	re, err := Compile(s.Input.String())
	if err != nil {
		s.Err = err.Error() // keep the previous Re so matches do not flicker
		return Typed
	}
	s.Err, s.Re = "", re
	return Typed
}

// Collect rebuilds Matches over n rows using the caller's hit test. It is a
// no-op set (empty) while hidden or without a pattern.
func (s *State) Collect(n int, hit func(i int) bool) {
	s.Matches = s.Matches[:0]
	s.set = map[int]bool{}
	if s.Re == nil || !s.Visible {
		return
	}
	for i := 0; i < n; i++ {
		if hit(i) {
			s.Matches = append(s.Matches, i)
			s.set[i] = true
		}
	}
}

// First is the first match at or after from, wrapping to the top.
func (s *State) First(from int) (int, bool) {
	if len(s.Matches) == 0 {
		return 0, false
	}
	for _, i := range s.Matches {
		if i >= from {
			return i, true
		}
	}
	return s.Matches[0], true
}

// Next is the match strictly after (dir > 0) or before (dir < 0) cur, wrapping.
func (s *State) Next(cur, dir int) (int, bool) {
	n := len(s.Matches)
	if n == 0 {
		return 0, false
	}
	if dir > 0 {
		for _, i := range s.Matches {
			if i > cur {
				return i, true
			}
		}
		return s.Matches[0], true
	}
	for k := n - 1; k >= 0; k-- {
		if s.Matches[k] < cur {
			return s.Matches[k], true
		}
	}
	return s.Matches[n-1], true
}

// Commit freezes the live pattern for n/N. An outstanding compile error
// refuses (committed=false, Err kept for the caller to show). An empty
// pattern hides the search. notFound reports a committed pattern with no
// matches (from the last Collect).
func (s *State) Commit() (committed, notFound bool) {
	if s.Err != "" {
		return false, false
	}
	s.Pattern = s.Input.String()
	s.Input.Reset()
	if s.Pattern == "" {
		s.Hide()
		s.Re = nil
		return true, false
	}
	return true, len(s.Matches) == 0
}

// Cancel closes the prompt and returns the anchor to restore. Pattern is kept
// so n/N still work with the previous committed search.
func (s *State) Cancel() (pos, top int) {
	s.Input.Reset()
	s.Re, s.Err = nil, ""
	s.Matches, s.set = s.Matches[:0], nil
	s.Visible = false
	return s.AnchorPos, s.AnchorTop
}

// Hide is vim :noh — highlights off, pattern kept.
func (s *State) Hide() {
	s.Visible = false
	s.Matches, s.set = s.Matches[:0], nil
}

// Rearm recompiles the committed pattern after Hide (n after :noh). False
// when there is nothing to re-arm.
func (s *State) Rearm() bool {
	if s.Pattern == "" {
		return false
	}
	re, err := Compile(s.Pattern)
	if err != nil {
		return false
	}
	s.Re, s.Visible = re, true
	return true
}

// Badge is the footer "/pattern [i/N]" (i = "-" when cur is not a match);
// empty while hidden or uncommitted.
func (s *State) Badge(cur int) string {
	if !s.Visible || s.Pattern == "" {
		return ""
	}
	pos := "-"
	for i, m := range s.Matches {
		if m == cur {
			pos = strconv.Itoa(i + 1)
			break
		}
	}
	return "/" + s.Pattern + " [" + pos + "/" + strconv.Itoa(len(s.Matches)) + "]"
}

// IsMatch reports whether row i is in the current match set.
func (s *State) IsMatch(i int) bool { return s.set[i] }
