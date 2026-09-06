package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/overrides"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/cmdline"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/search"
)

// parseValue turns the :set value token into a typed Value for item,
// rejecting anything the picker would not let you choose.
func parseValue(item resolve.Resolved, raw string) (*gffv1.Value, error) {
	path := item.Feature.GetPath()
	switch item.Feature.Default.(type) {
	case *gffv1.Feature_BoolDefault:
		switch raw {
		case "true":
			return &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: true}}, nil
		case "false":
			return &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: false}}, nil
		}
		return nil, fmt.Errorf("value for %s must be true or false, got %q", path, raw)
	case *gffv1.Feature_ChoiceDefault:
		cd := item.Feature.GetChoiceDefault()
		known := map[string]bool{}
		for _, opt := range cd.GetOptions() {
			known[opt.GetId()] = true
		}
		ids := strings.Split(raw, ",")
		seen := map[string]bool{}
		for _, id := range ids {
			if !known[id] {
				return nil, fmt.Errorf("unknown option %q for %s", id, path)
			}
			if seen[id] {
				return nil, fmt.Errorf("duplicate option %q for %s", id, path)
			}
			seen[id] = true
		}
		if cd.GetMode() != gffv1.ChoiceMode_CHOICE_MODE_MULTI && len(ids) != 1 {
			return nil, fmt.Errorf("%s is a single-choice flag: give exactly one id", path)
		}
		return &gffv1.Value{Kind: &gffv1.Value_ChoiceValue{ChoiceValue: &gffv1.ChoiceSelection{Selected: ids}}}, nil
	}
	return nil, fmt.Errorf("unsupported flag type for %s", path)
}

// findKey resolves "<ns>:<path>" or a bare "<path>" to an item index. A bare
// path in several namespaces resolves to the breadcrumb's namespace when it
// is one of them, otherwise it is an error.
func (m *Model) findKey(key string) (int, error) {
	ns, path := "", key
	if i := strings.IndexByte(key, ':'); i >= 0 {
		ns, path = key[:i], key[i+1:]
	}
	var hits []int
	for i, it := range m.items {
		if it.Feature.GetPath() == path && (ns == "" || it.Namespace() == ns) {
			hits = append(hits, i)
		}
	}
	switch len(hits) {
	case 0:
		return -1, fmt.Errorf("unknown key: %s", key)
	case 1:
		return hits[0], nil
	}
	for _, i := range hits {
		if m.scopeNS != "" && m.items[i].Namespace() == m.scopeNS {
			return i, nil
		}
	}
	return -1, fmt.Errorf("ambiguous key %q: qualify it as <namespace>:%s", key, path)
}

// completeKey lists in-scope key paths with the prefix, in item order.
func (m *Model) completeKey(prefix string) []string {
	var out []string
	for _, it := range m.items {
		if m.inScope(it) && strings.HasPrefix(it.Feature.GetPath(), prefix) {
			out = append(out, it.Feature.GetPath())
		}
	}
	return out
}

// registerCommands wires the : verbs: the sdk standard set plus gff's own
// set/unset, which go through the SAME writers as `gff set` / `gff unset`.
func (m *Model) registerCommands() {
	keyCompleter := func(argIdx int, prefix string) []string {
		if argIdx != 0 {
			return nil
		}
		return m.completeKey(prefix)
	}
	m.reg.Register(cmdline.Standard(
		func() { m.helpReturn = modeList; m.mode = modeHelp },
		func(p string) { // :/re — identical end state to "/re" Enter
			m.startSearch()
			m.search.Input.SetText(p)
			re, err := search.Compile(p)
			if err != nil {
				m.search.Err = err.Error()
			} else {
				m.search.Re = re
			}
			m.applySearch()
			m.commitSearch()
		},
	)...)
	m.reg.Register(
		cmdline.Spec{Name: "set", Help: "set <key> <value>  (bool: true/false; choice: id[,id])", Complete: keyCompleter,
			Run: func(args []string) (tea.Cmd, error) {
				if len(args) == 0 {
					return nil, errors.New("usage: :set <key> <value>")
				}
				if len(args) < 2 {
					return nil, fmt.Errorf("missing value for %s", args[0])
				}
				idx, err := m.findKey(args[0])
				if err != nil {
					return nil, err
				}
				item := m.items[idx]
				val, err := parseValue(item, args[1])
				if err != nil {
					return nil, err
				}
				if err := overrides.Write(m.p, item.Feature.GetPath(), val); err != nil {
					return nil, fmt.Errorf("write failed: %w", err)
				}
				m.items[idx] = item.WithValue(val, resolve.LayerUserOverride)
				m.refreshItem(idx)
				m.buildRows()
				return nil, nil
			}},
		cmdline.Spec{Name: "unset", Help: "unset <key>  (clear the user override)", Complete: keyCompleter,
			Run: func(args []string) (tea.Cmd, error) {
				if len(args) == 0 {
					return nil, errors.New("usage: :unset <key>")
				}
				idx, err := m.findKey(args[0])
				if err != nil {
					return nil, err
				}
				if err := overrides.Unset(m.p, m.items[idx].Feature.GetPath()); err != nil {
					return nil, fmt.Errorf("unset failed: %w", err)
				}
				m.refreshItem(idx)
				m.buildRows()
				return nil, nil
			}},
	)
}

// updateCommand handles keys while the : prompt is open.
func (m *Model) updateCommand(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ev := m.cmd.Key(msg, &m.reg)
	switch ev.Kind {
	case cmdline.Cancelled:
		m.mode = modeList
	case cmdline.Submitted:
		m.mode = modeList
		m.errMsg = ""
		cmd, err := m.reg.Run(ev.Command)
		if err != nil {
			m.errMsg = err.Error()
		}
		return m, cmd
	}
	return m, nil
}
