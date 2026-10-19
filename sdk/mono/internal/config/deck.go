package config

import (
	"fmt"
	"strconv"
	"strings"
)

// A deck is a pane holding several tools, each with several views; one view
// is on screen at a time (mono-gdash spec F8). A plain pane (cmd) is a deck
// of one tool with one view, both named after the pane.

// View is one command a tool can show: today's pane spec without name/size.
type View struct {
	Name      string   `json:"name"`
	Cmd       string   `json:"cmd"`
	Every     string   `json:"every,omitempty"`
	Fallbacks []string `json:"fallbacks,omitempty"`
	Probe     string   `json:"probe,omitempty"`
	Cwd       string   `json:"cwd,omitempty"`
}

// Tool is one column of a deck.
type Tool struct {
	Name  string `json:"name"`
	Probe string `json:"probe,omitempty"` // applies to every view; failing drops the tool
	Zoom  bool   `json:"zoom,omitempty"`  // fills the middle whenever it is on screen (spec F10)
	Views []View `json:"views"`
}

// Middle modes (spec F10).
const (
	MiddleSplit  = "split"
	MiddleSingle = "single"
)

// DefaultNavKey and DefaultEscMs are the nav-mode defaults (spec F9).
const (
	DefaultNavKey = "space"
	DefaultEscMs  = 300
)

// IsDeck reports whether the pane is a deck (tools) rather than one command.
func (p Pane) IsDeck() bool { return len(p.Tools) > 0 }

// Deck is the pane as tools and views; a plain pane is a 1×1 deck.
func (p Pane) Deck() []Tool {
	if p.IsDeck() {
		return p.Tools
	}
	return []Tool{{Name: p.Name, Views: []View{{
		Name: p.Name, Cmd: p.Cmd, Every: p.Every, Fallbacks: p.Fallbacks, Probe: p.Probe, Cwd: p.Cwd,
	}}}}
}

// CanZoom reports whether the pane is part of the middle (zoomable unset or true).
func (p Pane) CanZoom() bool { return p.Zoomable == nil || *p.Zoomable }

// MiddleMode is the middle setting with its default.
func (c Config) MiddleMode() string {
	if c.Middle == "" {
		return MiddleSplit
	}
	return c.Middle
}

// Nav is the nav-mode key with its default.
func (c Config) Nav() string {
	if c.NavKey == "" {
		return DefaultNavKey
	}
	return c.NavKey
}

// EscMs is the double-esc window in milliseconds; 0 disables double-esc.
func (c Config) EscMs() int {
	if c.NavEscMs == nil {
		return DefaultEscMs
	}
	return *c.NavEscMs
}

// Effective is the panes mono runs: as configured, or with middle: single,
// every middle pane merged into one deck named "mid" (spec F10). A layout
// with fewer than two middle panes has nothing to merge.
func (c Config) Effective() []Pane {
	if c.MiddleMode() != MiddleSingle {
		return c.Panes
	}
	var middle []Pane
	for _, p := range c.Panes {
		if p.CanZoom() {
			middle = append(middle, p)
		}
	}
	if len(middle) < 2 {
		return c.Panes
	}
	merged := Pane{Name: "mid"}
	total := 0
	for _, p := range middle {
		n, _ := ParseSize(p.Size)
		total += n
		merged.Tools = append(merged.Tools, p.Deck()...)
	}
	merged.Size = strconv.Itoa(min(total, 100)) + "%"
	var out []Pane
	placed := false
	for _, p := range c.Panes {
		if !p.CanZoom() {
			out = append(out, p)
			continue
		}
		if !placed {
			out = append(out, merged)
			placed = true
		}
	}
	return out
}

