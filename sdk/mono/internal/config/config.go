// Package config is mono's settings file: which tool runs in which pane, how
// tall each pane is, and the prefix key. It owns loading (a missing file is
// the built-in default), validation, the dotted-key get/set edits that both
// `mono config` and the config screen use, and atomic saves.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Pane is one stacked region. Cmd runs in a pseudo-terminal; with Every set
// it is re-run on that interval and its latest output shown (gsl), otherwise
// it is a long-lived interactive program (htop).
type Pane struct {
	Name      string   `json:"name"`
	Size      string   `json:"size"`                // percent of terminal rows, "60%"
	Cmd       string   `json:"cmd"`                 // run through sh -c
	Every     string   `json:"every,omitempty"`     // "1s": re-run interval
	Fallbacks []string `json:"fallbacks,omitempty"` // tried in order when Cmd is unusable
	Probe     string   `json:"probe,omitempty"`     // "": built-in check, "none": skip, else a command (exit 0 = usable)
	Cwd       string   `json:"cwd,omitempty"`       // "" = where mono was started
}

// Title is the top bar.
type Title struct {
	Size string `json:"size"`
}

// Config is the whole file.
type Config struct {
	Prefix string `json:"prefix"`
	Title  Title  `json:"title"`
	Panes  []Pane `json:"panes"`
}

// Default is the layout mono ships with: title 5%, htop 60%, a GPU monitor
// 30%, and gsl 5% refreshed every second.
func Default() Config {
	return Config{
		Prefix: "ctrl+b",
		Title:  Title{Size: "5%"},
		Panes: []Pane{
			{Name: "htop", Size: "60%", Cmd: "htop"},
			{Name: "gpu", Size: "30%", Cmd: "nvtop", Fallbacks: []string{"nvitop"}},
			{Name: "gsl", Size: "5%", Cmd: "gsl status", Every: "1s"},
		},
	}
}

// Path is $MONO_CONFIG, else ${XDG_CONFIG_HOME:-~/.config}/mono/config.json.
func Path() (string, error) {
	if p := os.Getenv("MONO_CONFIG"); p != "" {
		return p, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "mono", "config.json"), nil
}

// Load reads path; a missing file is Default(). The result is validated.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if c.Prefix == "" {
		c.Prefix = Default().Prefix
	}
	if c.Title.Size == "" {
		c.Title.Size = Default().Title.Size
	}
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Save validates and writes path atomically (temp file, fsync, rename). A
// symlinked config (dotfiles-managed) is written through: its target is
// replaced, the link stays.
func Save(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(append(b, '\n'))
	serr := tmp.Sync()
	cerr := tmp.Close()
	if werr != nil || serr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		return errors.Join(werr, serr, cerr)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Validate names the first offending field and value.
func (c Config) Validate() error {
	if _, err := PrefixByte(c.Prefix); err != nil {
		return err
	}
	if _, err := ParseSize(c.Title.Size); err != nil {
		return fmt.Errorf("title.size: %w", err)
	}
	if len(c.Panes) == 0 {
		return errors.New("panes: at least one pane is required")
	}
	seen := map[string]bool{}
	for i, p := range c.Panes {
		if !nameRe.MatchString(p.Name) {
			return fmt.Errorf("panes[%d].name must be lowercase letters, digits, - or _, got %q", i, p.Name)
		}
		if seen[p.Name] {
			return fmt.Errorf("panes[%d].name %q is used twice", i, p.Name)
		}
		seen[p.Name] = true
		if err := validatePaneField(p, "size", p.Size); err != nil {
			return err
		}
		if err := validatePaneField(p, "cmd", p.Cmd); err != nil {
			return err
		}
		if err := validatePaneField(p, "every", p.Every); err != nil {
			return err
		}
	}
	return nil
}

func validatePaneField(p Pane, field, v string) error {
	key := "panes." + p.Name + "." + field
	switch field {
	case "size":
		if _, err := ParseSize(v); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	case "cmd":
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s must not be empty", key)
		}
	case "every":
		if v == "" {
			return nil
		}
		d, err := time.ParseDuration(v)
		if err != nil || d < 100*time.Millisecond {
			return fmt.Errorf("%s must be a duration of at least 100ms (e.g. 1s), got %q", key, v)
		}
	}
	return nil
}

// ParseSize turns "60%" (or "60") into 60. Sizes are 1–100.
func ParseSize(s string) (int, error) {
	t := strings.TrimSuffix(strings.TrimSpace(s), "%")
	n, err := strconv.Atoi(t)
	if err != nil || n < 1 || n > 100 {
		return 0, fmt.Errorf("size must be a percentage from 1%% to 100%%, got %q", s)
	}
	return n, nil
}

