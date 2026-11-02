// Package config is gdash's configuration: Grafana profiles (where and how to
// authenticate) and views (which dashboard, over what range). The file holds
// no secrets: a profile names where its credential comes from (spec F6).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Defaults a view inherits when it leaves a field empty.
const (
	DefaultRange   = "1h"
	DefaultRefresh = "30s"
)

// Auth names where a profile's credential comes from. Exactly one of File,
// Env and Cmd is set; User switches from a bearer token to basic auth.
type Auth struct {
	File string `json:"file,omitempty"` // path to a 0600 file holding the token
	Env  string `json:"env,omitempty"`  // name of an environment variable
	Cmd  string `json:"cmd,omitempty"`  // command whose stdout is the credential
	User string `json:"user,omitempty"` // basic auth: the user name
}

// Sources lists the credential sources set, by field name.
func (a Auth) Sources() []string {
	var s []string
	for name, v := range map[string]string{"file": a.File, "env": a.Env, "cmd": a.Cmd} {
		if v != "" {
			s = append(s, name)
		}
	}
	sort.Strings(s)
	return s
}

// Profile is one Grafana and how to reach it.
type Profile struct {
	URL  string `json:"url"`
	Auth Auth   `json:"auth"`
}

// View is one dashboard as gdash draws it.
type View struct {
	Profile   string            `json:"profile,omitempty"` // "" = default_profile
	Dashboard string            `json:"dashboard"`         // uid or exact title
	Range     string            `json:"range,omitempty"`   // "1h", "7d"; empty = the dashboard's own
	Refresh   string            `json:"refresh,omitempty"` // "30s"; floor 5s at run time
	Vars      map[string]string `json:"vars,omitempty"`    // template-variable overrides
	Panels    []string          `json:"panels,omitempty"`  // optional subset, by title
}

// Config is the whole file.
type Config struct {
	Profiles       map[string]Profile `json:"profiles,omitempty"`
	DefaultProfile string             `json:"default_profile,omitempty"`
	Views          map[string]View    `json:"views,omitempty"`
}

// Path is where the config lives: $GDASH_CONFIG, else
// $XDG_CONFIG_HOME/gdash/config.json, else ~/.config/gdash/config.json.
func Path() (string, error) {
	if p := os.Getenv("GDASH_CONFIG"); p != "" {
		return p, nil
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "gdash", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gdash", "config.json"), nil
}

// Load reads the config. A missing file is an empty config, not an error.
func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path) //nolint:gosec // the user's own config path
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Save writes the config atomically: the file 0600, its directory 0700.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

var durationRE = regexp.MustCompile(`^([0-9]+)([smhdw])$`)

// ParseDuration parses a positive Grafana-style duration: 30s, 5m, 1h, 7d, 2w.
func ParseDuration(s string) (time.Duration, error) {
	m := durationRE.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("%q is not a duration like 30s, 5m, 1h or 7d", s)
	}
	n, _ := strconv.Atoi(m[1])
	if n == 0 {
		return 0, fmt.Errorf("%q must be more than zero", s)
	}
	unit := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour}[m[2]]
	return time.Duration(n) * unit, nil
}

// Validate checks the whole file and names the first bad field.
func (c Config) Validate() error {
	for _, name := range sortedKeys(c.Profiles) {
		p := c.Profiles[name]
		u, err := url.Parse(p.URL)
		if p.URL == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("profiles.%s.url: want an http(s) URL, got %q", name, p.URL)
		}
		if u.User != nil {
			return fmt.Errorf("profiles.%s.url: put no user or password in the URL; use auth.user and auth.file, auth.env or auth.cmd", name)
		}
		switch src := p.Auth.Sources(); len(src) {
		case 0:
			return fmt.Errorf("profiles.%s.auth: set one of auth.file, auth.env or auth.cmd", name)
		case 1:
		default:
			return fmt.Errorf("profiles.%s.auth: exactly one of auth.file, auth.env, auth.cmd may be set (found %s)", name, strings.Join(src, ", "))
		}
	}
	if c.DefaultProfile != "" {
		if _, ok := c.Profiles[c.DefaultProfile]; !ok {
			return fmt.Errorf("default_profile: no profile named %q", c.DefaultProfile)
		}
	}
	for _, name := range sortedKeys(c.Views) {
		v := c.Views[name]
		if v.Dashboard == "" {
			return fmt.Errorf("views.%s.dashboard: set a dashboard uid or title (gdash list)", name)
		}
		switch {
		case v.Profile != "":
			if _, ok := c.Profiles[v.Profile]; !ok {
				return fmt.Errorf("views.%s.profile: no profile named %q", name, v.Profile)
			}
		case c.DefaultProfile == "":
			return fmt.Errorf("views.%s: names no profile and default_profile is unset", name)
		}
		if v.Range != "" {
			if _, err := ParseDuration(v.Range); err != nil {
				return fmt.Errorf("views.%s.range: %w", name, err)
			}
		}
		if v.Refresh != "" {
			if _, err := ParseDuration(v.Refresh); err != nil {
				return fmt.Errorf("views.%s.refresh: %w", name, err)
			}
		}
	}
	return nil
}

// Resolve returns a view with its defaults filled in, and the profile it uses.
func (c Config) Resolve(name string) (View, Profile, error) {
	v, ok := c.Views[name]
	if !ok {
		return View{}, Profile{}, fmt.Errorf("no view named %q (views: %s)", name, strings.Join(sortedKeys(c.Views), ", "))
	}
	pname := v.Profile
	if pname == "" {
		pname = c.DefaultProfile
	}
	p, ok := c.Profiles[pname]
	if !ok {
		return View{}, Profile{}, fmt.Errorf("view %q: no profile named %q", name, pname)
	}
	if v.Refresh == "" {
		v.Refresh = DefaultRefresh
	}
	return v, p, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
