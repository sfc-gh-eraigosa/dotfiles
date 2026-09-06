package cmdline

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func r(s string) tea.KeyMsg      { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func k(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func TestParse(t *testing.T) {
	cases := []struct {
		line string
		want Command
		err  error
	}{
		{"set a.b.c true", Command{Name: "set", Args: []string{"a.b.c", "true"}}, nil},
		{"  unset   a.b.c  ", Command{Name: "unset", Args: []string{"a.b.c"}}, nil},
		{"q", Command{Name: "q"}, nil},
		{"/ai\\.(x|y) z", Command{Name: "search", Args: []string{"ai\\.(x|y) z"}}, nil},
		{"/", Command{Name: "search", Args: []string{""}}, nil},
		{"", Command{}, ErrEmpty},
		{"   ", Command{}, ErrEmpty},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			got, err := Parse(tc.line)
			if tc.err != nil {
				require.True(t, errors.Is(err, tc.err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want.Name, got.Name)
			assert.Equal(t, tc.want.Args, got.Args)
		})
	}
}

func newReg(t *testing.T) (*Registry, *[]string) {
	t.Helper()
	var log []string
	var reg Registry
	reg.Register(Standard(
		func() { log = append(log, "help") },
		func(p string) { log = append(log, "search:"+p) },
	)...)
	reg.Register(Spec{
		Name: "set", Help: "set <key> <value>",
		Run: func(args []string) (tea.Cmd, error) {
			if len(args) < 2 {
				return nil, errors.New("missing value for " + args[0])
			}
			log = append(log, "set:"+args[0]+"="+args[1])
			return nil, nil
		},
		Complete: func(argIdx int, prefix string) []string {
			if argIdx != 0 {
				return nil
			}
			var out []string
			for _, kp := range []string{"install.ai.claude", "install.ai.teams", "install.pkg.manager"} {
				if len(prefix) <= len(kp) && kp[:len(prefix)] == prefix {
					out = append(out, kp)
				}
			}
			return out
		},
	})
	return &reg, &log
}

func TestRegistryRunAliasesAndUnknown(t *testing.T) {
	reg, log := newReg(t)
	cmd, err := reg.Run(Command{Name: "quit"})
	require.NoError(t, err)
	assert.NotNil(t, cmd, "q/quit returns tea.Quit")
	_, err = reg.Run(Command{Name: "h"})
	require.NoError(t, err)
	_, err = reg.Run(Command{Name: "search", Args: []string{"ai"}})
	require.NoError(t, err)
	_, err = reg.Run(Command{Name: "search", Args: []string{""}})
	require.NoError(t, err)
	assert.Equal(t, []string{"help", "search:ai"}, *log, "empty search pattern is not forwarded")
	_, err = reg.Run(Command{Name: "frobnicate"})
	require.EqualError(t, err, "unknown command: frobnicate")
	_, err = reg.Run(Command{Name: "set", Args: []string{"k"}})
	require.EqualError(t, err, "missing value for k")
	_, ok := reg.Find("quit")
	assert.True(t, ok)
	assert.Len(t, reg.Specs(), 4)
}

func typeInto(s *State, reg *Registry, text string) {
	for _, c := range text {
		s.Key(r(string(c)), reg)
	}
}

func TestStateSubmitCancelEmpty(t *testing.T) {
	reg, _ := newReg(t)
	var s State
	typeInto(&s, reg, "set a true")
	ev := s.Key(k(tea.KeyEnter), reg)
	assert.Equal(t, Submitted, ev.Kind)
	assert.Equal(t, Command{Name: "set", Args: []string{"a", "true"}}, ev.Command)
	assert.Equal(t, "", s.Input.String(), "buffer cleared on submit")

	typeInto(&s, reg, "x")
	assert.Equal(t, Cancelled, s.Key(k(tea.KeyEscape), reg).Kind)
	assert.Equal(t, "", s.Input.String())

	assert.Equal(t, Cancelled, s.Key(k(tea.KeyEnter), reg).Kind, "empty line submits nothing")
	assert.Equal(t, Ignored, s.Key(k(tea.KeyUp), reg).Kind)
	assert.Equal(t, Typed, s.Key(r("j"), reg).Kind, "letters are text")
}

func TestTabCompletesArgumentCyclesAndResets(t *testing.T) {
	reg, _ := newReg(t)
	var s State
	typeInto(&s, reg, "set install.ai.")
	s.Key(k(tea.KeyTab), reg)
	assert.Equal(t, "set install.ai.claude", s.Input.String())
	s.Key(k(tea.KeyTab), reg)
	assert.Equal(t, "set install.ai.teams", s.Input.String())
	s.Key(k(tea.KeyTab), reg)
	assert.Equal(t, "set install.ai.claude", s.Input.String(), "wraps")
	s.Key(k(tea.KeyShiftTab), reg)
	assert.Equal(t, "set install.ai.teams", s.Input.String(), "shift-tab reverses")
	typeInto(&s, reg, "x")
	s.Key(k(tea.KeyTab), reg)
	assert.Equal(t, "set install.ai.teamsx", s.Input.String(), "no candidate → unchanged")
}

func TestTabCompletesCommandNamesAndNextArg(t *testing.T) {
	reg, _ := newReg(t)
	var s State
	typeInto(&s, reg, "se")
	s.Key(k(tea.KeyTab), reg)
	assert.Equal(t, "search", s.Input.String(), "command names complete in registry order")
	s.Key(k(tea.KeyTab), reg)
	assert.Equal(t, "set", s.Input.String())

	s.Input.Reset()
	typeInto(&s, reg, "set ")
	s.Key(k(tea.KeyTab), reg)
	assert.Equal(t, "set install.ai.claude", s.Input.String(), "trailing space = next argument, empty prefix")

	s.Input.Reset()
	typeInto(&s, reg, "set install.ai.claude ")
	s.Key(k(tea.KeyTab), reg)
	assert.Equal(t, "set install.ai.claude ", s.Input.String(), "argIdx 1 has no completer")

	s.Input.Reset()
	typeInto(&s, reg, "set install.ai.")
	s.Key(k(tea.KeyLeft), reg)
	s.Key(k(tea.KeyTab), reg)
	assert.Equal(t, "set install.ai.", s.Input.String(), "cursor not at the end → no completion")
}