// PrefixByte maps "ctrl+<letter>" to its control byte. ctrl+c, ctrl+i (tab),
// ctrl+j and ctrl+m (newline/enter) are refused: programs need them.
func PrefixByte(s string) (byte, error) {
	l, ok := strings.CutPrefix(s, "ctrl+")
	if !ok || len(l) != 1 || l[0] < 'a' || l[0] > 'z' || strings.ContainsAny(l, "cijm") {
		return 0, fmt.Errorf("prefix must be ctrl+<letter> other than c, i, j or m, got %q", s)
	}
	return l[0] - 'a' + 1, nil
}

// Find returns the index of the named pane, or -1.
func (c Config) Find(name string) int {
	for i, p := range c.Panes {
		if p.Name == name {
			return i
		}
	}
	return -1
}

// Keys lists every settable key, for help and completion.
func (c Config) Keys() []string {
	keys := []string{"prefix", "title.size"}
	for _, p := range c.Panes {
		for _, f := range paneFields {
			keys = append(keys, "panes."+p.Name+"."+f)
		}
	}
	return keys
}

var paneFields = []string{"cmd", "size", "every", "fallbacks", "probe", "cwd"}

// Get returns the value at a dotted key (fallbacks comma-joined).
func (c Config) Get(key string) (string, error) {
	switch key {
	case "prefix":
		return c.Prefix, nil
	case "title.size":
		return c.Title.Size, nil
	}
	i, field, err := c.paneKey(key)
	if err != nil {
		return "", err
	}
	p := c.Panes[i]
	switch field {
	case "cmd":
		return p.Cmd, nil
	case "size":
		return p.Size, nil
	case "every":
		return p.Every, nil
	case "fallbacks":
		return strings.Join(p.Fallbacks, ","), nil
	case "probe":
		return p.Probe, nil
	case "cwd":
		return p.Cwd, nil
	}
	return "", fmt.Errorf("unknown key %q", key)
}

// Set returns a copy with key set to value; the receiver is unchanged and the
// result is validated, so a rejected edit changes nothing.
func (c Config) Set(key, value string) (Config, error) {
	out := c.clone()
	switch key {
	case "prefix":
		out.Prefix = value
	case "title.size":
		out.Title.Size = value
	default:
		i, field, err := out.paneKey(key)
		if err != nil {
			return c, err
		}
		p := &out.Panes[i]
		switch field {
		case "cmd":
			p.Cmd = value
		case "size":
			p.Size = value
		case "every":
			p.Every = value
		case "fallbacks":
			p.Fallbacks = splitList(value)
		case "probe":
			p.Probe = value
		case "cwd":
			p.Cwd = value
		}
	}
	if err := out.Validate(); err != nil {
		return c, err
	}
	return out, nil
}

// Add inserts a pane at index at (clamped; -1 = end).
func (c Config) Add(p Pane, at int) (Config, error) {
	if c.Find(p.Name) >= 0 {
		return c, fmt.Errorf("a pane named %q already exists", p.Name)
	}
	out := c.clone()
	if at < 0 || at > len(out.Panes) {
		at = len(out.Panes)
	}
	out.Panes = append(out.Panes[:at], append([]Pane{p}, out.Panes[at:]...)...)
	if err := out.Validate(); err != nil {
		return c, err
	}
	return out, nil
}

// Remove drops the named pane.
func (c Config) Remove(name string) (Config, error) {
	i := c.Find(name)
	if i < 0 {
		return c, fmt.Errorf("no pane named %q", name)
	}
	out := c.clone()
	out.Panes = append(out.Panes[:i], out.Panes[i+1:]...)
	if err := out.Validate(); err != nil {
		return c, err
	}
	return out, nil
}

// Move shifts the named pane by delta places (clamped to the ends).
func (c Config) Move(name string, delta int) (Config, error) {
	i := c.Find(name)
	if i < 0 {
		return c, fmt.Errorf("no pane named %q", name)
	}
	j := max(0, min(len(c.Panes)-1, i+delta))
	out := c.clone()
	p := out.Panes[i]
	out.Panes = append(out.Panes[:i], out.Panes[i+1:]...)
	out.Panes = append(out.Panes[:j], append([]Pane{p}, out.Panes[j:]...)...)
	return out, nil
}

func (c Config) paneKey(key string) (int, string, error) {
	parts := strings.Split(key, ".")
	if len(parts) != 3 || parts[0] != "panes" {
		return 0, "", fmt.Errorf("unknown key %q (want prefix, title.size or panes.<name>.<field>)", key)
	}
	i := c.Find(parts[1])
	if i < 0 {
		return 0, "", fmt.Errorf("no pane named %q in key %q", parts[1], key)
	}
	for _, f := range paneFields {
		if f == parts[2] {
			return i, f, nil
		}
	}
	return 0, "", fmt.Errorf("unknown pane field %q in key %q (want one of %s)", parts[2], key, strings.Join(paneFields, ", "))
}

func (c Config) clone() Config {
	out := c
	out.Panes = make([]Pane, len(c.Panes))
	for i, p := range c.Panes {
		p.Fallbacks = append([]string(nil), p.Fallbacks...)
		out.Panes[i] = p
	}
	return out
}

func splitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
