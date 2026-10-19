package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// credentialWords are key names that would put a secret in the file. Set
// refuses them all and points at the three source fields instead (spec E13).
var credentialWords = []string{"token", "password", "secret", "apikey", "api_key"}

// Get returns the value at a dotted path, e.g. views.ci.dashboard.
func (c Config) Get(path string) (string, error) {
	parts := strings.Split(path, ".")
	switch {
	case path == "default_profile":
		return c.DefaultProfile, nil
	case len(parts) >= 3 && parts[0] == "profiles":
		p, ok := c.Profiles[parts[1]]
		if !ok {
			return "", fmt.Errorf("no profile named %q", parts[1])
		}
		return profileField(p, parts[2:], path)
	case len(parts) >= 3 && parts[0] == "views":
		v, ok := c.Views[parts[1]]
		if !ok {
			return "", fmt.Errorf("no view named %q", parts[1])
		}
		return viewField(v, parts[2:], path)
	}
	return "", unknown(path)
}

func profileField(p Profile, rest []string, path string) (string, error) {
	switch strings.Join(rest, ".") {
	case "url":
		return p.URL, nil
	case "auth.file":
		return p.Auth.File, nil
	case "auth.env":
		return p.Auth.Env, nil
	case "auth.cmd":
		return p.Auth.Cmd, nil
	case "auth.user":
		return p.Auth.User, nil
	}
	return "", unknown(path)
}

func viewField(v View, rest []string, path string) (string, error) {
	switch {
	case len(rest) == 1 && rest[0] == "profile":
		return v.Profile, nil
	case len(rest) == 1 && rest[0] == "dashboard":
		return v.Dashboard, nil
	case len(rest) == 1 && rest[0] == "range":
		return v.Range, nil
	case len(rest) == 1 && rest[0] == "refresh":
		return v.Refresh, nil
	case len(rest) == 1 && rest[0] == "panels":
		return strings.Join(v.Panels, ","), nil
	case len(rest) == 2 && rest[0] == "vars":
		return v.Vars[rest[1]], nil
	}
	return "", unknown(path)
}

// Set writes a value at a dotted path, creating the profile or view if it is
// new. It refuses inline credentials and a token file others can read.
func (c *Config) Set(path, value string) error {
	parts := strings.Split(path, ".")
	for _, p := range parts {
		for _, w := range credentialWords {
			if strings.EqualFold(p, w) {
				return fmt.Errorf("%s: gdash never stores a credential in its config; set auth.file (a 0600 file), auth.env or auth.cmd instead", path)
			}
		}
	}
	switch {
	case path == "default_profile":
		c.DefaultProfile = value
		return nil
	case len(parts) >= 3 && parts[0] == "profiles":
		if c.Profiles == nil {
			c.Profiles = map[string]Profile{}
		}
		p := c.Profiles[parts[1]]
		if err := setProfileField(&p, strings.Join(parts[2:], "."), value, path); err != nil {
			return err
		}
		c.Profiles[parts[1]] = p
		return nil
	case len(parts) >= 3 && parts[0] == "views":
		if c.Views == nil {
			c.Views = map[string]View{}
		}
		v := c.Views[parts[1]]
		if err := setViewField(&v, parts[2:], value, path); err != nil {
			return err
		}
		c.Views[parts[1]] = v
		return nil
	}
	return unknown(path)
}

func setProfileField(p *Profile, field, value, path string) error {
	switch field {
	case "url":
		p.URL = value
	case "auth.file":
		if err := CheckPrivateFile(value); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		p.Auth.File = value
	case "auth.env":
		p.Auth.Env = value
	case "auth.cmd":
		p.Auth.Cmd = value
	case "auth.user":
		p.Auth.User = value
	default:
		return unknown(path)
	}
	return nil
}

func setViewField(v *View, rest []string, value, path string) error {
	switch {
	case len(rest) == 1 && rest[0] == "profile":
		v.Profile = value
	case len(rest) == 1 && rest[0] == "dashboard":
		v.Dashboard = value
	case len(rest) == 1 && rest[0] == "range":
		v.Range = value
	case len(rest) == 1 && rest[0] == "refresh":
		v.Refresh = value
	case len(rest) == 1 && rest[0] == "panels":
		v.Panels = nil
		for _, t := range strings.Split(value, ",") {
			if t = strings.TrimSpace(t); t != "" {
				v.Panels = append(v.Panels, t)
			}
		}
	case len(rest) == 2 && rest[0] == "vars":
		if v.Vars == nil {
			v.Vars = map[string]string{}
		}
		v.Vars[rest[1]] = value
	default:
		return unknown(path)
	}
	return nil
}

func unknown(path string) error {
	return fmt.Errorf("unknown key %q (keys: default_profile, profiles.<p>.url, profiles.<p>.auth.{file,env,cmd,user}, views.<v>.{profile,dashboard,range,refresh,panels}, views.<v>.vars.<name>)", path)
}

// ExpandHome turns a leading ~/ into the home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// CheckPrivateFile resolves symlinks and requires the target to be a regular
// file with no group or other permissions (0600 or stricter).
func CheckPrivateFile(p string) error {
	real, err := filepath.EvalSymlinks(ExpandHome(p))
	if err != nil {
		return fmt.Errorf("%s: %w", p, err)
	}
	st, err := os.Stat(real)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", p)
	}
	if st.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s is mode %04o, readable by others: chmod 600 %s", p, st.Mode().Perm(), p)
	}
	return nil
}
