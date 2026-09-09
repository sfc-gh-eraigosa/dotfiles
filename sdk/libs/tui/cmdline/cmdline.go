// Package cmdline is the ex-style : prompt: a parser, a registry of commands
// the tool supplies, the standard q/help/search verbs, and Tab completion.
package cmdline

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/prompt"
)

// ErrEmpty is returned by Parse for a blank line.
var ErrEmpty = errors.New("empty command")

// Command is one parsed :-line.
type Command struct {
	Name string
	Args []string
}

// Parse tokenizes a :-line. "/re" is the search alias and keeps the whole
// remainder (spaces included) as its single argument.
func Parse(line string) (Command, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Command{}, ErrEmpty
	}
	if strings.HasPrefix(line, "/") {
		return Command{Name: "search", Args: []string{line[1:]}}, nil
	}
	f := strings.Fields(line)
	c := Command{Name: f[0]}
	if len(f) > 1 {
		c.Args = f[1:] // nil, not empty, when there are no arguments
	}
	return c, nil
}

// Spec describes one command a tool registers.
type Spec struct {
	Name     string
	Aliases  []string
	Help     string
	Run      func(args []string) (tea.Cmd, error)
	Complete func(argIdx int, prefix string) []string // nil = no completion
}

// Registry holds specs in registration order.
type Registry struct {
	specs []Spec
}

// Register adds specs.
func (r *Registry) Register(specs ...Spec) { r.specs = append(r.specs, specs...) }

// Find resolves a name or alias.
func (r *Registry) Find(name string) (Spec, bool) {
	for _, s := range r.specs {
		if s.Name == name {
			return s, true
		}
		for _, a := range s.Aliases {
			if a == name {
				return s, true
			}
		}
	}
	return Spec{}, false
}

// Run executes a parsed command.
func (r *Registry) Run(c Command) (tea.Cmd, error) {
	s, ok := r.Find(c.Name)
	if !ok {
		return nil, fmt.Errorf("unknown command: %s", c.Name)
	}
	return s.Run(c.Args)
}

// Specs lists the registered commands (for help).
func (r *Registry) Specs() []Spec { return r.specs }

// Standard returns the verbs every tool registers: q/quit, h/help, and the
// /pattern search alias (an empty pattern is not forwarded).
func Standard(onHelp func(), onSearch func(pattern string)) []Spec {
	return []Spec{
		{Name: "q", Aliases: []string{"quit"}, Help: "quit",
			Run: func([]string) (tea.Cmd, error) { return tea.Quit, nil }},
		{Name: "h", Aliases: []string{"help"}, Help: "help",
			Run: func([]string) (tea.Cmd, error) { onHelp(); return nil, nil }},
		{Name: "search", Help: "/<regex>  search (same as /)",
			Run: func(args []string) (tea.Cmd, error) {
				if len(args) > 0 && args[0] != "" {
					onSearch(args[0])
				}
				return nil, nil
			}},
	}
}

// Kind is what a key did to the prompt.
type Kind int

const (
	Ignored Kind = iota
	Typed
	Submitted
	Cancelled
)

// Event is the result of State.Key.
type Event struct {
	Kind    Kind
	Command Command // set when Kind == Submitted
}

type completion struct {
	head       string
	candidates []string
	idx        int
}

// State is the : prompt.
type State struct {
	Input prompt.Line
	comp  *completion
}

// Key feeds one key. Enter parses and clears the buffer (a blank line is
// Cancelled); Esc cancels; Tab / Shift-Tab complete; editing keys are Typed
// and reset any completion cycle.
func (s *State) Key(msg tea.KeyMsg, r *Registry) Event {
	switch msg.Type {
	case tea.KeyEscape:
		s.Input.Reset()
		s.comp = nil
		return Event{Kind: Cancelled}
	case tea.KeyEnter:
		line := s.Input.String()
		s.Input.Reset()
		s.comp = nil
		c, err := Parse(line)
		if err != nil {
			return Event{Kind: Cancelled}
		}
		return Event{Kind: Submitted, Command: c}
	case tea.KeyTab:
		s.Complete(1, r)
		return Event{Kind: Typed}
	case tea.KeyShiftTab:
		s.Complete(-1, r)
		return Event{Kind: Typed}
	}
	if s.Input.Handle(msg) {
		s.comp = nil
		return Event{Kind: Typed}
	}
	return Event{Kind: Ignored}
}

// Complete cycles candidates for the token under the cursor: the command
// name when it is the only token, else Spec.Complete(argIdx, prefix). A
// trailing space means "the next argument, empty prefix". Only when the
// cursor is at the end of the line.
func (s *State) Complete(dir int, r *Registry) {
	if !s.Input.AtEnd() {
		return
	}
	if s.comp == nil {
		text := s.Input.String()
		tokens := strings.Fields(text)
		trailing := strings.HasSuffix(text, " ")
		var cands []string
		head := ""
		switch {
		case len(tokens) == 0:
			return
		case len(tokens) == 1 && !trailing:
			for _, sp := range r.Specs() {
				if strings.HasPrefix(sp.Name, tokens[0]) {
					cands = append(cands, sp.Name)
				}
			}
		default:
			sp, ok := r.Find(tokens[0])
			if !ok || sp.Complete == nil {
				return
			}
			args := tokens[1:]
			argIdx, prefix := len(args), ""
			if !trailing { // len(args) ≥ 1 here: the one-token case was handled above
				argIdx, prefix = len(args)-1, args[len(args)-1]
			}
			cands = sp.Complete(argIdx, prefix)
			head = strings.Join(tokens[:1+argIdx], " ") + " "
		}
		if len(cands) == 0 {
			return
		}
		s.comp = &completion{head: head, candidates: cands, idx: -1}
		if dir < 0 {
			s.comp.idx = 0
		}
	}
	n := len(s.comp.candidates)
	s.comp.idx = ((s.comp.idx+dir)%n + n) % n
	s.Input.SetText(s.comp.head + s.comp.candidates[s.comp.idx])
}