// validateDeck checks a pane's cmd/tools shape and every tool and view.
func validateDeck(p Pane) error {
	key := "panes." + p.Name
	if p.IsDeck() && strings.TrimSpace(p.Cmd) != "" {
		return fmt.Errorf("%s: set either cmd (one program) or tools (a deck), not both", key)
	}
	if !p.IsDeck() {
		return validatePaneField(p, "cmd", p.Cmd)
	}
	tools := map[string]bool{}
	for i, t := range p.Tools {
		tk := fmt.Sprintf("%s.tools[%d]", key, i)
		if !nameRe.MatchString(t.Name) {
			return fmt.Errorf("%s.name must be lowercase letters, digits, - or _, got %q", tk, t.Name)
		}
		if tools[t.Name] {
			return fmt.Errorf("%s.tools: %q is used twice", key, t.Name)
		}
		tools[t.Name] = true
		if len(t.Views) == 0 {
			return fmt.Errorf("%s.tools.%s: a tool needs at least one view", key, t.Name)
		}
		views := map[string]bool{}
		for j, v := range t.Views {
			vk := fmt.Sprintf("%s.tools.%s.views[%d]", key, t.Name, j)
			if !nameRe.MatchString(v.Name) {
				return fmt.Errorf("%s.name must be lowercase letters, digits, - or _, got %q", vk, v.Name)
			}
			if views[v.Name] {
				return fmt.Errorf("%s.tools.%s.views: %q is used twice", key, t.Name, v.Name)
			}
			views[v.Name] = true
			vp := Pane{Name: p.Name + "." + t.Name + "." + v.Name, Cmd: v.Cmd, Every: v.Every}
			if err := validatePaneField(vp, "cmd", v.Cmd); err != nil {
				return err
			}
			if err := validatePaneField(vp, "every", v.Every); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateMerge refuses a middle: single layout whose merge would be
// ambiguous: a pane that stays separate named "mid", or two merged tools
// sharing a name (review of mono-gdash #414).
func (c Config) validateMerge() error {
	if c.MiddleMode() != MiddleSingle {
		return nil
	}
	var middle []Pane
	for _, p := range c.Panes {
		if p.CanZoom() {
			middle = append(middle, p)
		}
	}
	if len(middle) < 2 {
		return nil
	}
	for _, p := range c.Panes {
		if !p.CanZoom() && p.Name == "mid" {
			return fmt.Errorf("middle: single merges the middle panes into one named mid, but pane %q (zoomable: false) already has that name; rename it", p.Name)
		}
	}
	seen := map[string]string{}
	for _, p := range middle {
		for _, t := range p.Deck() {
			if other, ok := seen[t.Name]; ok {
				return fmt.Errorf("middle: single would merge two tools named %q (from panes %s and %s); rename one", t.Name, other, p.Name)
			}
			seen[t.Name] = p.Name
		}
	}
	return nil
}

// validateTop checks middle, nav_key and nav_esc_ms.
func (c Config) validateTop() error {
	switch c.Middle {
	case "", MiddleSplit, MiddleSingle:
	default:
		return fmt.Errorf("middle must be %q or %q, got %q", MiddleSplit, MiddleSingle, c.Middle)
	}
	if err := c.validateMerge(); err != nil {
		return err
	}
	if c.NavEscMs != nil && (*c.NavEscMs < 0 || *c.NavEscMs > 2000) {
		return fmt.Errorf("nav_esc_ms must be 0 (double-esc off) to 2000, got %d", *c.NavEscMs)
	}
	if k := c.NavKey; k != "" && k != "space" {
		if len([]rune(k)) != 1 || strings.ContainsAny(k, "jkhlz:?rqJKHLZ") {
			return fmt.Errorf("nav_key must be \"space\" or one character that is not already a mono key, got %q", k)
		}
	}
	return nil
}

// deckKey splits panes.<p>.tools.<t>[.views.<v>].<field>.
type deckKey struct {
	pane, tool, view int // indexes; view -1 = a tool field
}

func (c Config) parseDeckKey(key string) (deckKey, string, error) {
	parts := strings.Split(key, ".")
	bad := func() (deckKey, string, error) {
		return deckKey{}, "", fmt.Errorf("unknown key %q (deck keys: panes.<p>.tools.<t>.{probe,zoom}, panes.<p>.tools.<t>.views.<v>.{%s})", key, strings.Join(viewFields, ","))
	}
	if (len(parts) != 5 && len(parts) != 7) || parts[0] != "panes" || parts[2] != "tools" {
		return bad()
	}
	pi := c.Find(parts[1])
	if pi < 0 {
		return deckKey{}, "", fmt.Errorf("no pane named %q in key %q", parts[1], key)
	}
	ti := -1
	for i, t := range c.Panes[pi].Tools {
		if t.Name == parts[3] {
			ti = i
		}
	}
	if ti < 0 {
		return deckKey{}, "", fmt.Errorf("pane %q has no tool %q", parts[1], parts[3])
	}
	if len(parts) == 5 {
		if parts[4] != "probe" && parts[4] != "zoom" {
			return bad()
		}
		return deckKey{pane: pi, tool: ti, view: -1}, parts[4], nil
	}
	if parts[4] != "views" {
		return bad()
	}
	vi := -1
	for i, v := range c.Panes[pi].Tools[ti].Views {
		if v.Name == parts[5] {
			vi = i
		}
	}
	if vi < 0 {
		return deckKey{}, "", fmt.Errorf("tool %q has no view %q", parts[3], parts[5])
	}
	for _, f := range viewFields {
		if f == parts[6] {
			return deckKey{pane: pi, tool: ti, view: vi}, f, nil
		}
	}
	return bad()
}

var viewFields = []string{"cmd", "every", "fallbacks", "probe", "cwd"}

func (c Config) getDeck(key string) (string, error) {
	k, f, err := c.parseDeckKey(key)
	if err != nil {
		return "", err
	}
	t := c.Panes[k.pane].Tools[k.tool]
	if k.view < 0 {
		if f == "zoom" {
			return strconv.FormatBool(t.Zoom), nil
		}
		return t.Probe, nil
	}
	v := t.Views[k.view]
	return map[string]string{"cmd": v.Cmd, "every": v.Every, "fallbacks": strings.Join(v.Fallbacks, ","), "probe": v.Probe, "cwd": v.Cwd}[f], nil
}

func (out *Config) setDeck(key, value string) error {
	k, f, err := out.parseDeckKey(key)
	if err != nil {
		return err
	}
	t := &out.Panes[k.pane].Tools[k.tool]
	if k.view < 0 {
		if f == "zoom" {
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("%s must be true or false, got %q", key, value)
			}
			t.Zoom = b
			return nil
		}
		t.Probe = value
		return nil
	}
	v := &t.Views[k.view]
	switch f {
	case "cmd":
		v.Cmd = value
	case "every":
		v.Every = value
	case "fallbacks":
		v.Fallbacks = splitList(value)
	case "probe":
		v.Probe = value
	case "cwd":
		v.Cwd = value
	}
	return nil
}

func (c Config) deckKeys(p Pane) []string {
	var keys []string
	for _, t := range p.Tools {
		base := "panes." + p.Name + ".tools." + t.Name
		keys = append(keys, base+".probe", base+".zoom")
		for _, v := range t.Views {
			for _, f := range viewFields {
				keys = append(keys, base+".views."+v.Name+"."+f)
			}
		}
	}
	return keys
}

// AddTool inserts a tool into a deck pane at index at (-1 = end).
func (c Config) AddTool(pane string, t Tool, at int) (Config, error) {
	out := c.clone()
	i := out.Find(pane)
	if i < 0 {
		return c, fmt.Errorf("no pane named %q", pane)
	}
	if !out.Panes[i].IsDeck() {
		return c, fmt.Errorf("pane %q runs one command, not a deck of tools", pane)
	}
	ts := out.Panes[i].Tools
	if at < 0 || at > len(ts) {
		at = len(ts)
	}
	out.Panes[i].Tools = append(ts[:at], append([]Tool{t}, ts[at:]...)...)
	if err := out.Validate(); err != nil {
		return c, err
	}
	return out, nil
}

// RemoveTool drops a tool; a deck keeps at least one.
func (c Config) RemoveTool(pane, tool string) (Config, error) {
	out := c.clone()
	i := out.Find(pane)
	if i < 0 {
		return c, fmt.Errorf("no pane named %q", pane)
	}
	ts := out.Panes[i].Tools
	for j, t := range ts {
		if t.Name == tool {
			if len(ts) == 1 {
				return c, fmt.Errorf("%q is the last tool of pane %q; remove the pane instead", tool, pane)
			}
			out.Panes[i].Tools = append(ts[:j], ts[j+1:]...)
			return out, out.Validate()
		}
	}
	return c, fmt.Errorf("pane %q has no tool %q", pane, tool)
}

// AddView inserts a view into a tool at index at (-1 = end).
func (c Config) AddView(pane, tool string, v View, at int) (Config, error) {
	out := c.clone()
	i := out.Find(pane)
	if i < 0 {
		return c, fmt.Errorf("no pane named %q", pane)
	}
	for j := range out.Panes[i].Tools {
		t := &out.Panes[i].Tools[j]
		if t.Name != tool {
			continue
		}
		if at < 0 || at > len(t.Views) {
			at = len(t.Views)
		}
		t.Views = append(t.Views[:at], append([]View{v}, t.Views[at:]...)...)
		if err := out.Validate(); err != nil {
			return c, err
		}
		return out, nil
	}
	return c, fmt.Errorf("pane %q has no tool %q", pane, tool)
}

// RemoveView drops a view; a tool keeps at least one.
func (c Config) RemoveView(pane, tool, view string) (Config, error) {
	out := c.clone()
	i := out.Find(pane)
	if i < 0 {
		return c, fmt.Errorf("no pane named %q", pane)
	}
	for j := range out.Panes[i].Tools {
		t := &out.Panes[i].Tools[j]
		if t.Name != tool {
			continue
		}
		for k, v := range t.Views {
			if v.Name == view {
				if len(t.Views) == 1 {
					return c, fmt.Errorf("%q is the last view of tool %q; remove the tool instead", view, tool)
				}
				t.Views = append(t.Views[:k], t.Views[k+1:]...)
				return out, out.Validate()
			}
		}
		return c, fmt.Errorf("tool %q has no view %q", tool, view)
	}
	return c, fmt.Errorf("pane %q has no tool %q", pane, tool)
}
